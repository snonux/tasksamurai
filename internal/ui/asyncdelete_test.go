package ui

import (
	"context"
	"errors"
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
	}, reloadSnapshot{}, 4)
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
	if fake.exportCalls < 1 {
		t.Fatal("expected reload export in Cmd")
	}
}

func TestDeleteHotkeySchedulesMutatingFlight(t *testing.T) {
	fake := &statusRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := len(fake.statuses)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("delete should return async cmd")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("flight = %s, want mutating", m.taskFlight)
	}
	if len(fake.statuses) != baseline {
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
		data:     reloadData{tasks: []task.Task{{ID: 99, UUID: "stale", Description: "stale", Status: "pending"}}},
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
