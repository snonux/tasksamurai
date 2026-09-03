package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// deleteSeriesDoneMsg is delivered when an async delete (+ reload) finishes.
type deleteSeriesDoneMsg struct {
	gen       int
	count     int
	recurring bool
	restores  []undoRestore
	data      reloadData
	err       error
}

// undoActionDoneMsg is delivered when an async undo (+ reload) finishes.
type undoActionDoneMsg struct {
	gen     int
	action  undoAction
	blinkID int
	data    reloadData
	err     error
}

// deleteSeriesCmd deletes tsk (and its recurring series when needed), reloads
// pending tasks, and returns a DoneMsg. It must not close over *Model.
//
// Consistency: on partial multi-UUID failure, already-deleted UUIDs are rolled
// back via quitCtx (quit-cancellable timeout — never context.Background()).
// The undo stack is untouched here; Update pushes only after a successful msg.
func deleteSeriesCmd(quitCtx context.Context, tw task.Taskwarrior, tsk task.Task, snap reloadSnapshot, gen int) tea.Cmd {
	return func() tea.Msg {
		opCtx, cancel := context.WithTimeout(quitCtx, taskOperationTimeout)
		defer cancel()

		restores, recurring, err := deleteSeries(opCtx, quitCtx, tw, tsk)
		if err != nil {
			return deleteSeriesDoneMsg{gen: gen, recurring: recurring, err: err}
		}
		data, reloadErr := exportReloadData(opCtx, tw, snap)
		if reloadErr != nil {
			// Mutate succeeded; surface reload failure without inventing undo —
			// Update still pushes undo so U can restore.
			return deleteSeriesDoneMsg{
				gen:       gen,
				count:     len(restores),
				recurring: recurring,
				restores:  restores,
				err:       fmt.Errorf("reloading tasks: %w", reloadErr),
			}
		}
		return deleteSeriesDoneMsg{
			gen:       gen,
			count:     len(restores),
			recurring: recurring,
			restores:  restores,
			data:      data,
		}
	}
}

// undoActionCmd restores statuses for a snapshotted undo action, reloads, and
// resolves a blink task ID. It must not close over *Model.
//
// Consistency: the Model undo stack is only popped in Update on success.
// Partial restore failure rolls already-restored UUIDs back to their pre-undo
// status (deleted / completed) using quitCtx.
func undoActionCmd(quitCtx context.Context, tw task.Taskwarrior, action undoAction, snap reloadSnapshot, gen int) tea.Cmd {
	return func() tea.Msg {
		opCtx, cancel := context.WithTimeout(quitCtx, taskOperationTimeout)
		defer cancel()

		if err := undoActionRestores(opCtx, quitCtx, tw, action); err != nil {
			return undoActionDoneMsg{gen: gen, action: action, err: err}
		}
		data, err := exportReloadData(opCtx, tw, snap)
		if err != nil {
			return undoActionDoneMsg{gen: gen, action: action, err: fmt.Errorf("reloading tasks: %w", err)}
		}
		blinkID := resolveUndoBlinkID(opCtx, tw, action, data.tasks, snap.filters)
		return undoActionDoneMsg{
			gen:     gen,
			action:  action,
			blinkID: blinkID,
			data:    data,
		}
	}
}

// scheduleDeleteSeries arms a mutating flight and returns a delete+reload Cmd.
func (m *Model) scheduleDeleteSeries(tsk task.Task) tea.Cmd {
	if m.taskFlightBlocks() {
		_ = m.rejectIfBusy()
		return nil
	}
	if !m.beginTaskFlight(taskFlightMutating, "Deleting…") {
		m.statusMsg = "Busy: could not start delete"
		return nil
	}
	m.initTaskContext()
	gen := m.taskOpGen
	tw := m.taskwarriorClient()
	snap := m.captureReloadSnapshot()
	return deleteSeriesCmd(m.taskContext, tw, tsk, snap, gen)
}

// scheduleUndoAction arms a mutating flight for the top undo entry without
// popping the stack until a successful DoneMsg.
func (m *Model) scheduleUndoAction() tea.Cmd {
	if len(m.undoStack) == 0 {
		return nil
	}
	if m.taskFlightBlocks() {
		_ = m.rejectIfBusy()
		return nil
	}
	action := copyUndoAction(m.undoStack[len(m.undoStack)-1])
	if !m.beginTaskFlight(taskFlightMutating, "Undoing…") {
		m.statusMsg = "Busy: could not start undo"
		return nil
	}
	m.initTaskContext()
	gen := m.taskOpGen
	tw := m.taskwarriorClient()
	snap := m.captureReloadSnapshot()
	return undoActionCmd(m.taskContext, tw, action, snap, gen)
}

func (m *Model) handleDeleteSeriesDone(msg deleteSeriesDoneMsg) (tea.Model, tea.Cmd) {
	if !m.matchesTaskOpGen(msg.gen) || m.taskFlight != taskFlightMutating {
		return m, nil
	}
	m.endTaskFlight()
	if msg.err != nil {
		// Reload failure after a successful delete still gets an undo entry so
		// the user can restore; mutate failure leaves the stack unchanged.
		if len(msg.restores) > 0 {
			m.pushUndoAction("delete", msg.restores)
		}
		m.showError(msg.err)
		return m, nil
	}
	m.pushUndoAction("delete", msg.restores)
	data := msg.data
	m.processTasks(&data)
	m.renderTasks(data)
	if msg.recurring {
		m.statusMsg = fmt.Sprintf("Deleted %d recurring tasks", msg.count)
	} else {
		m.statusMsg = "Deleted task"
	}
	return m, nil
}

func (m *Model) handleUndoActionDone(msg undoActionDoneMsg) (tea.Model, tea.Cmd) {
	if !m.matchesTaskOpGen(msg.gen) || m.taskFlight != taskFlightMutating {
		return m, nil
	}
	m.endTaskFlight()
	if msg.err != nil {
		m.showError(msg.err)
		return m, nil
	}
	if len(m.undoStack) > 0 {
		m.undoStack = m.undoStack[:len(m.undoStack)-1]
	}
	data := msg.data
	m.processTasks(&data)
	m.renderTasks(data)
	if msg.blinkID == 0 {
		m.statusMsg = undoStatus(msg.action)
		return m, nil
	}
	return m, m.startBlink(msg.blinkID, false)
}

// deleteSeries deletes tsk and its recurring series when applicable.
// On partial failure it rolls back completed deletes using quitCtx.
func deleteSeries(opCtx, quitCtx context.Context, tw task.Taskwarrior, tsk task.Task) ([]undoRestore, bool, error) {
	if strings.TrimSpace(tsk.UUID) == "" {
		return nil, false, fmt.Errorf("task %d has no UUID", tsk.ID)
	}

	recurring := isRecurringTask(tsk)
	tasks := []task.Task{tsk}
	if recurring {
		series, err := tw.RecurringSeries(opCtx, recurringRootUUID(tsk))
		if err != nil {
			return nil, true, fmt.Errorf("loading recurring series: %w", err)
		}
		tasks = mergeTasksByUUID(series, tsk)
	}

	tasks = deleteOrder(tasks, recurringRootUUID(tsk))
	restores := make([]undoRestore, 0, len(tasks))
	for _, candidate := range tasks {
		if strings.TrimSpace(candidate.UUID) == "" {
			continue
		}
		restores = append(restores, undoRestore{uuid: candidate.UUID, status: undoStatusForTask(candidate)})
	}
	if len(restores) == 0 {
		return nil, recurring, fmt.Errorf("no task UUIDs to delete")
	}

	completed := make([]undoRestore, 0, len(restores))
	for _, restore := range restores {
		if err := tw.SetStatusUUIDContext(opCtx, restore.uuid, "deleted"); err != nil {
			if rollbackErr := rollbackUndoRestores(quitCtx, tw, completed); rollbackErr != nil {
				return nil, recurring, fmt.Errorf("deleting task %s: %w; rollback failed: %w", restore.uuid, err, rollbackErr)
			}
			return nil, recurring, fmt.Errorf("deleting task %s: %w", restore.uuid, err)
		}
		completed = append(completed, restore)
	}
	return restores, recurring, nil
}

// undoActionRestores applies each restore in action. On partial failure it
// rolls already-applied restores back to the pre-undo status.
func undoActionRestores(opCtx, quitCtx context.Context, tw task.Taskwarrior, action undoAction) error {
	completed := make([]undoRestore, 0, len(action.restores))
	for _, restore := range action.restores {
		if err := tw.SetStatusUUIDContext(opCtx, restore.uuid, restore.status); err != nil {
			if rollbackErr := rollbackUndoToPrior(quitCtx, tw, action.label, completed); rollbackErr != nil {
				return fmt.Errorf("restoring task %s: %w; rollback failed: %w", restore.uuid, err, rollbackErr)
			}
			return fmt.Errorf("restoring task %s: %w", restore.uuid, err)
		}
		completed = append(completed, restore)
	}
	return nil
}

// rollbackUndoRestores restores previously deleted tasks to their prior status.
// quitCtx ties cancellation to quit; a fresh timeout is derived so rollback can
// still run after an opCtx deadline without using immortal context.Background().
func rollbackUndoRestores(quitCtx context.Context, tw task.Taskwarrior, restores []undoRestore) error {
	ctx, cancel := context.WithTimeout(quitCtx, taskOperationTimeout)
	defer cancel()

	var errs []error
	for i := len(restores) - 1; i >= 0; i-- {
		if err := tw.SetStatusUUIDContext(ctx, restores[i].uuid, restores[i].status); err != nil {
			errs = append(errs, fmt.Errorf("restoring task %s to %s: %w", restores[i].uuid, restores[i].status, err))
		}
	}
	return errors.Join(errs...)
}

// rollbackUndoToPrior reverts a partial undo by setting completed restores back
// to the status they had before the undo attempt.
func rollbackUndoToPrior(quitCtx context.Context, tw task.Taskwarrior, label string, completed []undoRestore) error {
	prior := priorStatusForUndo(label)
	reverts := make([]undoRestore, len(completed))
	for i, r := range completed {
		reverts[i] = undoRestore{uuid: r.uuid, status: prior}
	}
	return rollbackUndoRestores(quitCtx, tw, reverts)
}

func priorStatusForUndo(label string) string {
	if label == "delete" {
		return "deleted"
	}
	return "completed"
}

func copyUndoAction(action undoAction) undoAction {
	return undoAction{
		label:    action.label,
		restores: append([]undoRestore(nil), action.restores...),
	}
}

func resolveUndoBlinkID(ctx context.Context, tw task.Taskwarrior, action undoAction, tasks []task.Task, filters []string) int {
	for _, restore := range action.restores {
		for _, tsk := range tasks {
			if tsk.UUID == restore.uuid && tsk.ID != 0 {
				return tsk.ID
			}
		}
	}
	for _, restore := range action.restores {
		lookup := []string{restore.uuid}
		if filters != nil {
			lookup = append(lookup, filters...)
		}
		lookup = append(lookup, "status:"+restore.status)
		exported, err := tw.Export(ctx, lookup...)
		if err == nil && len(exported) > 0 && exported[0].ID != 0 {
			return exported[0].ID
		}
	}
	return 0
}
