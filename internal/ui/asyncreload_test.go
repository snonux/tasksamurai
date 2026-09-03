package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

func TestTaskReloadCmdDoesNotCloseOverModel(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	cmd := taskReloadCmd(context.Background(), fake, reloadSnapshot{}, 3, reloadMeta{})
	msg := cmd()
	got, ok := msg.(taskReloadDoneMsg)
	if !ok {
		t.Fatalf("got %T, want taskReloadDoneMsg", msg)
	}
	if got.gen != 3 {
		t.Fatalf("gen = %d, want 3", got.gen)
	}
	if got.err != nil {
		t.Fatalf("err = %v", got.err)
	}
	if len(got.data.tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(got.data.tasks))
	}
	if fake.exports < 1 {
		t.Fatal("expected Export on snapshotted client")
	}
}

func TestTaskReloadCmdCancelledContext(t *testing.T) {
	fake := &ctxAwareExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := taskReloadCmd(ctx, fake, reloadSnapshot{}, 1, reloadMeta{})
	msg := cmd()
	got, ok := msg.(taskReloadDoneMsg)
	if !ok {
		t.Fatalf("got %T, want taskReloadDoneMsg", msg)
	}
	if got.err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestHandleRefreshSchedulesAsyncReload(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := fake.exports

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("refresh should return async reload cmd")
	}
	if m.taskFlight != taskFlightReloading {
		t.Fatalf("flight = %s, want reloading", m.taskFlight)
	}
	if fake.exports != baseline {
		t.Fatal("export ran synchronously in Update")
	}
	if !strings.Contains(m.statusMsg, "Reloading") {
		t.Fatalf("statusMsg = %q, want Reloading", m.statusMsg)
	}

	msg := cmd()
	done, ok := msg.(taskReloadDoneMsg)
	if !ok {
		t.Fatalf("got %T, want taskReloadDoneMsg", msg)
	}
	mv, follow := (&m).Update(done)
	m = *mv.(*Model)
	if follow != nil {
		t.Fatalf("plain reload unexpectedly returned follow-up cmd")
	}
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if fake.exports != baseline+1 {
		t.Fatalf("exports = %d, want %d", fake.exports, baseline+1)
	}
}

func TestRefreshRejectedWhenBusy(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := fake.exports
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("space should not start reload while busy")
	}
	if fake.exports != baseline {
		t.Fatal("busy refresh exported")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy", m.statusMsg)
	}
}

func TestStaleReloadDoneIgnored(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatal("arm reload")
	}
	liveGen := m.taskOpGen
	m.invalidateTaskFlights()
	if !m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatal("re-arm reload")
	}

	mv, _ := (&m).Update(taskReloadDoneMsg{
		gen:  liveGen,
		data: reloadData{tasks: []task.Task{{ID: 99, UUID: "stale", Description: "stale", Status: "pending"}}},
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightReloading {
		t.Fatalf("stale done cleared flight: %s", m.taskFlight)
	}
	for _, tsk := range m.tasks {
		if tsk.ID == 99 {
			t.Fatal("stale reload applied tasks")
		}
	}
}

func TestZeroGenReloadDoneIgnored(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatal("arm reload")
	}
	mv, _ := (&m).Update(taskReloadDoneMsg{
		gen:  0,
		data: reloadData{tasks: []task.Task{{ID: 99, UUID: "stale", Description: "stale", Status: "pending"}}},
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightReloading {
		t.Fatal("gen 0 done should be ignored")
	}
	for _, tsk := range m.tasks {
		if tsk.ID == 99 {
			t.Fatal("gen 0 reload applied tasks")
		}
	}
}

func TestReloadDoneRequiresMatchingFlightKind(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	gen := m.taskOpGen
	mv, _ := (&m).Update(taskReloadDoneMsg{
		gen:  gen,
		data: reloadData{tasks: []task.Task{{ID: 99, UUID: "stale", Description: "stale", Status: "pending"}}},
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightShell {
		t.Fatalf("wrong-kind reload done cleared flight: %s", m.taskFlight)
	}
	for _, tsk := range m.tasks {
		if tsk.ID == 99 {
			t.Fatal("wrong-kind reload applied tasks")
		}
	}
}

func TestReloadErrorClearsFlightWithoutApplyingTasks(t *testing.T) {
	fake := &errorExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	fake.err = errors.New("export boom")
	baseline := append([]task.Task(nil), m.tasks...)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("refresh should return reload cmd")
	}
	msg := cmd()
	done, ok := msg.(taskReloadDoneMsg)
	if !ok {
		t.Fatalf("got %T, want taskReloadDoneMsg", msg)
	}
	if done.err == nil {
		t.Fatal("expected export error")
	}
	mv, _ = (&m).Update(done)
	m = *mv.(*Model)
	if m.taskFlightActive() {
		t.Fatalf("error should clear flight: %s", m.taskFlight)
	}
	if len(m.tasks) != len(baseline) || m.tasks[0].ID != baseline[0].ID {
		t.Fatalf("error applied tasks: %+v", m.tasks)
	}
	if !strings.Contains(m.statusMsg, "export boom") {
		t.Fatalf("statusMsg = %q, want export error", m.statusMsg)
	}
}

func TestShellDoneSchedulesAsyncReload(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	gen := m.taskOpGen
	baseline := fake.exports

	mv, cmd := (&m).Update(shellDoneMsg{
		result: task.RunResult{Args: []string{"next"}},
		gen:    gen,
	})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("shell done should schedule reload")
	}
	if m.taskFlight != taskFlightReloading {
		t.Fatalf("flight = %s, want reloading", m.taskFlight)
	}
	if fake.exports != baseline {
		t.Fatal("shell done exported synchronously")
	}

	msg := cmd()
	done := msg.(taskReloadDoneMsg)
	mv, _ = (&m).Update(done)
	m = *mv.(*Model)
	if !strings.Contains(m.statusMsg, "task next completed") {
		t.Fatalf("statusMsg = %q, want shell completion status", m.statusMsg)
	}
}

func TestEditDoneSchedulesAsyncReloadThenBlink(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = *mv.(*Model)
	baseline := fake.exports
	m.editID = 1

	mv, cmd := (&m).Update(editDoneMsg{})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("edit done should schedule reload")
	}
	if m.editID != 0 {
		t.Fatalf("editID = %d, want 0 after scheduling reload", m.editID)
	}
	if fake.exports != baseline {
		t.Fatal("edit done exported synchronously")
	}
	if m.taskFlight != taskFlightReloading {
		t.Fatalf("flight = %s, want reloading", m.taskFlight)
	}

	msg := cmd()
	done := msg.(taskReloadDoneMsg)
	mv, blink := (&m).Update(done)
	m = *mv.(*Model)
	if blink == nil {
		t.Fatal("expected blink cmd after successful edit reload")
	}
	if m.blinkID != 1 {
		t.Fatalf("blinkID = %d, want 1", m.blinkID)
	}
}

func TestAgentToggleSchedulesAsyncReload(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := fake.exports

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("agent toggle should schedule reload")
	}
	if fake.exports != baseline {
		t.Fatal("agent toggle exported synchronously")
	}
	if got := m.filters; len(got) != 1 || got[0] != "+agent" {
		t.Fatalf("filters = %#v, want [+agent]", m.filters)
	}

	msg := cmd()
	done := msg.(taskReloadDoneMsg)
	mv, _ = (&m).Update(done)
	m = *mv.(*Model)
	if fake.exports != baseline+1 {
		t.Fatalf("exports = %d, want %d", fake.exports, baseline+1)
	}
	last := fake.exportFilters[len(fake.exportFilters)-1]
	found := false
	for _, f := range last {
		if f == "+agent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reload filters = %#v, want +agent", last)
	}
}

func TestSearchApplyAndClearAreAsync(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{
				{ID: 1, UUID: "u1", Description: "alpha task", Status: "pending", Project: "home"},
				{ID: 2, UUID: "u2", Description: "beta task", Status: "pending", Project: "work"},
			},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = *mv.(*Model)
	baseline := fake.exports

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = *mv.(*Model)
	for _, r := range "beta" {
		mv, _ = (&m).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("search apply should schedule reload")
	}
	if fake.exports != baseline {
		t.Fatal("search apply exported synchronously")
	}
	if m.searching {
		t.Fatal("search input should close before reload completes")
	}

	msg := cmd()
	done := msg.(taskReloadDoneMsg)
	mv, _ = (&m).Update(done)
	m = *mv.(*Model)
	if m.searchRegex == nil {
		t.Fatal("search regex not set")
	}
	if m.tbl.Cursor() != 1 {
		t.Fatalf("cursor after search = %d, want 1 (beta)", m.tbl.Cursor())
	}

	baseline = fake.exports
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = *mv.(*Model)
	if m.searchRegex != nil {
		t.Fatal("esc should clear search regex immediately")
	}
	if cmd == nil {
		t.Fatal("search clear should schedule reload")
	}
	if fake.exports != baseline {
		t.Fatal("search clear exported synchronously")
	}
}

func TestManualRefreshDoesNotArmAutoRefreshTick(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.autoRefresh = true
	m.autoRefreshGen = 1
	m.autoRefreshInterval = time.Hour

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("expected reload cmd")
	}
	mv, follow := (&m).Update(cmd().(taskReloadDoneMsg))
	m = *mv.(*Model)
	if follow != nil {
		t.Fatal("manual refresh must not start a second auto-refresh loop")
	}
}

func TestAutoRefreshReloadReschedulesTick(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.autoRefresh = true
	m.autoRefreshGen = 4
	m.autoRefreshInterval = time.Nanosecond
	baseline := fake.exports

	mv, cmd := (&m).Update(autoRefreshMsg{gen: 4})
	m = *mv.(*Model)
	if fake.exports != baseline {
		t.Fatal("auto-refresh exported synchronously")
	}
	if cmd == nil {
		t.Fatal("expected reload cmd")
	}
	mv, follow := (&m).Update(cmd().(taskReloadDoneMsg))
	m = *mv.(*Model)
	if follow == nil {
		t.Fatal("auto-refresh reload should reschedule the tick")
	}
	msg := follow()
	got, ok := msg.(autoRefreshMsg)
	if !ok {
		t.Fatalf("follow-up = %T, want autoRefreshMsg", msg)
	}
	if got.gen != 4 {
		t.Fatalf("tick gen = %d, want 4", got.gen)
	}
}

func TestAutoRefreshErrorKeepsLoopAlive(t *testing.T) {
	fake := &errorExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	fake.err = errors.New("export boom")
	m.autoRefresh = true
	m.autoRefreshGen = 2
	m.autoRefreshInterval = time.Nanosecond

	mv, cmd := (&m).Update(autoRefreshMsg{gen: 2})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("expected reload cmd")
	}
	mv, follow := (&m).Update(cmd().(taskReloadDoneMsg))
	m = *mv.(*Model)
	if follow == nil {
		t.Fatal("auto-refresh should keep ticking after export error")
	}
}

type errorExportTaskwarrior struct {
	fakeTaskwarrior
	err error
}

func (f *errorExportTaskwarrior) Export(ctx context.Context, filters ...string) ([]task.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeTaskwarrior.Export(ctx, filters...)
}

type ctxAwareExportTaskwarrior struct {
	fakeTaskwarrior
}

func (f *ctxAwareExportTaskwarrior) Export(ctx context.Context, filters ...string) ([]task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.fakeTaskwarrior.Export(ctx, filters...)
}
