package main

import (
	"flag"
	"fmt"
	"os"

	"runtime"

	"codeberg.org/snonux/tasksamurai/internal/debug"
	"codeberg.org/snonux/tasksamurai/internal/task"
	"codeberg.org/snonux/tasksamurai/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	// Set default browser command depending on OS.
	browserCmdDefault := "firefox"
	if runtime.GOOS == "darwin" {
		browserCmdDefault = "open"
	}

	debugLog := flag.String("debug-log", "", "path to debug log file")
	debugDir := flag.String("debug-dir", "", "directory for runtime debug output (goroutine dumps, profiles)")
	browserCmd := flag.String("browser-cmd", browserCmdDefault, "command used to open URLs")
	agentHotkey := flag.String("agent-hotkey", "3", "key used to toggle the +agent/-agent filter")
	disco := flag.Bool("disco", false, "enable disco mode")
	ultra := flag.Bool("ultra", false, "start directly in ultra mode")
	flag.Parse()

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

	m.SetAgentFilterHotkey(*agentHotkey)
	m.SetDisco(*disco)
	m.SetUltra(*ultra)

	// Clear the screen before starting the TUI to avoid leaving any
	// previous command line artefacts behind.
	fmt.Print("\033[H\033[2J")

	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}
