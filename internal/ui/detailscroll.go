package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// detailViewSize returns the viewport dimensions for the detail overlay.
// The height mirrors the help-viewport formula (padding + status/search
// areas); the width leaves two columns for the descStyle left padding used
// by wrapped description/annotation lines so viewport.View never clips
// wrapped text. Safe fallbacks cover tests and pre-resize windows.
func (m *Model) detailViewSize() (width, height int) {
	width = m.tbl.Width() - 2
	height = m.windowHeight - 6
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 20
	}
	return width, height
}

// initDetailViewport creates the detail viewport at the current size and
// fills it with the rendered detail screen. Called when the detail overlay
// opens; the viewport is reset to the zero value on close.
func (m *Model) initDetailViewport() {
	width, height := m.detailViewSize()
	m.detailViewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	m.detailViewport.SetContent(m.renderTaskDetail())
}

// syncDetailViewport keeps the viewport usable during rendering: it sizes an
// uninitialised viewport and refreshes the content (the detail screen is
// re-rendered from live model state, including selection/blink highlights
// and editing widgets). The scroll offset is preserved; the viewport clamps
// it when the content shrinks.
func (m *Model) syncDetailViewport() {
	width, height := m.detailViewSize()
	if m.detailViewport.Width() != width || m.detailViewport.Height() != height {
		m.detailViewport.SetWidth(width)
		m.detailViewport.SetHeight(height)
	}
	m.detailViewport.SetContent(m.renderTaskDetail())
}

// resetDetailViewport clears the detail viewport. Called from every path
// that closes the detail overlay so a stale viewport never leaks into the
// next open.
func (m *Model) resetDetailViewport() {
	m.detailViewport = viewport.Model{}
	m.detailFieldLines = nil
	m.detailFieldEndLines = nil
	m.detailSearchMatchLine = nil
	m.detailSearchMatchIdx = 0
	m.detailSearchFollow = false
	m.detailFieldFollow = false
	m.detailFollowPrevLine = 0
	m.detailFollowPrevEnd = 0
}

// detailFieldLine returns lines[field] or -1 when the offset is unavailable.
func detailFieldLine(lines []int, field int) int {
	if field < 0 || field >= len(lines) {
		return -1
	}
	return lines[field]
}

// scrollDetailToLine scrolls the detail viewport minimally so line is
// visible. No-op when the viewport is not sized yet.
func (m *Model) scrollDetailToLine(line int) {
	height := m.detailViewport.Height()
	if height == 0 {
		return
	}
	top := m.detailViewport.YOffset()
	bottom := top + height - 1
	switch {
	case line < top:
		m.detailViewport.SetYOffset(line)
	case line > bottom:
		m.detailViewport.SetYOffset(line - height + 1)
	}
}

// scrollDetailToField scrolls the highlighted detail field (or its whole
// section, for multi-line fields like Description/Annotations) into view.
func (m *Model) scrollDetailToField(field int) {
	if field < 0 || field >= len(m.detailFieldLines) {
		return
	}
	start := m.detailFieldLines[field]
	if start < 0 {
		return
	}
	end := start
	if field < len(m.detailFieldEndLines) && m.detailFieldEndLines[field] > end {
		end = m.detailFieldEndLines[field]
	}
	m.scrollDetailRangeIntoView(start, end)
}

// scrollDetailRangeIntoView scrolls minimally so the line range
// [start, end] is fully visible when it fits the viewport; sections taller
// than the viewport show their start.
func (m *Model) scrollDetailRangeIntoView(start, end int) {
	height := m.detailViewport.Height()
	if height == 0 {
		return
	}
	top := m.detailViewport.YOffset()
	bottom := top + height - 1
	if start >= top && end <= bottom {
		return // fully visible
	}
	if end-start+1 <= height {
		switch {
		case start < top:
			m.detailViewport.SetYOffset(start)
		case end > bottom:
			m.detailViewport.SetYOffset(end - height + 1)
		}
		return
	}
	if start < top || start > bottom {
		m.detailViewport.SetYOffset(start)
	}
}

// handleDetailScrollKey applies scroll keys to the detail viewport and
// reports whether the key was consumed. Field navigation (up/k/down/j,
// g/G home/end) is intentionally not handled here: those keys keep their
// field-highlight semantics (field-follow scrolling is layered on top).
func (m *Model) handleDetailScrollKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "pgup", "b":
		m.detailViewport.PageUp()
	case "pgdown", "space":
		m.detailViewport.PageDown()
	case "ctrl+u":
		m.detailViewport.HalfPageUp()
	case "ctrl+d":
		m.detailViewport.HalfPageDown()
	default:
		return false
	}
	return true
}

// detailStepSearchMatch moves the detail search match cursor by delta
// (wrapping) and scrolls the match into view.
func (m *Model) detailStepSearchMatch(delta int) {
	matches := m.detailSearchMatchLine
	if len(matches) == 0 {
		m.statusMsg = "No matches"
		return
	}
	// Clamp a stale index (e.g. after content edits removed matches).
	if m.detailSearchMatchIdx >= len(matches) {
		m.detailSearchMatchIdx = 0
	}
	idx := m.detailSearchMatchIdx + delta
	switch {
	case idx < 0:
		idx = len(matches) - 1
	case idx >= len(matches):
		idx = 0
	}
	m.detailSearchMatchIdx = idx
	m.scrollDetailToLine(matches[idx])
}

// detailScrollFooter renders the scroll-position indicator shown below the
// detail viewport when the content overflows the viewport height.
func (m *Model) detailScrollFooter() string {
	total := m.detailViewport.TotalLineCount()
	if total <= 0 || m.detailViewport.Height() == 0 {
		return ""
	}
	if total <= m.detailViewport.Height() {
		return ""
	}
	visibleTop := m.detailViewport.YOffset() + 1
	visibleBottom := min(m.detailViewport.YOffset()+m.detailViewport.Height(), total)
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	return st.Render(fmt.Sprintf("Lines %d-%d/%d - scroll: PgUp/PgDn, Ctrl+U/Ctrl+D, b/Space",
		visibleTop, visibleBottom, total))
}

// renderDetailScreen renders the scrollable detail overlay: the viewport
// with the task detail content plus a scroll-position footer and, while the
// user is searching, the search input below the viewport (so it stays
// visible even when the content is scrolled). View() calls this every
// frame, so the content is refreshed from live state here — render-time
// state sync, not user state (same pattern as the help screen).
func (m *Model) renderDetailScreen() string {
	m.syncDetailViewport()
	if m.detailFieldFollow {
		// The render just recomputed the line offsets for changed content;
		// bring the tracked field back into view with fresh geometry —
		// unless the reload that armed this follow left the field's line
		// range untouched (a no-op auto-refresh): re-scrolling would yank
		// a manually scrolled view back to the highlighted field.
		m.detailFieldFollow = false
		prevLine, prevEnd := m.detailFollowPrevLine, m.detailFollowPrevEnd
		if prevLine < 0 || prevEnd < 0 ||
			detailFieldLine(m.detailFieldLines, m.detailFollowField) != prevLine ||
			detailFieldLine(m.detailFieldEndLines, m.detailFollowField) != prevEnd {
			m.scrollDetailToField(m.detailFollowField)
		}
	}
	if m.detailSearchFollow {
		// The render just recomputed the match line offsets for the freshly
		// confirmed search; bring the first match into view.
		m.detailSearchFollow = false
		if len(m.detailSearchMatchLine) > 0 {
			m.detailSearchMatchIdx = 0
			m.scrollDetailToLine(m.detailSearchMatchLine[0])
		}
	}
	lines := []string{m.detailViewport.View()}
	if footer := m.detailScrollFooter(); footer != "" {
		lines = append(lines, footer)
	}
	if m.detailSearching {
		lines = append(lines, lipgloss.NewStyle().Padding(0, 2).
			Render("Search: "+m.detailSearchInput.View()))
	}
	if msg := m.statusMsg; msg != "" {
		// The table view shows statusMsg in its status bar; without this line
		// the detail overlay would swallow the only feedback for busy-commit
		// errors, detail search ("No matches"), invalid regexes, and similar.
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).
			Padding(0, 2).Render(msg))
	}
	return strings.Join(lines, "\n")
}
