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
	lines := []string{m.detailViewport.View()}
	if footer := m.detailScrollFooter(); footer != "" {
		lines = append(lines, footer)
	}
	if m.detailSearching {
		lines = append(lines, lipgloss.NewStyle().Padding(0, 2).
			Render("Search: "+m.detailSearchInput.View()))
	}
	return strings.Join(lines, "\n")
}
