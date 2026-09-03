package ui

import (
	"context"
	"fmt"
)

// taskFlightKind is the single-flight gate for external Taskwarrior work.
// At most one non-idle kind runs at a time so concurrent task(1) writers
// (and writer vs export races) cannot overlap. See docs/async-task-commands.md.
type taskFlightKind int

const (
	taskFlightIdle taskFlightKind = iota
	taskFlightCompleting
	taskFlightShell
	taskFlightMutating
	taskFlightReloading
)

func (k taskFlightKind) String() string {
	switch k {
	case taskFlightIdle:
		return "idle"
	case taskFlightCompleting:
		return "completing"
	case taskFlightShell:
		return "shell"
	case taskFlightMutating:
		return "mutating"
	case taskFlightReloading:
		return "reloading"
	default:
		return fmt.Sprintf("taskFlightKind(%d)", int(k))
	}
}

// taskFlightState tracks in-flight Taskwarrior pipelines and a generation
// token so late tea.Msg results from superseded or quit-cancelled Cmds are
// ignored in Update.
type taskFlightState struct {
	taskFlight      taskFlightKind
	taskFlightLabel string
	taskOpGen       int
}

// taskFlightBlocks reports whether a non-completion Taskwarrior pipeline is
// running. Completion loads are supersedable (shell Enter may replace them)
// and do not block the Busy reject path the same way.
func (m *Model) taskFlightBlocks() bool {
	switch m.taskFlight {
	case taskFlightShell, taskFlightMutating, taskFlightReloading:
		return true
	default:
		return false
	}
}

// taskFlightActive is true for any non-idle flight, including completion
// loads. Used for auto-refresh skipping.
func (m *Model) taskFlightActive() bool {
	return m.taskFlight != taskFlightIdle
}

// recreateTaskContext cancels the parent Taskwarrior context and installs a
// fresh one so superseded Cmds stop and later ops are not stuck on a
// cancelled parent. Do not call this from quit — quit should leave the
// context cancelled.
func (m *Model) recreateTaskContext() {
	if m.cancelTaskContext != nil {
		m.cancelTaskContext()
	}
	m.taskContext, m.cancelTaskContext = context.WithCancel(context.Background())
}

// beginTaskFlight arms the single-flight gate. Returns false when another
// blocking flight is already active. A completion load may be superseded by
// shell/mutate/reload; supersede cancels the previous Cmd via context
// recreate. Always bumps taskOpGen on success so stale msgs from the
// previous flight are ignored.
func (m *Model) beginTaskFlight(kind taskFlightKind, label string) bool {
	if kind == taskFlightIdle {
		return false
	}
	if m.taskFlightBlocks() {
		return false
	}
	if m.taskFlight == taskFlightCompleting && kind == taskFlightCompleting {
		return false
	}
	if m.taskFlight == taskFlightCompleting && kind != taskFlightCompleting {
		// Stop the completion Cmd (shared parent ctx) before starting a writer.
		m.shellCompletionLoad = false
		m.recreateTaskContext()
	}
	m.taskFlight = kind
	m.taskFlightLabel = label
	m.taskOpGen++
	if label != "" {
		m.statusMsg = label
	}
	return true
}

// endTaskFlight clears the gate after a matching DoneMsg is applied.
func (m *Model) endTaskFlight() {
	m.taskFlight = taskFlightIdle
	m.taskFlightLabel = ""
}

// invalidateTaskFlights bumps the generation and clears flight state so late
// Cmd results are ignored. Used on quit after cancelling the parent context.
func (m *Model) invalidateTaskFlights() {
	m.taskOpGen++
	m.shellCompletionLoad = false
	m.endTaskFlight()
}

// cancelCompletingFlight stops an in-flight completion load (e.g. Esc from
// the shell prompt) without starting a replacement pipeline.
func (m *Model) cancelCompletingFlight() {
	if m.taskFlight != taskFlightCompleting {
		return
	}
	m.shellCompletionLoad = false
	m.recreateTaskContext()
	m.taskOpGen++
	m.endTaskFlight()
}

// rejectIfBusy sets a Busy status and returns true when a blocking flight is
// active. Callers should leave UI mode flags unchanged and return nil Cmd.
func (m *Model) rejectIfBusy() bool {
	if !m.taskFlightBlocks() {
		return false
	}
	label := m.taskFlightLabel
	if label == "" {
		label = m.taskFlight.String()
	}
	m.statusMsg = fmt.Sprintf("Busy: %s", label)
	return true
}

// matchesTaskOpGen reports whether msgGen is the current live flight
// generation. Gen 0 is never live (no beginTaskFlight yet).
func (m *Model) matchesTaskOpGen(msgGen int) bool {
	return msgGen != 0 && msgGen == m.taskOpGen
}
