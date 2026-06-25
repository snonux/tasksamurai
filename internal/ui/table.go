package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"codeberg.org/snonux/tasksamurai/internal"
	atable "codeberg.org/snonux/tasksamurai/internal/atable"
	"codeberg.org/snonux/tasksamurai/internal/task"
	uihelp "codeberg.org/snonux/tasksamurai/internal/ui/help"
)

var priorityOptions = []string{"H", "M", "L", ""}

const taskOperationTimeout = 30 * time.Second

var (
	urlRegex         = regexp.MustCompile(`https?://\S+`)
	searchRegexCache = make(map[string]*regexp.Regexp, 16)
	searchRegexMu    sync.RWMutex
)

type cellMatch struct {
	row int
	col int
}

type undoRestore struct {
	uuid   string
	status string
}

type undoAction struct {
	label    string
	restores []undoRestore
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
	showTaskDetail              bool
	currentTaskDetailUUID       string
	currentTaskDetailFallbackID int
	detailSearching             bool
	detailSearchInput           textinput.Model
	detailSearchRegex           *regexp.Regexp
	detailFieldIndex            int  // currently selected field (-1 = none)
	detailBlinkField            int  // field currently blinking (-1 = none)
	detailBlinkOn               bool // whether the blink is currently on
	detailBlinkCount            int  // number of blink cycles completed so far
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

// detailEditState holds detail-overlay state that belongs to the external
// description editor flow instead of the overlay itself.
type detailEditState struct {
	detailDescEditing bool
}

// ultraModeState holds ultra-mode lifecycle flags that are separate from the
// cursor, search, and filter state.
type ultraModeState struct {
	ultraStartup bool
}

// helpState holds help-screen visibility and viewport state.
type helpState struct {
	showHelp     bool
	helpViewport viewport.Model
}

// shellState holds the Taskwarrior command prompt and captured output panel.
type shellState struct {
	shellActive         bool
	shellInput          textinput.Model
	shellHistory        []string
	shellOutputVisible  bool
	shellOutputTitle    string
	shellOutputViewport viewport.Model
	shellCompletion     task.CompletionSources
	shellCompletionLoad bool
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
	recurSeries  bool
	recurRoot    string
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

	blinkState      // row blink animation (see blinkState)
	searchState     // task-table and help-screen search (see searchState)
	detailViewState // task detail overlay (see detailViewState)
	ultraState      // ultra mode task list and search state (see ultraState)
	detailEditState // detail-overlay external description editor state
	ultraModeState  // ultra-mode lifecycle flags
	helpState       // help-screen viewport state
	shellState      // Taskwarrior command prompt and output panel
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

	filters           []string
	tasks             []task.Task
	undoStack         []undoAction
	browserCmd        string
	agentFilterHotkey string
	taskwarrior       task.Taskwarrior

	theme        Theme
	defaultTheme Theme
	disco        bool // disco mode changes theme on every task modification

	statusMsg string // temporary status message shown in status bar

	taskContext       context.Context
	cancelTaskContext context.CancelFunc
}

var _ tea.Model = (*Model)(nil)

// editDoneMsg is emitted when the external editor process finishes.
type editDoneMsg struct{ err error }

// descEditDoneMsg is emitted when the external editor for description finishes.
type descEditDoneMsg struct {
	err      error
	tempFile string
}

type descEditLaunchMsg struct {
	tempFile string
}

type shellDoneMsg struct {
	result     task.RunResult
	err        error
	selectedID int
}

type shellCompletionMsg struct {
	sources task.CompletionSources
}

type openURLDoneMsg struct {
	err    error
	taskID int
}

type blinkMsg struct{}

type descriptionTempFile interface {
	Name() string
	WriteString(string) (int, error)
	Close() error
}

type reloadData struct {
	tasks          []task.Task
	ultraFilterIDs []int
}

func (m *Model) initTaskContext() {
	if m.taskContext != nil && m.cancelTaskContext != nil {
		return
	}
	m.taskContext, m.cancelTaskContext = context.WithCancel(context.Background())
}

func (m *Model) cancelTaskOperations() {
	if m.cancelTaskContext != nil {
		m.cancelTaskContext()
	}
}

func (m *Model) taskOperationContext() (context.Context, context.CancelFunc) {
	m.initTaskContext()
	return context.WithTimeout(m.taskContext, taskOperationTimeout)
}

// blinkInterval controls how quickly the row flashes when a task changes.
// A shorter interval results in a faster blink.
const blinkInterval = 150 * time.Millisecond

// blinkCycles is the number of times to blink before stopping.
// The total blink duration is blinkInterval * blinkCycles.
const blinkCycles = 8

func prepareDescriptionTempFile(description string, newTempFile func() (descriptionTempFile, error)) (string, error) {
	tmpFile, err := newTempFile()
	if err != nil {
		return "", err
	}

	tmpPath := tmpFile.Name()

	_, writeErr := tmpFile.WriteString(description)
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}

	return tmpPath, nil
}

// editCmd returns a command that edits the task and sends an
// editDoneMsg once the process is complete.
func (m *Model) editCmd(id int) tea.Cmd {
	c := m.taskwarriorClient().EditCmd(id)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err: err} })
}

// editDescriptionCmd returns a command that prepares the description temp file.
func editDescriptionCmd(description string) tea.Cmd {
	return func() tea.Msg {
		tmpPath, err := prepareDescriptionTempFile(description, func() (descriptionTempFile, error) {
			return os.CreateTemp("", "tasksamurai-desc-*.txt")
		})
		if err != nil {
			return descEditDoneMsg{err: err, tempFile: ""}
		}

		return descEditLaunchMsg{tempFile: tmpPath}
	}
}

func launchDescriptionEditorCmd(tmpPath string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	c := exec.Command(editor, tmpPath)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return descEditDoneMsg{err: err, tempFile: tmpPath}
	})
}

func blinkCmd() tea.Cmd {
	return tea.Tick(blinkInterval, func(time.Time) tea.Msg { return blinkMsg{} })
}

func (m *Model) taskwarriorClient() task.Taskwarrior {
	if isNilTaskwarrior(m.taskwarrior) {
		panic("ui.Model Taskwarrior client is nil; use ui.New or ui.NewWithTaskwarrior")
	}
	return m.taskwarrior
}

func isNilTaskwarrior(tw task.Taskwarrior) bool {
	if tw == nil {
		return true
	}

	value := reflect.ValueOf(tw)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// clearEditingModes ensures only one editing mode is active at a time
func (m *Model) clearEditingModes() {
	m.annotating = false
	m.descEditing = false
	m.tagsEditing = false
	m.dueEditing = false
	m.recurEditing = false
	m.recurSeries = false
	m.recurRoot = ""
	m.projEditing = false
	m.filterEditing = false
	m.addingTask = false
	m.searching = false
	m.shellActive = false
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
					m.pushUndoAction("done", []undoRestore{{uuid: tsk.UUID, status: "pending"}})
					break
				}
			}
			ctx, cancel := m.taskOperationContext()
			err := m.taskwarriorClient().DoneContext(ctx, id)
			cancel()
			if err != nil {
				m.showError(err)
			}
		}
		m.blinkID = 0
		m.blinkMarkDone = false
		m.reloadAndReport()
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
	return NewWithTaskwarrior(filters, browserCmd, task.NewTaskwarrior())
}

// NewWithTaskwarrior creates a UI model using the provided Taskwarrior client.
func NewWithTaskwarrior(filters []string, browserCmd string, tw task.Taskwarrior) (Model, error) {
	if isNilTaskwarrior(tw) {
		return Model{}, errors.New("taskwarrior client is nil")
	}
	m := Model{filters: filters, browserCmd: browserCmd, agentFilterHotkey: "3", taskwarrior: tw, blinkState: blinkState{blinkEnabled: true}}
	m.initTaskContext()
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
	m.shellInput = textinput.New()
	m.shellInput.Prompt = "task "

	m.defaultTheme = DefaultTheme()
	m.theme = m.defaultTheme

	if err := m.reload(); err != nil {
		m.cancelTaskOperations()
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
	data, err := m.fetchTasks()
	if err != nil {
		return err
	}

	m.processTasks(&data)
	m.renderTasks(data)
	return nil
}

func (m *Model) fetchTasks() (reloadData, error) {
	// Always show only pending tasks by default.
	filters := append([]string(nil), m.filters...)
	filters = append(filters, "status:pending")
	ctx, cancel := m.taskOperationContext()
	defer cancel()

	tasks, err := m.taskwarriorClient().Export(ctx, filters...)
	if err != nil {
		return reloadData{}, err
	}

	m.taskwarriorClient().SortTasks(tasks)
	return reloadData{
		tasks:          tasks,
		ultraFilterIDs: m.ultraFilteredTaskIDs(),
	}, nil
}

func (m *Model) processTasks(data *reloadData) {
	m.tasks = data.tasks
	m.total = m.taskwarriorClient().TotalTasks(data.tasks)
	m.inProgress = m.taskwarriorClient().InProgressTasks(data.tasks)
	m.due = m.taskwarriorClient().DueTasks(data.tasks, time.Now())

	if m.showTaskDetail {
		m.refreshCurrentTaskDetail()
	}

	m.computeColumnWidths()

	if m.ultraSearchRegex != nil {
		m.ultraFiltered = m.ultraFilteredIndexes(m.ultraSearchRegex)
	} else {
		m.rebuildUltraFiltered(data.ultraFilterIDs)
	}
}

func (m *Model) renderTasks(data reloadData) {
	rows := m.buildTaskRows(data.tasks)
	if m.tbl.Columns() == nil {
		m.tbl, m.tblStyles = m.newTable(rows)
	} else {
		m.tbl.SetRows(rows)
	}
	m.reconcileUltraSelection()
	m.updateSelectionHighlight(-1, m.tbl.Cursor(), 0, m.tbl.ColumnCursor())
}

func (m *Model) buildTaskRows(tasks []task.Task) []atable.Row {
	rows := make([]atable.Row, 0, len(tasks))
	m.searchMatches = nil
	for i, tsk := range tasks {
		rows = append(rows, m.taskToRowSearch(tsk, m.searchRegex, m.tblStyles, -1))
		if m.searchRegex == nil {
			continue
		}
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
	if len(m.searchMatches) > 0 {
		m.searchIndex = 0
	}
	return rows
}

func (m *Model) reloadAndReport() bool {
	if err := m.reload(); err != nil {
		m.showError(fmt.Errorf("reloading tasks: %w", err))
		return false
	}
	return true
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Handle resize in all modes, including during input
		return m.handleWindowResize(msg)
	case editDoneMsg:
		return m.handleEditDone(msg)
	case descEditLaunchMsg:
		return m, launchDescriptionEditorCmd(msg.tempFile)
	case descEditDoneMsg:
		return m.handleDescEditDone(msg)
	case shellDoneMsg:
		return m.handleShellDone(msg)
	case shellCompletionMsg:
		return m.handleShellCompletion(msg)
	case openURLDoneMsg:
		return m.handleOpenURLDone(msg)
	case blinkMsg:
		return m.handleBlinkMsg()
	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil
	case tea.KeyPressMsg:
		// Handle blinking state first
		if m.blinkID != 0 {
			return m.handleBlinkingState(msg)
		}
		if m.shellOutputVisible {
			return m.handleShellOutputMode(msg)
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
	m.shellInput.SetWidth(msg.Width)
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
	if m.shellOutputVisible {
		height := msg.Height - 2
		if height < 1 {
			height = 1
		}
		m.shellOutputViewport.SetWidth(msg.Width)
		m.shellOutputViewport.SetHeight(height)
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
					m.pushUndoAction("done", []undoRestore{{uuid: tsk.UUID, status: "pending"}})
					break
				}
			}
			ctx, cancel := m.taskOperationContext()
			err := m.taskwarriorClient().DoneContext(ctx, id)
			cancel()
			if err != nil {
				m.showError(err)
			}
		}
		m.reloadAndReport()
		return m, nil
	}

	return m, blinkCmd()
}

// View renders the table UI.
func (m *Model) View() tea.View {
	var content string
	switch {
	case m.showHelp:
		m.updateHelpContent()
		content = m.renderHelpScreen()
	case m.showTaskDetail:
		content = m.renderDetailScreen()
	case m.shellOutputVisible:
		content = m.renderShellOutputScreen()
	case m.showUltra:
		content = m.renderUltraScreen()
	default:
		content = m.renderTableScreen()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// appendInlineInputOverlay appends whichever active inline-editing widget
// (annotate, due, priority, desc, tags, recur, project, filter, add, search)
// should be displayed below the table. At most one is active at a time.
func (m *Model) appendInlineInputOverlay(view string) string {
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
	case m.shellActive:
		overlay = m.shellInput.View()
	}

	if overlay != "" {
		view = lipgloss.JoinVertical(lipgloss.Left, view, overlay)
	}
	return view
}

func (m *Model) renderDetailScreen() string {
	return m.renderTaskDetail()
}

func (m *Model) renderUltraScreen() string {
	return m.renderUltraModus()
}

func (m *Model) renderTableScreen() string {
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
	return m.appendInlineInputOverlay(view)
}

// updateHelpContent updates the help viewport content
func (m *Model) updateHelpContent() {
	m.helpViewport.SetContent(m.activeHelpContent())
}

// buildHelpContent builds the help content
func (m *Model) buildHelpContent() string {
	return uihelp.Render(m.helpSections(), m.helpPalette(), m.helpSearchRegex)
}

// renderHelpScreen renders the help screen with optional search highlighting
func (m *Model) renderHelpScreen() string {
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

// getHelpLines returns searchable help content as plain text lines
func (m *Model) getHelpLines() []string {
	return uihelp.Lines(m.activeHelpSections())
}

func (m *Model) activeHelpContent() string {
	if m.showUltra {
		return m.buildUltraHelpContent()
	}
	return m.buildHelpContent()
}

func (m *Model) activeHelpSections() []uihelp.Section {
	if m.showUltra {
		return m.ultraHelpSections()
	}
	return m.helpSections()
}

func (m *Model) helpPalette() uihelp.Palette {
	return uihelp.Palette{
		HeaderFG: m.theme.HeaderFG,
		HeaderBG: m.theme.SelectedBG,
		KeyFG:    m.theme.SelectedFG,
		DescFG:   "250",
		SearchFG: m.theme.SearchFG,
		SearchBG: m.theme.SearchBG,
	}
}

func (m *Model) helpSections() []uihelp.Section {
	return []uihelp.Section{
		{
			Title: "Navigation",
			Items: []uihelp.Item{
				{Key: "↑/k, ↓/j", Desc: "move up/down"},
				{Key: "←/h, →/l", Desc: "move left/right"},
				{Key: "0, g, Home", Desc: "go to start"},
				{Key: "G, End", Desc: "go to end"},
				{Key: "pgup/pgdn, b", Desc: "page up/down"},
				{Key: "1", Desc: "jump to random task"},
				{Key: "2", Desc: "jump to random task (no due date)"},
			},
		},
		{
			Title: "Task Management",
			Items: []uihelp.Item{
				{Key: "Enter", Desc: "view task details"},
				{Key: "+", Desc: "add new task"},
				{Key: "e, E", Desc: "edit entire task"},
				{Key: "d", Desc: "mark task done"},
				{Key: "D", Desc: "delete task/recurring series"},
				{Key: "U", Desc: "undo last done/delete"},
				{Key: "s", Desc: "start/stop task"},
			},
		},
		{
			Title: "Task Fields",
			Items: []uihelp.Item{
				{Key: "i", Desc: "edit current field"},
				{Key: "p", Desc: "set priority"},
				{Key: "w, W", Desc: "set/remove due date"},
				{Key: "r", Desc: "set random due date"},
				{Key: "R", Desc: "edit recurrence"},
				{Key: "ctrl+r", Desc: "edit recurring series recurrence"},
				{Key: "t", Desc: "edit tags"},
				{Key: "J", Desc: "edit project"},
				{Key: "T", Desc: "convert first tag to project"},
				{Key: "a, A", Desc: "add/replace annotations"},
				{Key: "o", Desc: "open URL from description"},
			},
		},
		{
			Title: "View & Search",
			Items: []uihelp.Item{
				{Key: m.agentFilterHotkeyLabel(), Desc: "toggle +agent/-agent filter"},
				{Key: "f", Desc: "change filter"},
				{Key: ":", Desc: "run task command prompt"},
				{Key: ";", Desc: "run task command prompt for selected task"},
				{Key: "/, ?", Desc: "search"},
				{Key: "n, N", Desc: "next/previous match"},
				{Key: "space", Desc: "refresh tasks"},
			},
		},
		{
			Title: "Appearance",
			Items: []uihelp.Item{
				{Key: "c, C", Desc: "random/reset theme"},
				{Key: "x", Desc: "toggle disco mode"},
				{Key: "B", Desc: "toggle blinking"},
			},
		},
		{
			Title: "General",
			Items: []uihelp.Item{
				{Key: "H", Desc: "toggle help"},
				{Key: "ESC", Desc: "close dialogs/cancel"},
				{Key: "q", Desc: "quit"},
			},
		},
	}
}

func (m *Model) statusLine() string {
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

func (m *Model) topStatusLine() string {
	line := fmt.Sprintf("Task Samurai %s", internal.Version)
	if len(m.filters) > 0 {
		line += " | filter: " + strings.Join(m.filters, " ")
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(m.tbl.Width()).
		Render(line)
}

// formatDue returns a formatted due date string. Dates due today or tomorrow
// are returned as "today" or "tomorrow" respectively. Past due dates are
// highlighted in red.
func (m *Model) formatDue(s string, width int) string {
	val, days, ok := dueTextAndDays(s)
	if val == "" {
		return ""
	}
	if !ok {
		return val
	}

	style := lipgloss.NewStyle().Width(width)
	if days < 0 {
		style = style.Background(lipgloss.Color(m.theme.OverdueBG))
	}
	return style.Render(val)
}

func (m *Model) formatPriority(p string, width int) string {
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

func (m *Model) formatUrgency(u string, width int) string {
	if w := width - len(u); w > 0 {
		u = strings.Repeat(" ", w) + u
	}
	return u
}

func (m *Model) dueView(showLabel bool) string {
	if showLabel {
		return fmt.Sprintf("due: %s", m.dueDate.Format("2006-01-02"))
	}
	return m.dueDate.Format("2006-01-02")
}

func (m *Model) priorityView(showLabel bool) string {
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

func (m *Model) highlightCell(base lipgloss.Style, re *regexp.Regexp, raw string) string {
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

func (m *Model) highlightCellMatch(base lipgloss.Style, re *regexp.Regexp, raw, display string) string {
	if re != nil && re.MatchString(raw) {
		highlight := lipgloss.NewStyle().Background(lipgloss.Color(m.theme.SearchBG)).Foreground(lipgloss.Color(m.theme.SearchFG))
		return highlight.Inherit(base).Render(display)
	}
	return base.Render(display)
}

func (m *Model) taskToRowSearch(t task.Task, re *regexp.Regexp, styles atable.Styles, selectedCol int) atable.Row {
	rowStyle := lipgloss.NewStyle()
	if t.Start != "" {
		rowStyle = rowStyle.Background(lipgloss.Color(m.theme.StartBG))
	}
	if t.ID == m.blinkID && m.blinkOn {
		rowStyle = rowStyle.Reverse(true)
	}

	age, _ := taskAgeText(t.Entry)

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

func (m *Model) expandedCellView() string {
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
		val, _ = taskAgeText(t.Entry)
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
	if m.annotating || m.dueEditing || m.prioritySelecting || m.searching || m.descEditing || m.tagsEditing || m.recurEditing || m.projEditing || m.filterEditing || m.addingTask || m.shellActive {
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
		age, _ := taskAgeText(t.Entry)
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
// When u is true, q quits the application immediately rather than returning
// to the table view, because there is no table view to return to. esc always
// cancels/closes overlays instead of quitting.
func (m *Model) SetUltra(u bool) {
	m.showUltra = u
	m.ultraStartup = u
}

// SetAgentFilterHotkey configures the key that toggles the agent filter.
// The chosen key must not collide with any existing command in normal or
// ultra mode. If it does, the current hotkey is left unchanged and an error is
// returned so callers can surface the conflict.
func (m *Model) SetAgentFilterHotkey(key string) error {
	key = normalizeAgentFilterHotkey(key)
	if key == "" {
		return nil
	}
	if err := validateAgentFilterHotkey(key); err != nil {
		return err
	}
	m.agentFilterHotkey = key
	return nil
}

func (m *Model) agentFilterHotkeyLabel() string {
	if strings.TrimSpace(m.agentFilterHotkey) == "" {
		return "3"
	}
	return m.agentFilterHotkey
}

func validateAgentFilterHotkey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if sharedKeyBindingContains(key) || reservedAgentHotkeyContains(key) {
		return fmt.Errorf("agent hotkey %q conflicts with an existing command", key)
	}
	return nil
}

func sharedKeyBindingContains(key string) bool {
	for _, binding := range sharedKeyBindings {
		for _, candidate := range binding.keys {
			if candidate == key {
				return true
			}
		}
	}
	return false
}

func reservedAgentHotkeyContains(key string) bool {
	_, ok := reservedAgentHotkeys[key]
	return ok
}

func normalizeAgentFilterHotkey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || len(key) == 1 {
		return key
	}
	switch strings.ToLower(key) {
	case "down":
		return "down"
	case "end":
		return "end"
	case "enter":
		return "enter"
	case "esc", "escape":
		return "esc"
	case "home":
		return "home"
	case "left":
		return "left"
	case "pgdn", "pgdown":
		return "pgdn"
	case "pgup":
		return "pgup"
	case "right":
		return "right"
	case "space":
		return "space"
	case "tab":
		return "tab"
	case "up":
		return "up"
	}
	return key
}

var reservedAgentHotkeys = map[string]struct{}{
	"+":      {},
	"0":      {},
	"1":      {},
	"2":      {},
	"A":      {},
	"B":      {},
	"C":      {},
	"E":      {},
	"G":      {},
	"H":      {},
	"J":      {},
	"N":      {},
	"R":      {},
	"ctrl+r": {},
	"T":      {},
	"U":      {},
	"W":      {},
	"a":      {},
	"b":      {},
	"c":      {},
	"d":      {},
	"down":   {},
	"e":      {},
	"end":    {},
	"enter":  {},
	"esc":    {},
	"f":      {},
	"g":      {},
	"home":   {},
	"i":      {},
	"h":      {},
	"j":      {},
	"k":      {},
	"l":      {},
	"left":   {},
	"n":      {},
	"o":      {},
	"p":      {},
	"pgdn":   {},
	"pgdown": {},
	"pgup":   {},
	"q":      {},
	"r":      {},
	"right":  {},
	"s":      {},
	"space":  {},
	"t":      {},
	"u":      {},
	"up":     {},
	"w":      {},
	"x":      {},
	"?":      {},
	"/":      {},
}
