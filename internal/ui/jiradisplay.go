package ui

import "github.com/snonux/tasksamurai/internal/task"

// taskDisplayProject avoids repeating the project already present in issue keys.
func taskDisplayProject(t task.Task) string {
	if isJiraTask(&t) {
		return ""
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
