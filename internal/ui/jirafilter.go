package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/snonux/tasksamurai/internal/config"
)

type jiraFilterEntry struct {
	filter   config.Filter
	identity string
}

type jiraFilterPicker struct {
	open    bool
	cursor  int
	entries []jiraFilterEntry
}

func (m *Model) handleJiraFilterPicker() (tea.Model, tea.Cmd) {
	if m.jira.source == nil {
		return m, nil
	}
	entries := []jiraFilterEntry{{filter: config.Filter{Name: "none"}}}
	cursor := 0
	for _, filter := range m.jira.settings.Filters {
		entries = append(entries, jiraFilterEntry{filter: filter, identity: "local:" + filter.Name})
		if entries[len(entries)-1].identity == m.jira.filterID {
			cursor = len(entries) - 1
		}
	}
	m.jira.picker = jiraFilterPicker{open: true, entries: entries, cursor: cursor}
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleJiraFilterPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	picker := &m.jira.picker
	switch msg.String() {
	case "esc":
		picker.open = false
		m.updateTableHeight()
	case "j", "down":
		if picker.cursor < len(picker.entries)-1 {
			picker.cursor++
		}
	case "k", "up":
		if picker.cursor > 0 {
			picker.cursor--
		}
	case "enter":
		if picker.cursor < 0 || picker.cursor >= len(picker.entries) {
			return m, nil
		}
		entry := picker.entries[picker.cursor]
		picker.open = false
		m.updateTableHeight()
		m.statusMsg = "jira filter: " + entry.filter.Name
		return m, m.setJiraFilter(entry.filter, entry.identity)
	}
	return m, nil
}

func (m *Model) jiraFilterPickerView() string {
	picker := m.jira.picker
	width := m.ultraRenderWidth()
	lines := []string{ansi.Truncate("Jira filters · j/k/↑/↓ · Enter apply · Esc cancel", width, "…")}
	visible := 8
	if m.windowHeight > 0 {
		visible = max(1, min(visible, m.windowHeight/3))
	}
	start := max(0, picker.cursor-visible+1)
	end := min(len(picker.entries), start+visible)
	dim := lipgloss.NewStyle().Faint(true)
	for i := start; i < end; i++ {
		entry := picker.entries[i]
		prefix := "  "
		if i == picker.cursor {
			prefix = "> "
		}
		label := entry.filter.Name
		if entry.identity == "" {
			label = "(none) all issues"
		}
		label = ansi.Truncate(prefix+strings.Join(strings.Fields(label), " "), width, "…")
		if remaining := width - ansi.StringWidth(label) - 2; remaining > 0 && entry.filter.JQL != "" {
			hint := strings.Join(strings.Fields(entry.filter.JQL), " ")
			label += "  " + dim.Render(ansi.Truncate(hint, remaining, "…"))
		}
		lines = append(lines, label)
	}
	if len(picker.entries) > visible {
		lines = append(lines, ansi.Truncate(fmt.Sprintf("%d/%d", picker.cursor+1, len(picker.entries)), width, "…"))
	}
	return strings.Join(lines, "\n")
}
