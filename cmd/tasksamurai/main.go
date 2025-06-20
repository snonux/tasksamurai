package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"tasksamurai/internal/task"
	"tasksamurai/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	filter := flag.String("filter", "", "task filter expression")
	debugLog := flag.String("debug-log", "", "path to debug log file")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	if err := task.SetDebugLog(*debugLog); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable debug log:", err)
		os.Exit(1)
	}

	m, err := ui.New(*filter)
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
