package ui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"codeberg.org/snonux/tasksamurai/internal"
	atable "codeberg.org/snonux/tasksamurai/internal/atable"
	"codeberg.org/snonux/tasksamurai/internal/task"
)

var priorityOptions = []string{"H", "M", "L", ""}

var (
	urlRegex         = regexp.MustCompile(`https?://\S+`)
	searchRegexCache = make(map[string]*regexp.Regexp)
)

type cellMatch struct {
	row int
	col int
}

type helpItem struct {
	key  string
	desc string
}

type helpSection struct {
	title string
	items []helpItem
}

// blinkState holds row-level blink animation state for the task table.
// A blink cycles the selected row's highlight on/off after a modification.
type blinkState struct {
	blinkID       int  // task ID currently being blinked (0 = none)
	blinkRow      int  // row index in the table (-1 if not found)
	blinkOn       bool // whether the highlight is currently inverted
	blinkCount    int  // number of blink cycles completed so far
	blinkMarkDone bool // whether to mark the task done after blinking
	blinkEnabled  bool // when false, skip animation and complete immediately
}

// searchState holds task-table and help-screen search state.
// Both search modes share this struct because only one is active at a time.
type searchState struct {
	searching     bool
	searchInput   textinput.Model
	searchRegex   *regexp.Regexp
	searchMatches []cellMatch
	searchIndex   int

	helpSearching     bool
	helpSearchInput   textinput.Model
	helpSearchRegex   *regexp.Regexp
	helpSearchMatches []int // line indices that match
	helpSearchIndex   int
}

// detailViewState holds all state for the task detail overlay.
// Blink fields here are separate from blinkState because they drive a
// per-field highlight inside the detail view rather than a table row.
type detailViewState struct {
	showTaskDetail    bool
	currentTaskDetail *task.Task
	detailSearching   bool
	detailSearchInput textinput.Model
	detailSearchRegex *regexp.Regexp
	detailFieldIndex  int  // currently selected field (-1 = none)
	detailBlinkField  int  // field currently blinking (-1 = none)
	detailBlinkOn     bool // whether the blink is currently on
	detailBlinkCount  int  // number of blink cycles completed so far
	// detailDescEditing lives here (not in editState) because it drives an
	// external-editor launch from the detail overlay, not inline text input.
	detailDescEditing bool // whether the description editor is open
}

// ultraState holds the state for the ultra mode task list and its search UI.
type ultraState struct {
	showUltra        bool
	ultraCursor      int
	ultraOffset      int
	ultraSearching   bool
	ultraSearchInput textinput.Model
	ultraSearchRegex *regexp.Regexp
	ultraFiltered    []int
	ultraFocusedID   int
}

// editState holds inline field-editing state for the task table.
// Each editing mode (annotate, desc, tags, …) is mutually exclusive;
// clearEditingModes resets them all before activating a new one.
type editState struct {
	annotating         bool
	annotateID         int
	annotateInput      textinput.Model
	replaceAnnotations bool

	descEditing bool
	descID      int
	descInput   textinput.Model

	tagsEditing bool
	tagsID      int
	tagsInput   textinput.Model

	dueEditing bool
	dueID      int
	dueDate    time.Time

	recurEditing bool
	recurID      int
	recurInput   textinput.Model

	projEditing bool
	projID      int
	projInput   textinput.Model

	filterEditing bool
	filterInput   textinput.Model

	addingTask bool
	addInput   textinput.Model

	prioritySelecting bool
	priorityID        int
	priorityIndex     int

	editID int // task ID being edited in an external editor
}

// Model wraps a Bubble Tea table.Model to display tasks.
// Related fields are grouped into anonymous embedded sub-structs so that
// each concern can be reasoned about independently without changing the
// existing field-access syntax throughout the package.
type Model struct {
	tbl       atable.Model
	tblStyles atable.Styles
	showHelp  bool

	blinkState      // row blink animation (see blinkState)
	searchState     // task-table and help-screen search (see searchState)
	detailViewState // task detail overlay (see detailViewState)
	ultraState      // ultra mode task list and search state (see ultraState)
	editState       // inline field editing (see editState)

	cellExpanded bool

	windowHeight int

	idWidth    int
	priWidth   int
	ageWidth   int
	urgWidth   int
	dueWidth   int
	recurWidth int
	tagsWidth  int
	descWidth  int
	annWidth   int
	projWidth  int

	total      int
	inProgress int
	due        int

	filters    []string
	tasks      []task.Task
	undoStack  []string
	browserCmd string

	theme        Theme
	defaultTheme Theme
	disco        bool // disco mode changes theme on every task modification

	statusMsg string // temporary status message shown in status bar

	helpViewport viewport.Model
}

// editDoneMsg is emitted when the external editor process finishes.
type editDoneMsg struct{ err error }

// descEditDoneMsg is emitted when the external editor for description finishes.
type descEditDoneMsg struct {
	err      error
	tempFile string
}

type blinkMsg struct{}

// blinkInterval controls how quickly the row flashes when a task changes.
// A shorter interval results in a faster blink.
const blinkInterval = 150 * time.Millisecond

// blinkCycles is the number of times to blink before stopping.
// The total blink duration is blinkInterval * blinkCycles.
const blinkCycles = 8

// editCmd returns a command that edits the task and sends an
// editDoneMsg once the process is complete.
func editCmd(id int) tea.Cmd {
	c := task.EditCmd(id)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err: err} })
}

// editDescriptionCmd returns a command that opens the description in external editor
func editDescriptionCmd(description string) tea.Cmd {
	return func() tea.Msg {
		// Create temp file
		tmpFile, err := os.CreateTemp("", "tasksamurai-desc-*.txt")
		if err != nil {
			return descEditDoneMsg{err: err, tempFile: ""}
		}
		tmpPath := tmpFile.Name()

		// Write current description to temp file
		_, err = tmpFile.WriteString(description)
		_ = tmpFile.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
			return descEditDoneMsg{err: err, tempFile: ""}
		}

		// Get editor from environment
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi" // fallback to vi
		}

		// Create the command
		c := exec.Command(editor, tmpPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		// Use ExecProcess to properly handle the external TUI editor
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return descEditDoneMsg{err: err, tempFile: tmpPath}
		})()
	}
}

func blinkCmd() tea.Cmd {
	return tea.Tick(blinkInterval, func(time.Time) tea.Msg { return blinkMsg{} })
}

// clearEditingModes ensures only one editing mode is active at a time
func (m *Model) clearEditingModes() {
	m.annotating = false
	m.descEditing = false
	m.tagsEditing = false
	m.dueEditing = false
	m.recurEditing = false
	m.projEditing = false
	m.filterEditing = false
	m.addingTask = false
	m.searching = false
	m.prioritySelecting = false
}

// startDetailBlink starts blinking a field in the detail view
func (m *Model) startDetailBlink(fieldIndex int) tea.Cmd {
	if !m.showTaskDetail {
		return nil
	}
	if m.disco {
		m.theme = RandomTheme()
		m.applyTheme()
	}
	m.detailBlinkField = fieldIndex
	m.detailBlinkOn = true
	m.detailBlinkCount = blinkCycles
	return blinkCmd()
}

func (m *Model) startBlink(id int, markDone bool) tea.Cmd {
	m.blinkID = id
	m.blinkMarkDone = markDone

	if !m.blinkEnabled {
		// If blinking is disabled, still complete the task immediately
		// by simulating the end of the blink cycle
		if markDone {
			for _, tsk := range m.tasks {
				if tsk.ID == id {
					m.undoStack = append(m.undoStack, tsk.UUID)
					break
				}
			}
			if err := task.Done(id); err != nil {
				m.showError(err)
			}
		}
		m.blinkID = 0
		m.blinkMarkDone = false
		m.reload()
		return nil
	}

	m.blinkRow = -1
	for i, tsk := range m.tasks {
		if tsk.ID == id {
			m.blinkRow = i
			break
		}
	}
	if m.blinkRow == -1 {
		return nil
	}
	if m.disco {
		m.theme = RandomTheme()
		m.applyTheme()
	}
	m.blinkOn = true
	m.blinkCount = 0
	m.updateBlinkRow()
	return blinkCmd()
}

// New creates a new UI model with the provided rows.
func New(filters []string, browserCmd string) (Model, error) {
	m := Model{filters: filters, browserCmd: browserCmd, blinkState: blinkState{blinkEnabled: true}}
	m.annotateInput = textinput.New()
	m.annotateInput.Prompt = "annotation: "
	m.descInput = textinput.New()
	m.descInput.Prompt = "description: "
	m.tagsInput = textinput.New()
	m.tagsInput.Prompt = "tags: "
	m.recurInput = textinput.New()
	m.recurInput.Prompt = "recur: "
	m.projInput = textinput.New()
	m.projInput.Prompt = "project: "
	m.dueDate = time.Now()
	m.searchInput = textinput.New()
	m.searchInput.Prompt = "search: "
	m.helpSearchInput = textinput.New()
	m.helpSearchInput.Prompt = "help search: "
	m.ultraSearchInput = textinput.New()
	m.ultraSearchInput.Prompt = "ultra search: "
	m.filterInput = textinput.New()
	m.filterInput.Prompt = "filter: "

	m.addInput = textinput.New()
	m.addInput.Prompt = "add: "

	m.defaultTheme = DefaultTheme()
	m.theme = m.defaultTheme

	if err := m.reload(); err != nil {
		return Model{}, err
	}

	return m, nil
}

func (m *Model) newTable(rows []atable.Row) (atable.Model, atable.Styles) {
	cols := []atable.Column{
		{Title: "Pri", Width: m.priWidth},
		{Title: "ID", Width: m.idWidth},
		{Title: "Age", Width: m.ageWidth},
		{Title: "Due", Width: m.dueWidth},
		{Title: "Recur", Width: m.recurWidth},
		{Title: "Project", Width: m.projWidth},
		{Title: "Tags", Width: m.tagsWidth},
		{Title: "Annotations", Width: m.annWidth},
		{Title: "Description", Width: m.descWidth},
		{Title: "Urg", Width: m.urgWidth},
	}
	t := atable.New(
		atable.WithColumns(cols),
		atable.WithRows(rows),
		atable.WithFocused(true),
		atable.WithShowHeaders(false),
	)
	styles := atable.DefaultStyles()
	styles.Cell = styles.Cell.Padding(0, 1)
	t.SetStyles(styles)
	m.tbl = t
	m.tblStyles = styles
	m.applyTheme()
	return m.tbl, m.tblStyles
}

func (m *Model) reload() error {
	// Always show only pending tasks by default.
	filters := append([]string(nil), m.filters...)
	filters = append(filters, "status:pending")
	ultraFilterIDs := m.ultraFilteredTaskIDs()
	tasks, err := task.Export(filters...)
	if err != nil {
		return err
	}

	task.SortTasks(tasks)

	m.tasks = tasks
	m.total = task.TotalTasks(tasks)
	m.inProgress = task.InProgressTasks(tasks)
	m.due = task.DueTasks(tasks, time.Now())

	// Refresh current task detail if in detail view
	if m.showTaskDetail {
		m.refreshCurrentTaskDetail()
	}

	m.computeColumnWidths()

	var rows []atable.Row
	m.searchMatches = nil
	for i, tsk := range tasks {
		rows = append(rows, m.taskToRowSearch(tsk, m.searchRegex, m.tblStyles, -1))
		if m.searchRegex != nil {
			if m.searchRegex.MatchString(tsk.Project) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 5})
			}
			tags := strings.Join(tsk.Tags, " ")
			if m.searchRegex.MatchString(tags) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 6})
			}
			if m.searchRegex.MatchString(tsk.Description) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 8})
			}
			for _, a := range tsk.Annotations {
				if m.searchRegex.MatchString(a.Description) {
					m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 7})
					break
				}
			}
		}
	}
	if len(m.searchMatches) > 0 {
		m.searchIndex = 0
	}
	if m.ultraSearchRegex != nil {
		m.ultraFiltered = m.ultraFilteredIndexes(m.ultraSearchRegex)
	} else {
		m.rebuildUltraFiltered(ultraFilterIDs)
	}

	if m.tbl.Columns() == nil {
		m.tbl, m.tblStyles = m.newTable(rows)
	} else {
		m.tbl.SetRows(rows)
		m.applyColumns()
	}
	m.reconcileUltraSelection()
	m.updateSelectionHighlight(-1, m.tbl.Cursor(), 0, m.tbl.ColumnCursor())
	return nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Handle resize in all modes, including during input
		return m.handleWindowResize(msg)
	case editDoneMsg:
		return m.handleEditDone(msg)
	case descEditDoneMsg:
		return m.handleDescEditDone(msg)
	case blinkMsg:
		return m.handleBlinkMsg()
	case struct{ clearStatus bool }:
		m.statusMsg = ""
		return m, nil
	case tea.KeyPressMsg:
		// Handle blinking state first
		if m.blinkID != 0 {
			return m.handleBlinkingState(msg)
		}

		// Check if we're in detail view
		if m.showTaskDetail {
			// If we're editing in detail view, let editing modes handle it
			if m.prioritySelecting || m.tagsEditing || m.dueEditing || m.recurEditing {
				if handled, model, cmd := m.handleEditingModes(msg); handled {
					return model, cmd
				}
			}
			// Otherwise handle detail view navigation
			return m.handleTaskDetailMode(msg)
		}

		if m.showHelp {
			if handled, model, cmd := m.handleEditingModes(msg); handled {
				return model, cmd
			}
			return m.handleNormalMode(msg)
		}

		if m.showUltra {
			if m.ultraSearching {
				return m.handleUltraSearchMode(msg)
			}
			if handled, model, cmd := m.handleEditingModes(msg); handled {
				return model, cmd
			}
			return m.handleUltraMode(msg)
		}

		// Check if we're in any editing mode
		if handled, model, cmd := m.handleEditingModes(msg); handled {
			return model, cmd
		}

		// Otherwise handle normal mode
		return m.handleNormalMode(msg)
	}

	// Default case - pass through to appropriate component
	if m.showHelp {
		// Update help viewport for mouse wheel and other events
		var cmd tea.Cmd
		m.helpViewport, cmd = m.helpViewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

// handleWindowResize handles window resize events
func (m *Model) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.tbl.SetWidth(msg.Width)
	m.windowHeight = msg.Height
	m.computeColumnWidths()
	m.updateTableHeight()
	if m.showUltra {
		if m.ultraSearchRegex != nil {
			m.ultraFiltered = m.ultraFilteredIndexes(m.ultraSearchRegex)
		}
		m.ultraEnsureVisible()
		m.syncUltraTableSelection()
	}

	// Update help viewport if active
	if m.showHelp && m.helpViewport.Width() > 0 {
		width := msg.Width - 4
		height := msg.Height - 6
		if width > 0 && height > 0 {
			m.helpViewport.SetWidth(width)
			m.helpViewport.SetHeight(height)
		}
	}

	return m, nil
}

// handleEditDone handles completion of external editor
func (m *Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError(fmt.Errorf("editor: %w", msg.err))
	}
	if m.showUltra {
		m.ultraFocusedID = m.editID
	}
	m.reload()
	cmd := m.startBlink(m.editID, false)
	m.editID = 0
	return m, cmd
}

// handleDescEditDone handles the completion of description editing
func (m *Model) handleDescEditDone(msg descEditDoneMsg) (tea.Model, tea.Cmd) {
	m.detailDescEditing = false
	_ = os.Remove(msg.tempFile) // Clean up temp file

	if msg.err != nil {
		m.statusMsg = fmt.Sprintf("Edit error: %v", msg.err)
		cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return struct{ clearStatus bool }{true}
		})
		return m, cmd
	}

	// Read the edited content
	content, err := os.ReadFile(msg.tempFile)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error reading file: %v", err)
		cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return struct{ clearStatus bool }{true}
		})
		return m, cmd
	}

	// Update the description
	newDesc := strings.TrimSpace(string(content))
	if m.currentTaskDetail != nil {
		err = task.SetDescription(m.currentTaskDetail.ID, newDesc)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error updating description: %v", err)
			cmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return struct{ clearStatus bool }{true}
			})
			return m, cmd
		}

		// Reload and start blinking
		m.reload()
		return m, m.startDetailBlink(m.detailDescriptionFieldIndex())
	}

	return m, nil
}

// handleBlinkMsg handles the blinking animation timer
func (m *Model) handleBlinkMsg() (tea.Model, tea.Cmd) {
	// Handle detail view blinking
	if m.showTaskDetail && m.detailBlinkField != -1 {
		m.detailBlinkOn = !m.detailBlinkOn
		m.detailBlinkCount++

		if m.detailBlinkCount >= blinkCycles {
			m.detailBlinkField = -1
			m.detailBlinkOn = false
			m.detailBlinkCount = 0
		} else {
			return m, blinkCmd()
		}
		return m, nil
	}

	if m.blinkID == 0 {
		return m, nil
	}

	m.blinkOn = !m.blinkOn
	m.blinkCount++
	m.updateBlinkRow()

	if m.blinkCount >= blinkCycles {
		id := m.blinkID
		mark := m.blinkMarkDone
		m.blinkID = 0
		m.blinkOn = false
		m.blinkCount = 0
		m.blinkMarkDone = false

		if mark {
			for _, tsk := range m.tasks {
				if tsk.ID == id {
					m.undoStack = append(m.undoStack, tsk.UUID)
					break
				}
			}
			if err := task.Done(id); err != nil {
				m.showError(err)
			}
		}
		m.reload()
		return m, nil
	}

	return m, blinkCmd()
}

// View renders the table UI.
func (m Model) View() tea.View {
	var content string
	if m.showHelp {
		m.updateHelpContent()
		content = m.renderHelpScreen()
	} else if m.showTaskDetail {
		content = m.renderTaskDetail()
	} else if m.showUltra {
		content = m.renderUltraModus()
	} else {
		// expandedCellView is only appended when the user has toggled the
		// expanded-cell panel open; including it unconditionally caused a
		// double-render whenever cellExpanded was true.
		view := lipgloss.JoinVertical(lipgloss.Left,
			m.topStatusLine(),
			m.tbl.View(),
			m.statusLine(),
		)
		if m.cellExpanded {
			view = lipgloss.JoinVertical(lipgloss.Left, view, m.expandedCellView())
		}
		content = m.appendInlineInputOverlay(view)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// appendInlineInputOverlay appends whichever active inline-editing widget
// (annotate, due, priority, desc, tags, recur, project, filter, add, search)
// should be displayed below the table. At most one is active at a time.
func (m Model) appendInlineInputOverlay(view string) string {
	var overlay string
	switch {
	case m.annotating:
		overlay = m.annotateInput.View()
	case m.dueEditing:
		overlay = m.dueView(true)
	case m.prioritySelecting:
		overlay = m.priorityView(true)
	case m.descEditing:
		overlay = m.descInput.View()
	case m.tagsEditing:
		overlay = m.tagsInput.View()
	case m.recurEditing:
		overlay = m.recurInput.View()
	case m.projEditing:
		overlay = m.projInput.View()
	case m.filterEditing:
		overlay = m.filterInput.View()
	case m.addingTask:
		overlay = m.addInput.View()
	case m.searching:
		overlay = m.searchInput.View()
	}

	if overlay != "" {
		view = lipgloss.JoinVertical(lipgloss.Left, view, overlay)
	}
	return view
}

// updateHelpContent updates the help viewport content
func (m *Model) updateHelpContent() {
	m.helpViewport.SetContent(m.activeHelpContent())
}

// buildHelpContent builds the help content
func (m Model) buildHelpContent() string {
	return m.buildRenderedHelpContent(m.helpSections())
}

// renderHelpScreen renders the help screen with optional search highlighting
func (m Model) renderHelpScreen() string {
	containerStyle := lipgloss.NewStyle().
		Padding(1, 2)

	// Render viewport
	viewportView := m.helpViewport.View()

	result := containerStyle.Render(viewportView)

	// Add search input at the bottom if in help search mode
	if m.helpSearching {
		searchStyle := lipgloss.NewStyle().
			Padding(0, 2)
		result = lipgloss.JoinVertical(lipgloss.Left,
			result,
			searchStyle.Render(m.helpSearchInput.View()),
		)
	}

	return result
}

// formatHelpLine formats a help line with key and description styling
func (m Model) formatHelpLine(key, desc string, keyStyle, descStyle lipgloss.Style) string {
	// Pad key to consistent width for alignment
	paddedKey := fmt.Sprintf("%-12s", key)
	return keyStyle.Render(paddedKey) + " " + descStyle.Render(desc)
}

// highlightHelpLine applies search highlighting to a help line
func (m Model) highlightHelpLine(line string) string {
	if m.helpSearchRegex == nil {
		return line
	}

	matches := m.helpSearchRegex.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	highlighted := line
	offset := 0
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.SearchBG)).
		Foreground(lipgloss.Color(m.theme.SearchFG))

	for _, match := range matches {
		start := match[0] + offset
		end := match[1] + offset
		highlighted = highlighted[:start] + highlightStyle.Render(highlighted[start:end]) + highlighted[end:]
		offset += len(highlightStyle.Render(highlighted[start:end])) - (end - start)
	}

	return highlighted
}

// getHelpLines returns searchable help content as plain text lines
func (m Model) getHelpLines() []string {
	return flattenHelpSections(m.activeHelpSections())
}

func (m Model) activeHelpContent() string {
	if m.showUltra {
		return m.buildUltraHelpContent()
	}
	return m.buildHelpContent()
}

func (m Model) activeHelpSections() []helpSection {
	if m.showUltra {
		return m.ultraHelpSections()
	}
	return m.helpSections()
}

func (m Model) buildRenderedHelpContent(sections []helpSection) string {
	headerStyle, keyStyle, descStyle := m.helpStyles()
	lines := make([]string, 0, len(sections)*4)
	for i, section := range sections {
		lines = append(lines, headerStyle.Render(section.title))
		for _, item := range section.items {
			lines = append(lines, m.formatHelpLine(item.key, item.desc, keyStyle, descStyle))
		}
		if i < len(sections)-1 {
			lines = append(lines, "")
		}
	}

	if m.helpSearchRegex != nil {
		for i, line := range lines {
			if m.helpSearchRegex.MatchString(line) {
				lines[i] = m.highlightHelpLine(line)
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) helpStyles() (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.HeaderFG)).
		Background(lipgloss.Color(m.theme.SelectedBG)).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.SelectedFG))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))

	return headerStyle, keyStyle, descStyle
}

func (m Model) helpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation",
			items: []helpItem{
				{key: "↑/k, ↓/j", desc: "move up/down"},
				{key: "←/h, →/l", desc: "move left/right"},
				{key: "0, g, Home", desc: "go to start"},
				{key: "G, End", desc: "go to end"},
				{key: "pgup/pgdn, b", desc: "page up/down"},
				{key: "1", desc: "jump to random task"},
				{key: "2", desc: "jump to random task (no due date)"},
			},
		},
		{
			title: "Task Management",
			items: []helpItem{
				{key: "Enter", desc: "view task details"},
				{key: "+", desc: "add new task"},
				{key: "e, E", desc: "edit entire task"},
				{key: "d", desc: "mark task done"},
				{key: "U", desc: "undo last done"},
				{key: "s", desc: "start/stop task"},
			},
		},
		{
			title: "Task Fields",
			items: []helpItem{
				{key: "i", desc: "edit current field"},
				{key: "p", desc: "set priority"},
				{key: "w, W", desc: "set/remove due date"},
				{key: "r", desc: "set random due date"},
				{key: "R", desc: "edit recurrence"},
				{key: "t", desc: "edit tags"},
				{key: "J", desc: "edit project"},
				{key: "T", desc: "convert first tag to project"},
				{key: "a, A", desc: "add/replace annotations"},
				{key: "o", desc: "open URL from description"},
			},
		},
		{
			title: "View & Search",
			items: []helpItem{
				{key: "f", desc: "change filter"},
				{key: "/, ?", desc: "search"},
				{key: "n, N", desc: "next/previous match"},
				{key: "space", desc: "refresh tasks"},
			},
		},
		{
			title: "Appearance",
			items: []helpItem{
				{key: "c, C", desc: "random/reset theme"},
				{key: "x", desc: "toggle disco mode"},
				{key: "B", desc: "toggle blinking"},
			},
		},
		{
			title: "General",
			items: []helpItem{
				{key: "H", desc: "toggle help"},
				{key: "ESC", desc: "close dialogs/cancel"},
				{key: "q", desc: "quit"},
			},
		},
	}
}

func flattenHelpSections(sections []helpSection) []string {
	lines := make([]string, 0, len(sections)*4)
	for i, section := range sections {
		lines = append(lines, section.title)
		for _, item := range section.items {
			lines = append(lines, fmt.Sprintf("%s: %s", item.key, item.desc))
		}
		if i < len(sections)-1 {
			lines = append(lines, "")
		}
	}
	return lines
}

func (m Model) statusLine() string {
	status := fmt.Sprintf("Total:%d InProgress:%d Due:%d | press H for help", m.total, m.inProgress, m.due)
	if m.statusMsg != "" {
		status = m.statusMsg
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(m.tbl.Width()).
		Render(status)
}

func (m Model) topStatusLine() string {
	line := fmt.Sprintf("Task Samurai %s", internal.Version)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(m.tbl.Width()).
		Render(line)
}

// formatDue returns a formatted due date string. Dates due today or tomorrow
// are returned as "today" or "tomorrow" respectively. Past due dates are
// highlighted in red.
func (m Model) formatDue(s string, width int) string {
	if s == "" {
		return ""
	}
	ts, err := time.Parse(task.DateFormat, s)
	if err != nil {
		return s
	}

	days := int(time.Until(ts).Hours() / 24)
	var val string
	switch days {
	case 0:
		val = "today"
	case 1:
		val = "tomorrow"
	case -1:
		val = "yesterday"
	default:
		val = fmt.Sprintf("%dd", days)
	}
	style := lipgloss.NewStyle().Width(width)
	if days < 0 {
		style = style.Background(lipgloss.Color(m.theme.OverdueBG))
	}
	return style.Render(val)
}

func (m Model) formatPriority(p string, width int) string {
	style := lipgloss.NewStyle().Width(width)
	switch p {
	case "L":
		style = style.Background(lipgloss.Color(m.theme.PrioLowBG))
	case "M":
		style = style.Background(lipgloss.Color(m.theme.PrioMedBG))
	case "H":
		style = style.Background(lipgloss.Color(m.theme.PrioHighBG))
	default:
		return p
	}
	return style.Render(p)
}

func (m Model) formatUrgency(u string, width int) string {
	if w := width - len(u); w > 0 {
		u = strings.Repeat(" ", w) + u
	}
	return u
}

func (m Model) dueView(showLabel bool) string {
	if showLabel {
		return fmt.Sprintf("due: %s", m.dueDate.Format("2006-01-02"))
	}
	return m.dueDate.Format("2006-01-02")
}

func (m Model) priorityView(showLabel bool) string {
	var parts []string
	for i, p := range priorityOptions {
		label := p
		if label == "" {
			label = "none"
		}
		style := lipgloss.NewStyle()
		if i == m.priorityIndex {
			style = style.Foreground(lipgloss.Color(m.theme.SelectedFG)).Background(lipgloss.Color(m.theme.SelectedBG))
		}
		parts = append(parts, style.Render(label))
	}
	if showLabel {
		return "priority: " + strings.Join(parts, " ")
	}
	return strings.Join(parts, " ")
}

func (m Model) highlightCell(base lipgloss.Style, re *regexp.Regexp, raw string) string {
	if re == nil || !re.MatchString(raw) {
		return base.Render(raw)
	}

	highlight := lipgloss.NewStyle().Background(lipgloss.Color(m.theme.SearchBG)).Foreground(lipgloss.Color(m.theme.SearchFG))
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringIndex(raw, -1) {
		if loc[0] > last {
			b.WriteString(base.Render(raw[last:loc[0]]))
		}
		b.WriteString(highlight.Inherit(base).Render(raw[loc[0]:loc[1]]))
		last = loc[1]
	}
	if last < len(raw) {
		b.WriteString(base.Render(raw[last:]))
	}
	return b.String()
}

func (m Model) highlightCellMatch(base lipgloss.Style, re *regexp.Regexp, raw, display string) string {
	if re != nil && re.MatchString(raw) {
		highlight := lipgloss.NewStyle().Background(lipgloss.Color(m.theme.SearchBG)).Foreground(lipgloss.Color(m.theme.SearchFG))
		return highlight.Inherit(base).Render(display)
	}
	return base.Render(display)
}

func (m Model) taskToRowSearch(t task.Task, re *regexp.Regexp, styles atable.Styles, selectedCol int) atable.Row {
	rowStyle := lipgloss.NewStyle()
	if t.Start != "" {
		rowStyle = rowStyle.Background(lipgloss.Color(m.theme.StartBG))
	}
	if t.ID == m.blinkID && m.blinkOn {
		rowStyle = rowStyle.Reverse(true)
	}

	age := ""
	if ts, err := time.Parse(task.DateFormat, t.Entry); err == nil {
		days := int(time.Since(ts).Hours() / 24)
		age = fmt.Sprintf("%dd", days)
	}

	tags := strings.Join(t.Tags, " ")
	urg := fmt.Sprintf("%.1f", t.Urgency)
	recur := t.Recur

	var anns []string
	for _, a := range t.Annotations {
		anns = append(anns, a.Description)
	}

	cellStyle := rowStyle.Inherit(styles.Cell)
	selStyle := cellStyle.Inherit(styles.Selected)

	getStyle := func(col int) lipgloss.Style {
		if col == selectedCol {
			return selStyle
		}
		return cellStyle
	}

	priStr := m.formatPriority(t.Priority, m.priWidth)
	idStr := getStyle(1).Render(strconv.Itoa(t.ID))
	ageStr := getStyle(2).Render(age)
	dueStr := m.formatDue(t.Due, m.dueWidth)
	recurStr := m.highlightCell(getStyle(4), re, recur)
	projStr := m.highlightCell(getStyle(5), re, t.Project)
	tagStr := m.highlightCell(getStyle(6), re, tags)
	annRaw := strings.Join(anns, "; ")
	annCount := ""
	if n := len(anns); n > 0 {
		annCount = strconv.FormatInt(int64(n), 16)
	}
	annStr := m.highlightCellMatch(getStyle(7), re, annRaw, annCount)
	descStr := m.highlightCell(getStyle(8), re, t.Description)
	urgStr := getStyle(9).Render(m.formatUrgency(urg, m.urgWidth))

	return atable.Row{
		priStr,
		idStr,
		ageStr,
		dueStr,
		recurStr,
		projStr,
		tagStr,
		annStr,
		descStr,
		urgStr,
	}
}

func (m Model) expandedCellView() string {
	row := m.tbl.Cursor()
	col := m.tbl.ColumnCursor()
	if row < 0 || row >= len(m.tasks) || col < 0 || col > 9 {
		return ""
	}
	t := m.tasks[row]
	var val string
	switch col {
	case 0:
		val = ansi.Strip(m.formatPriority(t.Priority, m.priWidth))
	case 1:
		val = strconv.Itoa(t.ID)
	case 2:
		if ts, err := time.Parse(task.DateFormat, t.Entry); err == nil {
			days := int(time.Since(ts).Hours() / 24)
			val = fmt.Sprintf("%dd", days)
		}
	case 3:
		val = ansi.Strip(m.formatDue(t.Due, m.dueWidth))
	case 4:
		val = t.Recur
	case 5:
		val = t.Project
	case 6:
		val = strings.Join(t.Tags, " ")
	case 7:
		var anns []string
		for _, a := range t.Annotations {
			anns = append(anns, a.Description)
		}
		val = strings.Join(anns, "; ")
	case 8:
		val = t.Description
	case 9:
		val = fmt.Sprintf("%.1f", t.Urgency)
	}
	header := ""
	cols := m.tbl.Columns()
	if col >= 0 && col < len(cols) {
		header = cols[col].Title
	}
	if header != "" {
		val = header + ": " + val
	}
	style := lipgloss.NewStyle().Width(m.tbl.Width())
	return style.Render(val)
}

func (m *Model) updateSelectionHighlight(prevRow, newRow, prevCol, newCol int) {
	if m.searchRegex == nil {
		return
	}
	rows := m.tbl.Rows()
	if prevRow >= 0 && prevRow < len(rows) {
		rows[prevRow] = m.taskToRowSearch(m.tasks[prevRow], m.searchRegex, m.tblStyles, -1)
	}
	if newRow >= 0 && newRow < len(rows) {
		rows[newRow] = m.taskToRowSearch(m.tasks[newRow], m.searchRegex, m.tblStyles, newCol)
	}
	m.tbl.SetRows(rows)
}

func (m *Model) updateBlinkRow() {
	if m.blinkRow < 0 || m.blinkRow >= len(m.tasks) || m.tbl.Rows() == nil {
		return
	}
	rows := m.tbl.Rows()
	rows[m.blinkRow] = m.taskToRowSearch(m.tasks[m.blinkRow], m.searchRegex, m.tblStyles, -1)
	m.tbl.SetRows(rows)
}

// updateTableHeight recalculates the table height based on the current window
// size and which auxiliary views are open.
func (m *Model) updateTableHeight() {
	if m.windowHeight == 0 {
		return
	}
	h := m.windowHeight - 2 // space for top and bottom status bars
	if m.cellExpanded {
		h--
	}
	if m.annotating || m.dueEditing || m.prioritySelecting || m.searching || m.descEditing || m.tagsEditing || m.recurEditing || m.projEditing || m.filterEditing || m.addingTask {
		h--
	}
	if h < 1 {
		h = 1
	}
	m.tbl.SetHeight(h)
}

func (m *Model) computeColumnWidths() {
	maxID := 1
	maxAge := 0
	maxUrg := 0
	maxDue := 0
	maxRecur := 1
	maxTags := 0
	maxAnn := 1
	maxProj := 1
	for _, t := range m.tasks {
		if l := len(strconv.Itoa(t.ID)); l > maxID {
			maxID = l
		}
		age := ""
		if ts, err := time.Parse(task.DateFormat, t.Entry); err == nil {
			age = fmt.Sprintf("%dd", int(time.Since(ts).Hours()/24))
		}
		if l := len(age); l > maxAge {
			maxAge = l
		}
		urg := fmt.Sprintf("%.1f", t.Urgency)
		if l := len(urg); l > maxUrg {
			maxUrg = l
		}
		due := formatDueText(t.Due)
		if l := len(due); l > maxDue {
			maxDue = l
		}
		if l := len(t.Recur); l > maxRecur {
			maxRecur = l
		}
		if l := len(t.Project); l > maxProj {
			maxProj = l
		}
		tags := strings.Join(t.Tags, " ")
		if l := len(tags); l > maxTags {
			maxTags = l
		}
		ann := len(t.Annotations)
		if l := len(strconv.FormatInt(int64(ann), 16)); l > maxAnn {
			maxAnn = l
		}
	}

	m.idWidth = maxID
	m.priWidth = 1
	m.ageWidth = maxAge
	m.urgWidth = maxUrg
	m.dueWidth = maxDue
	m.recurWidth = maxRecur
	m.tagsWidth = maxTags
	m.annWidth = maxAnn
	m.projWidth = maxProj

	total := m.tbl.Width()
	if total == 0 {
		total = 80
	}
	base := m.idWidth + m.priWidth + m.ageWidth + m.dueWidth + m.recurWidth + m.tagsWidth + m.annWidth + m.urgWidth + m.projWidth
	base += 9 // spaces between columns
	m.descWidth = total - base
	if m.descWidth < 1 {
		m.descWidth = 1
	}

	if m.tbl.Columns() != nil {
		m.applyColumns()
	}
}

func (m *Model) applyColumns() {
	cols := []atable.Column{
		{Title: "Pri", Width: m.priWidth},
		{Title: "ID", Width: m.idWidth},
		{Title: "Age", Width: m.ageWidth},
		{Title: "Due", Width: m.dueWidth},
		{Title: "Recur", Width: m.recurWidth},
		{Title: "Project", Width: m.projWidth},
		{Title: "Tags", Width: m.tagsWidth},
		{Title: "Annotations", Width: m.annWidth},
		{Title: "Description", Width: m.descWidth},
		{Title: "Urg", Width: m.urgWidth},
	}
	m.tbl.SetColumns(cols)
}

func (m *Model) applyTheme() {
	m.tblStyles.Header = m.tblStyles.Header.Foreground(lipgloss.Color(m.theme.HeaderFG))
	m.tblStyles.Selected = m.tblStyles.Selected.Foreground(lipgloss.Color(m.theme.SelectedFG)).Background(lipgloss.Color(m.theme.SelectedBG))
	m.tblStyles.Highlight = m.tblStyles.Highlight.Background(lipgloss.Color(m.theme.RowBG)).Foreground(lipgloss.Color(m.theme.RowFG))
	m.tbl.SetStyles(m.tblStyles)
}

// SetDisco enables or disables disco mode.
func (m *Model) SetDisco(d bool) {
	m.disco = d
}

// SetUltra enables or disables ultra mode, causing the UI to start directly
// in the ultra task list view instead of the default table view.
func (m *Model) SetUltra(u bool) {
	m.showUltra = u
}
