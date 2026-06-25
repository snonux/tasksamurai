package ui

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

func (m *Model) handleEditTask() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	m.editID = id
	return m, m.editCmd(id)
}

func (m *Model) handleToggleStart() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	// Check if task is started
	started := false
	for _, tsk := range m.tasks {
		if tsk.ID == id {
			started = tsk.Start != ""
			break
		}
	}

	if started {
		ctx, cancel := m.taskOperationContext()
		err := m.taskwarriorClient().StopContext(ctx, id)
		cancel()
		if err != nil {
			m.showError(err)
			return m, nil
		}
	} else {
		ctx, cancel := m.taskOperationContext()
		err := m.taskwarriorClient().StartContext(ctx, id)
		cancel()
		if err != nil {
			m.showError(err)
			return m, nil
		}
	}

	if !m.reloadAndReport() {
		return m, nil
	}
	return m, m.startBlink(id, false)
}

func (m *Model) handleMarkDone() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}
	return m, m.startBlink(id, true)
}

func (m *Model) handleDeleteTask() (tea.Model, tea.Cmd) {
	tsk := m.getTaskForDelete()
	if tsk == nil {
		return m, nil
	}

	count, recurring, err := m.deleteTaskWithUndo(*tsk)
	if err != nil {
		m.showError(err)
		return m, nil
	}
	if !m.reloadAndReport() {
		return m, nil
	}

	if recurring {
		m.statusMsg = fmt.Sprintf("Deleted %d recurring tasks", count)
	} else {
		m.statusMsg = "Deleted task"
	}
	return m, nil
}

func (m *Model) handleOpenURL() (tea.Model, tea.Cmd) {
	task := m.getTaskForOpenURL()
	if task == nil {
		return m, nil
	}

	url := urlRegex.FindString(task.Description)
	if url == "" {
		for _, ann := range task.Annotations {
			url = urlRegex.FindString(ann.Description)
			if url != "" {
				break
			}
		}
	}
	if url == "" {
		return m, nil
	}

	return m, openURLCmd(m.browserCmd, url, task.ID)
}

func openURLCmd(browserCmd, url string, taskID int) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(browserCmd, url)
		if err := cmd.Start(); err != nil {
			return openURLDoneMsg{err: fmt.Errorf("opening browser: %w", err)}
		}
		go func() {
			_ = cmd.Wait()
		}()
		return openURLDoneMsg{taskID: taskID}
	}
}

func (m *Model) handleOpenURLDone(msg openURLDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError(msg.err)
		return m, nil
	}
	return m, m.startBlink(msg.taskID, false)
}

func (m *Model) handleUndo() (tea.Model, tea.Cmd) {
	if len(m.undoStack) == 0 {
		return m, nil
	}

	action := m.undoStack[len(m.undoStack)-1]
	ctx, cancel := m.taskOperationContext()
	for _, restore := range action.restores {
		if err := m.taskwarriorClient().SetStatusUUIDContext(ctx, restore.uuid, restore.status); err != nil {
			cancel()
			m.showError(err)
			return m, nil
		}
	}
	cancel()
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	// Reload the task list to get the updated task with its new ID
	if err := m.reload(); err != nil {
		m.showError(err)
		return m, nil
	}

	// Find the task ID for blinking
	var id int
	var found bool
	for _, restore := range action.restores {
		for _, tsk := range m.tasks {
			if tsk.UUID == restore.uuid {
				id = tsk.ID
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	// If task not found or has ID 0, try to get it directly from Taskwarrior
	if !found || id == 0 {
		// Use task export with UUID filter to get the specific task
		for _, restore := range action.restores {
			filters := []string{restore.uuid}
			if m.filters != nil {
				filters = append(filters, m.filters...)
			}
			filters = append(filters, "status:"+restore.status)

			ctx, cancel := m.taskOperationContext()
			tasks, err := m.taskwarriorClient().Export(ctx, filters...)
			cancel()
			if err == nil && len(tasks) > 0 {
				id = tasks[0].ID
				// Also update our local task list
				for i, tsk := range m.tasks {
					if tsk.UUID == restore.uuid {
						m.tasks[i].ID = id
						break
					}
				}
				break
			}
		}
	}

	// If we still don't have a valid ID, don't try to blink
	if id == 0 {
		m.statusMsg = undoStatus(action)
		return m, nil
	}

	return m, m.startBlink(id, false)
}

func (m *Model) getTaskForDelete() *task.Task {
	if m.showTaskDetail {
		return m.currentDetailTask()
	}
	return m.getTaskAtCursor()
}

func (m *Model) deleteTaskWithUndo(tsk task.Task) (int, bool, error) {
	if strings.TrimSpace(tsk.UUID) == "" {
		return 0, false, fmt.Errorf("task %d has no UUID", tsk.ID)
	}

	recurring := isRecurringTask(tsk)
	tasks := []task.Task{tsk}
	ctx, cancel := m.taskOperationContext()
	defer cancel()
	if recurring {
		series, err := m.taskwarriorClient().RecurringSeries(ctx, recurringRootUUID(tsk))
		if err != nil {
			return 0, true, fmt.Errorf("loading recurring series: %w", err)
		}
		tasks = mergeTasksByUUID(series, tsk)
	}

	tasks = deleteOrder(tasks, recurringRootUUID(tsk))
	restores := make([]undoRestore, 0, len(tasks))
	for _, candidate := range tasks {
		if strings.TrimSpace(candidate.UUID) == "" {
			continue
		}
		restores = append(restores, undoRestore{uuid: candidate.UUID, status: undoStatusForTask(candidate)})
	}
	if len(restores) == 0 {
		return 0, recurring, fmt.Errorf("no task UUIDs to delete")
	}

	completed := make([]undoRestore, 0, len(restores))
	for _, restore := range restores {
		if err := m.taskwarriorClient().SetStatusUUIDContext(ctx, restore.uuid, "deleted"); err != nil {
			if rollbackErr := m.rollbackUndoRestores(completed); rollbackErr != nil {
				return 0, recurring, fmt.Errorf("deleting task %s: %w; rollback failed: %w", restore.uuid, err, rollbackErr)
			}
			return 0, recurring, fmt.Errorf("deleting task %s: %w", restore.uuid, err)
		}
		completed = append(completed, restore)
	}

	m.pushUndoAction("delete", restores)
	return len(restores), recurring, nil
}

func (m *Model) pushUndoAction(label string, restores []undoRestore) {
	if len(restores) == 0 {
		return
	}
	copied := append([]undoRestore(nil), restores...)
	m.undoStack = append(m.undoStack, undoAction{label: label, restores: copied})
}

func isRecurringTask(tsk task.Task) bool {
	return tsk.Parent != "" || tsk.Status == "recurring" || tsk.RType != "" || tsk.Recur != ""
}

func recurringRootUUID(tsk task.Task) string {
	if tsk.Parent != "" {
		return tsk.Parent
	}
	return tsk.UUID
}

func mergeTasksByUUID(tasks []task.Task, selected task.Task) []task.Task {
	seen := make(map[string]struct{}, len(tasks)+1)
	merged := make([]task.Task, 0, len(tasks)+1)
	for _, tsk := range tasks {
		if tsk.UUID == "" {
			continue
		}
		if _, ok := seen[tsk.UUID]; ok {
			continue
		}
		seen[tsk.UUID] = struct{}{}
		merged = append(merged, tsk)
	}
	if selected.UUID != "" {
		if _, ok := seen[selected.UUID]; !ok {
			merged = append(merged, selected)
		}
	}
	return merged
}

func deleteOrder(tasks []task.Task, rootUUID string) []task.Task {
	ordered := make([]task.Task, 0, len(tasks))
	var root []task.Task
	for _, tsk := range tasks {
		if tsk.UUID == rootUUID {
			root = append(root, tsk)
			continue
		}
		ordered = append(ordered, tsk)
	}
	return append(ordered, root...)
}

func undoStatusForTask(tsk task.Task) string {
	if tsk.Status == "" || tsk.Status == "deleted" {
		return "pending"
	}
	return tsk.Status
}

func (m *Model) rollbackUndoRestores(restores []undoRestore) error {
	ctx, cancel := context.WithTimeout(context.Background(), taskOperationTimeout)
	defer cancel()

	var errs []error
	for i := len(restores) - 1; i >= 0; i-- {
		if err := m.taskwarriorClient().SetStatusUUIDContext(ctx, restores[i].uuid, restores[i].status); err != nil {
			errs = append(errs, fmt.Errorf("restoring task %s to %s: %w", restores[i].uuid, restores[i].status, err))
		}
	}
	return errors.Join(errs...)
}

func undoStatus(action undoAction) string {
	if action.label == "delete" && len(action.restores) > 1 {
		return "Tasks restored"
	}
	return "Task restored"
}

func (m *Model) handleSetDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	m.clearEditingModes()
	m.dueID = id
	m.dueEditing = true
	m.dueDate = time.Now()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleRemoveDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	// In Taskwarrior, passing an empty value to due: removes the due date
	ctx, cancel := m.taskOperationContext()
	err = m.taskwarriorClient().SetDueDateContext(ctx, id, "")
	cancel()
	if err != nil {
		m.showError(err)
		return m, nil
	}

	if !m.reloadAndReport() {
		return m, nil
	}
	return m, m.startBlink(id, false)
}

func (m *Model) handleRandomDueDate() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	days := rand.Intn(31) + 7
	due := time.Now().AddDate(0, 0, days).Format("2006-01-02")

	ctx, cancel := m.taskOperationContext()
	err = m.taskwarriorClient().SetDueDateContext(ctx, id, due)
	cancel()
	if err != nil {
		m.showError(err)
		return m, nil
	}

	if !m.reloadAndReport() {
		return m, nil
	}
	return m, m.startBlink(id, false)
}

func (m *Model) handleSetRecurrence() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	task := m.getTaskAtCursor()
	if task == nil {
		return m, nil
	}

	m.clearEditingModes()
	m.recurID = id
	m.recurEditing = true
	m.recurInput.SetValue(task.Recur)
	m.recurInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleSetPriority() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	m.clearEditingModes()
	m.priorityID = id
	m.prioritySelecting = true
	m.priorityIndex = 0
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleAnnotate(replace bool) (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	m.clearEditingModes()
	m.annotateID = id
	m.annotating = true
	m.replaceAnnotations = replace
	m.annotateInput.SetValue("")
	m.annotateInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleFilter() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.filterEditing = true
	m.filterInput.SetValue(strings.Join(m.filters, " "))
	m.filterInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleToggleAgentFilter() (tea.Model, tea.Cmd) {
	m.filters = toggleAgentFilter(m.filters)
	if !m.reloadAndReport() {
		return m, nil
	}
	return m, nil
}

func (m *Model) handleAddTask() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.addingTask = true
	m.addInput.SetValue("")
	m.addInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleEditTags() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	m.clearEditingModes()
	m.tagsID = id
	m.tagsEditing = true
	m.tagsInput.SetValue("")
	m.tagsInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleEditProject() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	m.clearEditingModes()
	m.projID = id
	m.projEditing = true

	// Get current project value
	task := m.getTaskAtCursor()
	if task != nil {
		m.projInput.SetValue(task.Project)
	} else {
		m.projInput.SetValue("")
	}
	m.projInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleTagToProject() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	// Get the task at cursor
	currentTask := m.getTaskAtCursor()
	if currentTask == nil || len(currentTask.Tags) == 0 {
		// No tags to convert
		return m, nil
	}

	// Get the first tag
	firstTag := currentTask.Tags[0]

	// Set the tag as project
	ctx, cancel := m.taskOperationContext()
	err = m.taskwarriorClient().SetProjectContext(ctx, id, firstTag)
	if err != nil {
		cancel()
		m.showError(err)
		return m, nil
	}

	// Remove the tag from the task
	if err := m.taskwarriorClient().RemoveTagsContext(ctx, id, []string{firstTag}); err != nil {
		cancel()
		m.showError(err)
		return m, nil
	}
	cancel()

	if !m.reloadAndReport() {
		return m, nil
	}
	return m, m.startBlink(id, false)
}

func (m *Model) handleRandomTheme() (tea.Model, tea.Cmd) {
	m.theme = RandomTheme()
	m.applyTheme()
	return m, nil
}

func (m *Model) handleResetTheme() (tea.Model, tea.Cmd) {
	m.theme = m.defaultTheme
	m.applyTheme()
	return m, nil
}

func (m *Model) handleToggleDisco() (tea.Model, tea.Cmd) {
	m.disco = !m.disco
	return m, nil
}

func (m *Model) handleToggleBlink() (tea.Model, tea.Cmd) {
	m.blinkEnabled = !m.blinkEnabled
	if m.blinkEnabled {
		m.statusMsg = "Blinking enabled"
	} else {
		m.statusMsg = "Blinking disabled"
	}
	return m, nil
}

func toggleAgentFilter(filters []string) []string {
	next := "+agent"
	index := -1

	for i, filter := range filters {
		switch filter {
		case "+agent":
			next = "-agent"
			index = i
		case "-agent":
			next = "+agent"
			index = i
		}
		if index != -1 {
			break
		}
	}

	if index == -1 {
		out := append([]string(nil), filters...)
		return append(out, next)
	}

	out := append([]string(nil), filters...)
	out[index] = next
	return out
}

func (m *Model) handleRefresh() (tea.Model, tea.Cmd) {
	m.reloadAndReport()
	return m, nil
}

func (m *Model) handleSearch() (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.searching = true
	m.searchIndex = 0
	m.searchMatches = nil
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleNextSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.searchMatches) == 0 {
		return m, nil
	}

	m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
	match := m.searchMatches[m.searchIndex]
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(match.row)
	m.tbl.SetColumnCursor(match.col)
	m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	return m, nil
}

func (m *Model) handlePrevSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.searchMatches) == 0 {
		return m, nil
	}

	m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	match := m.searchMatches[m.searchIndex]
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(match.row)
	m.tbl.SetColumnCursor(match.col)
	m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	return m, nil
}

func (m *Model) handleHelpSearch() (tea.Model, tea.Cmd) {
	m.helpSearching = true
	m.helpSearchIndex = 0
	m.helpSearchMatches = nil
	m.helpSearchInput.SetValue("")
	m.helpSearchInput.Focus()
	return m, nil
}

func (m *Model) handleNextHelpSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.helpSearchMatches) == 0 {
		return m, nil
	}

	m.helpSearchIndex = (m.helpSearchIndex + 1) % len(m.helpSearchMatches)
	// In the future, we could add visual indication of current match
	return m, nil
}

func (m *Model) handlePrevHelpSearchMatch() (tea.Model, tea.Cmd) {
	if len(m.helpSearchMatches) == 0 {
		return m, nil
	}

	m.helpSearchIndex = (m.helpSearchIndex - 1 + len(m.helpSearchMatches)) % len(m.helpSearchMatches)
	// In the future, we could add visual indication of current match
	return m, nil
}

func (m *Model) handleShowTaskDetail() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		return m, nil
	}

	if m.taskByID(id) != nil {
		m.showTaskDetail = true
		m.currentTaskDetailID = id
		m.detailSearching = false
		m.detailSearchRegex = nil
		m.detailFieldIndex = 0
		m.detailBlinkField = -1
		m.detailBlinkOn = false
		m.detailBlinkCount = 0
		m.detailSearchInput = textinput.New()
		m.detailSearchInput.Placeholder = "Search..."
		m.detailSearchInput.SetWidth(30)
	}

	return m, nil
}

// handleEnterOrEdit dispatches to the appropriate inline editor based on the
// column the cursor is on. Shared activation helpers (activatePriorityEdit,
// activateDueEdit, etc.) are defined in detail_handlers.go to avoid duplication
// with the detail-view editing path.
func (m *Model) handleEnterOrEdit() (tea.Model, tea.Cmd) {
	id, err := m.getSelectedTaskID()
	if err != nil {
		// No task selected — toggle expanded-cell panel instead.
		m.cellExpanded = !m.cellExpanded
		m.updateTableHeight()
		return m, nil
	}

	tsk := m.getTaskAtCursor()
	// taskStr extracts a string field from the cursor task, returning ""
	// when no task is selected so activation helpers get a safe zero value.
	taskStr := func(get func(*task.Task) string) string {
		if tsk == nil {
			return ""
		}
		return get(tsk)
	}

	switch m.tbl.ColumnCursor() {
	case 0: // Priority
		m.activatePriorityEdit(id, taskStr(func(t *task.Task) string { return t.Priority }))
	case 3: // Due date
		m.activateDueEdit(id, taskStr(func(t *task.Task) string { return t.Due }))
	case 4: // Recurrence
		m.activateRecurEdit(id, taskStr(func(t *task.Task) string { return t.Recur }))
	case 5: // Project
		m.activateProjectEdit(id, taskStr(func(t *task.Task) string { return t.Project }))
	case 6: // Tags
		m.activateTagsEdit(id)
	case 7: // Annotations
		return m.activateAnnotationsEdit(id, tsk)
	case 8: // Description
		return m.activateDescriptionEdit(id, tsk)
	default:
		// Other columns: toggle expanded-cell panel.
		m.cellExpanded = !m.cellExpanded
		m.updateTableHeight()
	}
	return m, nil
}

func (m *Model) handleTableNavigation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	if prevRow != m.tbl.Cursor() || prevCol != m.tbl.ColumnCursor() {
		m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	}
	return m, cmd
}

// showError displays an error in the status bar
func (m *Model) showError(err error) {
	m.statusMsg = fmt.Sprintf("Error: %v", err)
	// Note: we can't return a Cmd from here, so the error will stay until next update
}

// handleJumpToRandomTask jumps to a random pending task
func (m *Model) handleJumpToRandomTask() (tea.Model, tea.Cmd) {
	if len(m.tasks) == 0 {
		m.statusMsg = "No tasks to jump to"
		return m, nil
	}

	// Pick a random index
	randomIndex := rand.Intn(len(m.tasks))

	// Update cursor position
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(randomIndex)
	m.updateSelectionHighlight(prevRow, randomIndex, prevCol, m.tbl.ColumnCursor())

	// Blink the task to indicate jump
	if randomIndex < len(m.tasks) {
		taskID := m.tasks[randomIndex].ID
		return m, m.startBlink(taskID, false)
	}

	return m, nil
}

// handleJumpToRandomTaskNoDue jumps to a random pending task without a due date
func (m *Model) handleJumpToRandomTaskNoDue() (tea.Model, tea.Cmd) {
	// Find all tasks without due dates
	var noDueTasks []int
	for i, task := range m.tasks {
		if task.Due == "" {
			noDueTasks = append(noDueTasks, i)
		}
	}

	if len(noDueTasks) == 0 {
		m.statusMsg = "No tasks without due date to jump to"
		return m, nil
	}

	// Pick a random task from the no-due list
	randomChoice := rand.Intn(len(noDueTasks))
	randomIndex := noDueTasks[randomChoice]

	// Update cursor position
	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(randomIndex)
	m.updateSelectionHighlight(prevRow, randomIndex, prevCol, m.tbl.ColumnCursor())

	// Blink the task to indicate jump
	if randomIndex < len(m.tasks) {
		taskID := m.tasks[randomIndex].ID
		return m, m.startBlink(taskID, false)
	}

	return m, nil
}
