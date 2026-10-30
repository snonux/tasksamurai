package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type jiraWorklogEditor struct {
	editing bool
	uuid    string
	input   textinput.Model
}

func (m *Model) handleLogJiraWork() (tea.Model, tea.Cmd) {
	if m.showHelp || m.shellOutputVisible {
		return m, nil
	}
	t := m.getTaskForOpenURL()
	if !isJiraTask(t) {
		m.statusMsg = "Work logging is available on Jira issues"
		return m, nil
	}
	if _, ok := m.jira.source.(WorklogSource); !ok {
		m.statusMsg = "Jira work logging is unavailable"
		return m, nil
	}
	if m.jira.mutating || m.jira.commenting {
		m.statusMsg = "Jira write is still being submitted"
		return m, nil
	}
	m.clearEditingModes()
	input := textinput.New()
	input.Prompt = "Log work (1h 30m; optional note): "
	input.CharLimit = 1024
	input.SetWidth(max(10, m.tbl.Width()-len(input.Prompt)))
	m.worklog = jiraWorklogEditor{editing: true, uuid: t.UUID, input: input}
	m.updateTableHeight()
	return m, m.worklog.input.Focus()
}

func (m *Model) handleJiraWorklogInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.worklog.editing = false
		m.worklog.input.Blur()
		m.updateTableHeight()
		return m, nil
	case "enter":
		seconds, comment, err := parseWorklogInput(m.worklog.input.Value())
		if err != nil {
			m.showError(err)
			return m, nil
		}
		source, ok := m.jira.source.(WorklogSource)
		if !ok {
			m.statusMsg = "Jira work logging is unavailable"
			return m, nil
		}
		uuid := m.worklog.uuid
		started := time.Now().Add(-time.Duration(seconds) * time.Second)
		cmd, err := m.scheduleJiraMutation(uuid, func(ctx context.Context) (string, error) {
			_, err := source.AddWorklog(ctx, strings.TrimPrefix(uuid, "jira:"), started, seconds, comment)
			return "Logged " + formatWorkTime(seconds) + " on " + strings.TrimPrefix(uuid, "jira:"), err
		})
		if err != nil {
			m.showError(err)
			return m, nil
		}
		m.worklog.editing = false
		m.worklog.input.Blur()
		m.updateTableHeight()
		return m, cmd
	}
	var cmd tea.Cmd
	m.worklog.input, cmd = m.worklog.input.Update(msg)
	return m, cmd
}

func parseWorklogInput(value string) (int64, string, error) {
	durationText, comment, _ := strings.Cut(value, ";")
	duration, err := time.ParseDuration(strings.Join(strings.Fields(durationText), ""))
	seconds := int64(duration / time.Second)
	if err != nil || duration <= 0 || duration%time.Second != 0 || seconds > 2147483647 {
		return 0, "", fmt.Errorf("enter a positive duration in h/m/s, e.g. 1h 30m; optional note (whole seconds only)")
	}
	return seconds, strings.TrimSpace(comment), nil
}

func formatWorkTime(seconds int64) string {
	parts := []string{}
	if hours := seconds / 3600; hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes := seconds % 3600 / 60; minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if remainder := seconds % 60; remainder > 0 {
		parts = append(parts, fmt.Sprintf("%ds", remainder))
	}
	if len(parts) == 0 {
		return "0m"
	}
	return strings.Join(parts, " ")
}
