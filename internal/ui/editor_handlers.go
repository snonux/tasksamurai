package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleEditDone handles completion of external editor
func (m *Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError(fmt.Errorf("editor: %w", msg.err))
	}
	if m.showUltra {
		m.ultraFocusedID = m.editID
	}
	if !m.reloadAndReport() {
		m.editID = 0
		return m, nil
	}
	cmd := m.startBlink(m.editID, false)
	m.editID = 0
	return m, cmd
}

// handleDescEditDone handles the completion of description editing
func (m *Model) handleDescEditDone(msg descEditDoneMsg) (tea.Model, tea.Cmd) {
	m.detailDescEditing = false
	if msg.tempFile != "" {
		defer func() { _ = os.Remove(msg.tempFile) }()
	}

	if msg.err != nil {
		return m, m.showStatusTimed(fmt.Sprintf("Edit error: %v", msg.err))
	}

	// Read the edited content
	content, err := os.ReadFile(msg.tempFile)
	if err != nil {
		return m, m.showStatusTimed(fmt.Sprintf("Error reading file: %v", err))
	}

	// Update the description
	newDesc := strings.TrimSpace(string(content))
	if m.currentTaskDetail != nil {
		ctx, cancel := m.taskOperationContext()
		err = m.taskwarriorClient().SetDescriptionContext(ctx, m.currentTaskDetail.ID, newDesc)
		cancel()
		if err != nil {
			return m, m.showStatusTimed(fmt.Sprintf("Error updating description: %v", err))
		}

		// Reload and start blinking
		if !m.reloadAndReport() {
			return m, nil
		}
		return m, m.startDetailBlink(m.detailDescriptionFieldIndex())
	}

	return m, nil
}
