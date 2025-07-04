package ui

import (
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// handleNormalMode handles keyboard input in normal mode (not editing)
func (m *Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If help is shown, handle special cases
	if m.showHelp {
		switch msg.String() {
		case "H", "esc", "q":
			return m.handleQuitOrEscape()
		case "/", "?":
			return m.handleHelpSearch()
		case "n":
			return m.handleNextHelpSearchMatch()
		case "N":
			return m.handlePrevHelpSearchMatch()
		case "up", "k":
			m.helpViewport.LineUp(1)
			return m, nil
		case "down", "j":
			m.helpViewport.LineDown(1)
			return m, nil
		case "pgup", "b":
			m.helpViewport.ViewUp()
			return m, nil
		case "pgdown", " ":
			m.helpViewport.ViewDown()
			return m, nil
		case "g", "home":
			m.helpViewport.GotoTop()
			return m, nil
		case "G", "end":
			m.helpViewport.GotoBottom()
			return m, nil
		default:
			// Ignore other keys in help mode
			return m, nil
		}
	}
	
	switch msg.String() {
	case "H":
		return m.handleToggleHelp()
	case "q", "esc":
		return m.handleQuitOrEscape()
	case "e", "E":
		return m.handleEditTask()
	case "s":
		return m.handleToggleStart()
	case "d":
		return m.handleMarkDone()
	case "o":
		return m.handleOpenURL()
	case "U":
		return m.handleUndo()
	case "w":
		return m.handleSetDueDate()
	case "W":
		return m.handleRemoveDueDate()
	case "r":
		return m.handleRandomDueDate()
	case "R":
		return m.handleSetRecurrence()
	case "p":
		return m.handleSetPriority()
	case "a":
		return m.handleAnnotate(false)
	case "A":
		return m.handleAnnotate(true)
	case "f":
		return m.handleFilter()
	case "+":
		return m.handleAddTask()
	case "t":
		return m.handleEditTags()
	case "J":
		return m.handleEditProject()
	case "T":
		return m.handleTagToProject()
	case "c":
		return m.handleRandomTheme()
	case "C":
		return m.handleResetTheme()
	case "x":
		return m.handleToggleDisco()
	case " ":
		return m.handleRefresh()
	case "/", "?":
		return m.handleSearch()
	case "n":
		return m.handleNextSearchMatch()
	case "N":
		return m.handlePrevSearchMatch()
	case "enter":
		return m.handleShowTaskDetail()
	case "i":
		return m.handleEnterOrEdit()
	case "1":
		return m.handleJumpToRandomTask()
	case "2":
		return m.handleJumpToRandomTaskNoDue()
	default:
		// Pass through to table for navigation
		return m.handleTableNavigation(msg)
	}
}

func (m *Model) handleToggleHelp() (tea.Model, tea.Cmd) {
	m.showHelp = true
	// Initialize help viewport with proper dimensions
	width := m.tbl.Width() - 4 // Account for padding
	height := m.windowHeight - 6 // Leave room for status bars and search input
	if width <= 0 {
		width = 80 // Default width
	}
	if height <= 0 {
		height = 20 // Default height
	}
	m.helpViewport = viewport.New(width, height)
	// Set the content immediately
	content := m.buildHelpContent()
	m.helpViewport.SetContent(content)
	return m, nil
}

func (m *Model) handleQuitOrEscape() (tea.Model, tea.Cmd) {
	if m.showTaskDetail {
		m.showTaskDetail = false
		m.currentTaskDetail = nil
		m.detailSearching = false
		m.detailSearchRegex = nil
		m.detailSearchInput.SetValue("")
		return m, nil
	}
	if m.cellExpanded {
		m.cellExpanded = false
		m.updateTableHeight()
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		// Clear help search state
		m.helpSearchRegex = nil
		m.helpSearchMatches = nil
		m.helpSearchIndex = 0
		m.helpSearchInput.SetValue("")
		// Reset help viewport
		m.helpViewport = viewport.Model{}
		return m, nil
	}
	if m.searchRegex != nil {
		m.searchRegex = nil
		m.searchMatches = nil
		m.searchIndex = 0
		m.reload()
		return m, nil
	}
	return m, tea.Quit
}

func (m *Model) handleEditTask() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	m.editID = id
	return m, editCmd(id)
}

func (m *Model) handleToggleStart() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	// Check if task is started
	started := false
	for _, tsk := range m.tasks {
		if tsk.ID == id {
			started = tsk.Start != ""
			break
		}
	}
	
	if started {
		if err := task.Stop(id); err != nil {
			m.showError(err)
			return m, nil
		}
	} else {
		if err := task.Start(id); err != nil {
			m.showError(err)
			return m, nil
		}
	}
	
	m.reload()
	return m, m.startBlink(id, false)
}

func (m *Model) handleMarkDone() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	return m, m.startBlink(id, true)
}

func (m *Model) handleOpenURL() (tea.Model, tea.Cmd) {
	task := m.getTaskAtCursor()
	if task == nil {
		return m, nil
	}
	
	url := urlRegex.FindString(task.Description)
	if url == "" {
		return m, nil
	}
	
	if err := exec.Command(m.browserCmd, url).Run(); err != nil {
		m.showError(fmt.Errorf("opening browser: %w", err))
		return m, nil
	}
	
	return m, m.startBlink(task.ID, false)
}

func (m *Model) handleUndo() (tea.Model, tea.Cmd) {
	if len(m.undoStack) == 0 {
		return m, nil
	}
	
	uuid := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	
	if err := task.SetStatusUUID(uuid, "pending"); err != nil {
		m.showError(err)
		return m, nil
	}
	
	// Reload the task list to get the updated task with its new ID
	if err := m.reload(); err != nil {
		m.showError(err)
		return m, nil
	}
	
	// Find the task ID for blinking
	var id int
	var found bool
	for _, tsk := range m.tasks {
		if tsk.UUID == uuid {
			id = tsk.ID
			found = true
			break
		}
	}
	
	// If task not found or has ID 0, try to get it directly from Taskwarrior
	if !found || id == 0 {
		// Use task export with UUID filter to get the specific task
		filters := []string{uuid}
		if m.filters != nil {
			filters = append(filters, m.filters...)
		}
		filters = append(filters, "status:pending")
		
		tasks, err := task.Export(filters...)
		if err == nil && len(tasks) > 0 {
			id = tasks[0].ID
			// Also update our local task list
			for i, tsk := range m.tasks {
				if tsk.UUID == uuid {
					m.tasks[i].ID = id
					break
				}
			}
		}
	}
	
	// If we still don't have a valid ID, don't try to blink
	if id == 0 {
		m.statusMsg = "Task restored"
		return m, nil
	}
	
	return m, m.startBlink(id, false)
}

func (m *Model) handleSetDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.dueID = id
	m.dueEditing = true
	m.dueDate = time.Now()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleRemoveDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	// In Taskwarrior, passing an empty value to due: removes the due date
	if err := task.SetDueDate(id, ""); err != nil {
		m.showError(err)
		return m, nil
	}
	
	m.reload()
	return m, m.startBlink(id, false)
}

func (m *Model) handleRandomDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	days := rng.Intn(31) + 7
	due := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	
	if err := task.SetDueDate(id, due); err != nil {
		m.showError(err)
		return m, nil
	}
	
	m.reload()
	return m, m.startBlink(id, false)
}

func (m *Model) handleSetRecurrence() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	task := m.getTaskAtCursor()
	if task == nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.recurID = id
	m.recurEditing = true
	m.recurInput.SetValue(task.Recur)
	m.recurInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleSetPriority() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.priorityID = id
	m.prioritySelecting = true
	m.priorityIndex = 0
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleAnnotate(replace bool) (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.annotateID = id
	m.annotating = true
	m.replaceAnnotations = replace
	m.annotateInput.SetValue("")
	m.annotateInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleFilter() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.filterEditing = true
	m.filterInput.SetValue(strings.Join(m.filters, " "))
	m.filterInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleAddTask() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.addingTask = true
	m.addInput.SetValue("")
	m.addInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleEditTags() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.tagsID = id
	m.tagsEditing = true
	m.tagsInput.SetValue("")
	m.tagsInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleEditProject() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	m.clearEditingModes()
	m.projID = id
	m.projEditing = true
	
	// Get current project value
	task := m.getTaskAtCursor()
	if task != nil {
		m.projInput.SetValue(task.Project)
	} else {
		m.projInput.SetValue("")
	}
	m.projInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleTagToProject() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	// Get the task at cursor
	currentTask := m.getTaskAtCursor()
	if currentTask == nil || len(currentTask.Tags) == 0 {
		// No tags to convert
		return m, nil
	}
	
	// Get the first tag
	firstTag := currentTask.Tags[0]
	
	// Set the tag as project
	if err := task.SetProject(id, firstTag); err != nil {
		m.showError(err)
		return m, nil
	}
	
	// Remove the tag from the task
	if err := task.RemoveTags(id, []string{firstTag}); err != nil {
		m.showError(err)
		return m, nil
	}
	
	m.reload()
	return m, m.startBlink(id, false)
}

func (m *Model) handleRandomTheme() (tea.Model, tea.Cmd) {
	m.theme = RandomTheme()
	m.applyTheme()
	return m, nil
}

func (m *Model) handleResetTheme() (tea.Model, tea.Cmd) {
	m.theme = m.defaultTheme
	m.applyTheme()
	return m, nil
}

func (m *Model) handleToggleDisco() (tea.Model, tea.Cmd) {
	m.disco = !m.disco
	return m, nil
}

func (m *Model) handleRefresh() (tea.Model, tea.Cmd) {
	m.reload()
	return m, nil
}

func (m *Model) handleSearch() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.searching = true
	m.searchIndex = 0
	m.searchMatches = nil
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleNextSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.searchMatches) == 0 {
		return m, nil
	}
	
	m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
	match := m.searchMatches[m.searchIndex]
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(match.row)
	m.tbl.SetColumnCursor(match.col)
	m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	return m, nil
}

func (m *Model) handlePrevSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.searchMatches) == 0 {
		return m, nil
	}
	
	m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	match := m.searchMatches[m.searchIndex]
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(match.row)
	m.tbl.SetColumnCursor(match.col)
	m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	return m, nil
}

func (m *Model) handleHelpSearch() (tea.Model, tea.Cmd) {
	m.helpSearching = true
	m.helpSearchIndex = 0
	m.helpSearchMatches = nil
	m.helpSearchInput.SetValue("")
	m.helpSearchInput.Focus()
	return m, nil
}

func (m *Model) handleNextHelpSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.helpSearchMatches) == 0 {
		return m, nil
	}
	
	m.helpSearchIndex = (m.helpSearchIndex + 1) % len(m.helpSearchMatches)
	// In the future, we could add visual indication of current match
	return m, nil
}

func (m *Model) handlePrevHelpSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.helpSearchMatches) == 0 {
		return m, nil
	}
	
	m.helpSearchIndex = (m.helpSearchIndex - 1 + len(m.helpSearchMatches)) % len(m.helpSearchMatches)
	// In the future, we could add visual indication of current match
	return m, nil
}

func (m *Model) handleShowTaskDetail() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	
	// Find the task with this ID
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			m.showTaskDetail = true
			m.currentTaskDetail = &m.tasks[i]
			m.detailSearching = false
			m.detailSearchRegex = nil
			m.detailFieldIndex = 0
			m.detailBlinkField = -1
			m.detailBlinkOn = false
			m.detailBlinkCount = 0
			m.detailSearchInput = textinput.New()
			m.detailSearchInput.Placeholder = "Search..."
			m.detailSearchInput.Width = 30
			break
		}
	}
	
	return m, nil
}

func (m *Model) handleEnterOrEdit() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		// No task selected, toggle cell expansion
		m.cellExpanded = !m.cellExpanded
		m.updateTableHeight()
		return m, nil
	}
	
	col := m.tbl.ColumnCursor()
	switch col {
	case 0: // Priority
		m.clearEditingModes()
		m.priorityID = id
		m.prioritySelecting = true
		
		// Set current priority index
		task := m.getTaskAtCursor()
		if task != nil {
			switch task.Priority {
			case "H":
				m.priorityIndex = 0
			case "M":
				m.priorityIndex = 1
			case "L":
				m.priorityIndex = 2
			default:
				m.priorityIndex = 3
			}
		}
		m.updateTableHeight()
		return m, nil
		
	case 3: // Due date
		m.dueID = id
		task := m.getTaskAtCursor()
		if task != nil && task.Due != "" {
			if ts, err := parseTaskDate(task.Due); err == nil {
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
		return m, nil
		
	case 4: // Recurrence
		m.clearEditingModes()
		m.recurID = id
		m.recurEditing = true
		task := m.getTaskAtCursor()
		if task != nil {
			m.recurInput.SetValue(task.Recur)
		}
		m.recurInput.Focus()
		m.updateTableHeight()
		return m, nil
		
	case 5: // Project
		m.clearEditingModes()
		m.projID = id
		m.projEditing = true
		task := m.getTaskAtCursor()
		if task != nil {
			m.projInput.SetValue(task.Project)
		}
		m.projInput.Focus()
		m.updateTableHeight()
		return m, nil
		
	case 6: // Tags
		m.clearEditingModes()
		m.tagsID = id
		m.tagsEditing = true
		m.tagsInput.SetValue("")
		m.tagsInput.Focus()
		m.updateTableHeight()
		return m, nil
		
	case 7: // Annotations
		m.clearEditingModes()
		m.annotateID = id
		m.annotating = true
		m.replaceAnnotations = true
		
		// Get current annotations
		task := m.getTaskAtCursor()
		if task != nil {
			var anns []string
			for _, a := range task.Annotations {
				anns = append(anns, a.Description)
			}
			m.annotateInput.SetValue(strings.Join(anns, "; "))
		}
		m.annotateInput.Focus()
		m.updateTableHeight()
		return m, nil
		
	case 8: // Description
		m.clearEditingModes()
		m.descID = id
		m.descEditing = true
		task := m.getTaskAtCursor()
		if task != nil {
			m.descInput.SetValue(task.Description)
		}
		m.descInput.Focus()
		m.updateTableHeight()
		return m, nil
		
	default:
		// Toggle cell expansion for other columns
		m.cellExpanded = !m.cellExpanded
		m.updateTableHeight()
		return m, nil
	}
}

func (m *Model) handleTableNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	if prevRow != m.tbl.Cursor() || prevCol != m.tbl.ColumnCursor() {
		m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	}
	return m, cmd
}

// showError displays an error in the status bar
func (m *Model) showError(err error) {
	m.statusMsg = fmt.Sprintf("Error: %v", err)
	// Note: we can't return a Cmd from here, so the error will stay until next update
}

// handleJumpToRandomTask jumps to a random pending task
func (m *Model) handleJumpToRandomTask() (tea.Model, tea.Cmd) {
	if len(m.tasks) == 0 {
		m.statusMsg = "No tasks to jump to"
		return m, nil
	}
	
	// Pick a random index
	randomIndex := rand.Intn(len(m.tasks))
	
	// Update cursor position
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(randomIndex)
	m.updateSelectionHighlight(prevRow, randomIndex, prevCol, m.tbl.ColumnCursor())
	
	// Blink the task to indicate jump
	if randomIndex < len(m.tasks) {
		taskID := m.tasks[randomIndex].ID
		return m, m.startBlink(taskID, false)
	}
	
	return m, nil
}

// handleJumpToRandomTaskNoDue jumps to a random pending task without a due date
func (m *Model) handleJumpToRandomTaskNoDue() (tea.Model, tea.Cmd) {
	// Find all tasks without due dates
	var noDueTasks []int
	for i, task := range m.tasks {
		if task.Due == "" {
			noDueTasks = append(noDueTasks, i)
		}
	}
	
	if len(noDueTasks) == 0 {
		m.statusMsg = "No tasks without due date to jump to"
		return m, nil
	}
	
	// Pick a random task from the no-due list
	randomChoice := rand.Intn(len(noDueTasks))
	randomIndex := noDueTasks[randomChoice]
	
	// Update cursor position
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(randomIndex)
	m.updateSelectionHighlight(prevRow, randomIndex, prevCol, m.tbl.ColumnCursor())
	
	// Blink the task to indicate jump
	if randomIndex < len(m.tasks) {
		taskID := m.tasks[randomIndex].ID
		return m, m.startBlink(taskID, false)
	}
	
	return m, nil
}