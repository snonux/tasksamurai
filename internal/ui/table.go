package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	atable "tasksamurai/internal/atable"
	"tasksamurai/internal/task"
)

// Model wraps a Bubble Tea table.Model to display tasks.
type Model struct {
	tbl      atable.Model
	showHelp bool

	filter string
	tasks  []task.Task

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
func New(filter string) (Model, error) {
	m := Model{filter: filter}

	if err := m.reload(); err != nil {
		return Model{}, err
	}

	return m, nil
}

func newTable(rows []atable.Row) atable.Model {
	cols := []atable.Column{
		{Title: "ID", Width: 4},
		{Title: "Task", Width: 30},
		{Title: "Active", Width: 6},
		{Title: "Age", Width: 6},
		{Title: "Pri", Width: 4},
		{Title: "Tags", Width: 15},
		{Title: "Recur", Width: 6},
		{Title: "Due", Width: 10},
		{Title: "Urg", Width: 5},
		{Title: "Annotations", Width: 20},
	}
	t := atable.New(
		atable.WithColumns(cols),
		atable.WithRows(rows),
		atable.WithFocused(true),
	)
	styles := atable.DefaultStyles()
	styles.Header = styles.Header.Foreground(lipgloss.Color("205"))
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	styles.Cell = styles.Cell.Padding(0, 1)
	t.SetStyles(styles)
	return t
}

func (m *Model) reload() error {
	filters := append(strings.Fields(m.filter), "status:pending")
	tasks, err := task.Export(filters...)
	if err != nil {
		return err
	}

	var rows []atable.Row
	var filtered []task.Task
	for _, tsk := range tasks {
		if tsk.Status == "completed" {
			continue
		}
		filtered = append(filtered, tsk)
		rows = append(rows, taskToRow(tsk))
	}

	m.tasks = filtered
	m.total = task.TotalTasks(filtered)
	m.inProgress = task.InProgressTasks(filtered)
	m.due = task.DueTasks(filtered, time.Now())

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
		m.tbl.SetHeight(msg.Height - 2)
		return m, nil
	case editDoneMsg:
		// Ignore any error and reload tasks once editing completes.
		_ = msg.err
		m.reload()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.showHelp = true
			return m, nil
		case "q":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			return m, tea.Quit
		case "E":
			if row := m.tbl.SelectedRow(); row != nil {
				if id, err := strconv.Atoi(row[0]); err == nil {
					return m, editCmd(id)
				}
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
			"q: quit",
			"?: help", // show help toggle line
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.tbl.View(),
		m.statusLine(),
	)
}

func (m Model) statusLine() string {
	status := fmt.Sprintf("Total:%d InProgress:%d Due:%d", m.total, m.inProgress, m.due)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Render(status)
}

func taskToRow(t task.Task) atable.Row {
	active := ""
	if t.Start != "" {
		active = "yes"
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
		strconv.Itoa(t.ID),
		t.Description,
		active,
		age,
		formatPriority(t.Priority),
		tags,
		t.Recur,
		formatDue(t.Due),
		urg,
		strings.Join(anns, "; "),
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
