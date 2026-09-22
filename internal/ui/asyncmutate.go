package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// mutateOp is one sequential Taskwarrior mutation step. Ops are executed one
// after another inside a single tea.Cmd so the single-flight gate covers the
// whole series (Taskwarrior's on-disk DB must never see concurrent writers).
type mutateOp func(ctx context.Context, tw task.Taskwarrior) error

// mutateMeta carries follow-up UI work for handleMutateReloadDone after an
// async mutate+reload finishes. It is a value snapshot: no *Model references.
type mutateMeta struct {
	// blinkID starts a table-row blink after the reload (0 = none).
	blinkID int
	// detailBlink/detailField start a detail-view field blink instead.
	detailBlink bool
	detailField int
	// detailBlinkIfRecur decides the blink target in the DoneMsg, after the
	// reload: blink the detail recurrence field while the task still has a
	// Recur value, otherwise fall back to a row blink on blinkID.
	detailBlinkIfRecur bool
	// undo is pushed onto the undo stack only after the mutations succeeded.
	undo *undoAction
	// selectNewTask/oldIDs locate the row created by an add-task commit.
	selectNewTask bool
	oldIDs        map[int]struct{}
	// restoreInput re-opens the edit mode with the typed value when the
	// mutations failed, so the user does not lose their text. It runs in
	// Update (never inside the Cmd).
	restoreInput func(*Model)
}

// mutateReloadDoneMsg reports the result of a mutate+reload Cmd. mutateErr
// means no mutation was applied; reloadErr means mutations applied but the
// export failed (the UI keeps its previous list in that case).
type mutateReloadDoneMsg struct {
	gen       int
	data      reloadData
	mutateErr error
	reloadErr error
	meta      mutateMeta
}

// mutateReloadCmd runs ops sequentially and then re-exports. It must not
// close over *Model.
func mutateReloadCmd(parent context.Context, tw task.Taskwarrior, snap reloadSnapshot, gen int, meta mutateMeta, ops []mutateOp) tea.Cmd {
	return func() tea.Msg {
		msg := mutateReloadDoneMsg{gen: gen, meta: meta}
		// Mutations and the export each get their own full taskOperationTimeout
		// budget, matching the pre-async per-op timeouts.
		opCtx, cancel := context.WithTimeout(parent, taskOperationTimeout)
		for _, op := range ops {
			if err := op(opCtx, tw); err != nil {
				cancel()
				msg.mutateErr = err
				return msg
			}
		}
		cancel()
		exportCtx, cancelExport := context.WithTimeout(parent, taskOperationTimeout)
		defer cancelExport()
		data, err := exportReloadData(exportCtx, tw, snap)
		if err != nil {
			msg.reloadErr = fmt.Errorf("reloading tasks: %w", err)
			return msg
		}
		msg.data = data
		return msg
	}
}

// scheduleMutateReload arms a mutating flight that runs ops sequentially and
// re-exports. Returns nil (with a Busy status) when another flight is active.
func (m *Model) scheduleMutateReload(label string, meta mutateMeta, ops ...mutateOp) tea.Cmd {
	if m.taskFlightBlocks() {
		_ = m.rejectIfBusy()
		return nil
	}
	if !m.beginTaskFlight(taskFlightMutating, label) {
		m.statusMsg = "Busy: could not start " + label
		return nil
	}
	m.initTaskContext()
	gen := m.taskOpGen
	tw := m.taskwarriorClient()
	snap := m.captureReloadSnapshot()
	return mutateReloadCmd(m.taskContext, tw, snap, gen, meta, ops)
}

// busyCommitErr is the error text-input commits return while a flight is
// active. It keeps the input mode open (handleTextInput exits the mode only
// on success) and surfaces the busy state in the status line.
func (m *Model) busyCommitErr() error {
	label := m.taskFlightLabel
	if label == "" {
		label = m.taskFlight.String()
	}
	return fmt.Errorf("Busy: %s", label)
}

// doneOp marks addr done.
func doneOp(addr string) mutateOp {
	return func(ctx context.Context, tw task.Taskwarrior) error {
		return tw.DoneContext(ctx, addr)
	}
}

// doneUndoForID snapshots the undo restore for marking task id done. The
// undo entry is pushed by handleMutateReloadDone only after success.
func (m *Model) doneUndoForID(id int) *undoAction {
	for _, tsk := range m.tasks {
		if tsk.ID == id {
			return &undoAction{
				label:    "done",
				restores: []undoRestore{{uuid: tsk.UUID, status: "pending"}},
			}
		}
	}
	return nil
}

// handleMutateReloadDone applies a finished mutate+reload pipeline: push the
// undo entry (mutations succeeded), surface errors, apply the reloaded data,
// then run the deferred blink / add-task selection.
func (m *Model) handleMutateReloadDone(msg mutateReloadDoneMsg) (tea.Model, tea.Cmd) {
	if !m.matchesTaskOpGen(msg.gen) || m.taskFlight != taskFlightMutating {
		return m, nil
	}
	label := m.taskFlightLabel
	m.endTaskFlight()
	// Drop the in-flight label so it does not linger after success.
	if msg.mutateErr == nil && msg.reloadErr == nil && m.statusMsg == label {
		m.statusMsg = ""
	}
	if msg.mutateErr != nil {
		// Undo stack is untouched on mutate failure; re-open the edit mode
		// with the typed value so nothing the user entered is lost.
		if msg.meta.restoreInput != nil {
			msg.meta.restoreInput(m)
		}
		m.showErrorTimed(msg.mutateErr)
		return m, nil
	}
	if msg.meta.undo != nil {
		m.pushUndoAction(msg.meta.undo.label, msg.meta.undo.restores)
	}
	if msg.reloadErr != nil {
		m.showErrorTimed(msg.reloadErr)
		return m, nil
	}

	data := msg.data
	m.processTasks(&data)
	m.renderTasks(data)

	if msg.meta.selectNewTask {
		return m.selectAddedTask(msg.meta)
	}
	if msg.meta.detailBlink {
		return m, m.startDetailBlink(msg.meta.detailField)
	}
	if msg.meta.detailBlinkIfRecur {
		if t := m.currentDetailTask(); t != nil && t.Recur != "" {
			return m, m.startDetailBlink(fieldRecur)
		}
	}
	if msg.meta.blinkID != 0 {
		return m, m.startBlink(msg.meta.blinkID, false)
	}
	return m, nil
}

// selectAddedTask moves the cursor to the freshly added task and blinks it.
func (m *Model) selectAddedTask(meta mutateMeta) (tea.Model, tea.Cmd) {
	var newID int
	row := -1
	for i, tsk := range m.tasks {
		if _, ok := meta.oldIDs[tsk.ID]; !ok {
			newID = tsk.ID
			row = i
			break
		}
	}
	m.updateTableHeight()
	if row < 0 {
		return m, nil
	}
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(row)
	m.tbl.SetColumnCursor(7) // Description column
	m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	if m.showUltra {
		m.ultraFocusedID = newID
		m.selectTaskByID(newID)
		m.ultraFocusedID = 0
	}
	return m, m.startBlink(newID, false)
}
