package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/task"
)

// ModelOption configures optional model capabilities before the initial export.
type ModelOption func(*Model) error

type jiraState struct {
	source      IssueSource
	settings    config.Jira
	reloadToken func(context.Context) error
	view        string
	filter      config.Filter
	filterID    string
	tasks       []task.Task
	lastFetched time.Time
	loading     bool
	gen         int
	cancel      context.CancelFunc
	displayIDs  map[string]int
	nextID      int
	closed      bool
}

type jiraReloadDoneMsg struct {
	gen     int
	tasks   []task.Task
	fetched time.Time
	err     error
}

// WithJira enables the optional source. Credentials stay behind source and the
// reload callback; settings contain only non-secret configuration.
func WithJira(settings config.Jira, source IssueSource, reloadToken func(context.Context) error) ModelOption {
	return func(m *Model) error {
		if !settings.Enabled || source == nil {
			return nil
		}
		v := reflect.ValueOf(source)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return fmt.Errorf("Jira source is nil")
		}
		view := settings.DefaultView
		if view == "" {
			view = "both"
		}
		if view != "tw" && view != "both" && view != "jira" {
			return fmt.Errorf("invalid Jira source view")
		}
		m.jira = jiraState{source: source, settings: settings, reloadToken: reloadToken, view: view, filter: config.Filter{Name: "none"}, displayIDs: make(map[string]int), nextID: -1}
		for _, filter := range settings.Filters {
			if filter.Name == settings.DefaultFilter {
				m.jira.filter = filter
				m.jira.filterID = "local:" + filter.Name
				break
			}
		}
		return nil
	}
}

// scheduleJiraReload is independent of the Taskwarrior single-flight gate.
// Every command captures immutable arguments; only Update changes model data.
func (m *Model) scheduleJiraReload(rotateToken bool) tea.Cmd {
	if m.jira.source == nil || m.jira.closed {
		return nil
	}
	if m.jira.cancel != nil {
		m.jira.cancel()
	}
	timeout := m.jira.settings.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	m.jira.cancel = cancel
	m.jira.gen++
	m.jira.loading = true
	gen, source, jql, reloadToken := m.jira.gen, m.jira.source, m.jira.filter.JQL, m.jira.reloadToken
	return func() tea.Msg {
		defer cancel()
		if rotateToken && reloadToken != nil {
			if err := reloadToken(ctx); err != nil {
				return jiraReloadDoneMsg{gen: gen, err: err}
			}
		}
		tasks, err := source.FetchIssues(ctx, jql)
		return jiraReloadDoneMsg{gen: gen, tasks: tasks, err: err, fetched: time.Now()}
	}
}

func (m *Model) handleJiraReloadDone(msg jiraReloadDoneMsg) (tea.Model, tea.Cmd) {
	if m.jira.closed || msg.gen != m.jira.gen || !m.jira.loading {
		return m, nil
	}
	m.jira.loading = false
	m.jira.cancel = nil
	if msg.err != nil {
		m.showError(fmt.Errorf("Jira refresh: %w", msg.err))
		return m, nil
	}
	m.jira.tasks = cloneTasks(msg.tasks)
	m.jira.lastFetched = msg.fetched
	m.rebuildJiraRows()
	return m, nil
}

// setJiraFilter clears the previous filter's rows before fetching the new one.
// A same-filter refresh keeps its cache if the request fails.
func (m *Model) setJiraFilter(filter config.Filter, identity string) tea.Cmd {
	if m.jira.source == nil {
		return nil
	}
	if identity != m.jira.filterID || filter.JQL != m.jira.filter.JQL {
		m.jira.tasks = nil
		m.jira.lastFetched = time.Time{}
	}
	m.jira.filter = filter
	m.jira.filterID = identity
	m.rebuildJiraRows()
	return m.scheduleJiraReload(false)
}

func (m *Model) rebuildJiraRows() {
	data := reloadData{tasks: cloneTasks(m.twTasks), ultraFilterIDs: m.ultraFilteredTaskIDs()}
	m.processTasks(&data)
	m.renderTasks(data)
	if m.blinkID != 0 {
		m.blinkRow = m.taskIndexByID(m.blinkID)
		m.updateBlinkRow()
	}
}

func (m *Model) mergeJiraRows(tw []task.Task) []task.Task {
	m.twTasks = cloneTasks(tw)
	if m.jira.source == nil {
		return tw
	}
	combined := make([]task.Task, 0, len(tw)+len(m.jira.tasks))
	if m.jira.view != "jira" {
		combined = append(combined, cloneTasks(tw)...)
	}
	if m.jira.view != "tw" {
		cutoff := time.Now().Add(-m.completedWindow)
		for _, t := range cloneTasks(m.jira.tasks) {
			if !isJiraTask(&t) {
				continue
			}
			if t.Status == "completed" {
				end, err := time.Parse(task.DateFormat, t.End)
				if m.completedWindow <= 0 || err != nil || end.Before(cutoff) {
					continue
				}
			}
			combined = append(combined, t)
		}
	}
	m.assignJiraDisplayIDs(combined)
	m.taskwarriorClient().SortTasks(combined)
	return combined
}

func (m *Model) assignJiraDisplayIDs(tasks []task.Task) {
	used := make(map[int]string)
	for _, t := range tasks {
		if !isJiraTask(&t) && t.ID != 0 {
			used[t.ID] = t.UUID
		}
	}
	for i := range tasks {
		t := &tasks[i]
		if !isJiraTask(t) && !(t.Status == "completed" && t.ID <= 0) {
			continue
		}
		key := t.UUID
		if key == "" {
			key = fmt.Sprintf("legacy-completed:%d", i)
		}
		id, ok := m.jira.displayIDs[key]
		if owner, occupied := used[id]; !ok || (occupied && owner != key) {
			for {
				if _, occupied := used[m.jira.nextID]; m.jira.nextID != 0 && !occupied {
					break
				}
				m.jira.nextID--
			}
			id = m.jira.nextID
			m.jira.nextID--
			m.jira.displayIDs[key] = id
		}
		t.ID = id
		used[id] = key
	}
}

func (m *Model) jiraStatusToken() string {
	if m.jira.source == nil {
		return ""
	}
	age := "never"
	if !m.jira.lastFetched.IsZero() {
		age = m.jira.lastFetched.Local().Format("15:04")
	}
	if m.jira.loading {
		age += " ↻"
	}
	name := ansi.Truncate(m.jira.filter.Name, 18, "…")
	limit := ""
	if m.jira.settings.MaxResults > 0 && len(m.jira.tasks) >= m.jira.settings.MaxResults {
		limit = " · limit"
	}
	return fmt.Sprintf("[%s · jira: %s · %s%s]", m.jira.view, name, age, limit)
}

func cloneTasks(tasks []task.Task) []task.Task {
	copyTasks := append([]task.Task(nil), tasks...)
	for i := range copyTasks {
		copyTasks[i].Tags = append([]string(nil), copyTasks[i].Tags...)
		copyTasks[i].Annotations = append([]task.Annotation(nil), copyTasks[i].Annotations...)
	}
	return copyTasks
}

func isJiraTask(t *task.Task) bool { return t != nil && strings.HasPrefix(t.UUID, "jira:") }
