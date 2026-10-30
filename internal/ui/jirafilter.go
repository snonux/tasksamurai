package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/snonux/tasksamurai/internal/config"
)

type jiraFilterEntry struct {
	filter   config.Filter
	identity string
}

type jiraFilterMatch struct {
	entry jiraFilterEntry
	score int
}

type jiraFilterPicker struct {
	open       bool
	cursor     int
	entries    []jiraFilterEntry
	allEntries []jiraFilterEntry
	query      textinput.Model
}

func (m *Model) handleJiraFilterPicker() (tea.Model, tea.Cmd) {
	if m.jira.source == nil {
		return m, nil
	}
	entries := []jiraFilterEntry{{filter: config.Filter{Name: "none"}}}
	cursor := 0
	for _, filter := range m.jira.settings.Filters {
		entries = append(entries, jiraFilterEntry{filter: filter, identity: "local:" + filter.Name})
		if entries[len(entries)-1].identity == m.jira.filterID {
			cursor = len(entries) - 1
		}
	}
	query := textinput.New()
	query.Prompt = "Search: "
	_ = query.Focus()
	m.jira.picker = jiraFilterPicker{open: true, entries: entries, allEntries: entries, cursor: cursor, query: query}
	m.updateTableHeight()
	return m, nil
}

func (m *Model) handleJiraFilterPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	picker := &m.jira.picker
	switch msg.String() {
	case "esc":
		picker.open = false
		m.updateTableHeight()
	case "ctrl+n", "down":
		if picker.cursor < len(picker.entries)-1 {
			picker.cursor++
		}
	case "ctrl+p", "up":
		if picker.cursor > 0 {
			picker.cursor--
		}
	case "enter":
		if picker.cursor < 0 || picker.cursor >= len(picker.entries) {
			return m, nil
		}
		entry := picker.entries[picker.cursor]
		picker.open = false
		m.updateTableHeight()
		m.statusMsg = "jira filter: " + entry.filter.Name
		return m, m.setJiraFilter(entry.filter, entry.identity)
	default:
		previous := picker.query.Value()
		var cmd tea.Cmd
		picker.query, cmd = picker.query.Update(msg)
		if picker.query.Value() != previous {
			picker.refilter()
			m.updateTableHeight()
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) jiraFilterPickerView() string {
	picker := m.jira.picker
	width := m.ultraRenderWidth()
	lines := []string{ansi.Truncate("Jira filters · ↑/↓ Ctrl+n/p · Enter apply · Esc cancel", width, "…")}
	picker.query.SetWidth(max(1, width-8))
	lines = append(lines, ansi.Truncate(picker.query.View(), width, "…"))
	visible := 8
	if m.windowHeight > 0 {
		visible = max(1, min(visible, m.windowHeight/3))
	}
	start := max(0, picker.cursor-visible+1)
	end := min(len(picker.entries), start+visible)
	dim := lipgloss.NewStyle().Faint(true)
	for i := start; i < end; i++ {
		entry := picker.entries[i]
		prefix := "  "
		if i == picker.cursor {
			prefix = "> "
		}
		label := entry.filter.Name
		if entry.identity == "" {
			label = "(none) all issues"
		}
		label = ansi.Truncate(prefix+strings.Join(strings.Fields(label), " "), width, "…")
		if remaining := width - ansi.StringWidth(label) - 2; remaining > 0 && entry.filter.JQL != "" {
			hint := strings.Join(strings.Fields(entry.filter.JQL), " ")
			label += "  " + dim.Render(ansi.Truncate(hint, remaining, "…"))
		}
		lines = append(lines, label)
	}
	if len(picker.entries) > visible {
		lines = append(lines, ansi.Truncate(fmt.Sprintf("%d/%d", picker.cursor+1, len(picker.entries)), width, "…"))
	}
	return strings.Join(lines, "\n")
}

// refilter keeps the unrestricted entry first and ties in original source order.
func (p *jiraFilterPicker) refilter() {
	p.entries = nil
	p.cursor = 0
	query := p.query.Value()
	var matches []jiraFilterMatch
	for _, entry := range p.allEntries {
		if entry.identity == "" {
			p.entries = append(p.entries, entry)
			continue
		}
		if score, ok := fuzzyFilterScore(entry.filter.Name+" "+entry.filter.JQL, query); ok {
			matches = append(matches, jiraFilterMatch{entry, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score < matches[j].score })
	for _, match := range matches {
		p.entries = append(p.entries, match.entry)
	}
}

// fuzzyFilterScore ranks rune subsequences by start position and gaps (lower is
// better). Trying every matching start avoids penalizing a later contiguous hit.
func fuzzyFilterScore(text, query string) (int, bool) {
	haystack, needle := []rune(strings.ToLower(text)), []rune(strings.ToLower(query))
	if len(needle) == 0 {
		return 0, true
	}
	best := int(^uint(0) >> 1)
	for start, ch := range haystack {
		if ch != needle[0] {
			continue
		}
		next := 1
		end := start
		for i := start + 1; i < len(haystack) && next < len(needle); i++ {
			if haystack[i] == needle[next] {
				next++
				end = i
			}
		}
		if next == len(needle) {
			score := start + 2*(end-start+1-len(needle))
			best = min(best, score)
		}
	}
	return best, best != int(^uint(0)>>1)
}
