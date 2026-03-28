package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

func (m *Model) renderUltraModus() string {
	tasks := m.ultraTaskList()
	width := m.ultraRenderWidth()

	top := m.ultraStatusLine(m.ultraModeStatus(tasks), width)
	bottom := m.ultraStatusLine(m.ultraCursorStatus(tasks), width)
	overlay, overlayHeight := m.ultraOverlay()

	var lines []string
	lines = append(lines, top)
	lines = append(
		lines,
		m.ultraRenderCards(
			tasks,
			width,
			m.ultraVisibleCursor(tasks),
			m.ultraVisibleStart(len(tasks)),
			m.ultraCardBudget(top, bottom, overlayHeight),
		)...,
	)
	lines = append(lines, bottom)
	if overlay != "" {
		lines = append(lines, overlay)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) ultraRenderWidth() int {
	width := m.tbl.Width()
	if width <= 0 {
		return 80
	}
	return width
}

func (m *Model) ultraSearchOverlay() (string, int) {
	if !m.ultraSearching {
		return "", 0
	}

	overlay := lipgloss.NewStyle().
		Foreground(lipgloss.Color("248")).
		PaddingTop(1).
		Render(m.ultraSearchInput.View())
	return overlay, lipgloss.Height(overlay)
}

func (m *Model) ultraCardBudget(top, bottom string, overlayHeight int) int {
	budget := m.windowHeight - lipgloss.Height(top) - lipgloss.Height(bottom) - overlayHeight
	if budget < 0 {
		return 0
	}
	return budget
}

func (m *Model) ultraVisibleStart(total int) int {
	start := m.ultraOffset
	if start < 0 {
		return 0
	}
	if start > total {
		return total
	}
	return start
}

func (m *Model) ultraVisibleCount() int {
	tasks := m.ultraTaskList()
	if len(tasks) == 0 {
		return 0
	}

	width := m.ultraRenderWidth()
	top := m.ultraStatusLine(m.ultraModeStatus(tasks), width)
	bottom := m.ultraStatusLine(m.ultraCursorStatus(tasks), width)
	_, overlayHeight := m.ultraOverlay()

	budget := m.ultraCardBudget(top, bottom, overlayHeight)
	if budget <= 0 {
		return 0
	}

	start := m.ultraVisibleStart(len(tasks))
	selected := m.ultraVisibleCursor(tasks)
	used := 0
	count := 0
	for i := start; i < len(tasks); i++ {
		card := m.renderUltraCard(tasks[i], width, i == selected, m.ultraSearchRegex)
		if card == "" {
			continue
		}

		cardHeight := lipgloss.Height(card)
		if count > 0 {
			if used+1+cardHeight > budget {
				break
			}
			used++
		} else if cardHeight > budget {
			break
		}

		if used+cardHeight > budget {
			break
		}

		used += cardHeight
		count++
	}

	return count
}

func (m *Model) ultraTaskList() []task.Task {
	if m.ultraFiltered == nil {
		return m.tasks
	}

	tasks := make([]task.Task, 0, len(m.ultraFiltered))
	for _, idx := range m.ultraFiltered {
		if idx < 0 || idx >= len(m.tasks) {
			continue
		}
		tasks = append(tasks, m.tasks[idx])
	}
	return tasks
}

func (m *Model) ultraFilteredTaskIDs() []int {
	if m.ultraFiltered == nil {
		return nil
	}

	ids := make([]int, 0, len(m.ultraFiltered))
	for _, idx := range m.ultraFiltered {
		if idx < 0 || idx >= len(m.tasks) {
			continue
		}
		ids = append(ids, m.tasks[idx].ID)
	}
	return ids
}

func (m *Model) rebuildUltraFiltered(ids []int) {
	if m.ultraFiltered == nil {
		return
	}

	indexes := make([]int, 0, len(ids))
	for _, id := range ids {
		if idx := m.taskIndexByID(id); idx >= 0 {
			indexes = append(indexes, idx)
		}
	}
	m.ultraFiltered = indexes
}

func (m *Model) getUltraSelectedTaskID() (int, error) {
	tasks := m.ultraTaskList()
	if len(tasks) == 0 {
		return 0, fmt.Errorf("no ultra tasks available")
	}
	if m.ultraCursor < 0 || m.ultraCursor >= len(tasks) {
		return 0, fmt.Errorf("ultra cursor %d out of range", m.ultraCursor)
	}
	return tasks[m.ultraCursor].ID, nil
}

func (m *Model) ultraTaskIndexByID(id int) int {
	for i, t := range m.ultraTaskList() {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) taskIndexByID(id int) int {
	for i, t := range m.tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) selectTaskByID(id int) bool {
	row := m.taskIndexByID(id)
	if row < 0 {
		return false
	}

	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(row)

	if m.showUltra {
		if ultraRow := m.ultraTaskIndexByID(id); ultraRow >= 0 {
			m.ultraCursor = ultraRow
		}
		m.ultraEnsureVisible()
	}

	if prevRow != m.tbl.Cursor() || prevCol != m.tbl.ColumnCursor() {
		m.updateSelectionHighlight(prevRow, m.tbl.Cursor(), prevCol, m.tbl.ColumnCursor())
	}
	return true
}

func (m *Model) reconcileUltraSelection() {
	if !m.showUltra {
		return
	}

	if m.ultraFocusedID > 0 {
		_ = m.selectTaskByID(m.ultraFocusedID)
		m.ultraFocusedID = 0
		m.ultraEnsureVisible()
		return
	}

	m.ultraEnsureVisible()
}

func (m *Model) ultraOverlay() (string, int) {
	overlay, overlayHeight := m.ultraSearchOverlay()

	inputOverlay := m.ultraInputOverlay()
	if inputOverlay != "" {
		if overlay != "" {
			overlay += "\n" + inputOverlay
			overlayHeight += lipgloss.Height(inputOverlay)
		} else {
			overlay = inputOverlay
			overlayHeight = lipgloss.Height(inputOverlay)
		}
	}

	return overlay, overlayHeight
}

func (m *Model) ultraInputOverlay() string {
	switch {
	case m.annotating:
		return m.annotateInput.View()
	case m.dueEditing:
		return m.dueView(true)
	case m.prioritySelecting:
		return m.priorityView(true)
	case m.descEditing:
		return m.descInput.View()
	case m.tagsEditing:
		return m.tagsInput.View()
	case m.recurEditing:
		return m.recurInput.View()
	case m.projEditing:
		return m.projInput.View()
	case m.filterEditing:
		return m.filterInput.View()
	case m.addingTask:
		return m.addInput.View()
	case m.searching:
		return m.searchInput.View()
	default:
		return ""
	}
}

// ultraVisibleCursor treats ultraCursor as a cursor within the visible ultra task list.
func (m *Model) ultraVisibleCursor(tasks []task.Task) int {
	if len(tasks) == 0 {
		return -1
	}
	if m.ultraCursor < 0 {
		return 0
	}
	if m.ultraCursor >= len(tasks) {
		return len(tasks) - 1
	}
	return m.ultraCursor
}

func (m Model) ultraStatusLine(text string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(width).
		Render(text)
}

func (m *Model) ultraModeStatus(tasks []task.Task) string {
	filter := "all"
	if query := strings.TrimSpace(m.ultraSearchInput.Value()); query != "" {
		filter = query
	} else if m.ultraSearchRegex != nil {
		filter = m.ultraSearchRegex.String()
	} else if m.ultraFiltered != nil {
		filter = "filtered"
	}
	return fmt.Sprintf("ULTRA MODE | filter: %s | %d tasks", filter, len(tasks))
}

func (m *Model) ultraCursorStatus(tasks []task.Task) string {
	cursor := m.ultraVisibleCursor(tasks)
	if cursor < 0 {
		return fmt.Sprintf("0/%d", len(tasks))
	}
	return fmt.Sprintf("%d/%d", cursor+1, len(tasks))
}

// renderUltraCard assembles the card sections and applies the outer selection style.
func (m *Model) renderUltraCard(t task.Task, width int, selected bool, re *regexp.Regexp) string {
	card := ultraJoinSections(
		m.renderUltraHeaderWithRegex(t, width, re),
		m.renderUltraMetaWithRegex(t, width, re),
		m.renderUltraDescriptionWithRegex(t, width, re),
		m.renderUltraAnnotationsWithRegex(t, width, re),
	)
	if card == "" {
		return ""
	}
	blink := m.blinkID != 0 && m.blinkOn && t.ID == m.blinkID
	if blink {
		lines := strings.SplitN(card, "\n", 2)
		lines[0] = "! " + lines[0]
		card = strings.Join(lines, "\n")
	}
	return ultraCardStyle(m.theme, width, selected, blink).Render(card)
}

// renderUltraHeader renders the task's primary state line.
func (m *Model) renderUltraHeader(t task.Task, width int) string {
	return m.renderUltraHeaderWithRegex(t, width, m.ultraSearchRegex)
}

// renderUltraMeta renders the task's secondary metadata line.
func (m *Model) renderUltraMeta(t task.Task, width int) string {
	return m.renderUltraMetaWithRegex(t, width, m.ultraSearchRegex)
}

// renderUltraDescription renders the wrapped task description body.
func (m *Model) renderUltraDescription(t task.Task, width int) string {
	return m.renderUltraDescriptionWithRegex(t, width, m.ultraSearchRegex)
}

// renderUltraAnnotations renders wrapped annotation lines with timestamps.
func (m *Model) renderUltraAnnotations(t task.Task, width int) string {
	return m.renderUltraAnnotationsWithRegex(t, width, m.ultraSearchRegex)
}

func (m *Model) renderUltraHeaderWithRegex(t task.Task, width int, re *regexp.Regexp) string {
	_ = width
	id := m.ultraStyledText(re, lipgloss.NewStyle().Bold(true), fmt.Sprintf("#%d", t.ID))
	priority := ultraPriorityToken(m.theme, t.Priority)
	status := m.ultraStyledText(re, lipgloss.NewStyle().Foreground(lipgloss.Color("252")), ultraOrDash(t.Status))
	urgency := m.ultraStyledText(re, lipgloss.NewStyle().Foreground(lipgloss.Color("252")), fmt.Sprintf("%.1f", t.Urgency))
	age := m.ultraStyledText(re, lipgloss.NewStyle().Foreground(lipgloss.Color("252")), ultraTaskAge(t.Entry))
	return strings.Join([]string{id, priority, status, urgency, age}, " | ")
}

func (m *Model) renderUltraMetaWithRegex(t task.Task, width int, re *regexp.Regexp) string {
	_ = width
	parts := []string{
		m.ultraKeyValue(re, "project", ultraOrDash(t.Project)),
		m.ultraKeyValue(re, "tags", ultraOrDash(strings.Join(t.Tags, " "))),
		m.ultraKeyValue(re, "due", ultraDueValue(m, t.Due)),
		m.ultraKeyValue(re, "recur", ultraOrDash(t.Recur)),
		m.ultraKeyValue(re, "start", ultraOrDash(m.formatTaskDate(t.Start))),
	}
	return strings.Join(parts, " | ")
}

func (m *Model) renderUltraDescriptionWithRegex(t task.Task, width int, re *regexp.Regexp) string {
	bodyWidth := ultraBodyWidth(width)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	text := t.Description
	if text == "" {
		text = "-"
	}

	var lines []string
	for _, line := range wordWrap(text, bodyWidth) {
		if re != nil && re.MatchString(line) {
			line = m.highlightMatches(line, re)
		}
		lines = append(lines, style.Render("  "+line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderUltraAnnotationsWithRegex(t task.Task, width int, re *regexp.Regexp) string {
	if len(t.Annotations) == 0 {
		return ""
	}

	bodyWidth := ultraBodyWidth(width)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	var lines []string
	for _, ann := range t.Annotations {
		text := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ultraOrDash(strings.TrimSpace(ann.Description)))
		for i, line := range wordWrap(text, bodyWidth) {
			if re != nil && re.MatchString(line) {
				line = m.highlightMatches(line, re)
			}
			if i > 0 {
				line = "  " + line
			}
			lines = append(lines, style.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) ultraRenderCards(tasks []task.Task, width, selected, start, cardBudget int) []string {
	if start < 0 {
		start = 0
	}
	if start > len(tasks) {
		start = len(tasks)
	}

	var lines []string
	used := 0
	for i := start; i < len(tasks); i++ {
		card := m.renderUltraCard(tasks[i], width, i == selected, m.ultraSearchRegex)
		if card == "" {
			continue
		}

		cardHeight := lipgloss.Height(card)
		if len(lines) > 0 {
			if used+1+cardHeight > cardBudget {
				break
			}
			lines = append(lines, "")
			used++
		} else if cardHeight > cardBudget {
			break
		}

		if used+cardHeight > cardBudget {
			break
		}
		lines = append(lines, strings.Split(card, "\n")...)
		used += cardHeight
	}
	return lines
}

func (m *Model) ultraStyledText(re *regexp.Regexp, style lipgloss.Style, text string) string {
	if re != nil && re.MatchString(text) {
		text = m.highlightMatches(text, re)
	}
	return style.Render(ultraOrDash(text))
}

func (m *Model) ultraKeyValue(re *regexp.Regexp, label, value string) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.HeaderFG))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	return labelStyle.Render(label+":") + " " + m.ultraStyledText(re, valueStyle, value)
}

func ultraCardStyle(theme Theme, width int, selected, blink bool) lipgloss.Style {
	style := lipgloss.NewStyle().Width(width)
	if selected {
		style = style.Foreground(lipgloss.Color(theme.SelectedFG)).Background(lipgloss.Color(theme.SelectedBG))
	}
	if blink {
		style = style.Bold(true).Reverse(true)
	}
	return style
}

func ultraPriorityToken(theme Theme, priority string) string {
	if priority == "" {
		return "-"
	}
	style := lipgloss.NewStyle().Width(1)
	switch priority {
	case "H":
		style = style.Background(lipgloss.Color(theme.PrioHighBG))
	case "M":
		style = style.Background(lipgloss.Color(theme.PrioMedBG))
	case "L":
		style = style.Background(lipgloss.Color(theme.PrioLowBG))
	}
	return style.Render(priority)
}

func ultraJoinSections(sections ...string) string {
	var parts []string
	for _, sec := range sections {
		if sec == "" {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, sec)
	}
	return strings.Join(parts, "\n")
}

func ultraBodyWidth(width int) int {
	if width <= 2 {
		return 20
	}
	w := width - 2
	if w < 20 {
		return 20
	}
	return w
}

func ultraTaskAge(entry string) string {
	if entry == "" {
		return "-"
	}
	ts, err := time.Parse(task.DateFormat, entry)
	if err != nil {
		return entry
	}
	return fmt.Sprintf("%dd", int(time.Since(ts).Hours()/24))
}

func ultraOrDash(text string) string {
	if strings.TrimSpace(text) == "" {
		return "-"
	}
	return text
}

func ultraDueValue(m *Model, due string) string {
	val := m.formatDue(due, 0)
	return ultraOrDash(val)
}

func (m *Model) ultraEnsureVisible() {
	tasks := m.ultraTaskList()
	if len(tasks) == 0 {
		m.ultraCursor = 0
		m.ultraOffset = 0
		return
	}

	if m.ultraCursor < 0 {
		m.ultraCursor = 0
	}
	if m.ultraCursor >= len(tasks) {
		m.ultraCursor = len(tasks) - 1
	}
	if m.ultraOffset < 0 {
		m.ultraOffset = 0
	}
	if m.ultraOffset >= len(tasks) {
		m.ultraOffset = len(tasks) - 1
	}

	for range tasks {
		visible := m.ultraVisibleCount()
		if visible <= 0 {
			if m.ultraOffset == m.ultraCursor {
				return
			}
			m.ultraOffset = m.ultraCursor
			continue
		}

		start := m.ultraVisibleStart(len(tasks))
		end := start + visible - 1
		if m.ultraCursor < start {
			if m.ultraOffset == m.ultraCursor {
				return
			}
			m.ultraOffset = m.ultraCursor
			continue
		}
		if m.ultraCursor > end {
			next := m.ultraCursor - visible + 1
			if next < 0 {
				next = 0
			}
			if next == m.ultraOffset {
				return
			}
			m.ultraOffset = next
			continue
		}
		return
	}
}

// handleUltraMode handles keyboard input in ultra mode.
func (m *Model) handleUltraMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m.handleQuitOrEscape()
	case "j", "down":
		m.ultraMoveCursor(1)
	case "k", "up":
		m.ultraMoveCursor(-1)
	case "pgdn", "pgdown", "space":
		m.ultraMoveCursor(m.ultraVisibleCount())
	case "pgup", "b":
		m.ultraMoveCursor(-m.ultraVisibleCount())
	case "g", "home":
		m.ultraGoHome()
	case "G", "end":
		m.ultraGoEnd()
	case "enter", "e", "E":
		return m.handleUltraEditTask()
	case "s":
		return m.handleUltraToggleStart()
	case "d":
		return m.handleUltraMarkDone()
	case "p":
		return m.handleUltraSetPriority()
	case "w":
		return m.handleUltraSetDueDate()
	case "W":
		return m.handleUltraRemoveDueDate()
	case "t":
		return m.handleUltraEditTags()
	case "a":
		return m.handleUltraAnnotate(false)
	case "A":
		return m.handleUltraAnnotate(true)
	case "J":
		return m.handleUltraEditProject()
	case "R":
		return m.handleUltraSetRecurrence()
	case "f":
		return m.handleFilter()
	case "+":
		m.ultraClearFocusedID()
		return m.handleAddTask()
	case "U":
		return m.handleUndo()
	case "c":
		return m.handleRandomTheme()
	case "C":
		return m.handleResetTheme()
	case "x":
		return m.handleToggleDisco()
	case "B":
		return m.handleToggleBlink()
	}
	return m, nil
}

func (m *Model) ultraMoveCursor(delta int) {
	m.ultraFocusedID = 0

	tasks := m.ultraTaskList()
	last := len(tasks) - 1
	if last >= 0 {
		m.ultraCursor += delta
		if m.ultraCursor < 0 {
			m.ultraCursor = 0
		}
		if m.ultraCursor > last {
			m.ultraCursor = last
		}
	}

	m.ultraEnsureVisible()
}

func (m *Model) ultraGoHome() {
	m.ultraFocusedID = 0
	m.ultraCursor = 0
	m.ultraOffset = 0
}

func (m *Model) ultraGoEnd() {
	m.ultraFocusedID = 0

	tasks := m.ultraTaskList()
	if last := len(tasks) - 1; last >= 0 {
		m.ultraCursor = last
	} else {
		m.ultraCursor = 0
	}

	m.ultraEnsureVisible()
}

func (m *Model) ultraClearFocusedID() {
	m.ultraFocusedID = 0
}

func (m *Model) ultraPrepareSelectedTask() (int, bool) {
	id, err := m.getUltraSelectedTaskID()
	if err != nil {
		return 0, false
	}
	if !m.selectTaskByID(id) {
		return 0, false
	}

	m.ultraFocusedID = id
	return id, true
}

func (m *Model) handleUltraEditTask() (tea.Model, tea.Cmd) {
	id, ok := m.ultraPrepareSelectedTask()
	if !ok {
		return m, nil
	}

	m.editID = id
	return m, editCmd(id)
}

func (m *Model) handleUltraToggleStart() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleToggleStart()
}

func (m *Model) handleUltraMarkDone() (tea.Model, tea.Cmd) {
	id, ok := m.ultraPrepareSelectedTask()
	if !ok {
		return m, nil
	}

	return m, m.startBlink(id, true)
}

func (m *Model) handleUltraSetPriority() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleSetPriority()
}

func (m *Model) handleUltraSetDueDate() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleSetDueDate()
}

func (m *Model) handleUltraRemoveDueDate() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleRemoveDueDate()
}

func (m *Model) handleUltraEditTags() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleEditTags()
}

func (m *Model) handleUltraAnnotate(replace bool) (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleAnnotate(replace)
}

func (m *Model) handleUltraEditProject() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleEditProject()
}

func (m *Model) handleUltraSetRecurrence() (tea.Model, tea.Cmd) {
	if _, ok := m.ultraPrepareSelectedTask(); !ok {
		return m, nil
	}

	return m.handleSetRecurrence()
}
