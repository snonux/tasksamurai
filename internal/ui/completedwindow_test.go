package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

func TestStatusFilterCompletedWindow(t *testing.T) {
	if got := statusFilter(0); got != "status:pending" {
		t.Errorf("statusFilter(0) = %q, want status:pending", got)
	}

	got := statusFilter(4 * time.Hour)
	if !strings.Contains(got, "status:pending") {
		t.Errorf("statusFilter window missing status:pending: %q", got)
	}
	if !strings.Contains(got, "status:completed") {
		t.Errorf("statusFilter window missing status:completed: %q", got)
	}
	if !strings.Contains(got, "end.after:") {
		t.Errorf("statusFilter window missing end.after cutoff: %q", got)
	}
}

func TestCompletedWindowLabel(t *testing.T) {
	cases := map[string]struct {
		window time.Duration
		want   string
	}{
		"off": {0, "off"},
		"1h":  {time.Hour, "1h"},
		"4h":  {4 * time.Hour, "4h"},
		"1d":  {24 * time.Hour, "1d"},
	}
	for name, tc := range cases {
		if got := CompletedWindowLabel(tc.window); got != tc.want {
			t.Errorf("%s: CompletedWindowLabel = %q, want %q", name, got, tc.want)
		}
	}
}

func TestAssignCompletedDisplayIDs(t *testing.T) {
	tasks := []task.Task{
		{ID: 1, Status: "pending"},
		{ID: 0, Status: "completed"},
		{ID: 0, Status: "completed"},
		{ID: 4, Status: "pending"},
	}
	assignCompletedDisplayIDs(tasks)

	want := []int{1, -1, -2, 4}
	for i, w := range want {
		if tasks[i].ID != w {
			t.Errorf("tasks[%d].ID = %d, want %d", i, tasks[i].ID, w)
		}
	}
}

func TestCompletedWindowCycleSchedulesAsyncReload(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := len(fake.exportFilters)

	// Cycle off -> 1 day -> 4h -> 1h -> off, checking the export filter and
	// the status message after each keypress.
	wantWindows := []time.Duration{24 * time.Hour, 4 * time.Hour, time.Hour, 0}
	wantLabels := []string{"1d", "4h", "1h", "off"}
	for i, want := range wantWindows {
		mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
		m = *mv.(*Model)
		if cmd == nil {
			t.Fatalf("keypress %d: completed-window toggle should schedule reload", i)
		}
		if m.completedWindow != want {
			t.Fatalf("cycle %d: completedWindow = %v, want %v", i, m.completedWindow, want)
		}
		if len(fake.exportFilters) != baseline {
			t.Fatalf("cycle %d: toggle must not export synchronously", i)
		}

		msg := cmd()
		mv, _ = (&m).Update(msg)
		m = *mv.(*Model)
		if len(fake.exportFilters) != baseline+1 {
			t.Fatalf("cycle %d: exports = %d, want %d", i, len(fake.exportFilters), baseline+1)
		}
		last := fake.exportFilters[len(fake.exportFilters)-1]
		status := last[len(last)-1]
		if i == 3 {
			if status != "status:pending" {
				t.Fatalf("cycle %d: export status filter = %q, want status:pending", i, status)
			}
		} else {
			if !strings.Contains(status, "status:completed") || !strings.Contains(status, "end.after:") {
				t.Fatalf("cycle %d: export status filter = %q, want completed window filter", i, status)
			}
			if want := CompletedWindowLabel(m.completedWindow); want != wantLabels[i] {
				t.Fatalf("cycle %d: label = %q, want %q", i, want, wantLabels[i])
			}
		}
		baseline++
	}
}

func TestCompletedRowRendering(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "u1", Description: "still pending", Status: "pending"},
			{ID: 0, UUID: "u2", Description: "finished work", Status: "completed", End: "20250101T000000Z"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	// The completed task gets a synthetic display ID so selection and blink
	// flows keep working.
	if len(m.tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(m.tasks))
	}
	if m.tasks[0].ID != 1 || m.tasks[1].ID != -1 {
		t.Fatalf("display IDs = [%d, %d], want [1, -1]", m.tasks[0].ID, m.tasks[1].ID)
	}
	if m.completed != 1 {
		t.Fatalf("completed count = %d, want 1", m.completed)
	}

	// The completed row shows the check mark instead of an ID and a
	// strikethrough description.
	row := m.taskToRowSearch(m.tasks[1], nil, m.tblStyles, -1)
	joined := strings.Join(row, " ")
	if !strings.Contains(joined, "✓") {
		t.Errorf("completed row missing check mark: %q", joined)
	}
	if !strings.Contains(joined, "9m") {
		t.Errorf("completed description missing strikethrough: %q", joined)
	}

	// A pending row keeps its numeric ID and no strikethrough.
	pending := m.taskToRowSearch(m.tasks[0], nil, m.tblStyles, -1)
	joinedPending := strings.Join(pending, " ")
	if !strings.Contains(joinedPending, "1") {
		t.Errorf("pending row missing numeric ID: %q", joinedPending)
	}
	if strings.Contains(joinedPending, "9m") {
		t.Errorf("pending row must not be struck through: %q", joinedPending)
	}
}

func TestMarkDoneOnCompletedTaskIsNoop(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: -1, UUID: "u1", Description: "already done", Status: "completed"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("mark done on a completed task must not schedule anything")
	}
	if m.statusMsg != "Task is already completed" {
		t.Fatalf("statusMsg = %q, want completion hint", m.statusMsg)
	}
	if m.blinkID != 0 {
		t.Fatalf("blinkID = %d, want 0 (no blink started)", m.blinkID)
	}
}
func TestToggleStartOnCompletedTaskIsNoop(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: -1, UUID: "u1", Description: "already done", Status: "completed"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("start on a completed task must not schedule anything")
	}
	if m.statusMsg != "Cannot start a completed task" {
		t.Fatalf("statusMsg = %q, want completion hint", m.statusMsg)
	}
}

func TestCompletedWindowCycleInUltraMode(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.SetUltra(true)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("V in ultra mode should schedule reload")
	}
	if m.completedWindow != 24*time.Hour {
		t.Fatalf("completedWindow = %v, want 24h", m.completedWindow)
	}
	if !strings.Contains(m.ultraModeStatus(m.ultraTaskList()), "completed: on (1d)") {
		t.Fatalf("ultra status line missing completed indicator: %q", m.ultraModeStatus(m.ultraTaskList()))
	}
	mv, _ = (&m).Update(cmd())
	m = *mv.(*Model)

	// Cycle to off: each press schedules a reload that must be consumed
	// first — the busy-flight gate swallows keys while one is in flight.
	for range []int{0, 1, 2} {
		mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
		m = *mv.(*Model)
		if cmd != nil {
			mv, _ = (&m).Update(cmd())
			m = *mv.(*Model)
		}
	}
	if m.completedWindow != 0 {
		t.Fatalf("completedWindow = %v, want 0 after four presses", m.completedWindow)
	}
}

func TestUltraCompletedCardRendering(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "u1", Description: "pending thing", Status: "pending"},
			{ID: 0, UUID: "u2", Description: "done thing", Status: "completed", End: "20250101T000000Z", Priority: "H", Due: "20200101T000000Z"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.SetUltra(true)

	card := m.renderUltraStatus(m.tasks[1], 80)
	if !strings.Contains(card, "✓") {
		t.Errorf("completed ultra card missing check mark: %q", card)
	}

	desc := m.renderUltraDescription(m.tasks[1], 80)
	if !strings.Contains(desc, "9m") {
		t.Errorf("completed ultra description missing strikethrough: %q", desc)
	}

	// The plain-text search index must match the rendered check mark.
	if !strings.Contains(m.ultraStatusText(m.tasks[1]), "✓") {
		t.Errorf("ultraStatusText missing check mark: %q", m.ultraStatusText(m.tasks[1]))
	}
}

func TestReconcileUltraSelectionWithCompletedTask(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "u1", Description: "a", Status: "pending"},
			{ID: 0, UUID: "u2", Description: "done", Status: "completed"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.SetUltra(true)
	if m.tasks[1].ID != -1 {
		t.Fatalf("completed task display ID = %d, want -1", m.tasks[1].ID)
	}

	// Focus restore must work for the negative synthetic ID of a completed task.
	m.ultraFocusedID = m.tasks[1].ID
	m.reconcileUltraSelection()
	if m.ultraFocusedID != 0 {
		t.Fatalf("ultraFocusedID = %d, want 0 after reconcile restored the focus", m.ultraFocusedID)
	}
	if m.ultraTaskIndexByID(-1) != 1 {
		t.Fatalf("ultra cursor = %d, want index 1 (the completed task)", m.ultraTaskIndexByID(-1))
	}
}

func TestDeleteCompletedRecurringInstanceIsSingleTask(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
		// A completed recurring instance would otherwise expand to the whole
		// series; the D key on the dimmed row must only delete that instance.
		series: []task.Task{
			{ID: -1, UUID: "u2", Description: "done instance", Status: "completed", Parent: "u3", Recur: "weekly"},
			{ID: 2, UUID: "u4", Description: "pending instance", Status: "pending", Parent: "u3", Recur: "weekly"},
		},
	}
	done := deleteSeriesCmd(context.Background(), fake, task.Task{
		ID: -1, UUID: "u2", Description: "done instance", Status: "completed", Parent: "u3", Recur: "weekly",
	}, 1)
	msg := done()
	got, ok := msg.(deleteSeriesDoneMsg)
	if !ok {
		t.Fatalf("got %T, want deleteSeriesDoneMsg", msg)
	}
	if got.err != nil {
		t.Fatalf("delete series: %v", got.err)
	}
	if got.recurring {
		t.Fatal("completed recurring instance must not expand to the series")
	}
	if got.count != 1 {
		t.Fatalf("deleted %d tasks, want 1", got.count)
	}
	if len(fake.statuses) != 1 || fake.statuses[0].uuid != "u2" {
		t.Fatalf("statuses = %+v, want single delete of u2", fake.statuses)
	}
}
