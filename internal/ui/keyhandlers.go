package ui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

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

	switch msg.String() {
	case "H":
		return m.handleToggleHelp()
	case "q":
		return m.handleQuitKey()
	case "esc":
		return m.handleEscapeKey()
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
	case "B":
		return m.handleToggleBlink()
	case "space":
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
		// Active search: q clears the search filter first, same as in normal
		// table mode. Only proceed to exit/quit when no search is active.
		if m.ultraSearchRegex != nil {
			m.ultraSearchRegex = nil
			m.ultraFiltered = nil
			m.ultraCursor = 0
			m.ultraOffset = 0
			return m, nil
		}
		// When started via --ultra flag there is no table view to return to,
		// so q exits the application directly.
		if m.ultraStartup {
			return m, tea.Quit
		}
		m.ultraClearFocusedID()
		m.showUltra = false
		m.ultraSearchInput.SetValue("")
		return m, nil
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
		// Active search: esc clears the search filter first, same as in
		// normal table mode. It never quits the application.
		if m.ultraSearchRegex != nil {
			m.ultraSearchRegex = nil
			m.ultraFiltered = nil
			m.ultraCursor = 0
			m.ultraOffset = 0
			return m, nil
		}
		// When started via --ultra flag there is no table view to return to,
		// so esc just stays in ultra mode.
		if m.ultraStartup {
			return m, nil
		}
		m.ultraClearFocusedID()
		m.showUltra = false
		m.ultraSearchInput.SetValue("")
		return m, nil
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
