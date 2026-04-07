package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"codeberg.org/snonux/tasksamurai/internal"
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

	if len(tasks) == 0 {
		// No tasks available — render a centered placeholder instead of an empty card area.
		lines = append(lines, m.ultraNoTasksMessage(width, m.ultraCardBudget(top, bottom, overlayHeight)))
	} else {
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
	}

	lines = append(lines, bottom)
	if overlay != "" {
		lines = append(lines, overlay)
	}
	return strings.Join(lines, "\n")
}

// ultraNoTasksMessage renders a vertically and horizontally centered "No tasks" message
// sized to fill the available card budget height.
func (m *Model) ultraNoTasksMessage(width, budget int) string {
	msg := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("240")).
		Render("No tasks")

	// Pad vertically so the message appears centered in the card area.
	msgHeight := lipgloss.Height(msg)
	paddingTop := (budget - msgHeight) / 2
	if paddingTop < 0 {
		paddingTop = 0
	}

	var lines []string
	emptyLine := lipgloss.NewStyle().Width(width).Render("")
	for range paddingTop {
		lines = append(lines, emptyLine)
	}
	lines = append(lines, msg)
	return strings.Join(lines, "\n")
}

func (m Model) buildUltraHelpContent() string {
	return m.buildRenderedHelpContent(m.ultraHelpSections())
}

func (m Model) ultraHelpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation",
			items: []helpItem{
				{key: "j, k", desc: "move down/up"},
				{key: "pgup, pgdn", desc: "page up/down"},
				{key: "g, G", desc: "go to start/end"},
			},
		},
		{
			title: "Task Management",
			items: []helpItem{
				{key: "Enter, e, E", desc: "edit selected task"},
				{key: "s", desc: "start/stop task"},
				{key: "d", desc: "mark task done"},
				{key: "U", desc: "undo last done"},
				{key: "+", desc: "add new task"},
			},
		},
		{
			title: "Task Fields",
			items: []helpItem{
				{key: "p", desc: "set priority"},
				{key: "w", desc: "set due date"},
				{key: "W", desc: "remove due date"},
				{key: "t", desc: "edit tags"},
				{key: "a, A", desc: "add/replace annotations"},
				{key: "J", desc: "edit project"},
				{key: "R", desc: "edit recurrence"},
				{key: "f", desc: "change filter"},
			},
		},
		{
			title: "Search",
			items: []helpItem{
				{key: "/", desc: "search ultra cards"},
				{key: "n, N", desc: "next/previous match"},
			},
		},
		{
			title: "Appearance",
			items: []helpItem{
				{key: "c, C", desc: "random/reset theme"},
				{key: "x", desc: "toggle disco mode"},
				{key: "B", desc: "toggle blinking"},
			},
		},
		{
			title: "General",
			items: []helpItem{
				{key: "H", desc: "toggle help"},
				{key: "esc", desc: "close help/input or exit ultra mode"},
				{key: "q", desc: "exit ultra mode"},
			},
		},
	}
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
		if m.ultraTaskIndexByID(m.ultraFocusedID) >= 0 {
			_ = m.selectTaskByID(m.ultraFocusedID)
			m.ultraFocusedID = 0
			m.ultraEnsureVisible()
			return
		}
		m.ultraFocusedID = 0
	}

	m.ultraEnsureVisible()
	m.syncUltraTableSelection()
}

func (m *Model) syncUltraTableSelection() {
	tasks := m.ultraTaskList()
	cursor := m.ultraVisibleCursor(tasks)
	if cursor < 0 {
		return
	}

	row := m.taskIndexByID(tasks[cursor].ID)
	if row < 0 {
		return
	}

	prevRow := m.tbl.Cursor()
	prevCol := m.tbl.ColumnCursor()
	m.tbl.SetCursor(row)
	m.updateSelectionHighlight(prevRow, row, prevCol, m.tbl.ColumnCursor())
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
	if m.ultraSearching {
		if query := strings.TrimSpace(m.ultraSearchInput.Value()); query != "" {
			filter = query
		} else if m.ultraSearchRegex != nil {
			filter = m.ultraSearchRegex.String()
		} else if m.ultraFiltered != nil {
			filter = "filtered"
		}
	} else if m.ultraSearchRegex != nil {
		filter = m.ultraSearchRegex.String()
	} else if m.ultraFiltered != nil {
		filter = "filtered"
	}
	// Mirror the normal-mode title format so the app name is always visible,
	// with "(ultra)" appended to distinguish the view.
	return fmt.Sprintf("Task Samurai %s (ultra) | filter: %s | %d tasks", internal.Version, filter, len(tasks))
}

func (m *Model) ultraSearchText(t task.Task) string {
	return ultraJoinSections(
		m.ultraStatusText(t),
		ultraOrDash(strings.TrimSpace(t.Description)),
		m.ultraAnnotationsSearchText(t),
	)
}

func (m *Model) ultraFilteredIndexes(re *regexp.Regexp) []int {
	if re == nil {
		return nil
	}

	indexes := make([]int, 0, len(m.tasks))
	for i, t := range m.tasks {
		if re.MatchString(m.ultraSearchText(t)) {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// handleUltraSearchMode handles keystrokes while the ultra search input is open.
// Results are filtered live on every keystroke; Enter confirms (keeps the
// filter, closes the input); Esc cancels (clears the filter, closes the input).
// The search term is treated as a regular expression.
func (m *Model) handleUltraSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Empty input clears the filter; non-empty confirms and keeps it.
		if strings.TrimSpace(m.ultraSearchInput.Value()) == "" {
			m.ultraSearchRegex = nil
			m.ultraFiltered = nil
			m.ultraCursor = 0
			m.ultraOffset = 0
		}
		m.ultraSearching = false
		m.ultraSearchInput.Blur()
		return m, nil
	case "esc":
		// Cancel: clear the filter and close the search input.
		m.ultraSearching = false
		m.ultraSearchInput.SetValue("")
		m.ultraSearchInput.Blur()
		m.ultraSearchRegex = nil
		m.ultraFiltered = nil
		m.ultraCursor = 0
		m.ultraOffset = 0
		return m, nil
	}

	// Forward the keystroke to the text input widget.
	var cmd tea.Cmd
	m.ultraSearchInput, cmd = m.ultraSearchInput.Update(msg)

	// Recompile and refilter on every change for live results.
	m.ultraApplySearch(m.ultraSearchInput.Value())
	return m, cmd
}

// ultraApplySearch compiles value as a case-insensitive regex and updates the
// filtered task index. The pattern is automatically wrapped with (?i) so plain
// text searches like "foo" match "Foo" and "FOO" without extra effort. Explicit
// flags (e.g. (?-i)) in the pattern override this default. If value is empty
// the filter is cleared. If the regex is invalid (e.g. mid-typing an incomplete
// pattern) the existing filter is left unchanged so the display does not flicker.
func (m *Model) ultraApplySearch(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		m.ultraSearchRegex = nil
		m.ultraFiltered = nil
		m.ultraCursor = 0
		m.ultraOffset = 0
		return
	}
	// Prepend (?i) for case-insensitive matching unless the user already
	// supplied inline flags (patterns starting with "(?" opt in explicitly).
	pattern := value
	if !strings.HasPrefix(pattern, "(?") {
		pattern = "(?i)" + pattern
	}
	re, err := compileAndCacheRegex(pattern)
	if err != nil {
		// Keep the previous filter while the regex is incomplete.
		return
	}
	m.ultraSearchRegex = re
	m.ultraFiltered = m.ultraFilteredIndexes(re)
	m.ultraCursor = 0
	m.ultraOffset = 0
}

func (m *Model) ultraMoveSearchMatch(delta int) {
	if len(m.ultraFiltered) == 0 {
		return
	}

	m.ultraMoveCursor(delta)
}

func (m *Model) ultraCursorStatus(tasks []task.Task) string {
	cursor := m.ultraVisibleCursor(tasks)
	if cursor < 0 {
		return fmt.Sprintf("0/%d", len(tasks))
	}
	return fmt.Sprintf("%d/%d", cursor+1, len(tasks))
}

// ultraStatusText returns the plain-text representation of the single
// consolidated status line shown per card: ID, priority, status, urgency,
// due date, project, and tags. Age, recur, and start are omitted — started
// tasks are highlighted in yellow, and the remaining fields are available in
// the detail view.
func (m *Model) ultraStatusText(t task.Task) string {
	return strings.Join(
		[]string{
			fmt.Sprintf("#%d", t.ID),
			ultraOrDash(t.Priority),
			ultraOrDash(t.Status),
			fmt.Sprintf("%.1f", t.Urgency),
			"due: " + ultraDueValue(m, t.Due),
			"proj: " + ultraOrDash(t.Project),
			"tags: " + ultraOrDash(strings.Join(t.Tags, " ")),
		},
		" | ",
	)
}

func (m *Model) ultraDescriptionLines(t task.Task, width int) []string {
	text := t.Description
	if text == "" {
		text = "-"
	}

	// No leading indent — keep description flush with the card edge for a compact layout.
	return wordWrap(text, ultraBodyWidth(width))
}

func (m *Model) ultraDescriptionText(t task.Task, width int) string {
	return strings.Join(m.ultraDescriptionLines(t, width), "\n")
}

func (m *Model) ultraAnnotationsLines(t task.Task, width int) []string {
	if len(t.Annotations) == 0 {
		return nil
	}

	bodyWidth := ultraBodyWidth(width)
	var lines []string
	for _, ann := range t.Annotations {
		text := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ultraOrDash(strings.TrimSpace(ann.Description)))
		// No indent on continuation lines — keep annotations flush for a compact layout.
		lines = append(lines, wordWrap(text, bodyWidth)...)
	}
	return lines
}

func (m *Model) ultraAnnotationsText(t task.Task, width int) string {
	return strings.Join(m.ultraAnnotationsLines(t, width), "\n")
}

func (m *Model) ultraAnnotationsSearchText(t task.Task) string {
	if len(t.Annotations) == 0 {
		return ""
	}

	lines := make([]string, 0, len(t.Annotations))
	for _, ann := range t.Annotations {
		line := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ultraOrDash(strings.TrimSpace(ann.Description)))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderUltraCard assembles the card sections and applies the outer selection style.
// bg is threaded through all inner render calls so that ANSI resets emitted by
// per-field styles do not expose the terminal-default black background inside a
// selected (grey) card.
func (m *Model) renderUltraCard(t task.Task, width int, selected bool, re *regexp.Regexp) string {
	bg := ""
	if selected {
		bg = m.theme.SelectedBG
	}

	// Single status line (ID, priority, status, urgency, due, proj, tags)
	// followed by description and annotations — no blank lines between sections.
	card := ultraJoinSections(
		m.renderUltraStatusWithRegex(t, width, re, bg),
		m.renderUltraDescriptionWithRegex(t, width, re, bg),
		m.renderUltraAnnotationsWithRegex(t, width, re, bg),
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
	started := t.Start != ""
	return ultraCardStyle(m.theme, width, selected, started, blink).Render(card)
}

// renderUltraStatus renders the consolidated single-line card status (no selection bg).
func (m *Model) renderUltraStatus(t task.Task, width int) string {
	return m.renderUltraStatusWithRegex(t, width, m.ultraSearchRegex, "")
}

// renderUltraDescription renders the wrapped task description body (no selection bg).
func (m *Model) renderUltraDescription(t task.Task, width int) string {
	return m.renderUltraDescriptionWithRegex(t, width, m.ultraSearchRegex, "")
}

// renderUltraAnnotations renders wrapped annotation lines with timestamps (no selection bg).
func (m *Model) renderUltraAnnotations(t task.Task, width int) string {
	return m.renderUltraAnnotationsWithRegex(t, width, m.ultraSearchRegex, "")
}

// renderUltraStatusWithRegex renders a single consolidated status line combining
// the former header (ID, priority, status, urgency) and meta (due, project, tags)
// fields. Age, recur, and start are omitted for compactness.
func (m *Model) renderUltraStatusWithRegex(t task.Task, width int, re *regexp.Regexp, bg string) string {
	_ = width
	idText := fmt.Sprintf("#%d", t.ID)
	priorityText := ultraOrDash(t.Priority)
	statusText := ultraOrDash(t.Status)
	urgencyText := fmt.Sprintf("%.1f", t.Urgency)
	due := ultraDueValue(m, t.Due)
	project := ultraOrDash(t.Project)
	tags := ultraOrDash(strings.Join(t.Tags, " "))

	// Priority badges render as 3-char wide pills in the styled path (Width(3)
	// + Center). Pad the plain text to match so whole-line and styled paths
	// produce the same visible text after ANSI stripping.
	priorityPadded := priorityText
	if t.Priority == "H" || t.Priority == "M" || t.Priority == "L" {
		priorityPadded = " " + t.Priority + " "
	}

	line := strings.Join([]string{
		idText, priorityPadded, statusText, urgencyText,
		"due: " + due, "proj: " + project, "tags: " + tags,
	}, " | ")
	// Fall back to whole-line rendering when the regex spans a field separator
	// or a full "key: value" pair (e.g. "proj: home") that can't be matched by
	// checking label and value individually.
	if re != nil && re.MatchString(line) && !ultraRegexMatchesAny(re,
		idText, priorityText, statusText, urgencyText, due, project, tags,
	) {
		return m.renderUltraSearchLine(line, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("253")), re, bg)
	}

	sep := ultraFieldSep(bg)
	id := m.ultraStyledText(re, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("253")), idText, bg)
	// H/M/L badges keep their own coloured background; plain "-" uses the card bg.
	priorityBG := bg
	if t.Priority == "H" || t.Priority == "M" || t.Priority == "L" {
		priorityBG = ""
	}
	priority := m.ultraStyledText(re, ultraPriorityStyle(m.theme, t.Priority), priorityText, priorityBG)
	status := m.ultraStyledText(re, lipgloss.NewStyle().Foreground(lipgloss.Color("246")), statusText, bg)
	urgency := m.ultraStyledText(re, lipgloss.NewStyle().Foreground(lipgloss.Color("214")), urgencyText, bg)
	parts := []string{
		id, priority, status, urgency,
		m.ultraKeyValue(re, "due", due, bg),
		m.ultraKeyValue(re, "proj", project, bg),
		m.ultraKeyValue(re, "tags", tags, bg),
	}
	return strings.Join(parts, sep)
}

func (m *Model) renderUltraDescriptionWithRegex(t task.Task, width int, re *regexp.Regexp, bg string) string {
	// Brighter foreground ("253") than annotations to give description visual priority.
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("253"))
	if bg != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	var lines []string
	for _, line := range m.ultraDescriptionLines(t, width) {
		if re != nil && re.MatchString(line) {
			line = m.highlightMatches(line, re)
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderUltraAnnotationsWithRegex(t task.Task, width int, re *regexp.Regexp, bg string) string {
	lines := m.ultraAnnotationsLines(t, width)
	if len(lines) == 0 {
		return ""
	}

	// Dimmer than description ("244") to visually subordinate annotations.
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	if bg != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	for i, line := range lines {
		if re != nil && re.MatchString(line) {
			line = m.highlightMatches(line, re)
		}
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

// ultraSeparator returns a full-width dim line used between cards.
func ultraSeparator(width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(strings.Repeat("─", width))
}

func (m *Model) ultraRenderCards(tasks []task.Task, width, selected, start, cardBudget int) []string {
	if start < 0 {
		start = 0
	}
	if start > len(tasks) {
		start = len(tasks)
	}

	sep := ultraSeparator(width)
	var lines []string
	used := 0
	for i := start; i < len(tasks); i++ {
		card := m.renderUltraCard(tasks[i], width, i == selected, m.ultraSearchRegex)
		if card == "" {
			continue
		}

		cardHeight := lipgloss.Height(card)
		// Account for separator line (1 line) between cards.
		if len(lines) > 0 {
			if used+1+cardHeight > cardBudget {
				break
			}
			lines = append(lines, sep)
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

// ultraStyledText renders text with style, applying bg as the background so
// ANSI resets emitted by the inner Render call do not expose the terminal
// default (black) inside a selected card.
func (m *Model) ultraStyledText(re *regexp.Regexp, style lipgloss.Style, text, bg string) string {
	if bg != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	if re != nil && re.MatchString(text) {
		text = m.highlightMatches(text, re)
	}
	return style.Render(ultraOrDash(text))
}

// renderUltraSearchLine renders a full-line search match, applying bg when set.
func (m *Model) renderUltraSearchLine(text string, style lipgloss.Style, re *regexp.Regexp, bg string) string {
	if bg != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	if re != nil && re.MatchString(text) {
		text = m.highlightMatches(text, re)
	}
	return style.Render(text)
}

// ultraFieldSep returns the field separator with bg applied so it stays on the
// card background even between styled spans.
func ultraFieldSep(bg string) string {
	sep := " | "
	if bg != "" {
		return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(sep)
	}
	return sep
}

func (m *Model) ultraKeyValue(re *regexp.Regexp, label, value, bg string) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.HeaderFG))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	// The space between label and value must also carry bg; a plain " " would
	// expose the terminal default (black) between the two styled spans.
	space := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(" ")
	if bg == "" {
		space = " "
	}
	return m.ultraStyledText(re, labelStyle, label+":", bg) + space + m.ultraStyledText(re, valueStyle, value, bg)
}

func ultraCardStyle(theme Theme, width int, selected, started, blink bool) lipgloss.Style {
	style := lipgloss.NewStyle().Width(width)
	if started {
		// Amber yellow background marks in-progress tasks; selection overrides it
		// when the cursor is also on this card.
		fg := contrastColor(theme.UltraStartedBG)
		style = style.Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(theme.UltraStartedBG))
	}
	if selected {
		// Selection highlight takes priority over the started colour.
		style = style.Foreground(lipgloss.Color(theme.SelectedFG)).Background(lipgloss.Color(theme.SelectedBG))
	}
	if blink {
		style = style.Bold(true).Reverse(true)
	}
	return style
}

func ultraPriorityStyle(theme Theme, priority string) lipgloss.Style {
	// Width(3) so the badge reads as a pill rather than a single character.
	style := lipgloss.NewStyle().Width(3).Align(lipgloss.Center).Bold(true).Foreground(lipgloss.Color("255"))
	switch priority {
	case "H":
		return style.Background(lipgloss.Color(theme.PrioHighBG))
	case "M":
		return style.Background(lipgloss.Color(theme.PrioMedBG))
	case "L":
		return style.Background(lipgloss.Color(theme.PrioLowBG))
	}
	// No priority — render dimly without a badge background.
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
}

func ultraJoinSections(sections ...string) string {
	return ultraJoinSectionsWithBlank("", sections...)
}

// ultraJoinSectionsWithBlank joins non-empty sections separated by blankLine.
// When blankLine is a styled empty string (e.g. with a background colour),
// the inter-section gap carries that style instead of falling back to the
// terminal default. A blankLine of "" means no separator — sections are joined
// with a plain newline for a compact, gap-free layout.
func ultraJoinSectionsWithBlank(blankLine string, sections ...string) string {
	var parts []string
	for _, sec := range sections {
		if sec == "" {
			continue
		}
		if len(parts) > 0 && blankLine != "" {
			// Only insert the blank line when one was explicitly provided.
			parts = append(parts, blankLine)
		}
		parts = append(parts, sec)
	}
	return strings.Join(parts, "\n")
}

func ultraRegexMatchesAny(re *regexp.Regexp, parts ...string) bool {
	if re == nil {
		return false
	}
	for _, part := range parts {
		if re.MatchString(part) {
			return true
		}
	}
	return false
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
	case "H":
		return m.handleToggleHelp()
	case "q", "esc":
		return m.handleQuitOrEscape()
	case "u":
		// Toggle back to the traditional table view. Works even when started
		// via --ultra because the table model always exists; it was just never
		// shown. The user can press u again to return to ultra mode.
		m.ultraClearFocusedID()
		m.showUltra = false
		m.ultraStartup = false // no longer forced into ultra-only mode
		m.ultraSearchRegex = nil
		m.ultraFiltered = nil
		m.ultraSearchInput.SetValue("")
		// Sync the table cursor to the task we were on in ultra mode.
		tasks := m.ultraTaskList()
		if m.ultraCursor >= 0 && m.ultraCursor < len(tasks) {
			m.tbl.SetCursor(m.ultraCursor)
		}
		return m, nil
	case "/":
		m.ultraSearching = true
		m.ultraSearchInput.SetValue("")
		m.ultraSearchInput.Focus()
		return m, nil
	case "j", "down":
		m.ultraMoveCursor(1)
	case "k", "up":
		m.ultraMoveCursor(-1)
	case "n":
		m.ultraMoveSearchMatch(1)
	case "N":
		m.ultraMoveSearchMatch(-1)
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
