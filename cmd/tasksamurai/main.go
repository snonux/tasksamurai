package main

import (
	"flag"
	"fmt"
	"os"

	"tasksamurai/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	filter := flag.String("filter", "", "task filter expression")
	flag.Parse()

	m, err := ui.New(*filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load tasks:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}
