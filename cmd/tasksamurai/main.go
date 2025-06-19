package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
		formatDate(t.Due),
		urg,
		strings.Join(anns, "; "),
	}
}

func formatDate(s string) string {
	if s == "" {
		return ""
	}
	if ts, err := time.Parse("20060102T150405Z", s); err == nil {
		return ts.Format("2006-01-02")
	}
	return s
}
