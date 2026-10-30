// Package config loads TaskSamurai settings without retaining credentials.
package config

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Options controls config selection and the runtime Jira override.
type Options struct {
	Path string
	// PathSet distinguishes an explicitly empty --config from an absent flag.
	PathSet bool
	NoJira  bool
}

// Config contains settings from the selected file and built-in defaults.
type Config struct {
	Jira Jira `toml:"jira"`
}

// Jira contains non-secret Jira settings. TokenFile is resolved to an absolute path.
type Jira struct {
	Enabled          bool              `toml:"enabled"`
	Site             string            `toml:"site"`
	Email            string            `toml:"email"`
	TokenFile        string            `toml:"token_file"`
	SavedFilters     bool              `toml:"saved_filters"`
	DefaultFilter    string            `toml:"default_filter"`
	DefaultView      string            `toml:"default_view"`
	MaxResults       int               `toml:"max_results"`
	Timeout          time.Duration     `toml:"-"`
	Filters          []Filter          `toml:"filters"`
	Transitions      map[string]string `toml:"transitions"`
	TransitionFields map[string]any    `toml:"transition_fields"`
}

// Filter is a named JQL expression, kept in configuration order.
type Filter struct {
	Name string `toml:"name"`
	JQL  string `toml:"jql"`
}

type fileConfig struct {
	Jira fileJira `toml:"jira"`
}

type fileJira struct {
	Jira
	TimeoutText string `toml:"timeout"`
}

// Load selects and validates a config file. Missing default files disable Jira;
// explicitly selected files must exist. Disabled Jira skips semantic validation
// and credential reads; TOML syntax, field types and unknown keys still fail.
func Load(ctx context.Context, opts Options) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	path, explicit, err := configPath(opts)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	raw := fileConfig{Jira: fileJira{Jira: Jira{Enabled: true, SavedFilters: true, DefaultView: "both", MaxResults: 100}, TimeoutText: "10s"}}
	meta, err := toml.Decode(string(data), &raw)
	if err != nil {
		return Config{}, decodeError(path, err)
	}
	for _, key := range meta.Undecoded() {
		// TOML leaves nested map[string]any members undecoded. These are
		// intentionally open-ended native transition-field values.
		if len(key) > 2 && key[0] == "jira" && key[1] == "transition_fields" {
			continue
		}
		return Config{}, fmt.Errorf("config %q: unknown TOML keys", path)
	}
	cfg := Config{Jira: raw.Jira.Jira}
	cfg.Jira.Enabled = meta.IsDefined("jira") && cfg.Jira.Enabled && !opts.NoJira
	if !cfg.Jira.Enabled {
		return cfg, nil
	}
	if err := validateJira(&cfg.Jira, raw.Jira.TimeoutText, filepath.Dir(path)); err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	if _, err := cfg.Jira.ReadToken(ctx); err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	return cfg, nil
}

// ReadToken rereads the credential file, allowing a later client to reload
// rotated credentials without putting tokens in settings or UI model state.
// Callers must not log or retain the returned token in UI state.
func (j Jira) ReadToken(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !j.Enabled {
		return "", errors.New("Jira is disabled")
	}
	data, err := os.ReadFile(j.TokenFile)
	if err != nil {
		return "", fmt.Errorf("read jira.token_file %q: %w", j.TokenFile, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("jira.token_file is empty")
	}
	return token, nil
}

func configPath(opts Options) (string, bool, error) {
	path := opts.Path
	explicit := opts.PathSet || path != ""
	if !explicit {
		path = os.Getenv("TASKSAMURAI_CONFIG")
		explicit = path != ""
	}
	if explicit && path == "" {
		return "", true, errors.New("config path must not be empty")
	}
	if !explicit {
		path = "~/.config/tasksamurai/config.toml"
	}
	path, err := expandHome(path)
	if err != nil {
		return "", explicit, fmt.Errorf("config path: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", explicit, fmt.Errorf("resolve config path: %w", err)
	}
	return path, explicit, nil
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	if !strings.HasPrefix(path, "~/") {
		return "", errors.New("only ~/ home expansion is supported")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}

func decodeError(path string, err error) error {
	// Parser diagnostics can quote file contents. Report location only to avoid
	// echoing accidentally inlined credentials or sensitive malformed values.
	var parseErr toml.ParseError
	if errors.As(err, &parseErr) {
		return fmt.Errorf("config %q line %d: invalid TOML or field type", path, parseErr.Position.Line)
	}
	return fmt.Errorf("config %q: invalid TOML or field type", path)
}

func validateJira(j *Jira, timeout, dir string) error {
	origin, err := url.Parse(j.Site)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || strings.Contains(j.Site, "#") {
		return errors.New("jira.site must be an HTTPS origin without userinfo, path, query or fragment")
	}
	address, err := mail.ParseAddress(j.Email)
	if err != nil || address.Address != j.Email {
		return errors.New("jira.email must be an email address")
	}
	if j.MaxResults <= 0 {
		return errors.New("jira.max_results must be positive")
	}
	if j.DefaultView != "tw" && j.DefaultView != "both" && j.DefaultView != "jira" {
		return errors.New("jira.default_view must be tw, both or jira")
	}
	duration, err := time.ParseDuration(timeout)
	if err != nil || duration <= 0 {
		return errors.New("jira.timeout must be a positive duration")
	}
	j.Timeout = duration
	if err := validateFilters(j.Filters, j.DefaultFilter); err != nil {
		return err
	}
	if strings.TrimSpace(j.TokenFile) == "" {
		return errors.New("jira.token_file is required")
	}
	j.TokenFile, err = expandHome(j.TokenFile)
	if err != nil {
		return fmt.Errorf("jira.token_file: %w", err)
	}
	if !filepath.IsAbs(j.TokenFile) {
		j.TokenFile = filepath.Join(dir, j.TokenFile)
	}
	return nil
}

func validateFilters(filters []Filter, selected string) error {
	names := make(map[string]bool, len(filters))
	for _, filter := range filters {
		if strings.TrimSpace(filter.Name) == "" {
			return errors.New("jira.filters name must not be empty")
		}
		if names[filter.Name] {
			return errors.New("jira.filters names must be unique")
		}
		if strings.TrimSpace(filter.JQL) == "" {
			return errors.New("jira.filters JQL must not be empty")
		}
		names[filter.Name] = true
	}
	if selected != "" && !names[selected] {
		return errors.New("jira.default_filter must reference an existing filter")
	}
	return nil
}
