package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

// TestHandleToggleAutoRefresh verifies that Z cycles auto-refresh through
// off → 10s → 60s → 5m → 15m → off, scheduling a tick on each enable or
// interval advance and only reporting off after the longest interval.
func TestHandleToggleAutoRefresh(t *testing.T) {
	m := Model{}

	for i, want := range autoRefreshIntervals {
		mv, cmd := m.handleCycleAutoRefresh()
		m = *mv.(*Model)
		if !m.autoRefresh {
			t.Fatalf("step %d: expected autoRefresh enabled", i)
		}
		if cmd == nil {
			t.Fatalf("step %d: expected a tick command", i)
		}
		if m.autoRefreshInterval != want {
			t.Fatalf("step %d: interval = %s, want %s", i, m.autoRefreshInterval, want)
		}
		if !strings.Contains(m.statusMsg, "Auto-refresh on") {
			t.Fatalf("step %d: status = %q, want Auto-refresh on", i, m.statusMsg)
		}
	}

	// One more press turns auto-refresh off.
	mv, cmd := m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	if m.autoRefresh {
		t.Fatalf("expected autoRefresh disabled after cycling past the longest interval")
	}
	if cmd != nil {
		t.Fatalf("expected no tick command when disabling auto-refresh")
	}
	if !strings.Contains(m.statusMsg, "Auto-refresh off") {
		t.Fatalf("expected status message about auto-refresh off, got %q", m.statusMsg)
	}

	// The cycle restarts at the shortest interval.
	mv, cmd = m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	if !m.autoRefresh || m.autoRefreshInterval != autoRefreshIntervals[0] || cmd == nil {
		t.Fatalf("expected cycle to restart at %s, got on=%v interval=%s cmd=%v",
			autoRefreshIntervals[0], m.autoRefresh, m.autoRefreshInterval, cmd)
	}
}

// TestHandleAutoRefreshSkipsWhenDisabled ensures the handler is a no-op (and
// does not reschedule) when auto-refresh has been turned off.
func TestHandleAutoRefreshSkipsWhenDisabled(t *testing.T) {
	m := Model{}
	mv, cmd := m.handleAutoRefresh(autoRefreshMsg{})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command when auto-refresh disabled, got %v", cmd)
	}
}

// TestHandleAutoRefreshReschedules verifies that an enabled auto-refresh
// always reschedules the next tick even while the user is editing, so the
// loop survives transient input sessions.
func TestHandleAutoRefreshReschedules(t *testing.T) {
	m := Model{autoRefreshState: autoRefreshState{autoRefresh: true, autoRefreshInterval: 50 * time.Millisecond, autoRefreshGen: 3}}

	// While editing: reload is skipped but the loop keeps ticking.
	m.annotating = true
	mv, cmd := m.handleAutoRefresh(autoRefreshMsg{gen: 3})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatalf("expected rescheduled tick while editing")
	}
}

// TestHandleAutoRefreshDropsStaleTicks verifies that a tick whose generation
// token no longer matches the current loop incarnation is dropped without
// rescheduling. This is what prevents duplicate reload loops from
// accumulating when auto-refresh is toggled off then back on rapidly.
func TestHandleAutoRefreshDropsStaleTicks(t *testing.T) {
	m := Model{autoRefreshState: autoRefreshState{autoRefresh: true, autoRefreshInterval: 50 * time.Millisecond, autoRefreshGen: 5}}

	// A tick from the previous (now-superseded) loop incarnation.
	mv, cmd := m.handleAutoRefresh(autoRefreshMsg{gen: 4})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatalf("expected stale tick to be dropped without rescheduling, got %v", cmd)
	}

	// A current-generation tick still reschedules. Use an active input mode
	// so the reload is skipped (no taskwarrior client in this unit test) but
	// the loop is still rescheduled.
	m.annotating = true
	mv, cmd = m.handleAutoRefresh(autoRefreshMsg{gen: 5})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatalf("expected current-generation tick to reschedule")
	}
}

// TestToggleAutoRefreshBumpsGeneration ensures each enable or interval
// advance increments the generation token so prior in-flight ticks are
// invalidated.
func TestToggleAutoRefreshBumpsGeneration(t *testing.T) {
	m := Model{}
	mv, _ := m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	gen1 := m.autoRefreshGen
	if gen1 == 0 {
		t.Fatalf("expected generation to be bumped on enable, got %d", gen1)
	}

	// Each interval advance bumps the generation again.
	mv, _ = m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	if m.autoRefreshGen <= gen1 {
		t.Fatalf("expected generation to increase on interval advance, got %d (was %d)", m.autoRefreshGen, gen1)
	}

	// Turning off and re-enabling bumps it again.
	m.autoRefreshInterval = autoRefreshIntervals[len(autoRefreshIntervals)-1]
	mv, _ = m.handleCycleAutoRefresh() // off
	m = *mv.(*Model)
	mv, _ = m.handleCycleAutoRefresh() // back on
	m = *mv.(*Model)
	if m.autoRefreshGen <= gen1 {
		t.Fatalf("expected generation to increase after off/on cycle, got %d (was %d)", m.autoRefreshGen, gen1)
	}
}

// TestTopStatusLineAutoRefreshIndicator checks that the persistent indicator
// is shown only while auto-refresh is enabled, including the fallback to the
// default interval when unset.
func TestTopStatusLineAutoRefreshIndicator(t *testing.T) {
	m := Model{}
	m.tbl.SetWidth(80)

	if strings.Contains(m.topStatusLine(), "auto-refresh") {
		t.Fatalf("did not expect auto-refresh indicator when disabled")
	}

	m.autoRefresh = true
	m.autoRefreshInterval = 10 * time.Second
	if !strings.Contains(m.topStatusLine(), "auto-refresh: on") {
		t.Fatalf("expected auto-refresh indicator when enabled")
	}

	// Zero interval falls back to the default.
	m.autoRefreshInterval = 0
	if !strings.Contains(m.topStatusLine(), autoRefreshDefaultInterval.String()) {
		t.Fatalf("expected default interval %s in indicator, got %q", autoRefreshDefaultInterval, m.topStatusLine())
	}
}

// TestUltraModeStatusAutoRefreshIndicator checks that the auto-refresh
// indicator is also shown in ultra mode's status line when enabled.
func TestUltraModeStatusAutoRefreshIndicator(t *testing.T) {
	m := Model{}
	tasks := []task.Task{{ID: 1}}

	if strings.Contains(m.ultraModeStatus(tasks), "auto-refresh") {
		t.Fatalf("did not expect auto-refresh indicator in ultra status when disabled")
	}

	m.autoRefresh = true
	m.autoRefreshInterval = 10 * time.Second
	if !strings.Contains(m.ultraModeStatus(tasks), "auto-refresh: on") {
		t.Fatalf("expected auto-refresh indicator in ultra status when enabled")
	}
}

// TestStaleAutoReloadDoesNotRescheduleLoop reproduces the double-loop race:
// an auto tick arms a reload, Z advances the cycle while the reload is in
// flight, and the finished stale reload must NOT reschedule another tick on
// top of the one the advance already scheduled.
func TestStaleAutoReloadDoesNotRescheduleLoop(t *testing.T) {
	m := newBlinkTestModel(t, &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "one", Status: "pending"}}})

	// Enable the loop: gen g, one tick scheduled.
	mv, tickCmd := m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	if tickCmd == nil {
		t.Fatalf("expected a tick command when enabling auto-refresh")
	}

	// The tick fires and arms an auto reload (snapshotting gen g).
	mv, reloadCmd := (&m).Update(autoRefreshMsg{gen: m.autoRefreshGen})
	m = *mv.(*Model)
	if reloadCmd == nil {
		t.Fatalf("auto tick did not schedule a reload")
	}
	staleGen := m.autoRefreshGen

	// Z advances the cycle while the reload is in flight: gen g+1 plus its
	// own tick for the new interval.
	mv, advanceCmd := m.handleCycleAutoRefresh()
	m = *mv.(*Model)
	if !m.autoRefresh || m.autoRefreshGen == staleGen {
		t.Fatalf("expected the cycle to advance the generation")
	}
	if advanceCmd == nil {
		t.Fatalf("expected a new tick command after advancing the interval")
	}

	// The stale reload finishes: it must not reschedule the loop.
	mv, follow := (&m).Update(reloadCmd().(taskReloadDoneMsg))
	m = *mv.(*Model)
	if follow != nil {
		t.Fatalf("stale auto reload rescheduled the loop; duplicate chains would double the refresh rate")
	}
}
