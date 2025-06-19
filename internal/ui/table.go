package ui

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model wraps a Bubble Tea table.Model to display tasks.
type Model struct{ tbl table.Model }

// New creates a new UI model with the provided rows.
func New(rows []table.Row) Model {
	cols := []table.Column{
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
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Foreground(lipgloss.Color("205"))
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	styles.Cell = styles.Cell.Padding(0, 1)
	t.SetStyles(styles)
	return Model{tbl: t}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.tbl.SetWidth(msg.Width)
		m.tbl.SetHeight(msg.Height - 2)
	}
	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

// View renders the table UI.
func (m Model) View() string { return m.tbl.View() }
