package ui

import "github.com/snonux/tasksamurai/internal/task"

// taskDisplayProject uses the shared project column for Jira workflow status.
func taskDisplayProject(t task.Task) string {
	if isJiraTask(&t) {
		return t.JiraStatus
	}
	return t.Project
}

// taskDisplayTags hides the synthetic source tag while retaining real labels.
func taskDisplayTags(t task.Task) []string {
	if !isJiraTask(&t) {
		return t.Tags
	}
	if t.JiraLabels != nil {
		return t.JiraLabels
	}
	labels := make([]string, 0, len(t.Tags))
	for _, label := range t.Tags {
		if label != "jira" {
			labels = append(labels, label)
		}
	}
	return labels
}

// taskDisplayRecur uses the shared recurrence column for the Jira assignee.
func taskDisplayRecur(t task.Task) string {
	if isJiraTask(&t) {
		if t.Assignee == "" {
			return "Unassigned"
		}
		return t.Assignee
	}
	return t.Recur
}

func (m *Model) sourceColumnTitle(tw, jira string) string {
	if m.jira.source == nil || m.jira.view == "tw" {
		return tw
	}
	if m.jira.view == "jira" {
		return jira
	}
	return tw + " / " + jira
}
