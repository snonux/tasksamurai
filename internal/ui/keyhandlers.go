package ui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type sharedKeyHandlers struct {
	editTask       func() (tea.Model, tea.Cmd)
	toggleStart    func() (tea.Model, tea.Cmd)
	markDone       func() (tea.Model, tea.Cmd)
	deleteTask     func() (tea.Model, tea.Cmd)
	setPriority    func() (tea.Model, tea.Cmd)
	setDueDate     func() (tea.Model, tea.Cmd)
	removeDueDate  func() (tea.Model, tea.Cmd)
	editTags       func() (tea.Model, tea.Cmd)
	annotate       func(replace bool) (tea.Model, tea.Cmd)
	editProject    func() (tea.Model, tea.Cmd)
	setRecurrence  func() (tea.Model, tea.Cmd)
	setRecurSeries func() (tea.Model, tea.Cmd)
	addTask        func() (tea.Model, tea.Cmd)
}

type keyBindingMode uint8

const (
	keyBindingNormal keyBindingMode = 1 << iota
	keyBindingUltra

	keyBindingAll = keyBindingNormal | keyBindingUltra
)

type keyBindingAction func(*Model, sharedKeyHandlers) (bool, tea.Model, tea.Cmd)

type keyBinding struct {
	keys   []string
	modes  keyBindingMode
	desc   string
	action keyBindingAction
}

var sharedKeyBindings = []keyBinding{
	{keys: []string{"H"}, modes: keyBindingAll, desc: "toggle help", action: modelKeyAction((*Model).handleToggleHelp)},
	{keys: []string{"q"}, modes: keyBindingAll, desc: "quit or exit current view", action: modelKeyAction((*Model).handleQuitKey)},
	{keys: []string{"esc"}, modes: keyBindingAll, desc: "close help/input or cancel", action: modelKeyAction((*Model).handleEscapeKey)},
	{keys: []string{"e", "E"}, modes: keyBindingAll, desc: "edit selected task", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.editTask })},
	{keys: []string{"s"}, modes: keyBindingAll, desc: "start/stop task", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.toggleStart })},
	{keys: []string{"d"}, modes: keyBindingAll, desc: "mark task done", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.markDone })},
	{keys: []string{"D"}, modes: keyBindingAll, desc: "delete task/recurring series", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.deleteTask })},
	{keys: []string{"o"}, modes: keyBindingAll, desc: "open URL or @file reference from description", action: modelKeyAction((*Model).handleOpenURL)},
	{keys: []string{"U"}, modes: keyBindingAll, desc: "undo last done/delete", action: modelKeyAction((*Model).handleUndo)},
	{keys: []string{"w"}, modes: keyBindingAll, desc: "set due date", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.setDueDate })},
	{keys: []string{"W"}, modes: keyBindingAll, desc: "remove due date", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.removeDueDate })},
	{keys: []string{"r"}, modes: keyBindingAll, desc: "set random due date", action: modelKeyAction((*Model).handleRandomDueDate)},
	{keys: []string{"R"}, modes: keyBindingAll, desc: "edit recurrence", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.setRecurrence })},
	{keys: []string{"ctrl+r"}, modes: keyBindingAll, desc: "edit recurring series recurrence", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.setRecurSeries })},
	{keys: []string{"p"}, modes: keyBindingAll, desc: "set priority", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.setPriority })},
	{keys: []string{"a"}, modes: keyBindingAll, desc: "add annotations", action: sharedAnnotateKeyAction(false)},
	{keys: []string{"A"}, modes: keyBindingAll, desc: "replace annotations", action: sharedAnnotateKeyAction(true)},
	{keys: []string{"f"}, modes: keyBindingAll, desc: "change filter", action: modelKeyAction((*Model).handleFilter)},
	{keys: []string{":"}, modes: keyBindingAll, desc: "run task command prompt", action: modelKeyAction((*Model).handleShellPrompt)},
	{keys: []string{";"}, modes: keyBindingAll, desc: "run task command prompt for selected task", action: modelKeyAction((*Model).handleShellPromptForSelectedTask)},
	{keys: []string{"+"}, modes: keyBindingAll, desc: "add new task", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.addTask })},
	{keys: []string{"t"}, modes: keyBindingAll, desc: "edit tags", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.editTags })},
	{keys: []string{"J"}, modes: keyBindingAll, desc: "edit project", action: sharedKeyAction(func(h sharedKeyHandlers) func() (tea.Model, tea.Cmd) { return h.editProject })},
	{keys: []string{"c"}, modes: keyBindingAll, desc: "random theme", action: modelKeyAction((*Model).handleRandomTheme)},
	{keys: []string{"C"}, modes: keyBindingAll, desc: "reset theme", action: modelKeyAction((*Model).handleResetTheme)},
	{keys: []string{"x"}, modes: keyBindingAll, desc: "toggle disco mode", action: modelKeyAction((*Model).handleToggleDisco)},
	{keys: []string{"B"}, modes: keyBindingAll, desc: "toggle blinking", action: modelKeyAction((*Model).handleToggleBlink)},
	{keys: []string{"space"}, modes: keyBindingAll, desc: "refresh tasks", action: modelKeyAction((*Model).handleRefresh)},
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

	if handled, model, cmd := m.handleSharedKey(msg.String(), keyBindingNormal, m.normalSharedKeyHandlers()); handled {
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
		editTask:       m.handleEditTask,
		toggleStart:    m.handleToggleStart,
		markDone:       m.handleMarkDone,
		deleteTask:     m.handleDeleteTask,
		setPriority:    m.handleSetPriority,
		setDueDate:     m.handleSetDueDate,
		removeDueDate:  m.handleRemoveDueDate,
		editTags:       m.handleEditTags,
		annotate:       m.handleAnnotate,
		editProject:    m.handleEditProject,
		setRecurrence:  m.handleSetRecurrence,
		setRecurSeries: m.handleSetRecurringSeriesRecurrence,
		addTask:        m.handleAddTask,
	}
}

func (m *Model) ultraSharedKeyHandlers() sharedKeyHandlers {
	return sharedKeyHandlers{
		editTask:       m.handleUltraEditTask,
		toggleStart:    m.handleUltraToggleStart,
		markDone:       m.handleUltraMarkDone,
		deleteTask:     m.handleUltraDeleteTask,
		setPriority:    m.handleUltraSetPriority,
		setDueDate:     m.handleUltraSetDueDate,
		removeDueDate:  m.handleUltraRemoveDueDate,
		editTags:       m.handleUltraEditTags,
		annotate:       m.handleUltraAnnotate,
		editProject:    m.handleUltraEditProject,
		setRecurrence:  m.handleUltraSetRecurrence,
		setRecurSeries: m.handleUltraSetRecurringSeriesRecurrence,
		addTask: func() (tea.Model, tea.Cmd) {
			m.ultraClearFocusedID()
			return m.handleAddTask()
		},
	}
}

func (m *Model) handleSharedKey(key string, mode keyBindingMode, handlers sharedKeyHandlers) (bool, tea.Model, tea.Cmd) {
	if key == m.agentFilterHotkeyLabel() {
		model, cmd := m.handleToggleAgentFilter()
		return true, model, cmd
	}

	for _, binding := range sharedKeyBindings {
		if binding.matches(key, mode) {
			return binding.action(m, handlers)
		}
	}

	return false, m, nil
}

func (b keyBinding) matches(key string, mode keyBindingMode) bool {
	if b.modes&mode == 0 {
		return false
	}
	for _, candidate := range b.keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func modelKeyAction(handler func(*Model) (tea.Model, tea.Cmd)) keyBindingAction {
	return func(m *Model, _ sharedKeyHandlers) (bool, tea.Model, tea.Cmd) {
		model, cmd := handler(m)
		return true, model, cmd
	}
}

func sharedKeyAction(selectHandler func(sharedKeyHandlers) func() (tea.Model, tea.Cmd)) keyBindingAction {
	return func(m *Model, handlers sharedKeyHandlers) (bool, tea.Model, tea.Cmd) {
		return callSharedKeyHandler(m, selectHandler(handlers))
	}
}

func sharedAnnotateKeyAction(replace bool) keyBindingAction {
	return func(m *Model, handlers sharedKeyHandlers) (bool, tea.Model, tea.Cmd) {
		return callSharedAnnotateHandler(m, handlers.annotate, replace)
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
		m.clearCurrentTaskDetail()
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
		m.clearCurrentTaskDetail()
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
