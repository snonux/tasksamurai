package ui

import "strings"

// jiraBlockedReason is the baseline policy installed before Jira rows enter
// the table. Feature tasks replace planned controls with Jira implementations.
func jiraBlockedReason(key string) string {
	switch key {
	case "A":
		return "comments can't be replaced wholesale — use a to add"
	case "D":
		return "Jira issues can't be deleted from here"
	case "U":
		return "no undo for Jira issues"
	case "R", "ctrl+r":
		return "Jira issues have no recurrence"
	case "J", "T":
		return "moving Jira issues not supported (yet)"
	case ";":
		return "task commands don't work on Jira issues"
	case "a":
		return "not yet — Jira comment support is planned"
	case "d", "s":
		return "not yet — Jira write support for transitions is planned"
	case "w", "W", "r":
		return "not yet — Jira write support for due dates is planned"
	case "p":
		return "not yet — Jira write support for priority is planned"
	case "t":
		return "not yet — Jira write support for labels is planned"
	case "e", "E":
		return "not yet — Jira write support for summary is planned"
	}
	return ""
}

func (m *Model) blockJiraAction(key string) bool {
	t := m.getTaskForOpenURL()
	if !isJiraTask(t) {
		return false
	}
	return m.blockJiraAddress(t.UUID, key)
}

func (m *Model) blockJiraAddress(addr, key string) bool {
	if !strings.HasPrefix(addr, "jira:") {
		return false
	}
	if reason := jiraBlockedReason(key); reason != "" {
		m.statusMsg = reason
		return true
	}
	return false
}
