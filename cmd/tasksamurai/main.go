package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"tasksamurai/internal/task"
	"tasksamurai/internal/ui"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	tasks, err := task.Export()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to export tasks:", err)
		os.Exit(1)
	}

	var rows []table.Row
	for _, t := range tasks {
		if t.Status == "completed" {
			continue
		}
		rows = append(rows, taskToRow(t))
	}

	m := ui.New(rows)

	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running ui:", err)
		os.Exit(1)
	}
}

func taskToRow(t task.Task) table.Row {
	active := ""
	if t.Start != "" {
		active = "yes"
	}

	age := ""
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
		days := int(time.Since(ts).Hours() / 24)
		age = fmt.Sprintf("%dd", days)
	}

	tags := strings.Join(t.Tags, ",")
	urg := fmt.Sprintf("%.1f", t.Urgency)

	var anns []string
	for _, a := range t.Annotations {
		anns = append(anns, a.Description)
	}

	return table.Row{
		strconv.Itoa(t.ID),
		t.Description,
		active,
		age,
		t.Priority,
		tags,
		t.Recur,
		formatDue(t.Due),
		urg,
		strings.Join(anns, "; "),
	}
}

func formatDue(s string) string {
	if s == "" {
		return ""
	}
	ts, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		return s
	}

	days := int(time.Until(ts).Hours() / 24)
	val := fmt.Sprintf("%dd", days)
	if days < 0 {
		val = lipgloss.NewStyle().Background(lipgloss.Color("1")).Render(val)
	}
	return val
}
