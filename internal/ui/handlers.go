package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// handleTextInput provides generic text input handling for all input modes
func (m *Model) handleTextInput(msg tea.KeyPressMsg, input *textinput.Model, onEnter func(string) error, onExit func()) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		value := input.Value()
		if err := onEnter(value); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		input.Blur()
		onExit()
		m.updateTableHeight()
		return m, nil
	case "esc":
		input.Blur()
		onExit()
		m.updateTableHeight()
		return m, nil
	}
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	return m, cmd
}

// handleAnnotationMode handles keyboard input when in annotation mode
func (m *Model) handleAnnotationMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		// Annotation can be empty when replacing (to remove all)
		if !m.replaceAnnotations && strings.TrimSpace(value) == "" {
			return fmt.Errorf("annotation cannot be empty")
		}

		if m.replaceAnnotations {
			if err := task.ReplaceAnnotations(m.annotateID, value); err != nil {
				return err
			}
			m.replaceAnnotations = false
		} else {
			if err := task.Annotate(m.annotateID, value); err != nil {
				return err
			}
		}
		if err := m.reload(); err != nil {
			return fmt.Errorf("reloading tasks: %w", err)
		}
		return nil
	}

	onExit := func() {
		m.annotating = false
		m.replaceAnnotations = false
	}

	model, cmd := m.handleTextInput(msg, &m.annotateInput, onEnter, onExit)
	if msg.String() == "enter" && m.annotateInput.Value() != "" {
		// Start blink after successful annotation
		return model, m.startBlink(m.annotateID, false)
	}
	return model, cmd
}

// handleDescriptionMode handles keyboard input when editing description
func (m *Model) handleDescriptionMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		if err := validateDescription(value); err != nil {
			return err
		}
		if err := task.SetDescription(m.descID, value); err != nil {
			return err
		}
		if err := m.reload(); err != nil {
			return fmt.Errorf("reloading tasks: %w", err)
		}
		return nil
	}

	onExit := func() {
		m.descEditing = false
	}

	model, cmd := m.handleTextInput(msg, &m.descInput, onEnter, onExit)
	if msg.String() == "enter" {
		return model, m.startBlink(m.descID, false)
	}
	return model, cmd
}

// handleTagsMode handles keyboard input when editing tags
func (m *Model) handleTagsMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		words := strings.Fields(value)
		var adds, removes []string
		for _, w := range words {
			if strings.HasPrefix(w, "-") {
				if len(w) > 1 {
					tagName := w[1:]
					if err := validateTagName(tagName); err != nil {
						return fmt.Errorf("remove tag '%s': %w", tagName, err)
					}
					removes = append(removes, tagName)
				}
			} else {
				w = strings.TrimPrefix(w, "+")
				if w != "" {
					if err := validateTagName(w); err != nil {
						return fmt.Errorf("add tag '%s': %w", w, err)
					}
					adds = append(adds, w)
				}
			}
		}
		if len(adds) > 0 {
			if err := task.AddTags(m.tagsID, adds); err != nil {
				return err
			}
		}
		if len(removes) > 0 {
			if err := task.RemoveTags(m.tagsID, removes); err != nil {
				return err
			}
		}
		if err := m.reload(); err != nil {
			return fmt.Errorf("reloading tasks: %w", err)
		}
		return nil
	}

	onExit := func() {
		m.tagsEditing = false
	}

	model, cmd := m.handleTextInput(msg, &m.tagsInput, onEnter, onExit)
	if msg.String() == "enter" {
		if m.showTaskDetail {
			// In detail view, blink the tags field
			return model, m.startDetailBlink(4) // Tags is field index 4
		}
		return model, m.startBlink(m.tagsID, false)
	}
	return model, cmd
}

// handleDueEditMode handles due date editing
func (m *Model) handleDueEditMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if err := task.SetDueDate(m.dueID, m.dueDate.Format("2006-01-02")); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		m.dueEditing = false
		if !m.reloadAndReport() {
			return m, nil
		}
		var cmd tea.Cmd
		if m.showTaskDetail {
			// In detail view, blink the due field
			cmd = m.startDetailBlink(5) // Due is field index 5
		} else {
			cmd = m.startBlink(m.dueID, false)
		}
		m.updateTableHeight()
		return m, cmd
	case "esc":
		m.dueEditing = false
		m.updateTableHeight()
		return m, nil
	}

	switch msg.String() {
	case "h", "left":
		m.dueDate = m.dueDate.AddDate(0, 0, -1)
	case "l", "right":
		m.dueDate = m.dueDate.AddDate(0, 0, 1)
	case "k", "up":
		m.dueDate = m.dueDate.AddDate(0, 0, -7)
	case "j", "down":
		m.dueDate = m.dueDate.AddDate(0, 0, 7)
	}
	return m, nil
}

// handleRecurrenceMode handles recurrence editing
func (m *Model) handleRecurrenceMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		if err := validateRecurrence(value); err != nil {
			return err
		}
		if err := task.SetRecurrence(m.recurID, value); err != nil {
			return err
		}
		if err := m.reload(); err != nil {
			return fmt.Errorf("reloading tasks: %w", err)
		}
		return nil
	}

	onExit := func() {
		m.recurEditing = false
	}

	model, cmd := m.handleTextInput(msg, &m.recurInput, onEnter, onExit)
	if msg.String() == "enter" {
		if m.showTaskDetail {
			// In detail view, blink the recurrence field (dynamic index)
			// Need to calculate the index based on whether recurrence field exists
			fieldIndex := 8 // Base index for recurrence
			if m.currentTaskDetail != nil && m.currentTaskDetail.Recur != "" {
				return model, m.startDetailBlink(fieldIndex)
			}
		}
		return model, m.startBlink(m.recurID, false)
	}
	return model, cmd
}

// handleProjectMode handles project editing
func (m *Model) handleProjectMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		return task.SetProject(m.projID, value)
	}

	onExit := func() {
		m.projEditing = false
		m.reloadAndReport()
	}

	model, cmd := m.handleTextInput(msg, &m.projInput, onEnter, onExit)
	if msg.String() == "enter" {
		if m.showTaskDetail {
			// In detail view, blink the project field
			return model, m.startDetailBlink(fieldProject) // Project field index in detail view
		}
		return model, m.startBlink(m.projID, false)
	}
	return model, cmd
}

// handlePriorityMode handles priority selection
func (m *Model) handlePriorityMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		priority := priorityOptions[m.priorityIndex]
		if err := validatePriority(priority); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		if err := task.SetPriority(m.priorityID, priority); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		m.prioritySelecting = false
		if !m.reloadAndReport() {
			return m, nil
		}
		var cmd tea.Cmd
		if m.showTaskDetail {
			// In detail view, blink the priority field
			cmd = m.startDetailBlink(3) // Priority is field index 3
		} else {
			cmd = m.startBlink(m.priorityID, false)
		}
		m.updateTableHeight()
		return m, cmd
	case "esc":
		m.prioritySelecting = false
		m.updateTableHeight()
		return m, nil
	}

	switch msg.String() {
	case "h", "left":
		m.priorityIndex = (m.priorityIndex + len(priorityOptions) - 1) % len(priorityOptions)
	case "l", "right":
		m.priorityIndex = (m.priorityIndex + 1) % len(priorityOptions)
	}
	return m, nil
}

// handleFilterMode handles filter editing for both traditional and ultra mode.
// The filter value is split using shell-quoting rules (via parseFilterInput)
// so that expressions with quoted values (e.g. description:"my task") are
// passed to taskwarrior as a single argument. Any taskwarrior filter expression
// that is valid on the command line (proj:xxx, +tag, description:"...", etc.)
// is therefore accepted here too. Taskwarrior errors are propagated back to
// the user via the status bar rather than being silently discarded.
func (m *Model) handleFilterMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		fields, err := parseFilterInput(value)
		if err != nil {
			return err
		}
		m.filters = fields
		// Propagate taskwarrior errors so the user sees feedback when a
		// filter expression is rejected by taskwarrior.
		if err := m.reload(); err != nil {
			// Roll back the filters to avoid leaving the UI in a broken state
			// where an empty task list is shown without any explanation.
			m.filters = nil
			return fmt.Errorf("filter error: %w", err)
		}
		return nil
	}

	onExit := func() {
		m.filterEditing = false
	}

	return m.handleTextInput(msg, &m.filterInput, onEnter, onExit)
}

// handleAddTaskMode handles adding a new task
func (m *Model) handleAddTaskMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		oldIDs := make(map[int]struct{}, len(m.tasks))
		for _, tsk := range m.tasks {
			oldIDs[tsk.ID] = struct{}{}
		}

		if err := task.AddLine(m.addInput.Value()); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}

		m.addingTask = false
		m.addInput.Blur()
		if !m.reloadAndReport() {
			return m, nil
		}

		// Find the newly added task
		var newID int
		row := -1
		for i, tsk := range m.tasks {
			if _, ok := oldIDs[tsk.ID]; !ok {
				newID = tsk.ID
				row = i
				break
			}
		}

		m.updateTableHeight()
		if row >= 0 {
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
		return m, nil

	case "esc":
		m.addingTask = false
		m.addInput.Blur()
		m.updateTableHeight()
		return m, nil
	}

	var cmd tea.Cmd
	m.addInput, cmd = m.addInput.Update(msg)
	return m, cmd
}

// handleSearchMode handles search input
func (m *Model) handleSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := m.searchInput.Value()
		if pattern != "" {
			// Check cache first
			if cached, ok := cachedSearchRegex(pattern); ok {
				m.searchRegex = cached
			} else {
				// Compile and cache if not found
				re, err := compileAndCacheRegex(pattern)
				if err == nil {
					m.searchRegex = re
				} else {
					m.searchRegex = nil
					m.statusMsg = fmt.Sprintf("Invalid regex: %v", err)
				}
			}
		} else {
			m.searchRegex = nil
		}
		m.searching = false
		m.searchInput.Blur()
		if !m.reloadAndReport() {
			return m, nil
		}
		m.updateTableHeight()

		if len(m.searchMatches) > 0 {
			match := m.searchMatches[m.searchIndex]
			prevRow := m.tbl.Cursor()
			prevCol := m.tbl.ColumnCursor()
			m.tbl.SetCursor(match.row)
			m.tbl.SetColumnCursor(match.col)
			m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
		}
		return m, nil

	case "esc":
		m.searching = false
		m.searchInput.Blur()
		m.updateTableHeight()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// handleHelpSearchMode handles search input in help mode
func (m *Model) handleHelpSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := m.helpSearchInput.Value()
		if pattern != "" {
			// Check cache first
			if cached, ok := cachedSearchRegex(pattern); ok {
				m.helpSearchRegex = cached
			} else {
				// Compile and cache if not found
				re, err := compileAndCacheRegex(pattern)
				if err == nil {
					m.helpSearchRegex = re
				} else {
					m.helpSearchRegex = nil
					m.statusMsg = fmt.Sprintf("Invalid regex: %v", err)
				}
			}
		} else {
			m.helpSearchRegex = nil
		}
		m.helpSearching = false
		m.helpSearchInput.Blur()

		// Find matching help lines
		m.helpSearchMatches = nil
		if m.helpSearchRegex != nil {
			helpLines := m.getHelpLines()
			for i, line := range helpLines {
				if m.helpSearchRegex.MatchString(line) {
					m.helpSearchMatches = append(m.helpSearchMatches, i)
				}
			}
			// Set to first match
			if len(m.helpSearchMatches) > 0 {
				m.helpSearchIndex = 0
			}
		}
		return m, nil

	case "esc":
		m.helpSearching = false
		m.helpSearchInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.helpSearchInput, cmd = m.helpSearchInput.Update(msg)
	return m, cmd
}
