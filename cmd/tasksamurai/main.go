package main

import (
	"fmt"
	"os"

	"tasksamurai/internal"
	"tasksamurai/internal/task"
	"tasksamurai/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	tasks, err := task.Export()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to export tasks:", err)
		os.Exit(1)
	}

	var items []list.Item
	for _, t := range tasks {
		if t.Status != "completed" {
			items = append(items, taskItem{t})
		}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = fmt.Sprintf("tasksamurai %s", internal.Version)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	m := ui.New(l)

	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}

type taskItem struct{ t task.Task }

func (i taskItem) Title() string       { return i.t.Description }
func (i taskItem) Description() string { return "" }
func (i taskItem) FilterValue() string { return i.t.Description }
