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

// renderUltraModus is still a placeholder until the full ultra-mode layout lands.
func (m *Model) renderUltraModus() string {
	return "Ultra Modus (TODO)"
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
