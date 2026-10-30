package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

type routingTaskwarrior struct {
	fakeTaskwarrior
	calls []string
}

func (f *routingTaskwarrior) record(method, addr string) error {
	f.calls = append(f.calls, method+":"+addr)
	return nil
}
func (f *routingTaskwarrior) EditCmd(addr string) *exec.Cmd {
	_ = f.record("edit", addr)
	return exec.Command("true")
}
func (f *routingTaskwarrior) StartContext(_ context.Context, addr string) error {
	return f.record("start", addr)
}
func (f *routingTaskwarrior) DoneContext(_ context.Context, addr string) error {
	return f.record("done", addr)
}
func (f *routingTaskwarrior) SetDueDateContext(_ context.Context, addr, _ string) error {
	return f.record("due", addr)
}
func (f *routingTaskwarrior) SetProjectContext(_ context.Context, addr, _ string) error {
	return f.record("project", addr)
}
func (f *routingTaskwarrior) RemoveTagsContext(_ context.Context, addr string, _ []string) error {
	return f.record("remove tags", addr)
}
func (f *routingTaskwarrior) SetStatusUUIDContext(_ context.Context, addr, _ string) error {
	return f.record("status", addr)
}
func (f *routingTaskwarrior) LoadCompletionSources(context.Context) task.CompletionSources {
	return task.CompletionSources{}
}

func routingModel(t *testing.T, jiraRow bool) (Model, *routingTaskwarrior) {
	t.Helper()
	tw := &routingTaskwarrior{}
	if !jiraRow {
		tw.tasks = []task.Task{{ID: 1, UUID: "tw-1", Status: "pending", Description: "task", Tags: []string{"work"}}}
	}
	m := newJiraTestModel(t, tw, browserOnlyJiraSource{})
	if jiraRow {
		m.jira.tasks = jiraRows()
		m.rebuildJiraRows()
	}
	m.blinkEnabled = false
	m.pushUndoAction("done", []undoRestore{{uuid: "tw-undo", status: "pending"}})
	return m, tw
}

func routingKey(key string) tea.KeyPressMsg {
	if key == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if code, ok := map[string]rune{"esc": tea.KeyEscape, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight, "space": tea.KeySpace}[key]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	return jiraKeyPress(key)
}

// Expected results are literal user-visible behaviors, independent of the
// policy registry. Executing mutation commands also verifies the TW UUID.
func TestRoutingKeepsTaskwarriorControls(t *testing.T) {
	cases := []struct {
		key   string
		input string
		calls []string
	}{
		{"e", "", []string{"edit:tw-1"}}, {"E", "", []string{"edit:tw-1"}},
		{"s", "", []string{"start:tw-1"}}, {"d", "", []string{"done:tw-1"}},
		{"D", "", []string{"status:tw-1"}}, {"U", "", []string{"status:tw-undo"}},
		{"w", "due", nil}, {"W", "", []string{"due:tw-1"}}, {"r", "", []string{"due:tw-1"}},
		{"p", "priority", nil}, {"t", "tags", nil}, {"a", "annotation", nil}, {"A", "replace", nil},
		{"J", "project", nil}, {"T", "", []string{"project:tw-1", "remove tags:tw-1"}},
		{"R", "recurrence", nil}, {"ctrl+r", "series", nil}, {";", "selected shell", nil},
	}
	for _, mode := range []string{"table", "ultra"} {
		for _, tc := range cases {
			if mode == "ultra" && tc.key == "T" {
				continue
			} // table-only binding
			t.Run(mode+"/"+tc.key, func(t *testing.T) {
				m, tw := routingModel(t, false)
				m.showUltra = mode == "ultra"
				if tc.key == "ctrl+r" {
					m.tasks[0].Recur = "daily"
				}
				_, cmd := m.Update(routingKey(tc.key))
				if tc.key != "e" && tc.key != "E" {
					drainCmds(t, &m, cmd)
				}
				if !reflect.DeepEqual(tw.calls, tc.calls) {
					t.Fatalf("TW calls %v, want %v", tw.calls, tc.calls)
				}
				if got := routingInput(m); got != tc.input {
					t.Fatalf("input %q, want %q (status %s)", got, tc.input, m.statusMsg)
				}
			})
		}
	}
}

func routingInput(m Model) string {
	switch {
	case m.dueEditing:
		return "due"
	case m.prioritySelecting:
		return "priority"
	case m.tagsEditing:
		return "tags"
	case m.projEditing:
		return "project"
	case m.recurEditing && m.recurSeries:
		return "series"
	case m.recurEditing:
		return "recurrence"
	case m.annotating && m.replaceAnnotations:
		return "replace"
	case m.annotating:
		return "annotation"
	case m.shellActive:
		return "selected shell"
	}
	return ""
}

func TestRoutingFieldControlsBySource(t *testing.T) {
	cases := []struct {
		col, field    int
		input, reason string
	}{
		{colPri, fieldPriority, "priority", "not yet — Jira write support for priority is planned"},
		{colDue, fieldDue, "due", "not yet — Jira write support for due dates is planned"},
		{colProject, fieldProject, "project", "moving Jira issues not supported (yet)"},
		{colTags, fieldTags, "tags", "not yet — Jira write support for labels is planned"},
		{colRecur, fieldEntry + 1, "recurrence", "Jira issues have no recurrence"},
		{colDescription, fieldEntry + 2, "description", "not yet — Jira write support for summary is planned"},
	}
	for _, jiraRow := range []bool{false, true} {
		for _, mode := range []string{"column", "detail"} {
			for _, key := range []string{"i", "enter"} {
				if mode == "column" && key == "enter" {
					continue
				} // Enter opens detail in table mode
				for _, tc := range cases {
					t.Run(mode+"/"+key+"/"+tc.input+map[bool]string{true: "/jira", false: "/tw"}[jiraRow], func(t *testing.T) {
						m, tw := routingModel(t, jiraRow)
						m.tasks[0].Recur = "daily" // exercise the optional recurrence field too
						m.tbl.SetColumnCursor(m.logicalToDisplay(tc.col))
						if mode == "detail" {
							m.showTaskDetail = true
							m.setCurrentTaskDetail(&m.tasks[0])
							m.detailFieldIndex = tc.field
						}
						_, cmd := m.Update(routingKey(key))
						if jiraRow {
							if cmd != nil || m.anyInputActive() || m.statusMsg != tc.reason {
								t.Fatalf("Jira editor escaped guard: cmd=%v input=%v status=%q", cmd != nil, m.anyInputActive(), m.statusMsg)
							}
						} else if tc.input == "description" {
							if mode == "detail" && (cmd == nil || !m.detailDescEditing) {
								t.Fatal("TW detail editor did not open")
							}
							if mode == "column" && !m.descEditing {
								t.Fatal("TW inline editor did not open")
							}
						} else if got := routingInput(m); got != tc.input {
							t.Fatalf("TW input %q want %q", got, tc.input)
						}
						if len(tw.calls) != 0 {
							t.Fatalf("field activation mutated TW: %v", tw.calls)
						}
					})
				}
			}
		}
	}
}

func TestRoutingCommitBoundariesRejectJira(t *testing.T) {
	cases := []struct {
		name, reason string
		setup        func(*Model)
		handler      func(*Model, tea.KeyPressMsg) (tea.Model, tea.Cmd)
	}{
		{"summary", "not yet — Jira write support for summary is planned", func(m *Model) { m.descAddr = "jira:TST-42"; m.descEditing = true; m.descInput.SetValue("changed") }, (*Model).handleDescriptionMode},
		{"labels", "not yet — Jira write support for labels is planned", func(m *Model) { m.tagsAddr = "jira:TST-42"; m.tagsEditing = true; m.tagsInput.SetValue("changed") }, (*Model).handleTagsMode},
		{"due", "not yet — Jira write support for due dates is planned", func(m *Model) { m.dueAddr = "jira:TST-42"; m.dueEditing = true }, (*Model).handleDueEditMode},
		{"priority", "not yet — Jira write support for priority is planned", func(m *Model) { m.priorityAddr = "jira:TST-42"; m.prioritySelecting = true }, (*Model).handlePriorityMode},
		{"project", "moving Jira issues not supported (yet)", func(m *Model) { m.projAddr = "jira:TST-42"; m.projEditing = true }, (*Model).handleProjectMode},
		{"recurrence", "Jira issues have no recurrence", func(m *Model) { m.recurAddr = "jira:TST-42"; m.recurEditing = true }, (*Model).handleRecurrenceMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, tw := routingModel(t, true)
			tc.setup(&m)
			_, cmd := tc.handler(&m, routingKey("enter"))
			if cmd != nil || m.taskFlightActive() || m.statusMsg != tc.reason || len(tw.calls) != 0 {
				t.Fatalf("commit escaped guard: calls=%v status=%s", tw.calls, m.statusMsg)
			}
		})
	}
	m, tw := routingModel(t, true)
	if cmd := m.scheduleUndoAction(); cmd != nil || len(m.undoStack) != 1 {
		t.Fatal("Jira undid pending TW mutation")
	}
	if cmd := m.startBlink(m.tasks[0].ID, true); cmd != nil || m.blinkID != 0 {
		t.Fatal("Jira entered delayed completion")
	}
	if err := doneOp("jira:TST-42")(context.Background(), tw); err == nil || err.Error() != "Jira transitions must use the Jira source" {
		t.Fatalf("done boundary error: %v", err)
	}
	if len(tw.calls) != 0 {
		t.Fatalf("Jira mutated TW: %v", tw.calls)
	}
}

func (f *routingTaskwarrior) RunShellLine(_ context.Context, line string) (task.RunResult, error) {
	return task.RunResult{}, f.record("shell", line)
}

func TestRoutingGlobalTaskwarriorPrompts(t *testing.T) {
	for _, jiraRow := range []bool{false, true} {
		for _, mode := range []string{"table", "ultra"} {
			for _, key := range []string{":", "+"} {
				t.Run(mode+"/"+key+map[bool]string{true: "/jira", false: "/tw"}[jiraRow], func(t *testing.T) {
					m, tw := routingModel(t, jiraRow)
					m.showUltra = mode == "ultra"
					_, cmd := m.Update(routingKey(key))
					drainCmds(t, &m, cmd)
					if key == "+" {
						if !m.addingTask {
							t.Fatal("global add did not open")
						}
						m.addInput.SetValue("new TW task")
						_, cmd = m.Update(routingKey("enter"))
						drainCmds(t, &m, cmd)
						if !reflect.DeepEqual(tw.addLines, []string{"new TW task"}) {
							t.Fatalf("add routed incorrectly: %v", tw.addLines)
						}
					} else {
						if !m.shellActive || m.shellInput.Value() != "" {
							t.Fatal("global prompt incorrectly scoped to selected row")
						}
						m.shellInput.SetValue("count")
						_, cmd = m.Update(routingKey("enter"))
						drainCmds(t, &m, cmd)
						if !reflect.DeepEqual(tw.calls, []string{"shell:count"}) {
							t.Fatalf("global shell calls: %v", tw.calls)
						}
					}
				})
			}
		}
	}
}

func TestRoutingUnchangedGlobalControls(t *testing.T) {
	cases := []struct {
		key   string
		check func(Model) bool
	}{
		{"H", func(m Model) bool { return m.showHelp }},
		{"/", func(m Model) bool { return m.searching }},
		{"?", func(m Model) bool { return m.searching }},
		{"f", func(m Model) bool { return m.filterEditing }},
		{"x", func(m Model) bool { return m.disco }},
		{"B", func(m Model) bool { return m.blinkEnabled }},
		{"v", func(m Model) bool { return m.compactView }},
		{"u", func(m Model) bool { return m.showUltra }},
	}
	for _, jiraRow := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.key+map[bool]string{true: "/jira", false: "/tw"}[jiraRow], func(t *testing.T) {
				m, tw := routingModel(t, jiraRow)
				_, _ = m.Update(routingKey(tc.key))
				if !tc.check(m) || len(tw.calls) != 0 {
					t.Fatalf("global control changed behavior: status=%s calls=%v", m.statusMsg, tw.calls)
				}
			})
		}
	}
}

func TestRoutingGlobalNavigationAndPresentation(t *testing.T) {
	// This is the remaining unchanged matrix from docs/jira-support.md section 6.
	// Both sources accept the same keys without starting a TW mutation.
	for _, key := range []string{"q", "esc", "c", "C", "Z", "V", "space", "0", "1", "2", "3", "4", "5", "6", "g", "G", "up", "down", "left", "right"} {
		for _, jiraRow := range []bool{false, true} {
			t.Run(key+map[bool]string{true: "/jira", false: "/tw"}[jiraRow], func(t *testing.T) {
				m, tw := routingModel(t, jiraRow)
				_, cmd := m.Update(routingKey(key))
				if len(tw.calls) != 0 || m.anyInputActive() {
					t.Fatalf("global key became row mutation: calls=%v status=%s", tw.calls, m.statusMsg)
				}
				switch key {
				case "q", "space", "Z", "V", "3", "4", "5":
					if cmd == nil {
						t.Fatalf("global %s lost its command", key)
					}
				}
				if m.statusMsg != "" && routeControl("jira:", key).route == routeBlocked {
					t.Fatalf("global key blocked: %s", m.statusMsg)
				}
			})
		}
	}
}

func TestRoutingAnnotationsColumnBySource(t *testing.T) {
	for _, jiraRow := range []bool{false, true} {
		m, tw := routingModel(t, jiraRow)
		m.tbl.SetColumnCursor(m.logicalToDisplay(colAnnotations))
		_, cmd := m.Update(routingKey("i"))
		if cmd != nil || len(tw.calls) != 0 {
			t.Fatal("column activation invoked mutation")
		}
		if jiraRow {
			if m.anyInputActive() || m.statusMsg != "comments can't be replaced wholesale — use a to add" {
				t.Fatal("Jira annotation replacement not blocked")
			}
		} else if !m.annotating || !m.replaceAnnotations {
			t.Fatal("TW annotation replacement changed")
		}
	}
}

func TestRoutingDirectHandlersAndEditorCallback(t *testing.T) {
	cases := []struct {
		key, reason string
		handler     func(*Model) (tea.Model, tea.Cmd)
	}{
		{"E", "not yet — Jira write support for summary is planned", (*Model).handleEditTask},
		{"r", "not yet — Jira write support for due dates is planned", (*Model).handleRandomDueDate},
		{";", "task commands don't work on Jira issues", (*Model).handleShellPromptForSelectedTask},
		{"s", "Jira transitions are unavailable", (*Model).handleUltraToggleStart},
		{"d", "Jira transitions are unavailable", (*Model).handleDetailMarkDone},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m, tw := routingModel(t, true)
			_, cmd := tc.handler(&m)
			if cmd != nil || m.anyInputActive() || len(tw.calls) != 0 || m.statusMsg != tc.reason {
				t.Fatalf("direct handler escaped guard: status=%s calls=%v", m.statusMsg, tw.calls)
			}
		})
	}
	m, tw := routingModel(t, true)
	if _, cmd := m.activateRecurringSeriesRecurrenceEdit(m.tasks[0].ID, m.tasks[0]); cmd != nil || m.recurEditing || m.statusMsg != "Jira issues have no recurrence" {
		t.Fatal("direct series activation escaped guard")
	}
	m.showTaskDetail = true
	m.setCurrentTaskDetail(&m.tasks[0])
	path := filepath.Join(t.TempDir(), "description")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.handleDescEditDone(descEditDoneMsg{tempFile: path})
	if cmd != nil || m.taskFlightActive() || len(tw.calls) != 0 || m.statusMsg != "not yet — Jira write support for summary is planned" {
		t.Fatal("stale external editor completion entered TW mutation")
	}
}

func TestRoutingTaskwarriorDetailActions(t *testing.T) {
	for _, tc := range []struct {
		key    string
		calls  []string
		series bool
	}{
		{"d", []string{"done:tw-1"}, false},
		{"D", []string{"status:tw-1"}, false},
		{"U", []string{"status:tw-undo"}, false},
		{"ctrl+r", nil, true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			m, tw := routingModel(t, false)
			if tc.series {
				m.tasks[0].Recur = "daily"
			}
			m.showTaskDetail = true
			m.setCurrentTaskDetail(&m.tasks[0])
			_, cmd := m.Update(routingKey(tc.key))
			drainCmds(t, &m, cmd)
			if !reflect.DeepEqual(tw.calls, tc.calls) || m.recurSeries != tc.series {
				t.Fatalf("TW detail changed: calls=%v series=%v", tw.calls, m.recurSeries)
			}
		})
	}
}

func TestRoutingEnterOpensDetailBySource(t *testing.T) {
	for _, jiraRow := range []bool{false, true} {
		m, tw := routingModel(t, jiraRow)
		_, _ = m.Update(routingKey("enter"))
		if !m.showTaskDetail || m.anyInputActive() || len(tw.calls) != 0 {
			t.Fatal("Enter failed to open read-only detail")
		}
	}
}
