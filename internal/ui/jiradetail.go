package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type jiraDetailDoneMsg struct {
	gen, detailGen int
	uuid           string
	issue          task.Task
	comments       jira.CommentPage
	err            error
}

type jiraCommentDoneMsg struct {
	uuid, filterID, jql string
	annotation          task.Annotation
	err                 error
}

func (m *Model) jiraContext() (context.Context, context.CancelFunc) {
	timeout := m.jira.settings.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	// Taskwarrior recreates its gate context when a flight is superseded.
	// Jira requests have independent cancellation on detail close and quit.
	return context.WithTimeout(context.Background(), timeout)
}

// invalidateJiraReads prevents requests begun before a write from replacing it.
func (m *Model) invalidateJiraReads() {
	m.jira.gen++
	m.jira.loading = false
	if m.jira.cancel != nil {
		m.jira.cancel()
		m.jira.cancel = nil
	}
	m.cancelJiraDetail()
}

func (m *Model) cancelJiraDetail() {
	m.jira.detailGen++
	if m.jira.detailCancel != nil {
		m.jira.detailCancel()
		m.jira.detailCancel = nil
	}
}

func (m *Model) scheduleJiraDetail(t *task.Task) tea.Cmd {
	m.cancelJiraDetail()
	if !isJiraTask(t) || m.jira.source == nil || m.jira.closed {
		return nil
	}
	m.invalidateJiraReads()
	ctx, cancel := m.jiraContext()
	m.jira.detailCancel = cancel
	source, uuid, gen, detailGen := m.jira.source, t.UUID, m.jira.gen, m.jira.detailGen
	return func() tea.Msg {
		defer cancel()
		key := strings.TrimPrefix(uuid, "jira:")
		issue, err := source.FetchIssue(ctx, key)
		var comments jira.CommentPage
		if err == nil {
			comments, err = source.FetchComments(ctx, key)
		}
		return jiraDetailDoneMsg{gen: gen, detailGen: detailGen, uuid: uuid, issue: issue, comments: comments, err: err}
	}
}

func (m *Model) jiraCachedTask(uuid string) *task.Task {
	for i := range m.jira.tasks {
		if m.jira.tasks[i].UUID == uuid {
			return &m.jira.tasks[i]
		}
	}
	return nil
}

func (m *Model) handleJiraDetailDone(msg jiraDetailDoneMsg) (tea.Model, tea.Cmd) {
	if m.jira.closed || msg.gen != m.jira.gen || msg.detailGen != m.jira.detailGen || !m.showTaskDetail || msg.uuid != m.currentTaskDetailUUID {
		return m, nil
	}
	m.jira.detailCancel = nil
	if msg.err != nil {
		m.showError(fmt.Errorf("Jira detail: %w", msg.err))
		return m, nil
	}
	cached := m.jiraCachedTask(msg.uuid)
	if cached == nil {
		return m, nil
	}
	if msg.issue.UUID != msg.uuid {
		m.showError(fmt.Errorf("Jira detail: issue identity changed"))
		return m, nil
	}
	issue := cloneTasks([]task.Task{msg.issue})[0]
	issue.Annotations = append([]task.Annotation(nil), msg.comments.Annotations...)
	*cached = issue
	if m.jira.remaining == nil {
		m.jira.remaining = make(map[string]int)
	}
	m.jira.remaining[msg.uuid] = msg.comments.Remaining
	m.rebuildJiraRows()
	return m, nil
}

func (m *Model) scheduleJiraComment(value string) (tea.Cmd, error) {
	if m.replaceAnnotations {
		return nil, fmt.Errorf("%s", jiraBlockedReason("A"))
	}
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("comment cannot be empty")
	}
	if m.jira.closed || m.jira.source == nil || m.jiraCachedTask(m.annotateAddr) == nil {
		return nil, fmt.Errorf("Jira issue is no longer available")
	}
	if m.jira.commenting {
		return nil, fmt.Errorf("Jira comment is still being submitted")
	}
	if m.jira.mutating {
		return nil, fmt.Errorf("Jira write is still being submitted")
	}
	m.invalidateJiraReads()
	ctx, cancel := m.jiraContext()
	m.jira.commentCancel, m.jira.commenting = cancel, true
	source, uuid, filterID, jql := m.jira.source, m.annotateAddr, m.jira.filterID, m.jira.filter.JQL
	return func() tea.Msg {
		defer cancel()
		annotation, err := source.AddComment(ctx, strings.TrimPrefix(uuid, "jira:"), value)
		return jiraCommentDoneMsg{uuid: uuid, filterID: filterID, jql: jql, annotation: annotation, err: err}
	}, nil
}

func (m *Model) handleJiraCommentDone(msg jiraCommentDoneMsg) (tea.Model, tea.Cmd) {
	if m.jira.closed || !m.jira.commenting {
		return m, nil
	}
	m.jira.commenting, m.jira.commentCancel = false, nil
	if msg.err != nil {
		m.showError(fmt.Errorf("Jira comment: %w", msg.err))
		return m, nil
	}
	// Also invalidate searches opened while the POST was in flight.
	m.invalidateJiraReads()
	if msg.filterID != m.jira.filterID || msg.jql != m.jira.filter.JQL {
		return m, nil
	}
	cached := m.jiraCachedTask(msg.uuid)
	if cached == nil {
		return m, nil
	}
	cached.Annotations = append(cached.Annotations, msg.annotation)
	m.rebuildJiraRows()
	if m.showTaskDetail && m.currentTaskDetailUUID == msg.uuid {
		return m, m.startDetailBlink(m.detailDescriptionFieldIndex() + 1)
	}
	if t := m.taskByUUID(msg.uuid); t != nil {
		return m, m.startBlink(t.ID, false)
	}
	return m, nil
}
