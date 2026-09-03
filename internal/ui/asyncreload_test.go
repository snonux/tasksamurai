package ui

import (
	"context"
	"strings"
	"testing"

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
