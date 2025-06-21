package ui

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tasksamurai/internal"
	atable "tasksamurai/internal/atable"
	"tasksamurai/internal/task"
)

var priorityOptions = []string{"H", "M", "L", ""}

const (
	idWidth   = 4
	priWidth  = 4
	ageWidth  = 6
	urgWidth  = 5
	dueWidth  = 10
	tagsWidth = 15
	descWidth = 45
	annWidth  = 20
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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

	dueEditing bool
	dueID      int
	dueDate    time.Time

	searching     bool
	searchInput   textinput.Model
	searchRegex   *regexp.Regexp
	searchMatches []cellMatch
	searchIndex   int

	prioritySelecting bool
	priorityID        int
	priorityIndex     int

	filters []string
	tasks   []task.Task

	undoStack []string

	cellExpanded bool

	windowHeight int

	total      int
	inProgress int
	due        int
}

// editDoneMsg is emitted when the external editor process finishes.
type editDoneMsg struct{ err error }

// editCmd returns a command that edits the task and sends an
// editDoneMsg once the process is complete.
func editCmd(id int) tea.Cmd {
	c := task.EditCmd(id)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err: err} })
}

// New creates a new UI model with the provided rows.
func New(filters []string) (Model, error) {
	m := Model{filters: filters}
	m.annotateInput = textinput.New()
	m.annotateInput.Prompt = "annotation: "
	m.dueDate = time.Now()
	m.searchInput = textinput.New()
	m.searchInput.Prompt = "search: "

	if err := m.reload(); err != nil {
		return Model{}, err
	}

	return m, nil
}

func newTable(rows []atable.Row) (atable.Model, atable.Styles) {
	cols := []atable.Column{
		{Title: "ID", Width: idWidth},
		{Title: "Pri", Width: priWidth},
		{Title: "Age", Width: ageWidth},
		{Title: "Urg", Width: urgWidth},
		{Title: "Due", Width: dueWidth},
		{Title: "Tags", Width: tagsWidth},
		{Title: "Description", Width: descWidth},
		{Title: "Annotations", Width: annWidth},
	}
	t := atable.New(
		atable.WithColumns(cols),
		atable.WithRows(rows),
		atable.WithFocused(true),
		atable.WithShowHeaders(false),
	)
	styles := atable.DefaultStyles()
	styles.Header = styles.Header.Foreground(lipgloss.Color("205"))
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	styles.Cell = styles.Cell.Padding(0, 1)
	t.SetStyles(styles)
	return t, styles
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

	var rows []atable.Row
	m.searchMatches = nil
	for i, tsk := range tasks {
		rows = append(rows, taskToRowSearch(tsk, m.searchRegex, m.tblStyles, -1))
		if m.searchRegex != nil {
			tags := strings.Join(tsk.Tags, " ")
			if m.searchRegex.MatchString(tags) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 5})
			}
			if m.searchRegex.MatchString(tsk.Description) {
				m.searchMatches = append(m.searchMatches, cellMatch{row: i, col: 6})
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

	m.tasks = tasks
	m.total = task.TotalTasks(tasks)
	m.inProgress = task.InProgressTasks(tasks)
	m.due = task.DueTasks(tasks, time.Now())

	if m.tbl.Columns() == nil {
		m.tbl, m.tblStyles = newTable(rows)
	} else {
		m.tbl.SetRows(rows)
	}
	m.updateSelectionHighlight(-1, m.tbl.Cursor(), 0, m.tbl.ColumnCursor())
	return nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.tbl.SetWidth(msg.Width)
		m.windowHeight = msg.Height
		m.updateTableHeight()
		return m, nil
	case editDoneMsg:
		// Ignore any error and reload tasks once editing completes.
		_ = msg.err
		m.reload()
		return m, nil
	case tea.KeyMsg:
		if m.annotating {
			switch msg.Type {
			case tea.KeyEnter:
				if m.replaceAnnotations {
					task.ReplaceAnnotations(m.annotateID, m.annotateInput.Value())
					m.replaceAnnotations = false
				} else {
					task.Annotate(m.annotateID, m.annotateInput.Value())
				}
				m.annotating = false
				m.annotateInput.Blur()
				m.reload()
				m.updateTableHeight()
				return m, nil
			case tea.KeyEsc:
				m.annotating = false
				m.replaceAnnotations = false
				m.annotateInput.Blur()
				m.updateTableHeight()
				return m, nil
			}
			var cmd tea.Cmd
			m.annotateInput, cmd = m.annotateInput.Update(msg)
			return m, cmd
		}
		if m.dueEditing {
			switch msg.Type {
			case tea.KeyEnter:
				task.SetDueDate(m.dueID, m.dueDate.Format("2006-01-02"))
				m.dueEditing = false
				m.reload()
				m.updateTableHeight()
				return m, nil
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
		if m.prioritySelecting {
			switch msg.Type {
			case tea.KeyEnter:
				task.SetPriority(m.priorityID, priorityOptions[m.priorityIndex])
				m.prioritySelecting = false
				m.reload()
				m.updateTableHeight()
				return m, nil
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
		if m.searching {
			switch msg.Type {
			case tea.KeyEnter:
				re, err := regexp.Compile(m.searchInput.Value())
				if err == nil {
					m.searchRegex = re
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
		switch msg.String() {
		case "H":
			m.showHelp = true
			return m, nil
		case "q", "esc":
			if m.cellExpanded {
				m.cellExpanded = false
				m.updateTableHeight()
				return m, nil
			}
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			if m.searchRegex != nil {
				m.searchRegex = nil
				m.searchMatches = nil
				m.searchIndex = 0
				m.reload()
				return m, nil
			}
			if msg.String() == "q" {
				return m, tea.Quit
			}
			return m, nil
		case "e", "E":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					return m, editCmd(id)
				}
			}
		case "s":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					started := false
					for _, tsk := range m.tasks {
						if tsk.ID == id {
							started = tsk.Start != ""
							break
						}
					}
					if started {
						task.Stop(id)
					} else {
						task.Start(id)
					}
					m.reload()
				}
			}
		case "D":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					task.Done(id)
					for _, tsk := range m.tasks {
						if tsk.ID == id {
							m.undoStack = append(m.undoStack, tsk.UUID)
							break
						}
					}
					m.reload()
				}
			}
		case "U":
			if n := len(m.undoStack); n > 0 {
				uuid := m.undoStack[n-1]
				m.undoStack = m.undoStack[:n-1]
				task.SetStatusUUID(uuid, "pending")
				m.reload()
			}
		case "d":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					m.dueID = id
					m.dueEditing = true
					m.dueDate = time.Now()
					m.updateTableHeight()
					return m, nil
				}
			}
		case "r":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					days := rand.Intn(31) + 7
					due := time.Now().AddDate(0, 0, days).Format("2006-01-02")
					task.SetDueDate(id, due)
					m.reload()
				}
			}
		case "p":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					m.priorityID = id
					m.prioritySelecting = true
					m.priorityIndex = 0
					m.updateTableHeight()
					return m, nil
				}
			}
		case "a":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					m.annotateID = id
					m.annotating = true
					m.replaceAnnotations = false
					m.annotateInput.SetValue("")
					m.annotateInput.Focus()
					m.updateTableHeight()
					return m, nil
				}
			}
		case "A":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					m.annotateID = id
					m.annotating = true
					m.replaceAnnotations = true
					m.annotateInput.SetValue("")
					m.annotateInput.Focus()
					m.updateTableHeight()
					return m, nil
				}
			}
		case "/", "?":
			m.searching = true
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			m.updateTableHeight()
			return m, nil
		case "n":
			if len(m.searchMatches) > 0 {
				m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
				match := m.searchMatches[m.searchIndex]
				prevRow := m.tbl.Cursor()
				prevCol := m.tbl.ColumnCursor()
				m.tbl.SetCursor(match.row)
				m.tbl.SetColumnCursor(match.col)
				m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
				return m, nil
			}
		case "N":
			if len(m.searchMatches) > 0 {
				m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
				match := m.searchMatches[m.searchIndex]
				prevRow := m.tbl.Cursor()
				prevCol := m.tbl.ColumnCursor()
				m.tbl.SetCursor(match.row)
				m.tbl.SetColumnCursor(match.col)
				m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
				return m, nil
			}
		case "enter":
			m.cellExpanded = !m.cellExpanded
			m.updateTableHeight()
			return m, nil
		}
	}

	if m.showHelp {
		return m, nil
	}

	var cmd tea.Cmd
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl, cmd = m.tbl.Update(msg)
	if prevRow != m.tbl.Cursor() || prevCol != m.tbl.ColumnCursor() {
		m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	}
	return m, cmd
}

// View renders the table UI.
func (m Model) View() string {
	if m.showHelp {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.tbl.HelpView(),
			"E: edit task",
			"s: toggle start/stop",
			"D: mark task done",
			"U: undo done",
			"d: set due date",
			"r: random due date",
			"a: annotate task",
			"A: replace annotations",
			"p: set priority",
			"/, ?: search",
			"esc: close help/search",
			"q: quit",
			"H: help", // show help toggle line
		)
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
			m.dueView(),
		)
	}
	if m.prioritySelecting {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.priorityView(),
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

func (m Model) statusLine() string {
	status := fmt.Sprintf("Total:%d InProgress:%d Due:%d | press H for help", m.total, m.inProgress, m.due)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(m.tbl.Width()).
		Render(status)
}

func (m Model) topStatusLine() string {
	header := ""
	cols := m.tbl.Columns()
	if idx := m.tbl.ColumnCursor(); idx >= 0 && idx < len(cols) {
		header = cols[idx].Title
	}
	line := fmt.Sprintf("Task Samurai %s | %s", internal.Version, header)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(m.tbl.Width()).
		Render(line)
}

func taskToRow(t task.Task) atable.Row {
	style := lipgloss.NewStyle()
	if t.Start != "" {
		style = style.Background(lipgloss.Color("6"))
	}

	age := ""
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
		days := int(time.Since(ts).Hours() / 24)
		age = fmt.Sprintf("%dd", days)
	}

	tags := strings.Join(t.Tags, " ")
	urg := fmt.Sprintf("%.1f", t.Urgency)

	var anns []string
	for _, a := range t.Annotations {
		anns = append(anns, a.Description)
	}

	return atable.Row{
		style.Render(strconv.Itoa(t.ID)),
		formatPriority(t.Priority, priWidth),
		style.Render(age),
		style.Render(urg),
		formatDue(t.Due, dueWidth),
		style.Render(tags),
		style.Render(t.Description),
		style.Render(strings.Join(anns, "; ")),
	}
}

// formatDue returns a formatted due date string. Dates due today or tomorrow
// are returned as "today" or "tomorrow" respectively. Past due dates are
// highlighted in red.
func formatDue(s string, width int) string {
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
		style = style.Background(lipgloss.Color("1"))
	}
	return style.Render(val)
}

func formatPriority(p string, width int) string {
	style := lipgloss.NewStyle().Width(width)
	switch p {
	case "L":
		style = style.Background(lipgloss.Color("10"))
	case "M":
		style = style.Background(lipgloss.Color("12"))
	case "H":
		style = style.Background(lipgloss.Color("9"))
	default:
		return p
	}
	return style.Render(p)
}

func (m Model) dueView() string {
	return fmt.Sprintf("due: %s", m.dueDate.Format("2006-01-02"))
}

func (m Model) priorityView() string {
	var parts []string
	for i, p := range priorityOptions {
		label := p
		if label == "" {
			label = "none"
		}
		style := lipgloss.NewStyle()
		if i == m.priorityIndex {
			style = style.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
		}
		parts = append(parts, style.Render(label))
	}
	return "priority: " + strings.Join(parts, " ")
}

func highlightCell(base lipgloss.Style, re *regexp.Regexp, raw string) string {
	if re == nil || !re.MatchString(raw) {
		return base.Render(raw)
	}

	highlight := lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("21"))
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

func taskToRowSearch(t task.Task, re *regexp.Regexp, styles atable.Styles, selectedCol int) atable.Row {
	rowStyle := lipgloss.NewStyle()
	if t.Start != "" {
		rowStyle = rowStyle.Background(lipgloss.Color("6"))
	}

	age := ""
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
		days := int(time.Since(ts).Hours() / 24)
		age = fmt.Sprintf("%dd", days)
	}

	tags := strings.Join(t.Tags, " ")
	urg := fmt.Sprintf("%.1f", t.Urgency)

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

	idStr := getStyle(0).Render(strconv.Itoa(t.ID))
	priStr := formatPriority(t.Priority, priWidth)
	ageStr := getStyle(2).Render(age)
	dueStr := formatDue(t.Due, dueWidth)
	urgStr := getStyle(3).Render(urg)

	tagStr := highlightCell(getStyle(5), re, tags)
	descStr := highlightCell(getStyle(6), re, t.Description)
	annRaw := strings.Join(anns, "; ")
	annStr := highlightCell(getStyle(7), re, annRaw)

	return atable.Row{
		idStr,
		priStr,
		ageStr,
		urgStr,
		dueStr,
		tagStr,
		descStr,
		annStr,
	}
}

func (m Model) expandedCellView() string {
	row := m.tbl.Cursor()
	col := m.tbl.ColumnCursor()
	if row < 0 || row >= len(m.tasks) || col < 0 {
		return ""
	}
	t := m.tasks[row]
	var val string
	switch col {
	case 0:
		val = strconv.Itoa(t.ID)
	case 1:
		val = ansi.Strip(formatPriority(t.Priority, priWidth))
	case 2:
		if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
			days := int(time.Since(ts).Hours() / 24)
			val = fmt.Sprintf("%dd", days)
		}
	case 3:
		val = fmt.Sprintf("%.1f", t.Urgency)
	case 4:
		val = ansi.Strip(formatDue(t.Due, dueWidth))
	case 5:
		val = strings.Join(t.Tags, " ")
	case 6:
		val = t.Description
	case 7:
		var anns []string
		for _, a := range t.Annotations {
			anns = append(anns, a.Description)
		}
		val = strings.Join(anns, "; ")
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
		rows[prevRow] = taskToRowSearch(m.tasks[prevRow], m.searchRegex, m.tblStyles, -1)
	}
	if newRow >= 0 && newRow < len(rows) {
		rows[newRow] = taskToRowSearch(m.tasks[newRow], m.searchRegex, m.tblStyles, newCol)
	}
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
	if m.annotating || m.dueEditing || m.prioritySelecting || m.searching {
		h--
	}
	if h < 1 {
		h = 1
	}
	m.tbl.SetHeight(h)
}
