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
	overlay, overlayHeight := m.ultraSearchOverlay()

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
	return ultraCardStyle(m.theme, width, selected).Render(card)
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

func ultraCardStyle(theme Theme, width int, selected bool) lipgloss.Style {
	style := lipgloss.NewStyle().Width(width)
	if selected {
		return style.Foreground(lipgloss.Color(theme.SelectedFG)).Background(lipgloss.Color(theme.SelectedBG))
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

// handleUltraMode handles keyboard input in ultra mode.
func (m *Model) handleUltraMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m.handleQuitOrEscape()
	}
	return m, nil
}
