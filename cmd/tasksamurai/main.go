package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/snonux/tasksamurai/internal"
	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/debug"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
	"github.com/snonux/tasksamurai/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	// Set default browser command depending on OS.
	browserCmdDefault := "firefox"
	if runtime.GOOS == "darwin" {
		browserCmdDefault = "open"
	}

	configPath := flag.String("config", "", "path to TOML configuration (overrides TASKSAMURAI_CONFIG)")
	noJira := flag.Bool("no-jira", false, "disable Jira configuration and credential validation")
	version := flag.Bool("version", false, "print version and exit")
	debugLog := flag.String("debug-log", "", "path to debug log file")
	debugDir := flag.String("debug-dir", "", "directory for runtime debug output (goroutine dumps, profiles)")
	browserCmd := flag.String("browser-cmd", browserCmdDefault, "command used to open URLs")
	// Defaults to chromium so youtube.com/youtu.be links play in a browser
	// better suited for video than the general default (firefox). Set to an
	// empty string ("") to route YouTube links through --browser-cmd like any
	// other URL.
	youtubeBrowserCmd := flag.String("youtube-browser-cmd", "chromium", "command used to open youtube.com/youtu.be links (defaults to chromium; set to \"\" to use --browser-cmd)")
	hotkeys := ui.DefaultHotkeys()
	flag.StringVar(&hotkeys.First, "first-hotkey", hotkeys.First, "key used to jump to the first task")
	flag.StringVar(&hotkeys.Random, "random-hotkey", hotkeys.Random, "key used to jump to a random task (table mode)")
	flag.StringVar(&hotkeys.RandomNoDue, "random-no-due-hotkey", hotkeys.RandomNoDue, "key used to jump to a random task without a due date (table mode)")
	flag.StringVar(&hotkeys.Agent, "agent-hotkey", hotkeys.Agent, "key used to toggle the +agent/-agent filter")
	flag.StringVar(&hotkeys.Auto, "auto-hotkey", hotkeys.Auto, "key used to toggle the +auto/-auto filter")
	flag.StringVar(&hotkeys.Track, "track-hotkey", hotkeys.Track, "key used to toggle the +track/-track filter")
	flag.StringVar(&hotkeys.Source, "jira-hotkey", hotkeys.Source, "key used to cycle Taskwarrior and Jira source views")
	disco := flag.Bool("disco", false, "enable disco mode")
	ultra := flag.Bool("ultra", false, "start directly in ultra mode")
	flag.Parse()

	if *version {
		fmt.Println(internal.Version)
		return
	}

	configPathSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configPathSet = true
		}
	})
	configOpts := config.Options{Path: *configPath, PathSet: configPathSet, NoJira: *noJira}
	cfg, err := config.Load(context.Background(), configOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load configuration:", err)
		os.Exit(1)
	}
	// Resolved once so ctrl+e edits the same file that was loaded, even when
	// it does not exist yet (the editor then creates it).
	resolvedConfigPath, err := config.Path(configOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to resolve configuration path:", err)
		os.Exit(1)
	}

	if err := task.SetDebugLog(*debugLog); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable debug log:", err)
		os.Exit(1)
	}

	// Set up runtime debugging signal handlers
	debug.SetDebugDir(*debugDir)
	debug.InitSignalHandlers()

	startup, err := jiraRuntime(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to initialize Jira:", err)
		os.Exit(1)
	}
	options := []ui.ModelOption{ui.WithConfigEditor(resolvedConfigPath, configReloader(configOpts))}
	if startup.Source != nil {
		options = append(options, ui.WithJira(startup.Jira, startup.Source, startup.ReloadToken))
	}
	m, err := ui.New(flag.Args(), *browserCmd, options...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load tasks:", err)
		os.Exit(1)
	}

	if err := m.SetHotkeys(hotkeys); err != nil {
		fmt.Fprintln(os.Stderr, "invalid hotkey configuration:", err)
		fmt.Fprintln(os.Stderr, "using default hotkeys (0, 1, 2, 3, 4, 5, 6)")
	}
	m.SetYouTubeBrowserCmd(*youtubeBrowserCmd)
	m.SetDisco(*disco)
	m.SetUltra(*ultra)

	// Clear the screen before starting the TUI to avoid leaving any
	// previous command line artefacts behind.
	fmt.Print("\033[H\033[2J")

	p := tea.NewProgram(&m)
	// Registered so SIGUSR1 (debugsignals builds) can ask the running model
	// to dump its own UI state, not just goroutine stacks - see
	// docs/debugging.md.
	debug.SetSender(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}

// jiraRuntime builds the Jira client for cfg. A disabled Jira config yields a
// ConfigReload without Source, which the UI treats as "Jira off".
func jiraRuntime(ctx context.Context, cfg config.Config) (ui.ConfigReload, error) {
	if !cfg.Jira.Enabled {
		return ui.ConfigReload{Jira: cfg.Jira}, nil
	}
	client, err := jira.New(ctx, cfg.Jira, nil)
	if err != nil {
		return ui.ConfigReload{}, err
	}
	return ui.ConfigReload{Jira: cfg.Jira, Source: client, ReloadToken: client.ReloadToken}, nil
}

// configReloader rereads the config with the startup options after ctrl+e, so
// --config, TASKSAMURAI_CONFIG and --no-jira keep applying to the reload.
func configReloader(opts config.Options) ui.ConfigReloader {
	return func(ctx context.Context) (ui.ConfigReload, error) {
		cfg, err := config.Load(ctx, opts)
		if err != nil {
			return ui.ConfigReload{}, err
		}
		return jiraRuntime(ctx, cfg)
	}
}
