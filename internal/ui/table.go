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

	atable "tasksamurai/internal/atable"
	"tasksamurai/internal/task"
)

var priorityOptions = []string{"H", "M", "L", ""}

func init() {
	rand.Seed(time.Now().UnixNano())
}

type cellMatch struct {
	row int
	col int
}

// Model wraps a Bubble Tea table.Model to display tasks.

type Model struct {
	tbl      atable.Model
	showHelp bool

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

func newTable(rows []atable.Row) atable.Model {
	cols := []atable.Column{
		{Title: "ID", Width: 4},
		{Title: "Pri", Width: 4},
		{Title: "Age", Width: 6},
		{Title: "Due", Width: 10},
		{Title: "Urg", Width: 5},
		{Title: "Tags", Width: 15},
		{Title: "Description", Width: 45},
		{Title: "Annotations", Width: 20},
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
	return t
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
		rows = append(rows, taskToRowSearch(tsk, m.searchRegex))
		if m.searchRegex != nil {
			tags := strings.Join(tsk.Tags, ",")
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
		m.tbl = newTable(rows)
	} else {
		m.tbl.SetRows(rows)
	}
	return nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.tbl.SetWidth(msg.Width)
		// Leave room for two status bars and the optional annotation
		// input line.
		m.tbl.SetHeight(msg.Height - 3)
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
				return m, nil
			case tea.KeyEsc:
				m.annotating = false
				m.replaceAnnotations = false
				m.annotateInput.Blur()
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
				return m, nil
			case tea.KeyEsc:
				m.dueEditing = false
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
				return m, nil
			case tea.KeyEsc:
				m.prioritySelecting = false
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
				if len(m.searchMatches) > 0 {
					match := m.searchMatches[m.searchIndex]
					m.tbl.SetCursor(match.row)
					m.tbl.SetColumnCursor(match.col)
				}
				return m, nil
			case tea.KeyEsc:
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "h":
			m.showHelp = true
			return m, nil
		case "q", "esc":
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
					return m, nil
				}
			}
		case "/", "?":
			m.searching = true
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			return m, nil
		case "n":
			if len(m.searchMatches) > 0 {
				m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
				match := m.searchMatches[m.searchIndex]
				m.tbl.SetCursor(match.row)
				m.tbl.SetColumnCursor(match.col)
				return m, nil
			}
		case "N":
			if len(m.searchMatches) > 0 {
				m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
				match := m.searchMatches[m.searchIndex]
				m.tbl.SetCursor(match.row)
				m.tbl.SetColumnCursor(match.col)
				return m, nil
			}
		}
	}

	if m.showHelp {
		return m, nil
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
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
			"q: quit",
			"h: help", // show help toggle line
		)
	}
	view := lipgloss.JoinVertical(lipgloss.Left,
		m.topStatusLine(),
		m.tbl.View(),
		m.statusLine(),
	)
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
	status := fmt.Sprintf("Total:%d InProgress:%d Due:%d", m.total, m.inProgress, m.due)
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
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(m.tbl.Width()).
		Render(header)
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

	tags := strings.Join(t.Tags, ",")
	urg := fmt.Sprintf("%.1f", t.Urgency)

	var anns []string
	for _, a := range t.Annotations {
		anns = append(anns, a.Description)
	}

	return atable.Row{
		style.Render(strconv.Itoa(t.ID)),
		formatPriority(t.Priority),
		style.Render(age),
		formatDue(t.Due),
		style.Render(urg),
		style.Render(tags),
		style.Render(t.Description),
		style.Render(strings.Join(anns, "; ")),
	}
}

func formatDue(s string) string {
	if s == "" {
		return ""
	}
	ts, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		return s
	}

	days := int(time.Until(ts).Hours() / 24)
	val := fmt.Sprintf("%dd", days)
	style := lipgloss.NewStyle()
	if days < 0 {
		style = style.Background(lipgloss.Color("1"))
	}
	return style.Render(val)
}

func formatPriority(p string) string {
	style := lipgloss.NewStyle()
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

func highlightCell(rendered string, re *regexp.Regexp, raw string) string {
	if re == nil || !re.MatchString(raw) {
		return rendered
	}
	style := lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("21"))
	return style.Render(rendered)
}

func taskToRowSearch(t task.Task, re *regexp.Regexp) atable.Row {
	style := lipgloss.NewStyle()
	if t.Start != "" {
		style = style.Background(lipgloss.Color("6"))
	}

	age := ""
	if ts, err := time.Parse("20060102T150405Z", t.Entry); err == nil {
		days := int(time.Since(ts).Hours() / 24)
		age = fmt.Sprintf("%dd", days)
	}

	tags := strings.Join(t.Tags, ",")
	urg := fmt.Sprintf("%.1f", t.Urgency)

	var anns []string
	for _, a := range t.Annotations {
		anns = append(anns, a.Description)
	}

	idStr := style.Render(strconv.Itoa(t.ID))
	priStr := formatPriority(t.Priority)
	ageStr := style.Render(age)
	dueStr := formatDue(t.Due)
	urgStr := style.Render(urg)
	tagStr := style.Render(tags)
	descStr := style.Render(t.Description)
	annRaw := strings.Join(anns, "; ")
	annStr := style.Render(annRaw)

	tagStr = highlightCell(tagStr, re, tags)
	descStr = highlightCell(descStr, re, t.Description)
	annStr = highlightCell(annStr, re, annRaw)

	return atable.Row{
		idStr,
		priStr,
		ageStr,
		dueStr,
		urgStr,
		tagStr,
		descStr,
		annStr,
	}
}
