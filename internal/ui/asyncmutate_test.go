package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// annotateRecordingTaskwarrior records annotation mutations so the async
// mutate+reload pipeline can be asserted end to end.
type annotateRecordingTaskwarrior struct {
	fakeTaskwarrior
	annotations []string
	annotateErr error
}

func (f *annotateRecordingTaskwarrior) AnnotateContext(_ context.Context, addr string, value string) error {
	f.annotations = append(f.annotations, addr+" "+value)
	return f.annotateErr
}

func TestAnnotateCommitRunsAsyncMutateThenReload(t *testing.T) {
	fake := &annotateRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	m.clearEditingModes()
	m.annotateID = 1
	m.annotateAddr = "one"
	m.annotating = true
	m.annotateInput.SetValue("hello")
	m.annotateInput.Focus()

	mv, cmd := (&m).handleAnnotationMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	if cmd == nil {
		t.Fatalf("annotate commit returned no command")
	}
	if m.annotating {
		t.Fatalf("annotate commit did not exit edit mode")
	}
	if m.taskFlight != taskFlightMutating {
		t.Fatalf("taskFlight = %v, want mutating", m.taskFlight)
	}
	if len(fake.annotations) != 0 {
		t.Fatalf("mutation ran inside Update: %v", fake.annotations)
	}
	// Run the async Cmd manually and feed its DoneMsg back: draining would
	// also run the follow-up blink animation to completion.
	msg, ok := cmd().(mutateReloadDoneMsg)
	if !ok {
		t.Fatalf("annotate commit cmd returned %T", cmd())
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)

	if len(fake.annotations) != 1 || fake.annotations[0] != "one hello" {
		t.Fatalf("annotations = %v, want [one hello]", fake.annotations)
	}
	if m.taskFlight != taskFlightIdle {
		t.Fatalf("taskFlight = %v after DoneMsg, want idle", m.taskFlight)
	}
	if m.blinkID != 1 {
		t.Fatalf("blinkID = %d after annotate DoneMsg, want 1", m.blinkID)
	}
}

func TestMutateFailureReopensEditModeWithTypedValue(t *testing.T) {
	fake := &annotateRecordingTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()},
		annotateErr:     errors.New("rejected"),
	}
	m := newBlinkTestModel(t, fake)

	m.clearEditingModes()
	m.annotateID = 1
	m.annotateAddr = "one"
	m.annotating = true
	m.annotateInput.SetValue("hello")
	m.annotateInput.Focus()

	mv, cmd := (&m).handleAnnotationMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	msg, ok := cmd().(mutateReloadDoneMsg)
	if !ok {
		t.Fatalf("annotate commit cmd returned %T", cmd())
	}
	mv, _ = (&m).Update(msg)
	m = *mv.(*Model)

	// The mutation failed, so the annotation mode is re-opened with the
	// typed value intact instead of silently dropping it.
	if !m.annotating {
		t.Fatalf("annotation mode not restored after mutate failure")
	}
	if got := m.annotateInput.Value(); got != "hello" {
		t.Fatalf("input value = %q after failed commit, want hello", got)
	}
	if !strings.Contains(m.statusMsg, "rejected") {
		t.Fatalf("statusMsg = %q, want mutation error", m.statusMsg)
	}
	if m.blinkID != 0 {
		t.Fatalf("blink started for failed mutation: blinkID=%d", m.blinkID)
	}
}

func TestMutateCommitRejectedWhileFlightActive(t *testing.T) {
	fake := &annotateRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	// Arm a blocking flight to simulate an in-flight pipeline.
	if !m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatalf("could not arm the blocking flight")
	}

	m.clearEditingModes()
	m.annotateID = 1
	m.annotateAddr = "one"
	m.annotating = true
	m.annotateInput.SetValue("hello")
	m.annotateInput.Focus()

	mv, _ := (&m).handleAnnotationMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	if !strings.Contains(m.statusMsg, "Busy") {
		t.Fatalf("statusMsg = %q, want Busy reject", m.statusMsg)
	}
	// The input mode stays open so the user does not lose their text.
	if !m.annotating {
		t.Fatalf("edit mode was dropped on Busy reject")
	}
	if len(fake.annotations) != 0 {
		t.Fatalf("mutation ran during busy reject: %v", fake.annotations)
	}
}

func TestMutateReloadDoneIgnoresStaleMessages(t *testing.T) {
	fake := &annotateRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	stale := mutateReloadDoneMsg{
		gen:  m.taskOpGen + 7,
		data: reloadData{tasks: nil},
	}
	mv, cmd := (&m).handleMutateReloadDone(stale)
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatalf("stale gen DoneMsg scheduled follow-up work")
	}
	if len(m.tasks) != 2 {
		t.Fatalf("stale DoneMsg replaced the task list: %d tasks", len(m.tasks))
	}

	// Gen 0 is never live.
	zero := mutateReloadDoneMsg{gen: 0}
	mv, cmd2 := (&m).handleMutateReloadDone(zero)
	m = *mv.(*Model)
	if cmd2 != nil {
		t.Fatalf("gen-0 DoneMsg scheduled follow-up work")
	}
	if len(m.tasks) != 2 {
		t.Fatalf("gen-0 DoneMsg replaced the task list")
	}
}

func TestMutateReloadDonePushesUndoOnlyOnSuccess(t *testing.T) {
	fake := &annotateRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	// A mutate failure must leave the undo stack untouched and keep the
	// flight cleared.
	m.beginTaskFlight(taskFlightMutating, "Marking done…")
	failed := mutateReloadDoneMsg{
		gen:       m.taskOpGen,
		mutateErr: errors.New("done failed"),
		meta:      mutateMeta{undo: &undoAction{label: "done", restores: []undoRestore{{uuid: "one", status: "pending"}}}},
	}
	mv, _ := (&m).handleMutateReloadDone(failed)
	m = *mv.(*Model)
	if len(m.undoStack) != 0 {
		t.Fatalf("undo pushed after mutate failure: %#v", m.undoStack)
	}
	if m.taskFlight != taskFlightIdle {
		t.Fatalf("flight not cleared after mutate failure")
	}

	// On success the undo entry is pushed before the reload result is used.
	m.beginTaskFlight(taskFlightMutating, "Marking done…")
	ok := mutateReloadDoneMsg{
		gen:  m.taskOpGen,
		data: reloadData{tasks: m.tasks},
		meta: mutateMeta{undo: &undoAction{label: "done", restores: []undoRestore{{uuid: "one", status: "pending"}}}},
	}
	mv, _ = (&m).handleMutateReloadDone(ok)
	m = *mv.(*Model)
	if len(m.undoStack) != 1 || m.undoStack[0].label != "done" {
		t.Fatalf("undo stack = %#v, want one done entry", m.undoStack)
	}
}

func TestBlinkEndSchedulesAsyncDoneForMarkedTask(t *testing.T) {
	fake := &doneRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	if cmd := m.startBlink(1, true); cmd == nil {
		t.Fatalf("startBlink with blink enabled returned no tick command")
	}
	var doneCmd tea.Cmd
	for i := 0; i < blinkCycles; i++ {
		mv, c := (&m).Update(blinkMsg{})
		m = *mv.(*Model)
		doneCmd = c
	}
	if doneCmd == nil {
		t.Fatalf("final blink tick returned no async done command")
	}
	if m.blinkID != 0 {
		t.Fatalf("blinkID not cleared on final blink tick")
	}
	if len(fake.doneAddrs) != 0 {
		t.Fatalf("done ran inside Update: %v", fake.doneAddrs)
	}
	drainCmds(t, &m, doneCmd)
	if len(fake.doneAddrs) != 1 || fake.doneAddrs[0] != "one" {
		t.Fatalf("done addresses = %v, want [one]", fake.doneAddrs)
	}
	if m.taskFlight != taskFlightIdle {
		t.Fatalf("flight = %v after async done, want idle", m.taskFlight)
	}
	if len(m.undoStack) != 1 || m.undoStack[0].label != "done" {
		t.Fatalf("undo stack = %#v, want one done entry pushed on success", m.undoStack)
	}
}

var _ task.Taskwarrior = (*annotateRecordingTaskwarrior)(nil)
