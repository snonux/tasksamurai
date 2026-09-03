package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

type reloadReason int

const (
	reloadReasonPlain reloadReason = iota
	reloadReasonShell
	reloadReasonEdit
	reloadReasonSearch
	reloadReasonAuto
)

// reloadSnapshot is a value copy of everything taskReloadCmd needs so the
// Cmd never closes over *Model (see docs/async-task-commands.md).
type reloadSnapshot struct {
	filters        []string
	ultraFilterIDs []int
}

// reloadMeta carries follow-up UI work for handleTaskReloadDone after an
// async export finishes.
type reloadMeta struct {
	reason      reloadReason
	selectedID  int
	editID      int
	shellResult task.RunResult
	shellErr    error
}

type taskReloadDoneMsg struct {
	gen  int
	data reloadData
	err  error
	meta reloadMeta
}

// taskReloadCmd exports tasks using snapshotted filters. It must not close
// over *Model.
func taskReloadCmd(parent context.Context, tw task.Taskwarrior, snap reloadSnapshot, gen int, meta reloadMeta) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, taskOperationTimeout)
		defer cancel()
		data, err := exportReloadData(ctx, tw, snap)
		return taskReloadDoneMsg{gen: gen, data: data, err: err, meta: meta}
	}
}

func exportReloadData(ctx context.Context, tw task.Taskwarrior, snap reloadSnapshot) (reloadData, error) {
	filters := append(append([]string(nil), snap.filters...), "status:pending")
	tasks, err := tw.Export(ctx, filters...)
	if err != nil {
		return reloadData{}, err
	}
	tw.SortTasks(tasks)
	return reloadData{
		tasks:          tasks,
		ultraFilterIDs: snap.ultraFilterIDs,
	}, nil
}

// scheduleTaskReload arms a reloading flight and returns an export Cmd.
// When reportBusy is true and another blocking flight is active, the status
// line shows Busy and no Cmd is returned.
func (m *Model) scheduleTaskReload(label string, meta reloadMeta, reportBusy bool) tea.Cmd {
	if m.taskFlightBlocks() {
		if reportBusy {
			_ = m.rejectIfBusy()
		}
		return nil
	}
	if !m.beginTaskFlight(taskFlightReloading, label) {
		if reportBusy {
			m.statusMsg = "Busy: could not start reload"
		}
		return nil
	}
	m.initTaskContext()
	gen := m.taskOpGen
	tw := m.taskwarriorClient()
	snap := m.captureReloadSnapshot()
	return taskReloadCmd(m.taskContext, tw, snap, gen, meta)
}

func (m *Model) handleTaskReloadDone(msg taskReloadDoneMsg) (tea.Model, tea.Cmd) {
	if !m.matchesTaskOpGen(msg.gen) || m.taskFlight != taskFlightReloading {
		return m, nil
	}
	m.endTaskFlight()
	if msg.err != nil {
		m.showError(fmt.Errorf("reloading tasks: %w", msg.err))
		if msg.meta.reason == reloadReasonAuto {
			return m, m.maybeAutoRefreshTick()
		}
		return m, nil
	}

	data := msg.data
	m.processTasks(&data)
	m.renderTasks(data)

	switch msg.meta.reason {
	case reloadReasonShell:
		return m.finishShellReload(msg.meta)
	case reloadReasonEdit:
		return m.finishEditReload(msg.meta)
	case reloadReasonSearch:
		return m.finishSearchReload()
	case reloadReasonAuto:
		m.clearReloadingStatus()
		return m, m.maybeAutoRefreshTick()
	default:
		m.clearReloadingStatus()
		return m, nil
	}
}

func (m *Model) maybeAutoRefreshTick() tea.Cmd {
	if !m.autoRefresh {
		return nil
	}
	interval := m.autoRefreshInterval
	if interval <= 0 {
		interval = autoRefreshDefaultInterval
	}
	return autoRefreshCmd(interval, m.autoRefreshGen)
}

func (m *Model) finishShellReload(meta reloadMeta) (tea.Model, tea.Cmd) {
	if meta.selectedID > 0 {
		_ = m.selectTaskByID(meta.selectedID)
	}
	output := shellOutput(meta.shellResult, meta.shellErr)
	if strings.TrimSpace(output) == "" {
		if meta.shellErr != nil {
			m.showError(meta.shellErr)
		} else {
			m.statusMsg = fmt.Sprintf("task %s completed", strings.Join(meta.shellResult.Args, " "))
		}
		return m, nil
	}
	m.showShellOutput(shellTitle(meta.shellResult, meta.shellErr), output)
	return m, nil
}

func (m *Model) finishEditReload(meta reloadMeta) (tea.Model, tea.Cmd) {
	id := meta.editID
	m.editID = 0
	if id == 0 {
		return m, nil
	}
	return m, m.startBlink(id, false)
}

func (m *Model) finishSearchReload() (tea.Model, tea.Cmd) {
	m.updateTableHeight()
	if len(m.searchMatches) > 0 {
		match := m.searchMatches[m.searchIndex]
		prevRow := m.tbl.Cursor()
		prevCol := m.tbl.ColumnCursor()
		m.tbl.SetCursor(match.row)
		m.tbl.SetColumnCursor(match.col)
		m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	}
	m.clearReloadingStatus()
	return m, nil
}

func (m *Model) captureReloadSnapshot() reloadSnapshot {
	return reloadSnapshot{
		filters:        append([]string(nil), m.filters...),
		ultraFilterIDs: append([]int(nil), m.ultraFilteredTaskIDs()...),
	}
}

func (m *Model) clearReloadingStatus() {
	if m.statusMsg == "Reloading…" {
		m.statusMsg = ""
	}
}
