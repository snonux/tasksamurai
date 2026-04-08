package ui

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// handleBlinkingState handles input when a task is blinking
func (m *Model) handleBlinkingState(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		if m.showUltra {
			return m.handleUltraBlinkingState(msg.(tea.KeyPressMsg))
		}

		// Only allow navigation while blinking.
		prevRow := m.tbl.Cursor()
		prevCol := m.tbl.ColumnCursor()
		var cmd tea.Cmd
		m.tbl, cmd = m.tbl.Update(msg)
		if prevRow != m.tbl.Cursor() || prevCol != m.tbl.ColumnCursor() {
			m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleUltraBlinkingState(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.ultraMoveCursor(1)
	case "k", "up":
		m.ultraMoveCursor(-1)
	case "pgdn", "pgdown", "space":
		m.ultraMoveCursor(m.ultraVisibleCount())
	case "pgup", "b":
		m.ultraMoveCursor(-m.ultraVisibleCount())
	case "g", "home":
		m.ultraGoHome()
	case "G", "end":
		m.ultraGoEnd()
	}
	return m, nil
}

// handleEditingModes checks if we're in any editing mode and handles it
func (m *Model) handleEditingModes(msg tea.KeyPressMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	switch {
	case m.annotating:
		model, cmd = m.handleAnnotationMode(msg)
		return true, model, cmd
	case m.descEditing:
		model, cmd = m.handleDescriptionMode(msg)
		return true, model, cmd
	case m.tagsEditing:
		model, cmd = m.handleTagsMode(msg)
		return true, model, cmd
	case m.dueEditing:
		model, cmd = m.handleDueEditMode(msg)
		return true, model, cmd
	case m.recurEditing:
		model, cmd = m.handleRecurrenceMode(msg)
		return true, model, cmd
	case m.projEditing:
		model, cmd = m.handleProjectMode(msg)
		return true, model, cmd
	case m.prioritySelecting:
		model, cmd = m.handlePriorityMode(msg)
		return true, model, cmd
	case m.filterEditing:
		model, cmd = m.handleFilterMode(msg)
		return true, model, cmd
	case m.addingTask:
		model, cmd = m.handleAddTaskMode(msg)
		return true, model, cmd
	case m.searching:
		model, cmd = m.handleSearchMode(msg)
		return true, model, cmd
	case m.helpSearching:
		model, cmd = m.handleHelpSearchMode(msg)
		return true, model, cmd
	}
	return false, m, nil
}

// getSelectedTaskID extracts the task ID from the selected row
func (m *Model) getSelectedTaskID() (int, error) {
	row := m.tbl.SelectedRow()
	if row == nil {
		return 0, fmt.Errorf("no row selected")
	}
	idStr := ansi.Strip(row[1])
	return strconv.Atoi(idStr)
}

// getTaskAtCursor returns the task at the current cursor position
func (m *Model) getTaskAtCursor() *task.Task {
	cursor := m.tbl.Cursor()
	if cursor < 0 || cursor >= len(m.tasks) {
		return nil
	}
	return &m.tasks[cursor]
}

// getTaskForOpenURL returns the task that should be used by the open-URL
// hotkey, honoring the active view's highlighted task.
func (m *Model) getTaskForOpenURL() *task.Task {
	if m.showTaskDetail && m.currentTaskDetail != nil {
		return m.currentTaskDetail
	}

	if m.showUltra {
		tasks := m.ultraTaskList()
		if m.ultraCursor < 0 || m.ultraCursor >= len(tasks) {
			return nil
		}
		return &tasks[m.ultraCursor]
	}

	return m.getTaskAtCursor()
}
