package ui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type sharedKeyHandlers struct {
	editTask      func() (tea.Model, tea.Cmd)
	toggleStart   func() (tea.Model, tea.Cmd)
	markDone      func() (tea.Model, tea.Cmd)
	deleteTask    func() (tea.Model, tea.Cmd)
	setPriority   func() (tea.Model, tea.Cmd)
	setDueDate    func() (tea.Model, tea.Cmd)
	removeDueDate func() (tea.Model, tea.Cmd)
	editTags      func() (tea.Model, tea.Cmd)
	annotate      func(replace bool) (tea.Model, tea.Cmd)
	editProject   func() (tea.Model, tea.Cmd)
	setRecurrence func() (tea.Model, tea.Cmd)
	addTask       func() (tea.Model, tea.Cmd)
}

// handleNormalMode handles keyboard input in normal mode (not editing)
func (m *Model) handleNormalMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// If help is shown, handle special cases
	if m.showHelp {
		switch msg.String() {
		case "H", "q":
			return m.handleQuitKey()
		case "esc":
			return m.handleEscapeKey()
		case "/", "?":
			return m.handleHelpSearch()
		case "n":
			return m.handleNextHelpSearchMatch()
		case "N":
			return m.handlePrevHelpSearchMatch()
		case "up", "k":
			m.helpViewport.ScrollUp(1)
			return m, nil
		case "down", "j":
			m.helpViewport.ScrollDown(1)
			return m, nil
		case "pgup", "b":
			m.helpViewport.PageUp()
			return m, nil
		case "pgdown", "space":
			m.helpViewport.PageDown()
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

	if handled, model, cmd := m.handleSharedKey(msg.String(), m.normalSharedKeyHandlers()); handled {
		return model, cmd
	}

	switch msg.String() {
	case "T":
		return m.handleTagToProject()
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
	case "u":
		m.ultraClearFocusedID()
		m.showUltra = true
		m.ultraCursor = m.tbl.Cursor()
		m.ultraOffset = 0
		m.ultraEnsureVisible()
		return m, nil
	case "1":
		return m.handleJumpToRandomTask()
	case "2":
		return m.handleJumpToRandomTaskNoDue()
	default:
		// Pass through to table for navigation
		return m.handleTableNavigation(msg)
	}
}

func (m *Model) normalSharedKeyHandlers() sharedKeyHandlers {
	return sharedKeyHandlers{
		editTask:      m.handleEditTask,
		toggleStart:   m.handleToggleStart,
		markDone:      m.handleMarkDone,
		deleteTask:    m.handleDeleteTask,
		setPriority:   m.handleSetPriority,
		setDueDate:    m.handleSetDueDate,
		removeDueDate: m.handleRemoveDueDate,
		editTags:      m.handleEditTags,
		annotate:      m.handleAnnotate,
		editProject:   m.handleEditProject,
		setRecurrence: m.handleSetRecurrence,
		addTask:       m.handleAddTask,
	}
}

func (m *Model) ultraSharedKeyHandlers() sharedKeyHandlers {
	return sharedKeyHandlers{
		editTask:      m.handleUltraEditTask,
		toggleStart:   m.handleUltraToggleStart,
		markDone:      m.handleUltraMarkDone,
		deleteTask:    m.handleUltraDeleteTask,
		setPriority:   m.handleUltraSetPriority,
		setDueDate:    m.handleUltraSetDueDate,
		removeDueDate: m.handleUltraRemoveDueDate,
		editTags:      m.handleUltraEditTags,
		annotate:      m.handleUltraAnnotate,
		editProject:   m.handleUltraEditProject,
		setRecurrence: m.handleUltraSetRecurrence,
		addTask: func() (tea.Model, tea.Cmd) {
			m.ultraClearFocusedID()
			return m.handleAddTask()
		},
	}
}

func (m *Model) handleSharedKey(key string, handlers sharedKeyHandlers) (bool, tea.Model, tea.Cmd) {
	if key == m.agentFilterHotkeyLabel() {
		model, cmd := m.handleToggleAgentFilter()
		return true, model, cmd
	}

	switch key {
	case "H":
		model, cmd := m.handleToggleHelp()
		return true, model, cmd
	case "q":
		model, cmd := m.handleQuitKey()
		return true, model, cmd
	case "esc":
		model, cmd := m.handleEscapeKey()
		return true, model, cmd
	case "e", "E":
		return callSharedKeyHandler(m, handlers.editTask)
	case "s":
		return callSharedKeyHandler(m, handlers.toggleStart)
	case "d":
		return callSharedKeyHandler(m, handlers.markDone)
	case "D":
		return callSharedKeyHandler(m, handlers.deleteTask)
	case "o":
		model, cmd := m.handleOpenURL()
		return true, model, cmd
	case "U":
		model, cmd := m.handleUndo()
		return true, model, cmd
	case "w":
		return callSharedKeyHandler(m, handlers.setDueDate)
	case "W":
		return callSharedKeyHandler(m, handlers.removeDueDate)
	case "r":
		model, cmd := m.handleRandomDueDate()
		return true, model, cmd
	case "R":
		return callSharedKeyHandler(m, handlers.setRecurrence)
	case "p":
		return callSharedKeyHandler(m, handlers.setPriority)
	case "a":
		return callSharedAnnotateHandler(m, handlers.annotate, false)
	case "A":
		return callSharedAnnotateHandler(m, handlers.annotate, true)
	case "f":
		model, cmd := m.handleFilter()
		return true, model, cmd
	case ":":
		model, cmd := m.handleShellPrompt()
		return true, model, cmd
	case ";":
		model, cmd := m.handleShellPromptForSelectedTask()
		return true, model, cmd
	case "+":
		return callSharedKeyHandler(m, handlers.addTask)
	case "t":
		return callSharedKeyHandler(m, handlers.editTags)
	case "J":
		return callSharedKeyHandler(m, handlers.editProject)
	case "c":
		model, cmd := m.handleRandomTheme()
		return true, model, cmd
	case "C":
		model, cmd := m.handleResetTheme()
		return true, model, cmd
	case "x":
		model, cmd := m.handleToggleDisco()
		return true, model, cmd
	case "B":
		model, cmd := m.handleToggleBlink()
		return true, model, cmd
	case "space":
		model, cmd := m.handleRefresh()
		return true, model, cmd
	default:
		return false, m, nil
	}
}

func callSharedKeyHandler(m *Model, handler func() (tea.Model, tea.Cmd)) (bool, tea.Model, tea.Cmd) {
	if handler == nil {
		return false, m, nil
	}
	model, cmd := handler()
	return true, model, cmd
}

func callSharedAnnotateHandler(m *Model, handler func(bool) (tea.Model, tea.Cmd), replace bool) (bool, tea.Model, tea.Cmd) {
	if handler == nil {
		return false, m, nil
	}
	model, cmd := handler(replace)
	return true, model, cmd
}

func (m *Model) handleToggleHelp() (tea.Model, tea.Cmd) {
	m.showHelp = true
	// Initialize help viewport with proper dimensions
	width := m.tbl.Width() - 4   // Account for padding
	height := m.windowHeight - 6 // Leave room for status bars and search input
	if width <= 0 {
		width = 80 // Default width
	}
	if height <= 0 {
		height = 20 // Default height
	}
	m.helpViewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	// Set the content immediately
	m.helpViewport.SetContent(m.activeHelpContent())
	return m, nil
}

func (m *Model) handleQuitKey() (tea.Model, tea.Cmd) {
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
	if m.showUltra {
		return m.handleUltraExitKey(true)
	}
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
	if m.searchRegex != nil {
		m.searchRegex = nil
		m.searchMatches = nil
		m.searchIndex = 0
		m.reloadAndReport()
		return m, nil
	}
	m.cancelTaskOperations()
	return m, tea.Quit
}

func (m *Model) handleEscapeKey() (tea.Model, tea.Cmd) {
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
	if m.showUltra {
		return m.handleUltraExitKey(false)
	}
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
	if m.searchRegex != nil {
		m.searchRegex = nil
		m.searchMatches = nil
		m.searchIndex = 0
		m.reloadAndReport()
		return m, nil
	}
	return m, nil
}
