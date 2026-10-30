package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "token"), " \tsecret with interior =13A2ACBF\r\n")
	return filepath.Join(dir, "config.toml"), "[jira]\nsite = 'https://example.atlassian.net'\nemail = 'person@example.com'\ntoken_file = 'token'\n"
}

func loadText(t *testing.T, path, body string, disabled bool) (Config, error) {
	t.Helper()
	writeFile(t, path, body)
	return Load(context.Background(), Options{Path: path, NoJira: disabled})
}

func TestLoadDefaultsFiltersAndNativeFields(t *testing.T) {
	path, body := fixture(t)
	body += "default_filter = 'second'\n[[jira.filters]]\nname = 'first'\njql = 'project = ONE'\n[[jira.filters]]\nname = 'second'\njql = 'assignee = currentUser()'\n[jira.transitions]\ndone = 'Close Issue'\n[jira.transition_fields]\nresolution = { name = 'Done' }\ncount = 3\nactive = true\nlabels = ['one', 'two']\n"
	cfg, err := loadText(t, path, body, false)
	if err != nil {
		t.Fatal(err)
	}
	j := cfg.Jira
	if !j.Enabled || !j.SavedFilters || j.DefaultView != "both" || j.MaxResults != 100 || j.Timeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", j)
	}
	if len(j.Filters) != 2 || j.Filters[0].Name != "first" || j.Filters[1].Name != "second" || j.DefaultFilter != "second" {
		t.Fatalf("unexpected filters: %+v", j.Filters)
	}
	if j.Transitions["done"] != "Close Issue" {
		t.Fatalf("unexpected transitions: %+v", j.Transitions)
	}
	got, err := json.Marshal(j.TransitionFields)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"active":true,"count":3,"labels":["one","two"],"resolution":{"name":"Done"}}`
	if string(got) != want {
		t.Fatalf("native values: got %s, want %s", got, want)
	}
	token, err := j.ReadToken(context.Background())
	if err != nil || token != "secret with interior =13A2ACBF" {
		t.Fatalf("token trimming failed; error: %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", cfg), "secret with interior") {
		t.Fatal("credential retained in config")
	}
}

func TestValidation(t *testing.T) {
	tests := []struct{ name, old, new, extra, want string }{
		{name: "missing site", old: "site = 'https://example.atlassian.net'", want: "jira.site"},
		{name: "HTTP", old: "https://example.atlassian.net", new: "http://example.atlassian.net", want: "jira.site"},
		{name: "userinfo", old: "https://example.atlassian.net", new: "https://user:secret@example.atlassian.net", want: "jira.site"},
		{name: "path", old: "https://example.atlassian.net", new: "https://example.atlassian.net/", want: "jira.site"},
		{name: "query", old: "https://example.atlassian.net", new: "https://example.atlassian.net?x=y", want: "jira.site"},
		{name: "empty query", old: "https://example.atlassian.net", new: "https://example.atlassian.net?", want: "jira.site"},
		{name: "fragment", old: "https://example.atlassian.net", new: "https://example.atlassian.net#x", want: "jira.site"},
		{name: "empty fragment", old: "https://example.atlassian.net", new: "https://example.atlassian.net#", want: "jira.site"},
		{name: "missing host", old: "https://example.atlassian.net", new: "https://", want: "jira.site"},
		{name: "invalid URL", old: "https://example.atlassian.net", new: "https://bad%host", want: "jira.site"},
		{name: "bad email", old: "person@example.com", new: "no-email", want: "jira.email"},
		{name: "display email", old: "person@example.com", new: "Person <person@example.com>", want: "jira.email"},
		{name: "missing email", old: "person@example.com", want: "jira.email"},
		{name: "zero max", extra: "max_results=0", want: "jira.max_results"},
		{name: "negative max", extra: "max_results=-1", want: "jira.max_results"},
		{name: "view", extra: "default_view='other'", want: "jira.default_view"},
		{name: "timeout", extra: "timeout='later'", want: "jira.timeout"},
		{name: "zero timeout", extra: "timeout='0s'", want: "jira.timeout"},
		{name: "negative timeout", extra: "timeout='-1s'", want: "jira.timeout"},
		{name: "empty token path", old: "token_file = 'token'", new: "token_file = ''", want: "jira.token_file"},
		{name: "missing token", old: "token_file = 'token'", new: "token_file = 'absent'", want: "jira.token_file"},
		{name: "directory token", old: "token_file = 'token'", new: "token_file = '.'", want: "jira.token_file"},
		{name: "tilde user", old: "token_file = 'token'", new: "token_file = '~person/token'", want: "only ~/"},
		{name: "empty name", extra: "[[jira.filters]]\nname=' '\njql='project = ONE'", want: "name must not be empty"},
		{name: "duplicate name", extra: "[[jira.filters]]\nname='x'\njql='project = ONE'\n[[jira.filters]]\nname='x'\njql='project = TWO'", want: "unique"},
		{name: "empty JQL", extra: "[[jira.filters]]\nname='x'\njql=' '", want: "JQL must not be empty"},
		{name: "default filter", extra: "default_filter='missing'", want: "existing filter"},
		{name: "unknown key", extra: "api_token='secret-sentinel'", want: "unknown TOML keys"},
		{name: "unknown filter key", extra: "[[jira.filters]]\nname='x'\njql='x'\ntypo=true", want: "unknown TOML keys"},
		{name: "type mismatch", extra: "max_results='secret-sentinel'", want: "invalid TOML"},
		{name: "malformed", extra: "timeout = [ secret-sentinel", want: "line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, body := fixture(t)
			if tt.old != "" {
				body = strings.Replace(body, tt.old, tt.new, 1)
			}
			_, err := loadText(t, path, body+tt.extra, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), path) {
				t.Fatalf("got %v, want path and %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "secret-sentinel") || strings.Contains(err.Error(), "user:secret") {
				t.Fatalf("error exposes sensitive input: %v", err)
			}
		})
	}
}

func TestDisabledConfigurations(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		noJira     bool
		wantErr    string
	}{
		{name: "empty file"},
		{name: "disabled", body: "[jira]\nenabled=false\nsite='bad'\ntoken_file='missing'\ntimeout='bad'"},
		{name: "runtime disabled", body: "[jira]\nsite='bad'\ntoken_file='missing'\ntimeout='bad'", noJira: true},
		{name: "malformed disabled", body: "[jira]\nenabled=false\nsite=[", wantErr: "line"},
		{name: "unknown disabled", body: "[jira]\nenabled=false\ntypo=true", wantErr: "unknown"},
		{name: "malformed runtime disabled", body: "[jira", noJira: true, wantErr: "line"},
		{name: "structural type disabled", body: "[jira]\nenabled=false\nmax_results='text'", wantErr: "invalid TOML"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadText(t, filepath.Join(t.TempDir(), "config.toml"), tt.body, tt.noJira)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got %v, want %s", err, tt.wantErr)
				}
				return
			}
			if err != nil || cfg.Jira.Enabled {
				t.Fatalf("disabled: %+v, %v", cfg, err)
			}
		})
	}
}

func TestPathPrecedenceAndMissingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TASKSAMURAI_CONFIG", "")
	cfg, err := Load(context.Background(), Options{})
	if err != nil || cfg.Jira.Enabled {
		t.Fatalf("missing default: %+v %v", cfg, err)
	}
	defaultPath := filepath.Join(home, ".config/tasksamurai/config.toml")
	writeFile(t, defaultPath, "[jira]\nenabled=false\ndefault_view='tw'")
	envPath := filepath.Join(home, "env.toml")
	writeFile(t, envPath, "[jira]\nenabled=false\ndefault_view='jira'")
	flagPath := filepath.Join(home, "flag.toml")
	writeFile(t, flagPath, "[jira]\nenabled=false\ndefault_view='both'")
	for _, tt := range []struct {
		name, env string
		opts      Options
		view      string
	}{
		{name: "default", view: "tw"},
		{name: "env", env: envPath, view: "jira"},
		{name: "flag", env: envPath, opts: Options{Path: flagPath}, view: "both"},
		{name: "flag home", env: envPath, opts: Options{Path: "~/flag.toml"}, view: "both"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TASKSAMURAI_CONFIG", tt.env)
			cfg, err := Load(context.Background(), tt.opts)
			if err != nil || cfg.Jira.DefaultView != tt.view {
				t.Fatalf("got %+v %v", cfg, err)
			}
		})
	}
	for _, opts := range []Options{{Path: filepath.Join(home, "missing")}, {Path: "~other/config"}, {PathSet: true}} {
		if _, err := Load(context.Background(), opts); err == nil {
			t.Fatalf("expected explicit path error for %+v", opts)
		}
	}
	t.Setenv("TASKSAMURAI_CONFIG", filepath.Join(home, "missing-env"))
	if _, err := Load(context.Background(), Options{NoJira: true}); err == nil {
		t.Fatal("missing explicit env path should fail even when disabled")
	}
	t.Setenv("TASKSAMURAI_CONFIG", "")
	if err := os.Remove(defaultPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(defaultPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), Options{}); err == nil {
		t.Fatal("unreadable default config should fail")
	}
}

func TestTokenResolutionAndReload(t *testing.T) {
	path, body := fixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	tokenPath := filepath.Join(home, "token")
	writeFile(t, tokenPath, "first=13A2ACBF\n")
	body = strings.Replace(body, "'token'", "'~/token'", 1)
	cfg, err := loadText(t, path, body, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jira.TokenFile != tokenPath {
		t.Fatalf("got token path %q", cfg.Jira.TokenFile)
	}
	writeFile(t, tokenPath, "second=13A2ACBF\n")
	token, err := cfg.Jira.ReadToken(context.Background())
	if err != nil || token != "second=13A2ACBF" {
		t.Fatalf("reload failed: %v", err)
	}
	writeFile(t, tokenPath, " \n\t")
	if _, err := cfg.Jira.ReadToken(context.Background()); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := Load(context.Background(), Options{Path: path}); err == nil {
		t.Fatal("empty token accepted at startup")
	}
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Jira.ReadToken(context.Background()); err == nil {
		t.Fatal("removed credential accepted")
	}
	if _, err := (Jira{}).ReadToken(context.Background()); err == nil {
		t.Fatal("disabled credentials read")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, Options{Path: path}); err != context.Canceled {
		t.Fatalf("load cancellation: %v", err)
	}
	if _, err := cfg.Jira.ReadToken(ctx); err != context.Canceled {
		t.Fatalf("token cancellation: %v", err)
	}
}

func TestExplicitOverrides(t *testing.T) {
	path, body := fixture(t)
	body += "default_view='jira'\nmax_results=4\ntimeout='250ms'\nsaved_filters=false\n"
	cfg, err := loadText(t, path, body, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jira.DefaultView != "jira" || cfg.Jira.MaxResults != 4 || cfg.Jira.Timeout != 250*time.Millisecond || cfg.Jira.SavedFilters {
		t.Fatalf("overrides: %+v", cfg.Jira)
	}
}

func TestCredentialsStayOutOfConfigAndErrors(t *testing.T) {
	path, body := fixture(t)
	cfg, err := loadText(t, path, body, false)
	if err != nil {
		t.Fatal(err)
	}
	const token = "secret with interior =13A2ACBF"
	basic := base64.StdEncoding.EncodeToString([]byte(cfg.Jira.Email + ":" + token))
	serialized, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	outputs := []string{fmt.Sprintf("%+v %#v", cfg, cfg), string(serialized)}
	if err := os.Remove(cfg.Jira.TokenFile); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Jira.ReadToken(context.Background()); err == nil {
		t.Fatal("missing token accepted")
	} else {
		outputs = append(outputs, err.Error())
	}
	if _, err := Load(context.Background(), Options{Path: path}); err == nil {
		t.Fatal("missing token accepted on reload")
	} else {
		outputs = append(outputs, err.Error())
	}
	for _, output := range outputs {
		if strings.Contains(output, token) || strings.Contains(output, basic) {
			t.Fatal("credentials exposed in settings or errors")
		}
	}
}
