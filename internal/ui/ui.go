package ui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Model wraps a Bubble Tea list.Model to display tasks.
type Model struct {
	list list.Model
}

// New creates a new UI model with the provided Bubble Tea list.
func New(l list.Model) Model {
	return Model{list: l}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key and window events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "k":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		}
		return m.updateList(msg)
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	}
	return m.updateList(msg)
}

func (m *Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the list UI.
func (m Model) View() string {
	return m.list.View()
}
