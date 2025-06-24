package main

import (
	"flag"
	"fmt"
	"os"

	"codeberg.org/snonux/tasksamurai/internal/task"
	"codeberg.org/snonux/tasksamurai/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	debugLog := flag.String("debug-log", "", "path to debug log file")
	browserCmd := flag.String("browser-cmd", "firefox", "command used to open URLs")
	flag.Parse()

	if err := task.SetDebugLog(*debugLog); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable debug log:", err)
		os.Exit(1)
	}

	m, err := ui.New(flag.Args(), *browserCmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load tasks:", err)
		os.Exit(1)
	}

	// Clear the screen before starting the TUI to avoid leaving any
	// previous command line artefacts behind.
	fmt.Print("\033[H\033[2J")

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}
