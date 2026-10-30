package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type jiraMutationDoneMsg struct {
	detailField               int
	gen                       int
	filterEpoch               int
	uuid, filterID, jql, note string
	issue                     task.Task
	applied                   bool
	err                       error
}

func (m *Model) scheduleJiraTransition(t *task.Task, action string) tea.Cmd {
	if !isJiraTask(t) || m.jira.closed {
		return nil
	}
	if action == "done" && t.Status == "completed" {
		m.statusMsg = "Jira issue is already completed"
		return nil
	}
	if action == "toggle" && t.Status == "completed" {
		m.statusMsg = "Cannot start a completed Jira issue"
		return nil
	}
	source, ok := m.jira.source.(TransitionSource)
	if !ok {
		m.statusMsg = "Jira transitions are unavailable"
		return nil
	}
	category, overrideKey := "done", "done"
	if action == "toggle" {
		category, overrideKey = "indeterminate", "start"
		if t.Start != "" {
			category, overrideKey = "new", "stop"
		}
	}
	defaults, err := snapshotJiraFields(m.jira.settings.TransitionFields)
	if err != nil {
		m.showError(err)
		return nil
	}
	key, override := strings.TrimPrefix(t.UUID, "jira:"), m.jira.settings.Transitions[overrideKey]
	cmd, err := m.scheduleJiraMutation(t.UUID, func(ctx context.Context) (string, error) {
		available, err := source.Transitions(ctx, key)
		if err != nil {
			return "", err
		}
		selected, fields, err := jira.ChooseTransition(available, category, override, defaults)
		if err != nil {
			return "", err
		}
		if err := source.DoTransition(ctx, key, selected.ID, fields); err != nil {
			return "", err
		}
		return "transitioned via '" + selected.Name + "'", nil
	})
	if err != nil {
		m.showError(err)
	}
	return cmd
}

// scheduleJiraMutation serializes Jira writes, then reads just the affected
// issue. A failed read after a successful write never repeats the mutation.
func (m *Model) scheduleJiraMutation(uuid string, op func(context.Context) (string, error)) (tea.Cmd, error) {
	if m.jira.closed || m.jira.source == nil || m.jiraCachedTask(uuid) == nil {
		return nil, fmt.Errorf("Jira issue is no longer available")
	}
	if m.jira.mutating || m.jira.commenting {
		return nil, fmt.Errorf("Jira write is still being submitted")
	}
	m.invalidateJiraReads()
	ctx, cancel := m.jiraContext()
	m.jira.mutationCancel, m.jira.mutating = cancel, true
	m.jira.mutationGen++
	source, gen, filterID, jql, filterEpoch := m.jira.source, m.jira.mutationGen, m.jira.filterID, m.jira.filter.JQL, m.jira.filterEpoch
	return func() tea.Msg {
		defer cancel()
		note, err := op(ctx)
		msg := jiraMutationDoneMsg{gen: gen, uuid: uuid, filterID: filterID, jql: jql, filterEpoch: filterEpoch, note: note, err: err, detailField: fieldStatus}
		if err != nil {
			return msg
		}
		msg.applied = true
		msg.issue, msg.err = source.FetchIssue(ctx, strings.TrimPrefix(uuid, "jira:"))
		return msg
	}, nil
}

func (m *Model) handleJiraMutationDone(msg jiraMutationDoneMsg) (tea.Model, tea.Cmd) {
	if m.jira.closed || !m.jira.mutating || msg.gen != m.jira.mutationGen {
		return m, nil
	}
	m.jira.mutating, m.jira.mutationCancel = false, nil
	if !msg.applied {
		m.showError(fmt.Errorf("Jira write: %w", msg.err))
		return m, nil
	}
	// A different filter owns its own cache and reads; do not cancel those.
	sameFilter := msg.filterEpoch == m.jira.filterEpoch && msg.filterID == m.jira.filterID && msg.jql == m.jira.filter.JQL
	if sameFilter {
		m.invalidateJiraReads()
	}
	if msg.err != nil {
		m.showError(fmt.Errorf("Jira write succeeded, but issue refresh failed: %w", msg.err))
		return m, nil
	}
	if msg.issue.UUID != msg.uuid {
		m.showError(fmt.Errorf("Jira write succeeded, but refresh returned a different issue"))
		return m, nil
	}
	if !sameFilter {
		m.statusMsg = msg.note
		return m, nil
	}
	if cached := m.jiraCachedTask(msg.uuid); cached != nil {
		issue := cloneTasks([]task.Task{msg.issue})[0]
		// The fields-only GET contains no comments. Keep the separately fetched
		// comment page and its remaining count intact when patching fields.
		issue.Annotations = append([]task.Annotation(nil), cached.Annotations...)
		*cached = issue
	}
	m.rebuildJiraRows()
	m.statusMsg = msg.note
	if m.showTaskDetail && m.currentTaskDetailUUID == msg.uuid {
		return m, m.startDetailBlink(msg.detailField)
	}
	if t := m.taskByUUID(msg.uuid); t != nil {
		return m, m.startBlink(t.ID, false)
	}
	return m, nil
}

func snapshotJiraFields(fields map[string]any) (map[string]any, error) {
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("invalid Jira field defaults")
	}
	var copy map[string]any
	if json.Unmarshal(data, &copy) != nil {
		return nil, fmt.Errorf("invalid Jira field defaults")
	}
	return copy, nil
}
