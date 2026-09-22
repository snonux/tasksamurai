package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// handleEditDone handles completion of external editor
func (m *Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError(fmt.Errorf("editor: %w", msg.err))
	}
	if m.showUltra {
		m.ultraFocusedID = m.editID
	}
	editID := m.editID
	m.editID = 0
	return m, m.scheduleTaskReload("Reloading…", reloadMeta{
		reason: reloadReasonEdit,
		editID: editID,
	}, true)
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
	t := m.currentDetailTask()
	if t != nil {
		addr := taskAddress(*t)
		if m.taskFlightBlocks() {
			return m, m.showStatusTimed("Busy: description not saved")
		}
		// Save as an async gated Cmd (mutate + reload); the detail-view blink
		// starts from the DoneMsg after the reloaded data is applied.
		return m, m.scheduleMutateReload("Saving description…",
			mutateMeta{detailBlink: true, detailField: m.detailDescriptionFieldIndex()},
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetDescriptionContext(ctx, addr, newDesc)
			})
	}

	return m, nil
}
