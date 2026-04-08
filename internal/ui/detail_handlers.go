package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// handleTaskDetailMode handles keyboard input in task detail view
func (m *Model) handleTaskDetailMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.detailSearching {
		var cmd tea.Cmd
		switch msg.String() {
		case "enter":
			pattern := m.detailSearchInput.Value()
			if pattern != "" {
				re, err := compileAndCacheRegex(pattern)
				if err == nil {
					m.detailSearchRegex = re
				} else {
					m.detailSearchRegex = nil
					m.statusMsg = fmt.Sprintf("Invalid regex: %v", err)
				}
			} else {
				m.detailSearchRegex = nil
			}
			m.detailSearching = false
			m.detailSearchInput.Blur()
			return m, nil
		case "esc", "ctrl+c":
			m.detailSearching = false
			m.detailSearchInput.Blur()
			return m, nil
		default:
			m.detailSearchInput, cmd = m.detailSearchInput.Update(msg)
			return m, cmd
		}
	}

	// Normal task detail view mode
	switch msg.String() {
	case "q":
		return m.handleQuitKey()
	case "esc":
		return m.handleEscapeKey()
	case "/", "?":
		m.detailSearching = true
		m.detailSearchInput.SetValue("")
		m.detailSearchInput.Focus()
		return m, nil
	case "n":
		// Next search match - not implemented yet but could be added
		return m, nil
	case "N":
		// Previous search match - not implemented yet but could be added
		return m, nil
	case "up", "k":
		if m.detailFieldIndex > 0 {
			m.detailFieldIndex--
		}
		return m, nil
	case "down", "j":
		maxFields := m.getDetailFieldCount()
		if m.detailFieldIndex < maxFields-1 {
			m.detailFieldIndex++
		}
		return m, nil
	case "g", "home":
		m.detailFieldIndex = 0
		return m, nil
	case "G", "end":
		m.detailFieldIndex = m.getDetailFieldCount() - 1
		return m, nil
	case "o":
		return m.handleOpenURL()
	case "i", "enter":
		// Check if current field is editable
		return m.handleDetailFieldEdit()
	}

	return m, nil
}

// handleDetailFieldEdit starts editing for the currently-selected field in the
// detail view. Fields 0-2 (ID, UUID, Status) and 6, 8 (Start, Entry) are
// read-only; all others delegate to the appropriate activation helper.
func (m *Model) handleDetailFieldEdit() (tea.Model, tea.Cmd) {
	if m.currentTaskDetail == nil {
		return m, nil
	}
	t := m.currentTaskDetail
	id := t.ID

	// Fixed-position fields (indices always match the fieldXxx constants).
	switch m.detailFieldIndex {
	case fieldID, fieldUUID, fieldStatus, fieldStart, fieldEntry:
		return m, nil // read-only fields
	case fieldPriority:
		m.activatePriorityEdit(id, t.Priority)
		return m, nil
	case fieldTags:
		m.activateTagsEdit(id)
		return m, nil
	case fieldDue:
		m.activateDueEdit(id, t.Due)
		return m, nil
	case fieldProject:
		m.activateProjectEdit(id, t.Project)
		return m, nil
	}

	// Recurrence and Description occupy dynamic positions: recur is present
	// only when t.Recur != "", shifting description one slot later.
	return m.handleDetailDynamicFields(id, t)
}

// handleDetailDynamicFields handles editing activation for the task fields
// whose index depends on whether the optional Recur field is present.
func (m *Model) handleDetailDynamicFields(id int, t *task.Task) (tea.Model, tea.Cmd) {
	// fieldEntry is 8; the next slot is 9, which holds Recur when present.
	fieldPos := fieldEntry + 1
	if t.Recur != "" {
		if m.detailFieldIndex == fieldPos {
			m.activateRecurEdit(id, t.Recur)
			return m, nil
		}
		fieldPos++
	}
	if m.detailFieldIndex == fieldPos {
		// Launch external editor for description editing.
		m.detailDescEditing = true
		return m, editDescriptionCmd(t.Description)
	}
	// Annotations are read-only in the detail view.  They can be edited via
	// the table view's Annotations column (activateAnnotationsEdit).
	return m, nil
}

// activatePriorityEdit enables the priority-selector for task id,
// pre-selecting the option that matches currentPriority.
func (m *Model) activatePriorityEdit(id int, currentPriority string) {
	m.clearEditingModes()
	m.priorityID = id
	m.prioritySelecting = true
	switch currentPriority {
	case "H":
		m.priorityIndex = 0
	case "M":
		m.priorityIndex = 1
	case "L":
		m.priorityIndex = 2
	default:
		m.priorityIndex = 3
	}
	m.updateTableHeight()
}

// activateDueEdit enables due-date editing for task id, initialising the
// date picker from currentDue (falls back to now if empty or unparseable).
func (m *Model) activateDueEdit(id int, currentDue string) {
	m.dueID = id
	if currentDue != "" {
		if ts, err := parseTaskDate(currentDue); err == nil {
			m.dueDate = ts
		} else {
			m.dueDate = time.Now()
		}
	} else {
		m.dueDate = time.Now()
	}
	m.clearEditingModes()
	m.dueEditing = true
	m.updateTableHeight()
}

// activateTagsEdit enables tags editing for task id with an empty input.
func (m *Model) activateTagsEdit(id int) {
	m.clearEditingModes()
	m.tagsID = id
	m.tagsEditing = true
	m.tagsInput.SetValue("")
	m.tagsInput.Focus()
	m.updateTableHeight()
}

// activateProjectEdit enables project editing for task id,
// pre-filling the input with currentProject.
func (m *Model) activateProjectEdit(id int, currentProject string) {
	m.clearEditingModes()
	m.projID = id
	m.projEditing = true
	m.projInput.SetValue(currentProject)
	m.projInput.Focus()
	m.updateTableHeight()
}

// activateRecurEdit enables recurrence editing for task id,
// pre-filling the input with currentRecur.
func (m *Model) activateRecurEdit(id int, currentRecur string) {
	m.clearEditingModes()
	m.recurID = id
	m.recurEditing = true
	m.recurInput.SetValue(currentRecur)
	m.recurInput.Focus()
	m.updateTableHeight()
}

// activateAnnotationsEdit enables annotation editing for task id.
// The current annotations are joined with "; " and pre-filled in the input
// so the user can revise all annotations in one pass.
func (m *Model) activateAnnotationsEdit(id int, tsk *task.Task) (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.annotateID = id
	m.annotating = true
	m.replaceAnnotations = true
	if tsk != nil {
		var anns []string
		for _, a := range tsk.Annotations {
			anns = append(anns, a.Description)
		}
		m.annotateInput.SetValue(strings.Join(anns, "; "))
	}
	m.annotateInput.Focus()
	m.updateTableHeight()
	return m, nil
}

// activateDescriptionEdit enables inline description editing for task id,
// pre-filling the input with the current description.
func (m *Model) activateDescriptionEdit(id int, tsk *task.Task) (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.descID = id
	m.descEditing = true
	if tsk != nil {
		m.descInput.SetValue(tsk.Description)
	}
	m.descInput.Focus()
	m.updateTableHeight()
	return m, nil
}
