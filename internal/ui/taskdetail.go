package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Define field indices for navigation
const (
	fieldID = iota
	fieldUUID
	fieldStatus
	fieldPriority
	fieldTags
	fieldDue
	fieldStart
	fieldProject
	fieldEntry
	fieldRecur
	fieldDescription
	fieldAnnotations
	fieldCount // Total number of fields
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
	currentField := 0
	lines = append(lines, m.renderTaskFieldWithIndex("ID", fmt.Sprintf("%d", t.ID), labelStyle, valueStyle, currentField))
	currentField++
	lines = append(lines, m.renderTaskFieldWithIndex("UUID", t.UUID, labelStyle, valueStyle, currentField))
	currentField++
	lines = append(lines, m.renderTaskFieldWithIndex("Status", t.Status, labelStyle, valueStyle, currentField))
	currentField++

	// Priority with color
	if m.prioritySelecting && m.priorityID == t.ID {
		// Show priority selection UI
		lines = append(lines, m.renderEditingField("Priority", m.priorityView(false), labelStyle, currentField))
	} else {
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
		lines = append(lines, m.renderTaskFieldWithIndex("Priority", priorityValue, labelStyle, priorityStyle, currentField))
	}
	currentField++

	// Tags
	if m.tagsEditing && m.tagsID == t.ID {
		// Show tags editing UI without prompt
		originalPrompt := m.tagsInput.Prompt
		m.tagsInput.Prompt = ""
		tagsView := m.tagsInput.View()
		m.tagsInput.Prompt = originalPrompt
		lines = append(lines, m.renderEditingField("Tags", tagsView, labelStyle, currentField))
	} else {
		tagStr := strings.Join(t.Tags, ", ")
		if tagStr == "" {
			tagStr = "-"
		}
		lines = append(lines, m.renderTaskFieldWithIndex("Tags", tagStr, labelStyle, valueStyle, currentField))
	}
	currentField++

	// Dates
	if m.dueEditing && m.dueID == t.ID {
		// Show due date editing UI
		lines = append(lines, m.renderEditingField("Due", m.dueView(false), labelStyle, currentField))
	} else {
		lines = append(lines, m.renderTaskFieldWithIndex("Due", m.formatTaskDate(t.Due), labelStyle, valueStyle, currentField))
	}
	currentField++
	lines = append(lines, m.renderTaskFieldWithIndex("Start", m.formatTaskDate(t.Start), labelStyle, valueStyle, currentField))
	currentField++
	
	// Project
	if m.projEditing && m.projID == t.ID {
		// Show project editing UI without prompt
		originalPrompt := m.projInput.Prompt
		m.projInput.Prompt = ""
		projView := m.projInput.View()
		m.projInput.Prompt = originalPrompt
		lines = append(lines, m.renderEditingField("Project", projView, labelStyle, currentField))
	} else {
		projectValue := t.Project
		if projectValue == "" {
			projectValue = "-"
		}
		lines = append(lines, m.renderTaskFieldWithIndex("Project", projectValue, labelStyle, valueStyle, currentField))
	}
	currentField++
	
	lines = append(lines, m.renderTaskFieldWithIndex("Entry", m.formatTaskDate(t.Entry), labelStyle, valueStyle, currentField))
	currentField++

	// Recurrence
	if t.Recur != "" {
		if m.recurEditing && m.recurID == t.ID {
			// Show recurrence editing UI without prompt
			originalPrompt := m.recurInput.Prompt
			m.recurInput.Prompt = ""
			recurView := m.recurInput.View()
			m.recurInput.Prompt = originalPrompt
			lines = append(lines, m.renderEditingField("Recurrence", recurView, labelStyle, currentField))
		} else {
			lines = append(lines, m.renderTaskFieldWithIndex("Recurrence", t.Recur, labelStyle, valueStyle, currentField))
		}
		currentField++
	}

	// Description - with full space
	lines = append(lines, "")
	descLabelStyle := labelStyle.Copy()
	descValueStyle := descStyle.Copy()
	// Apply blinking if this field is blinking
	if m.detailBlinkField == currentField && m.detailBlinkOn {
		blinkBG := lipgloss.Color("226") // Bright yellow
		descLabelStyle = descLabelStyle.Background(blinkBG).Foreground(lipgloss.Color("0"))
		descValueStyle = descValueStyle.Background(blinkBG).Foreground(lipgloss.Color("0"))
	} else if m.detailFieldIndex == currentField {
		descLabelStyle = descLabelStyle.Background(lipgloss.Color(m.theme.SelectedBG))
		descValueStyle = descValueStyle.Background(lipgloss.Color(m.theme.SelectedBG))
	}
	lines = append(lines, descLabelStyle.Render("Description:"))
	if m.detailDescEditing {
		// Show editing indicator
		editingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Italic(true)
		lines = append(lines, editingStyle.Render("  [Editing in external editor...]"))
	} else if t.Description != "" {
		// Highlight search matches if searching
		desc := t.Description
		if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(desc) {
			desc = m.highlightMatches(desc, m.detailSearchRegex)
		}
		lines = append(lines, descValueStyle.Render(desc))
	} else {
		lines = append(lines, descValueStyle.Render("-"))
	}
	currentField++

	// Annotations
	if len(t.Annotations) > 0 {
		lines = append(lines, "")
		annLabelStyle := labelStyle.Copy()
		annValueStyle := descStyle.Copy()
		if m.detailFieldIndex == currentField {
			annLabelStyle = annLabelStyle.Background(lipgloss.Color(m.theme.SelectedBG))
			annValueStyle = annValueStyle.Background(lipgloss.Color(m.theme.SelectedBG))
		}
		lines = append(lines, annLabelStyle.Render("Annotations:"))
		for _, ann := range t.Annotations {
			annText := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ann.Description)
			// Highlight search matches
			if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(annText) {
				annText = m.highlightMatches(annText, m.detailSearchRegex)
			}
			lines = append(lines, annValueStyle.Render(annText))
		}
	}

	// Instructions at bottom
	lines = append(lines, "")
	lines = append(lines, "")
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)
	// Check if we're in any editing mode
	if m.prioritySelecting || m.tagsEditing || m.dueEditing || m.recurEditing || m.detailDescEditing {
		lines = append(lines, instructionStyle.Render("Editing mode - Follow on-screen prompts"))
	} else {
		lines = append(lines, instructionStyle.Render("Press ESC or q to return to table view"))
		lines = append(lines, instructionStyle.Render("Use ↑/k and ↓/j to navigate fields"))
		lines = append(lines, instructionStyle.Render("Press i or Enter to edit (Priority, Tags, Due, Recurrence, Description)"))
		if m.detailSearching {
			lines = append(lines, instructionStyle.Render("Type to search, Enter to confirm"))
		} else {
			lines = append(lines, instructionStyle.Render("Press / to search"))
		}
	}

	// Add search input if searching
	if m.detailSearching {
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
			PaddingTop(1)
		lines = append(lines, searchStyle.Render("Search: "+m.detailSearchInput.View()))
	}

	return strings.Join(lines, "\n")
}

// renderEditingField renders a field that is currently being edited
func (m *Model) renderEditingField(label, editView string, labelStyle lipgloss.Style, fieldIndex int) string {
	// Apply selection highlighting if this field is selected
	if m.detailFieldIndex == fieldIndex {
		labelStyle = labelStyle.Background(lipgloss.Color(m.theme.SelectedBG))
	}

	return fmt.Sprintf("%s %s", labelStyle.Render(label+":"), editView)
}

// renderTaskFieldWithIndex renders a single field with highlighting based on index
func (m *Model) renderTaskFieldWithIndex(label, value string, labelStyle, valueStyle lipgloss.Style, fieldIndex int) string {
	if value == "" {
		value = "-"
	}

	// Apply blinking if this field is blinking
	if m.detailBlinkField == fieldIndex && m.detailBlinkOn {
		// Use a bright background for blinking
		blinkBG := lipgloss.Color("226") // Bright yellow
		labelStyle = labelStyle.Background(blinkBG).Foreground(lipgloss.Color("0"))
		valueStyle = valueStyle.Background(blinkBG).Foreground(lipgloss.Color("0"))
	} else if m.detailFieldIndex == fieldIndex {
		// Apply selection highlighting if this field is selected
		labelStyle = labelStyle.Background(lipgloss.Color(m.theme.SelectedBG))
		valueStyle = valueStyle.Background(lipgloss.Color(m.theme.SelectedBG))
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

// refreshCurrentTaskDetail updates the current task detail pointer after a reload
func (m *Model) refreshCurrentTaskDetail() {
	if m.currentTaskDetail == nil {
		return
	}

	id := m.currentTaskDetail.ID
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			m.currentTaskDetail = &m.tasks[i]
			return
		}
	}

	// Task no longer exists, clear detail view
	m.showTaskDetail = false
	m.currentTaskDetail = nil
}

// getDetailFieldCount returns the actual number of navigable fields for the current task
func (m *Model) getDetailFieldCount() int {
	if m.currentTaskDetail == nil {
		return 0
	}

	// Basic fields that are always present: ID, UUID, Status, Priority, Tags, Due, Start, Entry, Description
	count := 9

	// Add recurrence if present
	if m.currentTaskDetail.Recur != "" {
		count++
	}

	// Add annotations if present
	if len(m.currentTaskDetail.Annotations) > 0 {
		count++
	}

	return count
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
