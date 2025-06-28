package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

)

// renderTaskDetail renders the detailed view of a single task
func (m *Model) renderTaskDetail() string {
	if m.currentTaskDetail == nil {
		return "No task selected"
	}

	t := m.currentTaskDetail
	
	// Create styles based on theme
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.SelectedFG)).
		Background(lipgloss.Color(m.theme.SelectedBG)).
		Padding(0, 1)
		
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.HeaderFG))
		
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
		
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		PaddingLeft(2)
	
	// Build the detail view
	var lines []string
	
	// Title bar
	title := fmt.Sprintf("Task %d Details", t.ID)
	lines = append(lines, titleStyle.Render(title))
	lines = append(lines, "")
	
	// Task fields
	lines = append(lines, m.renderTaskField("ID", fmt.Sprintf("%d", t.ID), labelStyle, valueStyle))
	lines = append(lines, m.renderTaskField("UUID", t.UUID, labelStyle, valueStyle))
	lines = append(lines, m.renderTaskField("Status", t.Status, labelStyle, valueStyle))
	
	// Priority with color
	priorityValue := t.Priority
	if priorityValue == "" {
		priorityValue = "-"
	}
	priorityStyle := valueStyle.Copy()
	switch t.Priority {
	case "H":
		priorityStyle = priorityStyle.Background(lipgloss.Color(m.theme.PrioHighBG))
		priorityValue = "H (High)"
	case "M":
		priorityStyle = priorityStyle.Background(lipgloss.Color(m.theme.PrioMedBG))
		priorityValue = "M (Medium)"
	case "L":
		priorityStyle = priorityStyle.Background(lipgloss.Color(m.theme.PrioLowBG))
		priorityValue = "L (Low)"
	}
	lines = append(lines, m.renderTaskField("Priority", priorityValue, labelStyle, priorityStyle))
	
	// Tags
	tagStr := strings.Join(t.Tags, ", ")
	if tagStr == "" {
		tagStr = "-"
	}
	lines = append(lines, m.renderTaskField("Tags", tagStr, labelStyle, valueStyle))
	
	// Dates
	lines = append(lines, m.renderTaskField("Due", m.formatTaskDate(t.Due), labelStyle, valueStyle))
	lines = append(lines, m.renderTaskField("Start", m.formatTaskDate(t.Start), labelStyle, valueStyle))
	// End field doesn't exist in Task struct, removed
	lines = append(lines, m.renderTaskField("Entry", m.formatTaskDate(t.Entry), labelStyle, valueStyle))
	// Modified field doesn't exist in Task struct, removed
	
	// Recurrence
	if t.Recur != "" {
		lines = append(lines, m.renderTaskField("Recurrence", t.Recur, labelStyle, valueStyle))
	}
	
	// Description - with full space
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Description:"))
	if t.Description != "" {
		// Highlight search matches if searching
		desc := t.Description
		if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(desc) {
			desc = m.highlightMatches(desc, m.detailSearchRegex)
		}
		lines = append(lines, descStyle.Render(desc))
	} else {
		lines = append(lines, descStyle.Render("-"))
	}
	
	// Annotations
	if len(t.Annotations) > 0 {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Annotations:"))
		for _, ann := range t.Annotations {
			annText := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ann.Description)
			// Highlight search matches
			if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(annText) {
				annText = m.highlightMatches(annText, m.detailSearchRegex)
			}
			lines = append(lines, descStyle.Render(annText))
		}
	}
	
	// Instructions at bottom
	lines = append(lines, "")
	lines = append(lines, "")
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)
	lines = append(lines, instructionStyle.Render("Press ESC or Q to return to table view"))
	if m.detailSearching {
		lines = append(lines, instructionStyle.Render("Type to search, Enter to confirm"))
	} else {
		lines = append(lines, instructionStyle.Render("Press / to search, N/n to navigate matches"))
	}
	
	// Add search input if searching
	if m.detailSearching {
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
			PaddingTop(1)
		lines = append(lines, searchStyle.Render("Search: " + m.detailSearchInput.View()))
	}
	
	return strings.Join(lines, "\n")
}

// renderTaskField renders a single field in the task detail view
func (m *Model) renderTaskField(label, value string, labelStyle, valueStyle lipgloss.Style) string {
	if value == "" {
		value = "-"
	}
	// Highlight search matches
	if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(value) {
		value = m.highlightMatches(value, m.detailSearchRegex)
	}
	return fmt.Sprintf("%s %s", labelStyle.Render(label+":"), valueStyle.Render(value))
}

// formatTaskDate formats a task date for display
func (m *Model) formatTaskDate(dateStr string) string {
	if dateStr == "" {
		return "-"
	}
	// Try to parse and format nicely
	if ts, err := parseTaskDate(dateStr); err == nil {
		return ts.Format("2006-01-02 15:04")
	}
	return dateStr
}

// highlightMatches highlights regex matches in a string
func (m *Model) highlightMatches(text string, re *regexp.Regexp) string {
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.SearchBG)).
		Foreground(lipgloss.Color(m.theme.SearchFG))
	
	matches := re.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	
	var result strings.Builder
	lastEnd := 0
	
	for _, match := range matches {
		start, end := match[0], match[1]
		// Add text before match
		if start > lastEnd {
			result.WriteString(text[lastEnd:start])
		}
		// Add highlighted match
		result.WriteString(highlightStyle.Render(text[start:end]))
		lastEnd = end
	}
	
	// Add remaining text
	if lastEnd < len(text) {
		result.WriteString(text[lastEnd:])
	}
	
	return result.String()
}