package ui

import (
	"fmt"
	"math/rand"
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

// Model wraps a Bubble Tea table.Model to display tasks.
type Model struct {
	tbl      atable.Model
	showHelp bool

	annotating         bool
	annotateID         int
	annotateInput      textinput.Model
	replaceAnnotations bool

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
	m.annotateInput = textinput.New()
	m.annotateInput.Prompt = "annotation: "

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
	filters := append(strings.Fields(m.filter), "status:pending")
	tasks, err := task.Export(filters...)
	if err != nil {
		return err
	}

	var filtered []task.Task
	for _, tsk := range tasks {
		if tsk.Status == "completed" {
			continue
		}
		filtered = append(filtered, tsk)
	}

	task.SortTasks(filtered)

	var rows []atable.Row
	for _, tsk := range filtered {
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
		case "d":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					task.Done(id)
					m.reload()
				}
			}
		case "D":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					task.Delete(id)
					m.reload()
				}
			}
		case "r":
			if row := m.tbl.SelectedRow(); row != nil {
				idStr := ansi.Strip(row[0])
				if id, err := strconv.Atoi(idStr); err == nil {
					days := rand.Intn(31) + 7 // 7 to 37 days
					due := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC().Format("20060102T150405Z")
					task.SetDueDate(id, due)
					m.reload()
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
			"d: mark task done",
			"D: delete task",
			"r: random due date",
			"a: annotate task",
			"A: replace annotations",
			"q: quit",
			"?: help", // show help toggle line
		)
	}
	view := lipgloss.JoinVertical(lipgloss.Left,
		m.tbl.View(),
		m.statusLine(),
	)
	if m.annotating {
		view = lipgloss.JoinVertical(lipgloss.Left,
			view,
			m.annotateInput.View(),
		)
	}
	return view
}

func (m Model) statusLine() string {
	header := ""
	cols := m.tbl.Columns()
	if idx := m.tbl.ColumnCursor(); idx >= 0 && idx < len(cols) {
		header = cols[idx].Title
	}
	status := fmt.Sprintf("%s Total:%d InProgress:%d Due:%d", header, m.total, m.inProgress, m.due)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Render(status)
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
