package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// commitFunc performs the Enter action for a text input mode. It returns the
// tea.Cmd that runs the mutation+reload asynchronously, or an error (which
// keeps the input mode open and shows the error via showErrorTimed). While a
// Taskwarrior flight is active, commits return m.busyCommitErr() so the user
// keeps their input instead of losing it to a Busy reject.
type commitFunc func(value string) (tea.Cmd, error)

// handleTextInput provides generic text input handling for all input modes.
// Enter schedules the commit Cmd and exits the mode immediately; the result
// (blink, error) is applied by the *DoneMsg handler.
func (m *Model) handleTextInput(msg tea.KeyPressMsg, input *textinput.Model, onEnter commitFunc, onExit func()) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		cmd, err := onEnter(input.Value())
		if err != nil {
			return m, m.showErrorTimed(err)
		}
		input.Blur()
		onExit()
		m.updateTableHeight()
		return m, cmd
	case "esc":
		input.Blur()
		onExit()
		m.updateTableHeight()
		return m, nil
	}
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	return m, cmd
}

// handleAnnotationMode handles keyboard input when in annotation mode
func (m *Model) handleAnnotationMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.annotating = false
		m.replaceAnnotations = false
	}

	onEnter := func(value string) (tea.Cmd, error) {
		// Annotation can be empty when replacing (to remove all)
		if !m.replaceAnnotations && strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("annotation cannot be empty")
		}
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		addr := m.annotateAddr
		replace := m.replaceAnnotations
		var op mutateOp
		if replace {
			op = func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.ReplaceAnnotations(ctx, addr, value)
			}
		} else {
			op = func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.AnnotateContext(ctx, addr, value)
			}
		}
		// No blink when replacing all annotations with an empty value.
		var blinkID int
		if value != "" {
			blinkID = m.annotateID
		}
		meta := mutateMeta{blinkID: blinkID}
		meta.restoreInput = func(mm *Model) {
			mm.annotating = true
			mm.replaceAnnotations = replace
			mm.annotateInput.SetValue(value)
			mm.annotateInput.Focus()
			mm.updateTableHeight()
		}
		return m.scheduleMutateReload("Annotating…", meta, op), nil
	}

	return m.handleTextInput(msg, &m.annotateInput, onEnter, onExit)
}

// handleDescriptionMode handles keyboard input when editing description
func (m *Model) handleDescriptionMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.descEditing = false
	}

	onEnter := func(value string) (tea.Cmd, error) {
		if err := validateDescription(value); err != nil {
			return nil, err
		}
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		addr := m.descAddr
		meta := mutateMeta{blinkID: m.descID}
		meta.restoreInput = func(mm *Model) {
			mm.descEditing = true
			mm.descInput.SetValue(value)
			mm.descInput.Focus()
			mm.updateTableHeight()
		}
		return m.scheduleMutateReload("Saving description…", meta,
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetDescriptionContext(ctx, addr, value)
			}), nil
	}

	return m.handleTextInput(msg, &m.descInput, onEnter, onExit)
}

// handleTagsMode handles keyboard input when editing tags
func (m *Model) handleTagsMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.tagsEditing = false
	}

	onEnter := func(value string) (tea.Cmd, error) {
		words := strings.Fields(value)
		var adds, removes []string
		for _, w := range words {
			if strings.HasPrefix(w, "-") {
				if len(w) > 1 {
					tagName := w[1:]
					if err := validateTagName(tagName); err != nil {
						return nil, fmt.Errorf("remove tag '%s': %w", tagName, err)
					}
					removes = append(removes, tagName)
				}
			} else {
				w = strings.TrimPrefix(w, "+")
				if w != "" {
					if err := validateTagName(w); err != nil {
						return nil, fmt.Errorf("add tag '%s': %w", w, err)
					}
					adds = append(adds, w)
				}
			}
		}
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		addr := m.tagsAddr
		var ops []mutateOp
		if len(adds) > 0 {
			added := adds
			ops = append(ops, func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.AddTagsContext(ctx, addr, added)
			})
		}
		if len(removes) > 0 {
			removed := removes
			ops = append(ops, func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.RemoveTagsContext(ctx, addr, removed)
			})
		}
		meta := mutateMeta{blinkID: m.tagsID}
		if m.showTaskDetail {
			// In detail view, blink the tags field
			meta = mutateMeta{detailBlink: true, detailField: 4} // Tags is field index 4
		}
		meta.restoreInput = func(mm *Model) {
			mm.tagsEditing = true
			mm.tagsInput.SetValue(value)
			mm.tagsInput.Focus()
			mm.updateTableHeight()
		}
		return m.scheduleMutateReload("Updating tags…", meta, ops...), nil
	}

	return m.handleTextInput(msg, &m.tagsInput, onEnter, onExit)
}

// handleDueEditMode handles due date editing
func (m *Model) handleDueEditMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.taskFlightBlocks() {
			_ = m.rejectIfBusy()
			return m, nil
		}
		// In Taskwarrior, an empty due value would remove the date; the
		// picker always commits a concrete date here.
		due := m.dueDate.Format("2006-01-02")
		addr := m.dueAddr
		meta := mutateMeta{blinkID: m.dueID}
		if m.showTaskDetail {
			// In detail view, blink the due field
			meta = mutateMeta{detailBlink: true, detailField: 5} // Due is field index 5
		}
		m.dueEditing = false
		meta.restoreInput = func(mm *Model) {
			mm.dueEditing = true
			mm.updateTableHeight()
		}
		cmd := m.scheduleMutateReload("Setting due…", meta,
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetDueDateContext(ctx, addr, due)
			})
		m.updateTableHeight()
		return m, cmd
	case "esc":
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

// handleRecurrenceMode handles recurrence editing
func (m *Model) handleRecurrenceMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.recurEditing = false
		m.recurSeries = false
		m.recurRoot = ""
	}

	onEnter := func(value string) (tea.Cmd, error) {
		if err := validateRecurrence(value); err != nil {
			return nil, err
		}
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		series, root, addr := m.recurSeries, m.recurRoot, m.recurAddr
		var op mutateOp
		if series {
			op = func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetRecurringSeriesRecurrenceContext(ctx, root, value)
			}
		} else {
			op = func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetRecurrenceContext(ctx, addr, value)
			}
		}
		// The detail-vs-row blink decision needs the reloaded task data, so it
		// is made in handleMutateReloadDone via detailBlinkIfRecur.
		meta := mutateMeta{blinkID: m.recurID, detailBlinkIfRecur: m.showTaskDetail}
		meta.restoreInput = func(mm *Model) {
			mm.recurEditing = true
			mm.recurSeries = series
			mm.recurRoot = root
			mm.recurInput.SetValue(value)
			mm.recurInput.Focus()
			mm.updateTableHeight()
		}
		return m.scheduleMutateReload("Setting recur…", meta, op), nil
	}

	return m.handleTextInput(msg, &m.recurInput, onEnter, onExit)
}

// handleProjectMode handles project editing
func (m *Model) handleProjectMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.projEditing = false
	}

	onEnter := func(value string) (tea.Cmd, error) {
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		addr := m.projAddr
		meta := mutateMeta{blinkID: m.projID}
		if m.showTaskDetail {
			// In detail view, blink the project field
			meta = mutateMeta{detailBlink: true, detailField: fieldProject}
		}
		meta.restoreInput = func(mm *Model) {
			mm.projEditing = true
			mm.projInput.SetValue(value)
			mm.projInput.Focus()
			mm.updateTableHeight()
		}
		return m.scheduleMutateReload("Setting project…", meta,
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetProjectContext(ctx, addr, value)
			}), nil
	}

	return m.handleTextInput(msg, &m.projInput, onEnter, onExit)
}

// handlePriorityMode handles priority selection
func (m *Model) handlePriorityMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		priority := priorityOptions[m.priorityIndex]
		if err := validatePriority(priority); err != nil {
			return m, m.showErrorTimed(err)
		}
		if m.taskFlightBlocks() {
			_ = m.rejectIfBusy()
			return m, nil
		}
		addr := m.priorityAddr
		meta := mutateMeta{blinkID: m.priorityID}
		if m.showTaskDetail {
			// In detail view, blink the priority field
			meta = mutateMeta{detailBlink: true, detailField: 3} // Priority is field index 3
		}
		m.prioritySelecting = false
		meta.restoreInput = func(mm *Model) {
			mm.prioritySelecting = true
			mm.updateTableHeight()
		}
		cmd := m.scheduleMutateReload("Setting priority…", meta,
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.SetPriorityContext(ctx, addr, priority)
			})
		m.updateTableHeight()
		return m, cmd
	case "esc":
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

// handleFilterMode handles filter editing for both traditional and ultra mode.
// The filter value is split using shell-quoting rules (via parseFilterInput)
// so that expressions with quoted values (e.g. description:"my task") are
// passed to taskwarrior as a single argument. Any taskwarrior filter expression
// that is valid on the command line (proj:xxx, +tag, description:"...", etc.)
// is therefore accepted here too. The filter runs as an async reload Cmd; if
// taskwarrior rejects the expression, handleTaskReloadDone restores the
// previous filters so the UI never shows an unexplained empty list.
func (m *Model) handleFilterMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	onExit := func() {
		m.filterEditing = false
	}

	onEnter := func(value string) (tea.Cmd, error) {
		fields, err := parseFilterInput(value)
		if err != nil {
			return nil, err
		}
		if m.taskFlightBlocks() {
			return nil, m.busyCommitErr()
		}
		prev := append([]string(nil), m.filters...)
		m.filters = fields
		return m.scheduleTaskReload("Reloading…", reloadMeta{
			reason:         reloadReasonFilter,
			restoreFilters: true,
			prevFilters:    prev,
		}, true), nil
	}

	return m.handleTextInput(msg, &m.filterInput, onEnter, onExit)
}

// handleAddTaskMode handles adding a new task
func (m *Model) handleAddTaskMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.taskFlightBlocks() {
			_ = m.rejectIfBusy()
			return m, nil
		}
		oldIDs := make(map[int]struct{}, len(m.tasks))
		for _, tsk := range m.tasks {
			oldIDs[tsk.ID] = struct{}{}
		}
		value := m.addInput.Value()
		m.addingTask = false
		m.addInput.Blur()
		meta := mutateMeta{selectNewTask: true, oldIDs: oldIDs}
		meta.restoreInput = func(mm *Model) {
			mm.addingTask = true
			mm.addInput.SetValue(value)
			mm.addInput.Focus()
			mm.updateTableHeight()
		}
		cmd := m.scheduleMutateReload("Adding…", meta,
			func(ctx context.Context, tw task.Taskwarrior) error {
				return tw.AddLineContext(ctx, value)
			})
		m.updateTableHeight()
		return m, cmd

	case "esc":
		m.addingTask = false
		m.addInput.Blur()
		m.updateTableHeight()
		return m, nil
	}

	var cmd tea.Cmd
	m.addInput, cmd = m.addInput.Update(msg)
	return m, cmd
}

// handleSearchMode handles search input
func (m *Model) handleSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := m.searchInput.Value()
		invalidMsg := ""
		if pattern != "" {
			// Check cache first
			if cached, ok := cachedSearchRegex(pattern); ok {
				m.searchRegex = cached
			} else {
				// Compile and cache if not found
				re, err := compileAndCacheRegex(pattern)
				if err == nil {
					m.searchRegex = re
				} else {
					m.searchRegex = nil
					invalidMsg = fmt.Sprintf("Invalid regex: %v", err)
				}
			}
		} else {
			m.searchRegex = nil
		}
		m.searching = false
		m.searchInput.Blur()
		m.updateTableHeight()
		cmd := m.scheduleTaskReload("Reloading…", reloadMeta{reason: reloadReasonSearch}, true)
		if invalidMsg != "" {
			m.statusMsg = invalidMsg
		}
		return m, cmd

	case "esc":
		m.searching = false
		m.searchInput.Blur()
		m.updateTableHeight()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// handleHelpSearchMode handles search input in help mode
func (m *Model) handleHelpSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := m.helpSearchInput.Value()
		if pattern != "" {
			// Check cache first
			if cached, ok := cachedSearchRegex(pattern); ok {
				m.helpSearchRegex = cached
			} else {
				// Compile and cache if not found
				re, err := compileAndCacheRegex(pattern)
				if err == nil {
					m.helpSearchRegex = re
				} else {
					m.helpSearchRegex = nil
					m.statusMsg = fmt.Sprintf("Invalid regex: %v", err)
				}
			}
		} else {
			m.helpSearchRegex = nil
		}
		m.helpSearching = false
		m.helpSearchInput.Blur()

		// Find matching help lines
		m.helpSearchMatches = nil
		if m.helpSearchRegex != nil {
			helpLines := m.getHelpLines()
			for i, line := range helpLines {
				if m.helpSearchRegex.MatchString(line) {
					m.helpSearchMatches = append(m.helpSearchMatches, i)
				}
			}
			// Set to first match
			if len(m.helpSearchMatches) > 0 {
				m.helpSearchIndex = 0
			}
		}
		return m, nil

	case "esc":
		m.helpSearching = false
		m.helpSearchInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.helpSearchInput, cmd = m.helpSearchInput.Update(msg)
	return m, cmd
}
