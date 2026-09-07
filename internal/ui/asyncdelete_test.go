package ui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

type statusRecordingTaskwarrior struct {
	fakeTaskwarrior
	statuses      []statusChange
	series        []task.Task
	seriesErr     error
	failOnDeleted map[string]error
	failOnStatus  map[string]error // uuid -> error for any SetStatus
	failExport    error
	exportCalls   int
}

type statusChange struct {
	uuid   string
	status string
}

func (f *statusRecordingTaskwarrior) SetStatusUUIDContext(_ context.Context, uuid, status string) error {
	if err, ok := f.failOnDeleted[uuid]; ok && status == "deleted" {
		f.statuses = append(f.statuses, statusChange{uuid: uuid, status: status})
		return err
	}
	if err, ok := f.failOnStatus[uuid]; ok {
		f.statuses = append(f.statuses, statusChange{uuid: uuid, status: status})
		return err
	}
	f.statuses = append(f.statuses, statusChange{uuid: uuid, status: status})
	return nil
}

func (f *statusRecordingTaskwarrior) RecurringSeries(context.Context, string) ([]task.Task, error) {
	if f.seriesErr != nil {
		return nil, f.seriesErr
	}
	return append([]task.Task(nil), f.series...), nil
}

func (f *statusRecordingTaskwarrior) Export(ctx context.Context, filters ...string) ([]task.Task, error) {
	f.exportCalls++
	if f.failExport != nil {
		return nil, f.failExport
	}
	return f.fakeTaskwarrior.Export(ctx, filters...)
}

func TestDeleteSeriesCmdDoesNotCloseOverModel(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	cmd := deleteSeriesCmd(context.Background(), fake, task.Task{
		ID: 1, UUID: "u1", Description: "a", Status: "pending",
	}, 4)
	msg := cmd()
	got, ok := msg.(deleteSeriesDoneMsg)
	if !ok {
		t.Fatalf("got %T, want deleteSeriesDoneMsg", msg)
	}
	if got.gen != 4 {
		t.Fatalf("gen = %d, want 4", got.gen)
	}
	if got.err != nil {
		t.Fatalf("err = %v", got.err)
	}
	if len(got.restores) != 1 || got.restores[0].uuid != "u1" {
		t.Fatalf("restores = %#v", got.restores)
	}
	if fake.exportCalls != 0 {
		t.Fatalf("delete Cmd must not export (saw %d exports): the view is patched locally instead", fake.exportCalls)
	}
}

func TestDeleteHotkeySchedulesMutatingFlight(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{
				{ID: 1, UUID: "u1", Description: "a", Status: "pending"},
				{ID: 2, UUID: "u2", Description: "b", Status: "pending"},
			},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baselineExports := fake.exportCalls

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("delete should return async cmd")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("flight = %s, want mutating", m.taskFlight)
	}
	if len(fake.statuses) != 0 {
		t.Fatal("delete ran synchronously in Update")
	}
	if !strings.Contains(m.statusMsg, "Deleting") {
		t.Fatalf("statusMsg = %q, want Deleting", m.statusMsg)
	}
	if len(m.undoStack) != 0 {
		t.Fatal("undo stack pushed before DoneMsg")
	}

	msg := cmd()
	done, ok := msg.(deleteSeriesDoneMsg)
	if !ok {
		t.Fatalf("got %T, want deleteSeriesDoneMsg", msg)
	}
	mv, _ = (&m).Update(done)
	m = *mv.(*Model)
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if len(m.undoStack) != 1 {
		t.Fatalf("undo stack = %d, want 1", len(m.undoStack))
	}
	if m.statusMsg != "Deleted task" {
		t.Fatalf("statusMsg = %q", m.statusMsg)
	}
	if fake.exportCalls != baselineExports {
		t.Fatalf("export calls = %d, want %d: delete must not re-export", fake.exportCalls, baselineExports)
	}
	if len(m.tasks) != 1 || m.tasks[0].UUID != "u2" {
		t.Fatalf("tasks = %#v, want only u2 after local delete", m.tasks)
	}
	if m.total != 1 {
		t.Fatalf("total = %d, want 1 after local delete", m.total)
	}
	rows := m.tbl.Rows()
	if len(rows) != 1 {
		t.Fatalf("table rows = %d, want 1 after local delete", len(rows))
	}
}

func TestDeleteRejectedWhenBusy(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
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
	baseline := len(fake.statuses)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("delete should be rejected while busy")
	}
	if len(fake.statuses) != baseline {
		t.Fatal("busy delete mutated")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy", m.statusMsg)
	}
}

func TestStaleDeleteDoneIgnored(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightMutating, "Deleting…") {
		t.Fatal("arm mutate")
	}
	liveGen := m.taskOpGen
	m.invalidateTaskFlights()
	if !m.beginTaskFlight(taskFlightMutating, "Deleting…") {
		t.Fatal("re-arm mutate")
	}

	mv, _ := (&m).Update(deleteSeriesDoneMsg{
		gen:      liveGen,
		restores: []undoRestore{{uuid: "stale", status: "pending"}},
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("stale done cleared flight: %s", m.taskFlight)
	}
	if len(m.undoStack) != 0 {
		t.Fatal("stale done pushed undo")
	}
}

func TestDeleteDoneRequiresMatchingFlightKind(t *testing.T) {
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
	gen := m.taskOpGen
	mv, _ := (&m).Update(deleteSeriesDoneMsg{
		gen:      gen,
		restores: []undoRestore{{uuid: "u1", status: "pending"}},
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightReloading {
		t.Fatalf("wrong-kind delete done cleared flight: %s", m.taskFlight)
	}
	if len(m.undoStack) != 0 {
		t.Fatal("wrong-kind delete pushed undo")
	}
}

func TestDeletePartialFailureRollsBackWithoutUndo(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "child", Parent: "parent", Description: "c", Status: "pending", Recur: "daily"}},
		},
		series: []task.Task{
			{ID: 0, UUID: "parent", Description: "t", Status: "recurring", Recur: "daily", RType: "periodic"},
			{ID: 1, UUID: "child", Parent: "parent", Description: "c", Status: "pending", Recur: "daily"},
		},
		failOnDeleted: map[string]error{"parent": errors.New("delete boom")},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("expected delete cmd")
	}
	msg := cmd().(deleteSeriesDoneMsg)
	if msg.err == nil {
		t.Fatal("expected partial delete error")
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if len(m.undoStack) != 0 {
		t.Fatalf("undo stack = %d after failed delete", len(m.undoStack))
	}

	var sawRollback bool
	for _, s := range fake.statuses {
		if s.uuid == "child" && s.status == "pending" {
			sawRollback = true
		}
	}
	if !sawRollback {
		t.Fatalf("expected child rollback; statuses=%#v", fake.statuses)
	}
}

func TestUndoRejectedWhenBusy(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.pushUndoAction("delete", []undoRestore{{uuid: "u1", status: "pending"}})
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("undo should be rejected while busy")
	}
	if len(m.undoStack) != 1 {
		t.Fatal("busy undo should leave stack intact")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy", m.statusMsg)
	}
}

func TestUndoSuccessPopsStack(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.pushUndoAction("delete", []undoRestore{{uuid: "u1", status: "pending"}})

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("undo should return cmd")
	}
	if len(m.undoStack) != 1 {
		t.Fatal("stack should remain until DoneMsg")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("flight = %s, want mutating", m.taskFlight)
	}

	msg := cmd().(undoActionDoneMsg)
	if msg.err != nil {
		t.Fatalf("undo err: %v", msg.err)
	}
	mv, follow := (&m).Update(msg)
	m = *mv.(*Model)
	if len(m.undoStack) != 0 {
		t.Fatalf("stack = %d after success", len(m.undoStack))
	}
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if follow == nil {
		t.Fatal("expected blink follow-up after undo")
	}
}

func TestUndoPartialFailureRollsBackKeepsStack(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{},
		},
		failOnStatus: map[string]error{"u2": errors.New("restore boom")},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.pushUndoAction("delete", []undoRestore{
		{uuid: "u1", status: "pending"},
		{uuid: "u2", status: "pending"},
	})

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *mv.(*Model)
	msg := cmd().(undoActionDoneMsg)
	if msg.err == nil {
		t.Fatal("expected partial undo error")
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if len(m.undoStack) != 1 {
		t.Fatalf("stack = %d, want 1 after failed undo", len(m.undoStack))
	}

	var sawReDelete bool
	for _, s := range fake.statuses {
		if s.uuid == "u1" && s.status == "deleted" {
			sawReDelete = true
		}
	}
	if !sawReDelete {
		t.Fatalf("expected u1 re-deleted on undo rollback; statuses=%#v", fake.statuses)
	}
}

func TestStaleUndoDoneIgnored(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.pushUndoAction("delete", []undoRestore{{uuid: "u1", status: "pending"}})
	if !m.beginTaskFlight(taskFlightMutating, "Undoing…") {
		t.Fatal("arm mutate")
	}
	liveGen := m.taskOpGen
	m.invalidateTaskFlights()
	if !m.beginTaskFlight(taskFlightMutating, "Undoing…") {
		t.Fatal("re-arm")
	}

	mv, _ := (&m).Update(undoActionDoneMsg{
		gen:    liveGen,
		action: undoAction{label: "delete", restores: []undoRestore{{uuid: "u1", status: "pending"}}},
		data:   reloadData{tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}}},
	})
	m = *mv.(*Model)
	if len(m.undoStack) != 1 {
		t.Fatal("stale undo done popped stack")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("stale undo cleared flight: %s", m.taskFlight)
	}
}

func TestRollbackUndoRestoresRespectsCancelledQuitContext(t *testing.T) {
	fake := &ctxCheckingStatusTaskwarrior{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := rollbackUndoRestores(ctx, fake, []undoRestore{{uuid: "u1", status: "pending"}})
	if err == nil {
		t.Fatal("expected error from cancelled quit context")
	}
}

type ctxCheckingStatusTaskwarrior struct {
	fakeTaskwarrior
}

func (f *ctxCheckingStatusTaskwarrior) SetStatusUUIDContext(ctx context.Context, uuid, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func TestPriorStatusForUndo(t *testing.T) {
	if got := priorStatusForUndo("delete"); got != "deleted" {
		t.Fatalf("delete prior = %q", got)
	}
	if got := priorStatusForUndo("done"); got != "completed" {
		t.Fatalf("done prior = %q", got)
	}
}

func TestQuitWithSearchLeavesMutatingFlightIntact(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.searchRegex = mustCompileSearch("a")
	liveGen := 0
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("delete should return cmd")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("flight = %s, want mutating", m.taskFlight)
	}
	liveGen = m.taskOpGen

	mv, qcmd := (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = *mv.(*Model)
	if qcmd != nil {
		t.Fatal("q should not start reload during mutating flight")
	}
	if m.searchRegex != nil {
		t.Fatal("q should still clear search")
	}
	if m.taskFlight != taskFlightMutating || m.taskOpGen != liveGen {
		t.Fatalf("mutating flight mutated: flight=%s gen=%d want %d", m.taskFlight, m.taskOpGen, liveGen)
	}

	msg := cmd().(deleteSeriesDoneMsg)
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if len(m.undoStack) != 1 {
		t.Fatalf("undo stack = %d, want 1 after delete DoneMsg", len(m.undoStack))
	}
}

func mustCompileSearch(pat string) *regexp.Regexp {
	return regexp.MustCompile(pat)
}

func TestDeleteDoneAppliesLocalRemovalWithoutExport(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			// The fake's Export always returns this same list, so a stale
			// "reload after delete" would resurrect u1 here; the local path
			// must not, and must not call Export again either.
			tasks: []task.Task{
				{ID: 1, UUID: "u1", Description: "a", Status: "pending"},
				{ID: 2, UUID: "u2", Description: "b", Status: "pending"},
			},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baselineExports := fake.exportCalls

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	msg := cmd().(deleteSeriesDoneMsg)
	if msg.err != nil {
		t.Fatalf("delete err: %v", msg.err)
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if fake.exportCalls != baselineExports {
		t.Fatalf("export calls = %d, want %d", fake.exportCalls, baselineExports)
	}
	for _, tsk := range m.tasks {
		if tsk.UUID == "u1" {
			t.Fatalf("deleted task still in list: %#v", m.tasks)
		}
	}
	if len(m.tasks) != 1 || m.tasks[0].UUID != "u2" {
		t.Fatalf("tasks = %#v, want only u2", m.tasks)
	}
	if len(m.tbl.Rows()) != 1 {
		t.Fatalf("table rows = %d, want 1", len(m.tbl.Rows()))
	}
}

func TestDeleteRecurringSeriesRemovesAllLocally(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{
				{ID: 1, UUID: "child-1", Parent: "root", Description: "d1", Status: "pending", Recur: "daily"},
				{ID: 2, UUID: "child-2", Parent: "root", Description: "d2", Status: "pending", Recur: "daily"},
				{ID: 3, UUID: "other", Description: "keep", Status: "pending"},
			},
		},
		series: []task.Task{
			{ID: 0, UUID: "root", Description: "series", Status: "recurring", Recur: "daily", RType: "periodic"},
			{ID: 1, UUID: "child-1", Parent: "root", Description: "d1", Status: "pending", Recur: "daily"},
			{ID: 2, UUID: "child-2", Parent: "root", Description: "d2", Status: "pending", Recur: "daily"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baselineExports := fake.exportCalls

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	msg := cmd().(deleteSeriesDoneMsg)
	if msg.err != nil {
		t.Fatalf("delete err: %v", msg.err)
	}
	if !msg.recurring || msg.count != 3 {
		t.Fatalf("msg = %#v, want recurring count 3 (children + recurring root)", msg)
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if fake.exportCalls != baselineExports {
		t.Fatalf("export calls = %d, want %d", fake.exportCalls, baselineExports)
	}
	if len(m.tasks) != 1 || m.tasks[0].UUID != "other" {
		t.Fatalf("tasks = %#v, want only other", m.tasks)
	}
	if m.statusMsg != "Deleted 3 recurring tasks" {
		t.Fatalf("statusMsg = %q", m.statusMsg)
	}
	if len(m.undoStack) != 1 {
		t.Fatalf("undo stack = %d, want 1", len(m.undoStack))
	}
}

// deleteAndDoneRecordingTaskwarrior allows the delete flow's status flip and
// records done commands by address.
type deleteAndDoneRecordingTaskwarrior struct {
	doneRecordingTaskwarrior
}

func (f *deleteAndDoneRecordingTaskwarrior) SetStatusUUIDContext(_ context.Context, _, _ string) error {
	return nil
}

// Taskwarrior renumbers pending IDs whenever the working set changes, so
// after a local delete the remaining in-memory IDs are stale. Every later
// mutation must therefore address its task by UUID (taskAddress), which stays
// valid.
func TestDeleteKeepsLaterMutationsAddressedByUUID(t *testing.T) {
	fake := &deleteAndDoneRecordingTaskwarrior{doneRecordingTaskwarrior: doneRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "u1", Description: "a", Status: "pending"},
			{ID: 2, UUID: "u2", Description: "b", Status: "pending"},
		},
	}}}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	// Delete u1 via the hotkey flow.
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	msg := cmd().(deleteSeriesDoneMsg)
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if len(m.tasks) != 1 || m.tasks[0].UUID != "u2" {
		t.Fatalf("tasks = %#v, want only u2", m.tasks)
	}

	// Complete the task after the deleted one. The in-memory list still
	// labels it ID 2, while Taskwarrior's working set has renumbered it to 1:
	// the done command must carry the UUID, not any numeric ID.
	m.blinkEnabled = false
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = *mv.(*Model)
	if len(fake.doneAddrs) != 1 {
		t.Fatalf("DoneContext calls = %d, want 1", len(fake.doneAddrs))
	}
	if fake.doneAddrs[0] != "u2" {
		t.Fatalf("done address = %q, want UUID u2", fake.doneAddrs[0])
	}
}

func TestDeleteLastRemainingTaskLeavesEmptyTable(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	msg := cmd().(deleteSeriesDoneMsg)
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)

	if len(m.tasks) != 0 {
		t.Fatalf("tasks = %#v, want none", m.tasks)
	}
	if len(m.tbl.Rows()) != 0 {
		t.Fatalf("table rows = %d, want none", len(m.tbl.Rows()))
	}
	if m.total != 0 || m.inProgress != 0 || m.due != 0 {
		t.Fatalf("stats = total %d inProgress %d due %d, want zeros", m.total, m.inProgress, m.due)
	}
	if len(m.undoStack) != 1 {
		t.Fatalf("undo stack = %d, want 1", len(m.undoStack))
	}
}

func TestDeleteDropsTaskFromUltraFilter(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{
				{ID: 1, UUID: "u1", Description: "alpha", Status: "pending"},
				{ID: 2, UUID: "u2", Description: "beta", Status: "pending"},
			},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.ultraApplySearch("alpha")
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	msg := cmd().(deleteSeriesDoneMsg)
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	for _, idx := range m.ultraFiltered {
		if idx >= len(m.tasks) {
			t.Fatalf("ultraFiltered index %d out of range after delete (len=%d)", idx, len(m.tasks))
		}
	}
	if got := len(m.ultraTaskList()); got != 0 {
		t.Fatalf("ultra task list = %d entries, want 0 after deleting the only match", got)
	}
}

func TestUndoReloadFailurePopsStack(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	fake.failExport = errors.New("export boom")
	m.pushUndoAction("delete", []undoRestore{{uuid: "u1", status: "pending"}})
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *mv.(*Model)
	msg := cmd().(undoActionDoneMsg)
	if !msg.applied {
		t.Fatal("expected applied after restores succeeded")
	}
	if msg.err == nil {
		t.Fatal("expected reload error")
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)
	if len(m.undoStack) != 0 {
		t.Fatalf("stack = %d, want pop after undo+reload failure", len(m.undoStack))
	}
}
