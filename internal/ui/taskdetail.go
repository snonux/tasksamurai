package ui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// wordWrap wraps text to fit within the specified width, breaking at word boundaries
func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	currentLine := words[0]
	for i := 1; i < len(words); i++ {
		word := words[i]
		testLine := currentLine + " " + word
		if len(testLine) > width {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

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

// renderTaskDetail renders the detailed view of a single task.
// It delegates each visual section to a focused helper so that the
// overall structure is easy to follow at a glance.
func (m *Model) renderTaskDetail() string {
	if m.currentTaskDetail == nil {
		return "No task selected"
	}
	t := m.currentTaskDetail

	titleStyle, labelStyle, valueStyle, descStyle := m.detailStyles()

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("Task %d Details", t.ID)))
	lines = append(lines, "")
	lines, nextField := m.renderDetailFieldRows(lines, labelStyle, valueStyle)
	lines = m.renderDetailDescription(lines, nextField, labelStyle, descStyle)
	nextField++
	lines = m.renderDetailAnnotations(lines, nextField, labelStyle, descStyle)
	lines = m.renderDetailFooter(lines)
	return strings.Join(lines, "\n")
}

// detailStyles returns the four lipgloss styles shared by the detail-view helpers.
func (m *Model) detailStyles() (title, label, value, desc lipgloss.Style) {
	title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.SelectedFG)).
		Background(lipgloss.Color(m.theme.SelectedBG)).
		Padding(0, 1)
	label = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.theme.HeaderFG))
	value = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	desc = lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		PaddingLeft(2)
	return
}

// renderDetailFieldRows appends the fixed and optional task fields (ID through
// optional Recurrence) to lines and returns the updated slice together with the
// field index of the next unrendered field (Description).
func (m *Model) renderDetailFieldRows(lines []string, labelStyle, valueStyle lipgloss.Style) ([]string, int) {
	t := m.currentTaskDetail
	cf := 0 // current field counter

	lines = append(lines, m.renderTaskFieldWithIndex("ID", fmt.Sprintf("%d", t.ID), labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderTaskFieldWithIndex("UUID", t.UUID, labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderTaskFieldWithIndex("Status", t.Status, labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderDetailPriorityField(labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderDetailTagsField(labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderDetailDueField(labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderTaskFieldWithIndex("Start", m.formatTaskDate(t.Start), labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderDetailProjectField(labelStyle, valueStyle, cf))
	cf++
	lines = append(lines, m.renderTaskFieldWithIndex("Entry", m.formatTaskDate(t.Entry), labelStyle, valueStyle, cf))
	cf++
	if t.Recur != "" {
		lines = append(lines, m.renderDetailRecurField(labelStyle, cf))
		cf++
	}
	return lines, cf
}

// renderDetailPriorityField renders the Priority row, showing the selection
// widget when the user is actively changing it.
func (m *Model) renderDetailPriorityField(labelStyle, valueStyle lipgloss.Style, cf int) string {
	t := m.currentTaskDetail
	if m.prioritySelecting && m.priorityID == t.ID {
		return m.renderEditingField("Priority", m.priorityView(false), labelStyle, cf)
	}
	pv := t.Priority
	if pv == "" {
		pv = "-"
	}
	ps := valueStyle
	switch t.Priority {
	case "H":
		ps = ps.Background(lipgloss.Color(m.theme.PrioHighBG))
		pv = "H (High)"
	case "M":
		ps = ps.Background(lipgloss.Color(m.theme.PrioMedBG))
		pv = "M (Medium)"
	case "L":
		ps = ps.Background(lipgloss.Color(m.theme.PrioLowBG))
		pv = "L (Low)"
	}
	return m.renderTaskFieldWithIndex("Priority", pv, labelStyle, ps, cf)
}

// renderDetailTagsField renders the Tags row, showing the text input when
// the user is actively editing it.
func (m *Model) renderDetailTagsField(labelStyle, valueStyle lipgloss.Style, cf int) string {
	t := m.currentTaskDetail
	if m.tagsEditing && m.tagsID == t.ID {
		orig := m.tagsInput.Prompt
		m.tagsInput.Prompt = ""
		v := m.tagsInput.View()
		m.tagsInput.Prompt = orig
		return m.renderEditingField("Tags", v, labelStyle, cf)
	}
	tagStr := strings.Join(t.Tags, ", ")
	if tagStr == "" {
		tagStr = "-"
	}
	return m.renderTaskFieldWithIndex("Tags", tagStr, labelStyle, valueStyle, cf)
}

// renderDetailDueField renders the Due row, showing the date picker when
// the user is actively editing it.
func (m *Model) renderDetailDueField(labelStyle, valueStyle lipgloss.Style, cf int) string {
	t := m.currentTaskDetail
	if m.dueEditing && m.dueID == t.ID {
		return m.renderEditingField("Due", m.dueView(false), labelStyle, cf)
	}
	return m.renderTaskFieldWithIndex("Due", m.formatTaskDate(t.Due), labelStyle, valueStyle, cf)
}

// renderDetailProjectField renders the Project row, showing the text input
// when the user is actively editing it.
func (m *Model) renderDetailProjectField(labelStyle, valueStyle lipgloss.Style, cf int) string {
	t := m.currentTaskDetail
	if m.projEditing && m.projID == t.ID {
		orig := m.projInput.Prompt
		m.projInput.Prompt = ""
		v := m.projInput.View()
		m.projInput.Prompt = orig
		return m.renderEditingField("Project", v, labelStyle, cf)
	}
	pv := t.Project
	if pv == "" {
		pv = "-"
	}
	return m.renderTaskFieldWithIndex("Project", pv, labelStyle, valueStyle, cf)
}

// renderDetailRecurField renders the Recurrence row, showing the text input
// when the user is actively editing it.
func (m *Model) renderDetailRecurField(labelStyle lipgloss.Style, cf int) string {
	t := m.currentTaskDetail
	if m.recurEditing && m.recurID == t.ID {
		orig := m.recurInput.Prompt
		m.recurInput.Prompt = ""
		v := m.recurInput.View()
		m.recurInput.Prompt = orig
		return m.renderEditingField("Recurrence", v, labelStyle, cf)
	}
	return m.renderTaskFieldWithIndex("Recurrence", t.Recur, labelStyle, lipgloss.NewStyle(), cf)
}

// renderDetailDescription appends the Description section (label + wrapped body)
// to lines, applying selection/blink highlighting and search match colouring.
func (m *Model) renderDetailDescription(lines []string, cf int, labelStyle, descStyle lipgloss.Style) []string {
	t := m.currentTaskDetail
	lines = append(lines, "")

	ls, vs := labelStyle, descStyle
	if m.detailBlinkField == cf && m.detailBlinkOn {
		bg := lipgloss.Color("226")
		ls = ls.Background(bg).Foreground(lipgloss.Color("0"))
		vs = vs.Background(bg).Foreground(lipgloss.Color("0"))
	} else if m.detailFieldIndex == cf {
		ls = ls.Background(lipgloss.Color(m.theme.SelectedBG))
		vs = vs.Background(lipgloss.Color(m.theme.SelectedBG))
	}
	lines = append(lines, ls.Render("Description:"))

	if m.detailDescEditing {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Italic(true).
			Render("  [Editing in external editor...]"))
		return lines
	}
	if t.Description == "" {
		return append(lines, vs.Render("-"))
	}
	w := m.tbl.Width() - 4
	if w < 20 {
		w = 20
	}
	for _, l := range wordWrap(t.Description, w) {
		d := l
		if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(l) {
			d = m.highlightMatches(l, m.detailSearchRegex)
		}
		lines = append(lines, vs.Render(d))
	}
	return lines
}

// renderDetailAnnotations appends the Annotations section to lines when the
// task has annotations, applying selection highlighting and search colouring.
func (m *Model) renderDetailAnnotations(lines []string, cf int, labelStyle, descStyle lipgloss.Style) []string {
	t := m.currentTaskDetail
	if len(t.Annotations) == 0 {
		return lines
	}
	lines = append(lines, "")
	ls, vs := labelStyle, descStyle
	if m.detailFieldIndex == cf {
		ls = ls.Background(lipgloss.Color(m.theme.SelectedBG))
		vs = vs.Background(lipgloss.Color(m.theme.SelectedBG))
	}
	lines = append(lines, ls.Render("Annotations:"))
	w := m.tbl.Width() - 4
	if w < 20 {
		w = 20
	}
	for _, ann := range t.Annotations {
		text := fmt.Sprintf("[%s] %s", m.formatTaskDate(ann.Entry), ann.Description)
		for i, l := range wordWrap(text, w) {
			d := l
			if i > 0 {
				d = "  " + d
			}
			if m.detailSearchRegex != nil && m.detailSearchRegex.MatchString(l) {
				d = m.highlightMatches(d, m.detailSearchRegex)
			}
			lines = append(lines, vs.Render(d))
		}
	}
	return lines
}

// renderDetailFooter appends the instruction lines and optional search input
// at the bottom of the detail view.
func (m *Model) renderDetailFooter(lines []string) []string {
	lines = append(lines, "", "")
	ist := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	if m.prioritySelecting || m.tagsEditing || m.dueEditing || m.recurEditing || m.detailDescEditing {
		lines = append(lines, ist.Render("Editing mode - Follow on-screen prompts"))
	} else {
		lines = append(lines, ist.Render("Press ESC or q to return to table view"))
		lines = append(lines, ist.Render("Use ↑/k and ↓/j to navigate fields"))
		lines = append(lines, ist.Render("Press i or Enter to edit (Priority, Tags, Due, Recurrence, Description)"))
		if m.detailSearching {
			lines = append(lines, ist.Render("Type to search, Enter to confirm"))
		} else {
			lines = append(lines, ist.Render("Press / to search"))
		}
	}
	if m.detailSearching {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("248")).PaddingTop(1).
			Render("Search: "+m.detailSearchInput.View()))
	}
	return lines
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

// detailDescriptionFieldIndex returns the navigable field index for the
// Description field.  When the task has a non-empty Recur the Recurrence row
// occupies index fieldRecur (9), pushing Description to index 10.  Without
// Recur, Description is at index 9.
func (m *Model) detailDescriptionFieldIndex() int {
	if m.currentTaskDetail != nil && m.currentTaskDetail.Recur != "" {
		return fieldRecur + 1 // 10
	}
	return fieldRecur // 9
}

// getDetailFieldCount returns the actual number of navigable fields for the current task
func (m *Model) getDetailFieldCount() int {
	if m.currentTaskDetail == nil {
		return 0
	}

	// Basic fields that are always present: ID, UUID, Status, Priority, Tags, Due, Start, Project, Entry, Description
	count := 10

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
