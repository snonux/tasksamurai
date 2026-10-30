package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) scheduleJiraField(uuid, field string, value any) (tea.Cmd, error) {
	source, ok := m.jira.source.(FieldSource)
	if !ok {
		return nil, fmt.Errorf("Jira field edits are unavailable")
	}
	// Commands own their payload, including slices/maps from UI state.
	switch payload := value.(type) {
	case []string:
		value = append([]string{}, payload...)
	case map[string]string:
		copy := make(map[string]string, len(payload))
		for key, text := range payload {
			copy[key] = text
		}
		value = copy
	case map[string]any:
		copy, err := snapshotJiraFields(payload)
		if err != nil {
			return nil, err
		}
		value = copy
	}
	key := strings.TrimPrefix(uuid, "jira:")
	cmd, err := m.scheduleJiraMutation(uuid, func(ctx context.Context) (string, error) {
		if err := source.SetField(ctx, key, field, value); err != nil {
			return "", err
		}
		return "Jira " + field + " updated", nil
	})
	if err != nil {
		return nil, err
	}
	detailField := map[string]int{"duedate": fieldDue, "priority": fieldPriority, "labels": fieldTags, "summary": m.detailDescriptionFieldIndex()}[field]
	return func() tea.Msg {
		msg := cmd().(jiraMutationDoneMsg)
		msg.detailField = detailField
		return msg
	}, nil
}

func (m *Model) jiraFieldCommand(uuid, field string, value any) tea.Cmd {
	cmd, err := m.scheduleJiraField(uuid, field, value)
	if err != nil {
		m.showError(err)
	}
	return cmd
}

func (m *Model) jiraLabels(uuid string, adds, removes []string) ([]string, error) {
	cached := m.jiraCachedTask(uuid)
	if cached == nil {
		return nil, fmt.Errorf("Jira issue is no longer available")
	}
	removed := map[string]bool{}
	existing := cached.JiraLabels
	if existing == nil {
		for _, label := range cached.Tags {
			if label != "jira" {
				existing = append(existing, label)
			}
		}
	}
	// The display-only tag cannot create a real label. Existing source labels
	// named jira remain intact unless explicitly removed by the user.
	additions := []string{}
	for _, label := range adds {
		if label != "jira" {
			additions = append(additions, label)
		}
	}
	for _, label := range removes {
		removed[label] = true
	}
	labels, seen := []string{}, map[string]bool{}
	for _, label := range append(append([]string(nil), existing...), additions...) {
		if !removed[label] && !seen[label] {
			labels = append(labels, label)
			seen[label] = true
		}
	}
	return labels, nil
}

func (m *Model) priorityChoices() []string {
	if strings.HasPrefix(m.priorityAddr, "jira:") {
		return priorityOptions[:3]
	}
	return priorityOptions
}
