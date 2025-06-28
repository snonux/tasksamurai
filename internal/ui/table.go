package ui

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeberg.org/snonux/tasksamurai/internal"
	atable "codeberg.org/snonux/tasksamurai/internal/atable"
	"codeberg.org/snonux/tasksamurai/internal/task"
)

var priorityOptions = []string{"H", "M", "L", ""}

var (
	urlRegex         = regexp.MustCompile(`https?://\S+`)
	searchRegexCache = make(map[string]*regexp.Regexp)
	rng              = rand.New(rand.NewSource(time.Now().UnixNano()))
)

type cellMatch struct {
	row int
	col int
}

// Model wraps a Bubble Tea table.Model to display tasks.

type Model struct {
	tbl       atable.Model
	tblStyles atable.Styles
	showHelp  bool

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

	filterEditing bool
	filterInput   textinput.Model

	addingTask bool
	addInput   textinput.Model

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

	prioritySelecting bool
	priorityID        int
	priorityIndex     int

	filters []string
	tasks   []task.Task

	undoStack []string

	browserCmd string

	editID int

	blinkID       int
	blinkRow      int
	blinkOn       bool
	blinkCount    int
	blinkMarkDone bool

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

	total      int
	inProgress int
	due        int

	theme        Theme
	defaultTheme Theme
	disco        bool   // disco mode changes theme on every task modification

	statusMsg string // temporary status message shown in status bar

	// Task detail view fields
	showTaskDetail    bool
	currentTaskDetail *task.Task
	detailSearching   bool
	detailSearchInput textinput.Model
	detailSearchRegex *regexp.Regexp
	detailFieldIndex  int    // Current selected field in detail view
	detailBlinkField  int    // Field that is currently blinking (-1 for none)
	detailBlinkOn     bool   // Whether the blink is currently on
	detailBlinkCount  int    // Number of blinks remaining
	detailDescEditing bool   // Whether we're editing description in detail view
	detailDescTempFile string // Temp file path for description editing
	
	// Help view fields
	helpViewport viewport.Model
}

// editDoneMsg is emitted when the external editor process finishes.
type editDoneMsg struct{ err error }

// descEditDoneMsg is emitted when the external editor for description finishes.
type descEditDoneMsg struct{ 
	err error
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
		tmpFile.Close()
		if err != nil {
			os.Remove(tmpPath)
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
	m := Model{filters: filters, browserCmd: browserCmd}
	m.annotateInput = textinput.New()
	m.annotateInput.Prompt = "annotation: "
	m.descInput = textinput.New()
	m.descInput.Prompt = "description: "
	m.tagsInput = textinput.New()
	m.tagsInput.Prompt = "tags: "
	m.recurInput = textinput.New()
	m.recurInput.Prompt = "recur: "
	m.dueDate = time.Now()
	m.searchInput = textinput.New()
	m.searchInput.Prompt = "search: "
	m.helpSearchInput = textinput.New()
	m.helpSearchInput.Prompt = "help search: "
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
			tags := strings.Join(tsk.Tags, " ")
			if m.searchRegex.MatchString(tags) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 5})
			}
			if m.searchRegex.MatchString(tsk.Description) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 7})
			}
			for _, a := range tsk.Annotations {
				if m.searchRegex.MatchString(a.Description) {
					m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 6})
					break
				}
			}
		}
	}
	if len(m.searchMatches) > 0 {
		m.searchIndex = 0
	}

	if m.tbl.Columns() == nil {
		m.tbl, m.tblStyles = m.newTable(rows)
	} else {
		m.tbl.SetRows(rows)
		m.applyColumns()
	}
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
	case tea.KeyMsg:
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
		
		// Check if we're in any editing mode
		if handled, model, cmd := m.handleEditingModes(msg); handled {
			return model, cmd
		}
		
		// Otherwise handle normal mode
		return m.handleNormalMode(msg)
	}
	
	// Default case - pass through to table
	if m.showHelp {
		return m, nil
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
	
	// Update help viewport if active
	if m.showHelp && m.helpViewport.Width > 0 {
		width := msg.Width - 4
		height := msg.Height - 6
		if width > 0 && height > 0 {
			m.helpViewport.Width = width
			m.helpViewport.Height = height
		}
	}
	
	return m, nil
}

// handleEditDone handles completion of external editor
func (m *Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError(fmt.Errorf("editor: %w", msg.err))
	}
	m.reload()
	cmd := m.startBlink(m.editID, false)
	m.editID = 0
	return m, cmd
}

// handleDescEditDone handles the completion of description editing
func (m *Model) handleDescEditDone(msg descEditDoneMsg) (tea.Model, tea.Cmd) {
	m.detailDescEditing = false
	defer os.Remove(msg.tempFile) // Clean up temp file
	
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
		return m, m.startDetailBlink(9) // Description field index
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
func (m Model) View() string {
	if m.showHelp {
		// Update help content before rendering
		m.updateHelpContent()
		return m.renderHelpScreen()
	}
	if m.showTaskDetail {
		return m.renderTaskDetail()
	}
	view := lipgloss.JoinVertical(lipgloss.Left,
		m.topStatusLine(),
		m.tbl.View(),
		m.expandedCellView(),
		m.statusLine(),
	)
	if m.cellExpanded {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.expandedCellView(),
		)
	}
	if m.annotating {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.annotateInput.View(),
		)
	}
	if m.dueEditing {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.dueView(true),
		)
	}
	if m.prioritySelecting {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.priorityView(true),
		)
	}
	if m.descEditing {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.descInput.View(),
		)
	}
	if m.tagsEditing {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.tagsInput.View(),
		)
	}
	if m.recurEditing {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.recurInput.View(),
		)
	}
	if m.filterEditing {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.filterInput.View(),
		)
	}
	if m.addingTask {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.addInput.View(),
		)
	}
	if m.searching {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.searchInput.View(),
		)
	}
	return view
}

// updateHelpContent updates the help viewport content
func (m *Model) updateHelpContent() {
	content := m.buildHelpContent()
	m.helpViewport.SetContent(content)
}

// buildHelpContent builds the help content
func (m Model) buildHelpContent() string {
	// Create styles using theme colors
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.HeaderFG)).
		Background(lipgloss.Color(m.theme.SelectedBG)).
		Padding(0, 1)
		
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.SelectedFG))
		
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")) // Light gray for readability

	// Build help content with styled headers
	var sections []string
	
	// Navigation section
	sections = append(sections, headerStyle.Render("Navigation"),
		m.formatHelpLine("↑/k, ↓/j", "move up/down", keyStyle, descStyle),
		m.formatHelpLine("←/h, →/l", "move left/right", keyStyle, descStyle),
		m.formatHelpLine("g/Home, G/End", "go to start/end", keyStyle, descStyle),
		m.formatHelpLine("pgup/pgdn, b", "page up/down", keyStyle, descStyle),
		"")
	
	// Task Management section
	sections = append(sections, headerStyle.Render("Task Management"),
		m.formatHelpLine("Enter", "view task details", keyStyle, descStyle),
		m.formatHelpLine("+", "add new task", keyStyle, descStyle),
		m.formatHelpLine("E", "edit entire task", keyStyle, descStyle),
		m.formatHelpLine("d", "mark task done", keyStyle, descStyle),
		m.formatHelpLine("U", "undo last done", keyStyle, descStyle),
		m.formatHelpLine("s", "start/stop task", keyStyle, descStyle),
		"")
	
	// Task Fields section
	sections = append(sections, headerStyle.Render("Task Fields"),
		m.formatHelpLine("i", "edit current field", keyStyle, descStyle),
		m.formatHelpLine("p", "set priority", keyStyle, descStyle),
		m.formatHelpLine("w, W", "set/remove due date", keyStyle, descStyle),
		m.formatHelpLine("r", "set random due date", keyStyle, descStyle),
		m.formatHelpLine("R", "edit recurrence", keyStyle, descStyle),
		m.formatHelpLine("t", "edit tags", keyStyle, descStyle),
		m.formatHelpLine("a, A", "add/replace annotations", keyStyle, descStyle),
		m.formatHelpLine("o", "open URL from description", keyStyle, descStyle),
		"")
	
	// View & Search section
	sections = append(sections, headerStyle.Render("View & Search"),
		m.formatHelpLine("f", "change filter", keyStyle, descStyle),
		m.formatHelpLine("/", "search", keyStyle, descStyle),
		m.formatHelpLine("n, N", "next/previous match", keyStyle, descStyle),
		m.formatHelpLine("space", "refresh tasks", keyStyle, descStyle),
		"")
	
	// Appearance section
	sections = append(sections, headerStyle.Render("Appearance"),
		m.formatHelpLine("c, C", "random/reset theme", keyStyle, descStyle),
		m.formatHelpLine("x", "toggle disco mode", keyStyle, descStyle),
		"")
	
	// General section
	sections = append(sections, headerStyle.Render("General"),
		m.formatHelpLine("H", "toggle help", keyStyle, descStyle),
		m.formatHelpLine("ESC", "close dialogs/cancel", keyStyle, descStyle),
		m.formatHelpLine("q", "quit", keyStyle, descStyle))
	
	// Apply search highlighting if active
	if m.helpSearchRegex != nil {
		for i, line := range sections {
			if m.helpSearchRegex.MatchString(line) {
				sections[i] = m.highlightHelpLine(line)
			}
		}
	}
	
	// Join all sections
	return strings.Join(sections, "\n")
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
	return []string{
		"Navigation",
		"↑/k, ↓/j: move up/down",
		"←/h, →/l: move left/right", 
		"g/Home, G/End: go to start/end",
		"pgup/pgdn, b: page up/down",
		"",
		"Task Management",
		"Enter: view task details",
		"+: add new task",
		"E: edit entire task",
		"d: mark task done",
		"U: undo last done",
		"s: start/stop task",
		"",
		"Task Fields",
		"i: edit current field",
		"p: set priority",
		"w, W: set/remove due date",
		"r: set random due date",
		"R: edit recurrence",
		"t: edit tags",
		"a, A: add/replace annotations",
		"o: open URL from description",
		"",
		"View & Search",
		"f: change filter",
		"/: search",
		"n, N: next/previous match",
		"space: refresh tasks",
		"",
		"Appearance",
		"c, C: random/reset theme",
		"x: toggle disco mode",
		"",
		"General",
		"H: toggle help",
		"ESC: close dialogs/cancel",
		"q: quit",
	}
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

func (m Model) taskToRow(t task.Task) atable.Row {
	style := lipgloss.NewStyle()
	if t.Start != "" {
		style = style.Background(lipgloss.Color(m.theme.StartBG))
	}
	if t.ID == m.blinkID && m.blinkOn {
		style = style.Reverse(true)
	}

	age := ""
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
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

	annStr := ""
	if n := len(anns); n > 0 {
		annStr = strconv.FormatInt(int64(n), 16)
	}

	return atable.Row{
		m.formatPriority(t.Priority, m.priWidth),
		style.Render(strconv.Itoa(t.ID)),
		style.Render(age),
		m.formatDue(t.Due, m.dueWidth),
		style.Render(recur),
		style.Render(tags),
		style.Render(annStr),
		style.Render(t.Description),
		style.Render(m.formatUrgency(urg, m.urgWidth)),
	}
}

// formatDue returns a formatted due date string. Dates due today or tomorrow
// are returned as "today" or "tomorrow" respectively. Past due dates are
// highlighted in red.
func (m Model) formatDue(s string, width int) string {
	if s == "" {
		return ""
	}
	ts, err := time.Parse("20060102T150405Z", s)
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
		b.WriteString(highlight.Copy().Inherit(base).Render(raw[loc[0]:loc[1]]))
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
		return highlight.Copy().Inherit(base).Render(display)
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
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
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

	cellStyle := rowStyle.Copy().Inherit(styles.Cell)
	selStyle := cellStyle.Copy().Inherit(styles.Selected)

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
	tagStr := m.highlightCell(getStyle(5), re, tags)
	annRaw := strings.Join(anns, "; ")
	annCount := ""
	if n := len(anns); n > 0 {
		annCount = strconv.FormatInt(int64(n), 16)
	}
	annStr := m.highlightCellMatch(getStyle(6), re, annRaw, annCount)
	descStr := m.highlightCell(getStyle(7), re, t.Description)
	urgStr := getStyle(8).Render(m.formatUrgency(urg, m.urgWidth))

	return atable.Row{
		priStr,
		idStr,
		ageStr,
		dueStr,
		recurStr,
		tagStr,
		annStr,
		descStr,
		urgStr,
	}
}

func (m Model) expandedCellView() string {
	row := m.tbl.Cursor()
	col := m.tbl.ColumnCursor()
	if row < 0 || row >= len(m.tasks) || col < 0 || col > 8 {
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
		if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
			days := int(time.Since(ts).Hours() / 24)
			val = fmt.Sprintf("%dd", days)
		}
	case 3:
		val = ansi.Strip(m.formatDue(t.Due, m.dueWidth))
	case 4:
		val = t.Recur
	case 5:
		val = strings.Join(t.Tags, " ")
	case 6:
		var anns []string
		for _, a := range t.Annotations {
			anns = append(anns, a.Description)
		}
		val = strings.Join(anns, "; ")
	case 7:
		val = t.Description
	case 8:
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
	h := m.windowHeight - 3 // space for two status bars and base expanded line
	if m.cellExpanded {
		h--
	}
	if m.annotating || m.dueEditing || m.prioritySelecting || m.searching || m.descEditing || m.tagsEditing || m.recurEditing || m.filterEditing || m.addingTask {
		h--
	}
	if h < 1 {
		h = 1
	}
	m.tbl.SetHeight(h)
}

func dueText(s string) string {
	if s == "" {
		return ""
	}
	ts, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		return s
	}
	days := int(time.Until(ts).Hours() / 24)
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	case -1:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd", days)
	}
}

func (m *Model) computeColumnWidths() {
	maxID := 1
	maxAge := 0
	maxUrg := 0
	maxDue := 0
	maxRecur := 1
	maxTags := 0
	maxAnn := 1
	for _, t := range m.tasks {
		if l := len(strconv.Itoa(t.ID)); l > maxID {
			maxID = l
		}
		age := ""
		if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
			age = fmt.Sprintf("%dd", int(time.Since(ts).Hours()/24))
		}
		if l := len(age); l > maxAge {
			maxAge = l
		}
		urg := fmt.Sprintf("%.1f", t.Urgency)
		if l := len(urg); l > maxUrg {
			maxUrg = l
		}
		due := dueText(t.Due)
		if l := len(due); l > maxDue {
			maxDue = l
		}
		if l := len(t.Recur); l > maxRecur {
			maxRecur = l
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

	total := m.tbl.Width()
	if total == 0 {
		total = 80
	}
	base := m.idWidth + m.priWidth + m.ageWidth + m.dueWidth + m.recurWidth + m.tagsWidth + m.annWidth + m.urgWidth
	base += 8 // spaces between columns
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

func centerLines(s string, width int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	for i, l := range lines {
		lines[i] = style.Render(l)
	}
	return strings.Join(lines, "\n")
}
