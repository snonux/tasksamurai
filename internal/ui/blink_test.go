package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// doneRecordingTaskwarrior records DoneContext calls instead of panicking, so
// tests can assert that a deferred completion still runs when the blink
// animation is skipped.
type doneRecordingTaskwarrior struct {
	fakeTaskwarrior
	doneAddrs []string
}

func (f *doneRecordingTaskwarrior) DoneContext(_ context.Context, addr string) error {
	f.doneAddrs = append(f.doneAddrs, addr)
	return nil
}

func newBlinkTestModel(t *testing.T, tw task.Taskwarrior) Model {
	t.Helper()
	m, err := NewWithTaskwarrior(nil, "firefox", tw)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return *mv.(*Model)
}

func blinkTasks() []task.Task {
	return []task.Task{
		{ID: 1, UUID: "one", Description: "alpha", Status: "pending"},
		{ID: 2, UUID: "two", Description: "beta", Status: "pending"},
	}
}

// A blink for a task that is not in the current table has no row to animate
// and schedules no tick, so it must not leave blinkID set: Update() would
// route every later keypress to handleBlinkingState forever.
func TestStartBlinkOnInvisibleTaskClearsBlinkID(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: blinkTasks()}
	m := newBlinkTestModel(t, fake)

	cmd := m.startBlink(99, false)

	if cmd != nil {
		t.Fatalf("startBlink for a task with no row returned a command; want nil")
	}
	if m.blinkID != 0 {
		t.Fatalf("blinkID = %d after blinking an invisible task, want 0", m.blinkID)
	}
	if m.blinkMarkDone {
		t.Fatal("blinkMarkDone stayed set after an invisible-task blink")
	}
}

// The wedge this reproduces: a task edited out of the active filter used to
// strand blinkID, after which navigation still worked but every action key
// was silently swallowed. Reported as "s does not start a task".
func TestInvisibleTaskBlinkKeepsHotkeysResponsive(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: blinkTasks()}
	m := newBlinkTestModel(t, fake)

	m.startBlink(99, false)

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = *mv.(*Model)

	if !m.showHelp {
		t.Fatal("H was swallowed after an invisible-task blink; keypresses are stuck in blinking state")
	}
}

// Skipping the animation must not skip the work the animation was covering
// for: the deferred completion still has to run.
// Task 99 is not in the fake's list, so no address is captured and the
// deferred completion falls back to the raw numeric ID.
func TestStartBlinkOnInvisibleTaskStillMarksDone(t *testing.T) {
	fake := &doneRecordingTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: blinkTasks()}}
	m := newBlinkTestModel(t, fake)

	if cmd := m.startBlink(99, true); cmd != nil {
		t.Fatalf("startBlink for a task with no row returned a command; want nil")
	}

	if len(fake.doneAddrs) != 1 || fake.doneAddrs[0] != "99" {
		t.Fatalf("DoneContext calls = %v, want [99]", fake.doneAddrs)
	}
	if m.blinkID != 0 {
		t.Fatalf("blinkID = %d after completing an invisible task, want 0", m.blinkID)
	}
}

// Both blinks are driven by the same blinkMsg tick. The detail blink used to
// consume a tick and return without rescheduling, stranding an in-flight row
// blink with blinkID set and nothing left to clear it.
func TestDetailBlinkDoesNotStrandRowBlink(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: blinkTasks()}
	m := newBlinkTestModel(t, fake)

	if cmd := m.startBlink(1, false); cmd == nil {
		t.Fatal("startBlink on a visible task returned no tick command")
	}
	m.showTaskDetail = true
	if cmd := m.startDetailBlink(0); cmd == nil {
		t.Fatal("startDetailBlink returned no tick command")
	}

	// Drive the shared tick for longer than either animation can last.
	for i := 0; i < 4*blinkCycles; i++ {
		mv, cmd := (&m).Update(blinkMsg{})
		m = *mv.(*Model)
		if m.blinkID != 0 && cmd == nil {
			t.Fatalf("tick %d ended the blink chain while blinkID=%d is still set", i, m.blinkID)
		}
		if m.blinkID == 0 && m.detailBlinkField == -1 {
			break
		}
	}

	if m.blinkID != 0 {
		t.Fatalf("blinkID = %d after the animations finished, want 0", m.blinkID)
	}
	if m.detailBlinkField != -1 {
		t.Fatalf("detailBlinkField = %d after the animations finished, want -1", m.detailBlinkField)
	}

	// Leave the detail overlay so the keypress is dispatched by normal mode.
	m.showTaskDetail = false
	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = *mv.(*Model)
	if !m.showHelp {
		t.Fatal("H was swallowed after the blinks finished; keypresses are stuck in blinking state")
	}
}

// The detail blink counts up from zero, so it flashes for the full
// blinkInterval * blinkCycles rather than stopping on its first tick.
func TestDetailBlinkRunsFullCycle(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: blinkTasks()}
	m := newBlinkTestModel(t, fake)

	m.showTaskDetail = true
	m.startDetailBlink(0)

	for i := 1; i < blinkCycles; i++ {
		if !m.advanceDetailBlink() {
			t.Fatalf("detail blink stopped after %d of %d ticks", i, blinkCycles)
		}
	}
	if m.advanceDetailBlink() {
		t.Fatalf("detail blink still running after %d ticks", blinkCycles)
	}
	if m.detailBlinkField != -1 || m.detailBlinkOn || m.detailBlinkCount != 0 {
		t.Fatalf("detail blink state not reset: field=%d on=%v count=%d",
			m.detailBlinkField, m.detailBlinkOn, m.detailBlinkCount)
	}
}
