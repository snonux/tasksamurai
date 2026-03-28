package ui

import tea "charm.land/bubbletea/v2"

// renderUltraModus renders the ultra mode view.
// It is a placeholder until the full ultra mode layout is implemented.
func (m *Model) renderUltraModus() string {
	return "Ultra Modus (TODO)"
}

// handleUltraMode handles keyboard input in ultra mode.
func (m *Model) handleUltraMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
