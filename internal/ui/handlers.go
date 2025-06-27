package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// handleTextInput provides generic text input handling for all input modes
func (m *Model) handleTextInput(msg tea.KeyMsg, input *textinput.Model, onEnter func(string) error, onExit func()) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		value := input.Value()
		if err := onEnter(value); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		input.Blur()
		onExit()
		m.updateTableHeight()
		return m, nil
	case tea.KeyEsc:
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
func (m *Model) handleAnnotationMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.reload()
		return nil
	}
	
	onExit := func() {
		m.annotating = false
		m.replaceAnnotations = false
	}
	
	model, cmd := m.handleTextInput(msg, &m.annotateInput, onEnter, onExit)
	if msg.Type == tea.KeyEnter && m.annotateInput.Value() != "" {
		// Start blink after successful annotation
		return model, m.startBlink(m.annotateID, false)
	}
	return model, cmd
}

// handleDescriptionMode handles keyboard input when editing description
func (m *Model) handleDescriptionMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		if err := validateDescription(value); err != nil {
			return err
		}
		if err := task.SetDescription(m.descID, value); err != nil {
			return err
		}
		m.reload()
		return nil
	}
	
	onExit := func() {
		m.descEditing = false
	}
	
	model, cmd := m.handleTextInput(msg, &m.descInput, onEnter, onExit)
	if msg.Type == tea.KeyEnter {
		return model, m.startBlink(m.descID, false)
	}
	return model, cmd
}

// handleTagsMode handles keyboard input when editing tags
func (m *Model) handleTagsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
				if strings.HasPrefix(w, "+") {
					w = w[1:]
				}
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
		m.reload()
		return nil
	}
	
	onExit := func() {
		m.tagsEditing = false
	}
	
	model, cmd := m.handleTextInput(msg, &m.tagsInput, onEnter, onExit)
	if msg.Type == tea.KeyEnter {
		return model, m.startBlink(m.tagsID, false)
	}
	return model, cmd
}

// handleDueEditMode handles due date editing
func (m *Model) handleDueEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if err := task.SetDueDate(m.dueID, m.dueDate.Format("2006-01-02")); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		m.dueEditing = false
		m.reload()
		cmd := m.startBlink(m.dueID, false)
		m.updateTableHeight()
		return m, cmd
	case tea.KeyEsc:
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
func (m *Model) handleRecurrenceMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		if err := validateRecurrence(value); err != nil {
			return err
		}
		if err := task.SetRecurrence(m.recurID, value); err != nil {
			return err
		}
		m.reload()
		return nil
	}
	
	onExit := func() {
		m.recurEditing = false
	}
	
	model, cmd := m.handleTextInput(msg, &m.recurInput, onEnter, onExit)
	if msg.Type == tea.KeyEnter {
		return model, m.startBlink(m.recurID, false)
	}
	return model, cmd
}

// handlePriorityMode handles priority selection
func (m *Model) handlePriorityMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		priority := priorityOptions[m.priorityIndex]
		if err := validatePriority(priority); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		if err := task.SetPriority(m.priorityID, priority); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		m.prioritySelecting = false
		m.reload()
		cmd := m.startBlink(m.priorityID, false)
		m.updateTableHeight()
		return m, cmd
	case tea.KeyEsc:
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

// handleFilterMode handles filter editing
func (m *Model) handleFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onEnter := func(value string) error {
		m.filters = strings.Fields(value)
		m.reload()
		return nil
	}
	
	onExit := func() {
		m.filterEditing = false
	}
	
	return m.handleTextInput(msg, &m.filterInput, onEnter, onExit)
}

// handleAddTaskMode handles adding a new task
func (m *Model) handleAddTaskMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		oldIDs := make(map[int]struct{})
		for _, tsk := range m.tasks {
			oldIDs[tsk.ID] = struct{}{}
		}
		
		if err := task.AddLine(m.addInput.Value()); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			cmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}
		
		m.addingTask = false
		m.addInput.Blur()
		m.reload()
		
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
			return m, m.startBlink(newID, false)
		}
		return m, nil
		
	case tea.KeyEsc:
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
func (m *Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		pattern := m.searchInput.Value()
		if pattern != "" {
			// Check cache first
			if cached, ok := searchRegexCache[pattern]; ok {
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
		m.reload()
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
		
	case tea.KeyEsc:
		m.searching = false
		m.searchInput.Blur()
		m.updateTableHeight()
		return m, nil
	}
	
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// handleBlinkingState handles input when a task is blinking
func (m *Model) handleBlinkingState(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		// Only allow navigation while blinking
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

// handleEditingModes checks if we're in any editing mode and handles it
func (m *Model) handleEditingModes(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
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