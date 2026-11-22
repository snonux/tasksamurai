package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// Queued task actions.
//
// Deletes, start/stop and done run asynchronously behind the single-flight
// Taskwarrior gate (taskflight.go), and done first plays a row blink. Pressing
// D, d or s on another task while that work was still running used to fall
// through to the table's navigation keymap: d is the table's half-page-down
// binding, so the cursor jumped, and D/s were silently rejected as Busy.
//
// Instead those presses are captured here together with the UUID of the task
// under the cursor at press time, and replayed in order once the gate is idle
// and no blink is running. The UUID (not the row or numeric ID) is captured
// because the async result re-renders the table and Taskwarrior renumbers
// pending IDs, so the row under the cursor at replay time may be a different
// task. Overlapping task(1) writers are still impossible: replay only starts a
// new flight after the previous one has finished.

// maxQueuedTaskActions bounds the queue so a held-down key cannot pile up an
// unbounded backlog of writes.
const maxQueuedTaskActions = 32

// queuedTaskAction is one deferred D/d/s press.
type queuedTaskAction struct {
	key  string // "D", "d" or "s"
	uuid string // target task, captured at press time
}

// isQueueableTaskKey reports whether key is an immediate single-task action
// that may be deferred while busy. Keys that open prompts or editors are not
// queued: popping an input up seconds later would be surprising.
func isQueueableTaskKey(key string) bool {
	switch key {
	case "D", "d", "s":
		return true
	default:
		return false
	}
}

// Update handles every tea.Msg via update and then, once the Taskwarrior gate
// is free, replays the next queued task action. Draining after every message
// covers all the places a flight or blink can end (each *DoneMsg handler,
// blink ticks, errors) without having to hook each of them.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	if model != tea.Model(m) {
		return model, cmd
	}
	if next := m.drainQueuedTaskAction(); next != nil {
		cmd = tea.Batch(cmd, next)
	}
	return m, cmd
}

// queueTaskActionKey defers a D/d/s press made while a Taskwarrior flight or
// row blink is running. It returns false when key is not queueable, so the
// caller can fall back to its usual navigation / Busy handling.
func (m *Model) queueTaskActionKey(key string) bool {
	if !isQueueableTaskKey(key) {
		return false
	}
	t := m.getTaskForOpenURL()
	if t == nil || strings.TrimSpace(t.UUID) == "" {
		m.statusMsg = "No task selected"
		return true
	}
	action := queuedTaskAction{key: key, uuid: t.UUID}
	for _, queued := range m.queuedActions {
		if queued == action {
			// Repeating the same key on the same task (e.g. D pressed twice)
			// must not run the action twice.
			m.statusMsg = fmt.Sprintf("Already queued: %s", queuedActionLabel(key))
			return true
		}
	}
	if len(m.queuedActions) >= maxQueuedTaskActions {
		m.statusMsg = "Busy: action queue is full"
		return true
	}
	m.queuedActions = append(m.queuedActions, action)
	if m.showTaskDetail && key != "s" {
		// Mirror the detail view's own D/d handlers, which close the overlay
		// because the task is about to leave the pending list.
		m.closeDetailView()
	}
	m.statusMsg = fmt.Sprintf("Queued: %s (%d pending)", queuedActionLabel(key), len(m.queuedActions))
	return true
}

// drainQueuedTaskAction pops queued actions while the gate is idle and runs the
// first one that still applies. Tasks that vanished in the meantime (deleted,
// filtered out) are skipped. It waits while the user is typing in a prompt so
// a replay never yanks an input away.
func (m *Model) drainQueuedTaskAction() tea.Cmd {
	for len(m.queuedActions) > 0 {
		if m.taskFlightActive() || m.blinkID != 0 || m.anyInputActive() {
			return nil
		}
		action := m.queuedActions[0]
		m.queuedActions = m.queuedActions[1:]
		t := m.taskByUUID(action.uuid)
		if t == nil {
			m.statusMsg = fmt.Sprintf("Skipped queued %s: task is gone", queuedActionLabel(action.key))
			continue
		}
		tsk := *t
		cmd := m.runQueuedTaskAction(action.key, tsk)
		if cmd != nil || m.taskFlightActive() || m.blinkID != 0 {
			return cmd
		}
		// The action finished synchronously without starting a flight
		// (e.g. "already completed"); keep draining.
	}
	return nil
}

// runQueuedTaskAction dispatches a queued key against its captured task.
func (m *Model) runQueuedTaskAction(key string, tsk task.Task) tea.Cmd {
	switch key {
	case "D":
		if m.blockJiraAddress(tsk.UUID, "D") {
			return nil
		}
		return m.deleteTask(tsk)
	case "d":
		return m.markTaskDone(tsk)
	case "s":
		return m.toggleStartTask(tsk)
	default:
		return nil
	}
}

func queuedActionLabel(key string) string {
	switch key {
	case "D":
		return "delete"
	case "d":
		return "done"
	case "s":
		return "start/stop"
	default:
		return key
	}
}
