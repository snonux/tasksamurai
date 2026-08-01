package ui

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// TestHandleToggleAutoRefresh verifies that toggling auto-refresh flips the
// flag, sets an informative status message, and only schedules a tick when
// enabled.
func TestHandleToggleAutoRefresh(t *testing.T) {
	m := Model{}

	// Enable.
	mv, cmd := m.handleToggleAutoRefresh()
	m = *mv.(*Model)
	if !m.autoRefresh {
		t.Fatalf("expected autoRefresh enabled after toggle")
	}
	if cmd == nil {
		t.Fatalf("expected a tick command when enabling auto-refresh")
	}
	if m.autoRefreshInterval != autoRefreshDefaultInterval {
		t.Fatalf("expected default interval %s, got %s", autoRefreshDefaultInterval, m.autoRefreshInterval)
	}
	if !strings.Contains(m.statusMsg, "Auto-refresh on") {
		t.Fatalf("expected status message about auto-refresh on, got %q", m.statusMsg)
	}

	// Disable.
	mv, cmd = m.handleToggleAutoRefresh()
	m = *mv.(*Model)
	if m.autoRefresh {
		t.Fatalf("expected autoRefresh disabled after second toggle")
	}
	if cmd != nil {
		t.Fatalf("expected no tick command when disabling auto-refresh")
	}
	if !strings.Contains(m.statusMsg, "Auto-refresh off") {
		t.Fatalf("expected status message about auto-refresh off, got %q", m.statusMsg)
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

// TestToggleAutoRefreshBumpsGeneration ensures each (re)enable increments the
// generation token so prior in-flight ticks are invalidated.
func TestToggleAutoRefreshBumpsGeneration(t *testing.T) {
	m := Model{}
	mv, _ := m.handleToggleAutoRefresh()
	m = *mv.(*Model)
	gen1 := m.autoRefreshGen
	if gen1 == 0 {
		t.Fatalf("expected generation to be bumped on enable, got %d", gen1)
	}

	// Disable then re-enable.
	mv, _ = m.handleToggleAutoRefresh()
	m = *mv.(*Model)
	mv, _ = m.handleToggleAutoRefresh()
	m = *mv.(*Model)
	if m.autoRefreshGen <= gen1 {
		t.Fatalf("expected generation to increase on re-enable, got %d (was %d)", m.autoRefreshGen, gen1)
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
