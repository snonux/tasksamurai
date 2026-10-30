package ui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type fakeJiraSource struct {
	fetch    func(context.Context, string) ([]task.Task, error)
	calls    atomic.Int32
	issue    func(context.Context, string) (task.Task, error)
	comments func(context.Context, string) (jira.CommentPage, error)
	comment  func(context.Context, string, string) (task.Annotation, error)
}

func (f *fakeJiraSource) FetchIssues(ctx context.Context, jql string) ([]task.Task, error) {
	f.calls.Add(1)
	return f.fetch(ctx, jql)
}
func (f *fakeJiraSource) FetchIssue(ctx context.Context, key string) (task.Task, error) {
	if f.issue != nil {
		return f.issue(ctx, key)
	}
	return task.Task{}, errors.New("unexpected issue fetch")
}
func (f *fakeJiraSource) FetchComments(ctx context.Context, key string) (jira.CommentPage, error) {
	if f.comments != nil {
		return f.comments(ctx, key)
	}
	return jira.CommentPage{}, errors.New("unexpected comments fetch")
}
func (f *fakeJiraSource) AddComment(ctx context.Context, key, value string) (task.Annotation, error) {
	if f.comment != nil {
		return f.comment(ctx, key, value)
	}
	return task.Annotation{}, errors.New("unexpected comment mutation")
}

func jiraRows() []task.Task {
	return []task.Task{{UUID: "jira:TST-42", IDText: "TST-42", Description: "Jira issue", Status: "pending", Priority: "H", Tags: []string{"jira"}}}
}

func newJiraTestModel(t *testing.T, tw task.Taskwarrior, source IssueSource) Model {
	t.Helper()
	m, err := NewWithTaskwarrior(nil, "firefox", tw, WithJira(config.Jira{Enabled: true, DefaultView: "both", MaxResults: 100, Timeout: time.Second, Filters: []config.Filter{{Name: "mine", JQL: "project=TST"}}, DefaultFilter: "mine"}, source, nil))
	if err != nil {
		t.Fatal(err)
	}
	m.tbl.SetWidth(160)
	m.tbl.SetHeight(10)
	t.Cleanup(m.cancelTaskOperations)
	return m
}

func TestJiraStartupIsAsyncAndNavigationUsesCache(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	source := &fakeJiraSource{fetch: func(ctx context.Context, jql string) ([]task.Task, error) {
		close(started)
		if jql != "project=TST" {
			t.Errorf("default JQL %q", jql)
		}
		select {
		case <-release:
			return jiraRows(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW visible", Status: "pending"}}}
	m := newJiraTestModel(t, tw, source)
	if source.calls.Load() != 0 || len(m.tasks) != 1 || m.tasks[0].UUID != "tw-1" || !strings.Contains(ansi.Strip(m.tbl.View()), "TW visible") {
		t.Fatal("constructor waited for or fetched Jira")
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("missing initial async Jira command")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	if m.taskFlightActive() || len(m.tasks) != 1 {
		t.Fatal("Jira fetch gated or altered TW rows before completion")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	close(release)
	select {
	case msg := <-result:
		_, _ = m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("fetch did not finish")
	}
	if len(m.tasks) != 2 || m.taskByUUID("jira:TST-42") == nil || !strings.Contains(ansi.Strip(m.tbl.View()), "TST-42") {
		t.Fatalf("merged rows %+v", m.tasks)
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyUp}} {
		_, _ = m.Update(key)
	}
	if source.calls.Load() != 1 {
		t.Fatal("navigation refetched Jira")
	}
	if line := ansi.Strip(m.statusLine()); !strings.Contains(line, "jira: mine") || strings.Contains(line, "never") {
		t.Fatalf("missing cache timestamp %q", line)
	}
}

func TestJiraFailureRetainsSameFilterAndFilterChangeDropsRows(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}}}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	stamp := m.jira.lastFetched
	source.fetch = func(context.Context, string) ([]task.Task, error) { return nil, errors.New("no permission") }
	drainCmds(t, &m, m.scheduleJiraReload(false))
	if len(m.tasks) != 2 || !m.jira.lastFetched.Equal(stamp) || !strings.Contains(m.statusMsg, "no permission") {
		t.Fatal("refresh failure erased cache or timestamp/error incorrect")
	}
	cmd := m.setJiraFilter(config.Filter{Name: "other", JQL: "project=OTHER"}, "local:other")
	if len(m.tasks) != 1 || m.taskByUUID("tw-1") == nil || !m.jira.lastFetched.IsZero() {
		t.Fatal("filter change retained old-filter rows")
	}
	drainCmds(t, &m, cmd)
	if len(m.tasks) != 1 || m.taskByUUID("tw-1") == nil {
		t.Fatal("failed new filter affected TW")
	}
}

func TestJiraSupersededResponsesAndQuitAreIgnored(t *testing.T) {
	var contexts []context.Context
	source := &fakeJiraSource{fetch: func(ctx context.Context, jql string) ([]task.Task, error) {
		contexts = append(contexts, ctx)
		rows := jiraRows()
		rows[0].Description = jql
		return rows, nil
	}}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	old := m.Init()
	oldGen := m.jira.gen
	latest := m.setJiraFilter(config.Filter{Name: "new", JQL: "project=NEW"}, "local:new")
	oldMsg := old()
	if contexts[0].Err() != context.Canceled {
		t.Fatalf("old command not canceled: %v", contexts[0].Err())
	}
	drainCmds(t, &m, latest)
	_, _ = m.Update(oldMsg)
	if m.jira.gen == oldGen || len(m.tasks) != 1 || m.tasks[0].Description != "project=NEW" {
		t.Fatalf("stale result overwrote current filter %+v", m.tasks)
	}
	cmd := m.scheduleJiraReload(false)
	m.cancelTaskOperations()
	_, _ = m.Update(cmd())
	if m.tasks[0].Description != "project=NEW" {
		t.Fatal("result applied after quit")
	}
}

func TestJiraRefreshAndTWMutationCacheRules(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW", Status: "pending"}}}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	var rotations atomic.Int32
	m.jira.reloadToken = func(context.Context) error { rotations.Add(1); return nil }
	_, cmd := m.handleRefresh()
	drainCmds(t, &m, cmd)
	if source.calls.Load() != 2 || rotations.Load() != 1 {
		t.Fatal("space did not refetch/reread token")
	}
	// Real TW mutation+export pipeline, not merely a simulated reload message.
	cmd = m.scheduleMutateReload("Adding…", mutateMeta{}, func(ctx context.Context, tw task.Taskwarrior) error { return tw.AddLineContext(ctx, "new TW task") })
	drainCmds(t, &m, cmd)
	if source.calls.Load() != 2 || m.taskByUUID("jira:TST-42") == nil || len(m.tasks) != 3 {
		t.Fatal("TW mutation refetched/lost Jira")
	}
	m.autoRefresh = true
	m.autoRefreshGen = 1
	m.autoRefreshInterval = time.Hour
	_, cmd = m.handleAutoRefresh(autoRefreshMsg{gen: 1})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("auto refresh did not schedule independent flights")
	}
	for _, child := range batch {
		_, _ = m.Update(child())
	}
	if source.calls.Load() != 3 || rotations.Load() != 1 {
		t.Fatal("auto refresh missing Jira fetch or unexpectedly rotated token")
	}
	// A credential read failure aborts Jira refresh and retains cache.
	m.jira.reloadToken = func(context.Context) error { return errors.New("token file missing") }
	drainCmds(t, &m, m.scheduleJiraReload(true))
	if source.calls.Load() != 3 || len(m.jira.tasks) != 1 || !strings.Contains(m.statusMsg, "token file missing") {
		t.Fatal("failed rotation used stale credentials or lost cache")
	}
}

func TestJiraIDsWindowAndSelectionRemainStable(t *testing.T) {
	rows := jiraRows()
	rows = append(rows, task.Task{UUID: "jira:TST-43", IDText: "TST-43", Status: "completed", End: time.Now().Add(-time.Minute).UTC().Format(task.DateFormat)}, task.Task{UUID: "jira:TST-44", IDText: "TST-44", Status: "completed", End: time.Now().Add(-48 * time.Hour).UTC().Format(task.DateFormat)})
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return rows, nil }}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}, {UUID: "tw-done", Status: "completed"}}}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	if m.taskByUUID("jira:TST-43") != nil {
		t.Fatal("completed Jira shown with V off")
	}
	id := m.taskByUUID("jira:TST-42").ID
	if id >= 0 || id == m.taskByUUID("tw-done").ID {
		t.Fatal("Jira ID is zero/positive or collides with completed TW")
	}
	for i, v := range m.tasks {
		if v.UUID == "jira:TST-42" {
			m.tbl.SetCursor(i)
		}
	}
	m.completedWindow = time.Hour
	m.rebuildJiraRows()
	if m.taskByUUID("jira:TST-43") == nil || m.taskByUUID("jira:TST-44") != nil || m.getTaskAtCursor().UUID != "jira:TST-42" {
		t.Fatal("completed window or selected UUID wrong")
	}
	seen := make(map[int]bool)
	for _, v := range m.tasks {
		if v.ID == 0 || seen[v.ID] {
			t.Fatalf("duplicate/zero display ID %+v", m.tasks)
		}
		seen[v.ID] = true
	}
	m.jira.tasks = append(m.jira.tasks, task.Task{UUID: "jira:TST-1", IDText: "TST-1", Status: "pending", Priority: "H", Start: time.Now().Format(task.DateFormat)})
	m.rebuildJiraRows()
	if m.taskByUUID("jira:TST-42").ID != id || m.getTaskAtCursor().UUID != "jira:TST-42" {
		t.Fatal("resort changed identity/selection")
	}
	m.showUltra = true
	_ = m.selectTaskByID(id)
	m.jira.tasks = append(m.jira.tasks, task.Task{UUID: "jira:TST-2", IDText: "TST-2", Status: "pending", Priority: "H", Due: "20200101T000000Z"})
	m.rebuildJiraRows()
	if got, err := m.getUltraSelectedTaskID(); err != nil || got != id {
		t.Fatalf("ultra selected identity changed: %d %v", got, err)
	}
	m.tbl.SetWidth(45)
	if width := ansi.StringWidth(m.statusLine()); width > 45 {
		t.Fatalf("Jira status overflow %d", width)
	}
}

func TestJiraArrivalPreservesUltraNavigationIdentity(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}, {ID: 2, UUID: "tw-2", Status: "pending"}}}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	m.showUltra = true
	m.ultraCursor = 0
	m.tbl.SetCursor(0)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selected := m.getTaskForOpenURL().UUID
	if selected == m.getTaskAtCursor().UUID {
		t.Fatal("fixture did not exercise unsynchronized ultra navigation")
	}
	m.jira.tasks = append(m.jira.tasks, task.Task{UUID: "jira:TST-1", IDText: "TST-1", Status: "pending", Due: "20000101T000000Z"})
	m.rebuildJiraRows()
	if got := m.getTaskForOpenURL().UUID; got != selected {
		t.Fatalf("Jira arrival moved ultra selection from %s to %s", selected, got)
	}
}

func TestJiraProjectsRenderDistinctKeys(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) {
		return []task.Task{
			{UUID: "jira:TST-1", IDText: "TST-1", Project: "TST", Description: "first project", Status: "pending"},
			{UUID: "jira:TST2-1", IDText: "TST2-1", Project: "TST2", Description: "second project", Status: "pending"},
		}, nil
	}}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	m.jira.filter = config.Filter{Name: "both projects", JQL: "project in (TST, TST2)"}
	drainCmds(t, &m, m.Init())
	if len(m.tasks) != 2 || m.tasks[0].ID == m.tasks[1].ID {
		t.Fatalf("cross-project issues lost distinct identities: %+v", m.tasks)
	}
	for i, value := range m.tasks {
		row := m.tbl.Rows()[i]
		if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(colProject)])); got != value.Project {
			t.Fatalf("project cell %q, want %q", got, value.Project)
		}
		if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(colID)])); got != value.IDText {
			t.Fatalf("issue-key cell %q, want %q", got, value.IDText)
		}
		if !strings.HasPrefix(m.ultraStatusText(value), value.Project+"-1") {
			t.Fatalf("ultra lost project prefix: %s", m.ultraStatusText(value))
		}
	}
}

func TestJiraBaselineBlocksAllMutationAliases(t *testing.T) {
	cases := []struct{ key, reason string }{
		{"e", "not yet — Jira write support for summary is planned"},
		{"E", "not yet — Jira write support for summary is planned"},
		{"s", "not yet — Jira write support for transitions is planned"},
		{"d", "not yet — Jira write support for transitions is planned"},
		{"D", "Jira issues can't be deleted from here"},
		{"U", "no undo for Jira issues"},
		{"w", "not yet — Jira write support for due dates is planned"},
		{"W", "not yet — Jira write support for due dates is planned"},
		{"r", "not yet — Jira write support for due dates is planned"},
		{"p", "not yet — Jira write support for priority is planned"},
		{"t", "not yet — Jira write support for labels is planned"},
		{"A", "comments can't be replaced wholesale — use a to add"},
		{"J", "moving Jira issues not supported (yet)"},
		{"T", "moving Jira issues not supported (yet)"},
		{"R", "Jira issues have no recurrence"},
		{"ctrl+r", "Jira issues have no recurrence"},
		{";", "task commands don't work on Jira issues"},
	}
	for _, mode := range []string{"table", "ultra", "detail"} {
		for _, tc := range cases {
			key := tc.key
			t.Run(mode+"/"+key, func(t *testing.T) {
				source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
				m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
				drainCmds(t, &m, m.Init())
				if mode == "ultra" {
					m.showUltra = true
					m.reconcileUltraSelection()
				}
				if mode == "detail" {
					m.showTaskDetail = true
					m.setCurrentTaskDetail(&m.tasks[0])
				}
				m.pushUndoAction("done", []undoRestore{{uuid: "tw-pending-undo", status: "pending"}})
				_, cmd := m.Update(jiraKeyPress(key))
				if len(m.undoStack) != 1 {
					t.Fatal("blocked Jira action consumed TW undo")
				}
				if cmd != nil || m.anyInputActive() || m.taskFlightActive() || m.blinkID != 0 || m.statusMsg != tc.reason {
					t.Fatalf("mutation escaped guard key=%s mode=%s status=%s", key, mode, m.statusMsg)
				}
			})
		}
	}
}

func jiraKeyPress(key string) tea.KeyPressMsg {
	if key == "ctrl+r" {
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
}

func TestJiraFieldEditorsAndDeleteBoundaryAreGuarded(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	drainCmds(t, &m, m.Init())
	for _, col := range []int{colPri, colDue, colRecur, colProject, colTags, colAnnotations, colDescription} {
		m.tbl.SetColumnCursor(m.logicalToDisplay(col))
		_, cmd := m.handleEnterOrEdit()
		if cmd != nil || m.anyInputActive() {
			t.Fatalf("Jira column editor %d opened", col)
		}
	}
	m.showTaskDetail = true
	m.setCurrentTaskDetail(&m.tasks[0])
	for _, field := range []int{fieldPriority, fieldTags, fieldDue, fieldProject, fieldEntry + 1} {
		m.detailFieldIndex = field
		_, cmd := m.handleDetailFieldEdit()
		if cmd != nil || m.anyInputActive() {
			t.Fatalf("Jira detail editor %d opened", field)
		}
	}
	if cmd := m.scheduleDeleteSeries(m.tasks[0]); cmd != nil || m.taskFlightActive() {
		t.Fatal("Jira entered delete pipeline")
	}
	if cmd := m.editCmd(m.tasks[0].UUID); cmd != nil {
		t.Fatal("Jira entered TW external editor")
	}
}
