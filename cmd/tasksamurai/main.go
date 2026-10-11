package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/snonux/tasksamurai/internal"
	"github.com/snonux/tasksamurai/internal/debug"
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
	disco := flag.Bool("disco", false, "enable disco mode")
	ultra := flag.Bool("ultra", false, "start directly in ultra mode")
	flag.Parse()

	if *version {
		fmt.Println(internal.Version)
		return
	}

	if err := task.SetDebugLog(*debugLog); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable debug log:", err)
		os.Exit(1)
	}

	// Set up runtime debugging signal handlers
	debug.SetDebugDir(*debugDir)
	debug.InitSignalHandlers()

	m, err := ui.New(flag.Args(), *browserCmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load tasks:", err)
		os.Exit(1)
	}

	if err := m.SetHotkeys(hotkeys); err != nil {
		fmt.Fprintln(os.Stderr, "invalid hotkey configuration:", err)
		fmt.Fprintln(os.Stderr, "using default hotkeys (0, 1, 2, 4, 5)")
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
