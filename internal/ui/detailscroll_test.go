package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

func newDetailScrollTestModel(t *testing.T, tasks []task.Task) Model {
	t.Helper()
	fake := &fakeTaskwarrior{tasks: tasks}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return *mv.(*Model)
}

func openDetail(t *testing.T, m *Model) {
	t.Helper()
	mv, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	*m = *mv.(*Model)
	if !m.showTaskDetail {
		t.Fatalf("enter did not open the detail view")
	}
}

func detailContent(lines int) []task.Task {
	desc := ""
	for i := 0; i < lines; i++ {
		desc += "description line for scrolling content\n"
	}
	return []task.Task{{ID: 1, UUID: "one", Description: desc, Status: "pending"}}
}

func TestDetailViewportInitializedOnOpen(t *testing.T) {
	m := newDetailScrollTestModel(t, detailContent(60))
	openDetail(t, &m)

	if m.detailViewport.Width() == 0 || m.detailViewport.Height() == 0 {
		t.Fatalf("detail viewport not sized on open: %dx%d", m.detailViewport.Width(), m.detailViewport.Height())
	}
	if m.detailViewport.TotalLineCount() <= m.detailViewport.Height() {
		t.Fatalf("detail viewport content = %d lines, want more than height %d (scrollable)",
			m.detailViewport.TotalLineCount(), m.detailViewport.Height())
	}
	if m.detailViewport.YOffset() != 0 {
		t.Fatalf("detail viewport YOffset = %d on open, want 0", m.detailViewport.YOffset())
	}
}

func TestDetailViewportScrollKeysMoveOffset(t *testing.T) {
	m := newDetailScrollTestModel(t, detailContent(80))
	openDetail(t, &m)

	// pgdown pages forward
	mv, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("pgdown did not move YOffset from 0")
	}

	// pgup returns to the top
	mv, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() != 0 {
		t.Fatalf("pgup did not return to top: YOffset = %d", m.detailViewport.YOffset())
	}

	// ctrl+d half page, ctrl+u back
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	half := m.detailViewport.YOffset()
	if half == 0 {
		t.Fatalf("ctrl+d did not scroll half a page")
	}
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() != 0 {
		t.Fatalf("ctrl+u did not scroll back: YOffset = %d", m.detailViewport.YOffset())
	}
}

func TestDetailViewportResetsOnClose(t *testing.T) {
	m := newDetailScrollTestModel(t, detailContent(80))
	openDetail(t, &m)

	mv, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = *mv.(*Model)
	mv, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = *mv.(*Model)

	if m.showTaskDetail {
		t.Fatalf("q did not close the detail view")
	}
	if m.detailViewport.Width() != 0 {
		t.Fatalf("detail viewport not reset on q: width = %d", m.detailViewport.Width())
	}

	// Reopening starts at the top with fresh content.
	openDetail(t, &m)
	if m.detailViewport.YOffset() != 0 {
		t.Fatalf("reopened detail viewport YOffset = %d, want 0", m.detailViewport.YOffset())
	}
	if m.detailViewport.TotalLineCount() <= m.detailViewport.Height() {
		t.Fatalf("reopened detail viewport content not scrollable")
	}
}

func TestDetailViewportResizedOnWindowSizeMsg(t *testing.T) {
	m := newDetailScrollTestModel(t, detailContent(80))
	openDetail(t, &m)

	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = *mv.(*Model)

	if m.detailViewport.Width() != 98 || m.detailViewport.Height() != 34 {
		t.Fatalf("detail viewport after resize = %dx%d, want 98x34", m.detailViewport.Width(), m.detailViewport.Height())
	}
}

func TestDetailScreenRendersViewportWithFooter(t *testing.T) {
	m := newDetailScrollTestModel(t, detailContent(120))
	openDetail(t, &m)

	got := m.renderDetailScreen()
	if !strings.Contains(got, "Task 1 Details") {
		t.Fatalf("detail screen missing content: %q", got[:min(200, len(got))])
	}
	if !strings.Contains(got, "Lines 1-") {
		t.Fatalf("detail screen missing scroll footer: %q", got)
	}

	// Short content in a tall window renders no scroll footer.
	short := newDetailScrollTestModel(t, []task.Task{{ID: 2, UUID: "two", Description: "tiny", Status: "pending"}})
	mv, _ := (&short).Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	short = *mv.(*Model)
	openDetail(t, &short)
	if strings.Contains(short.renderDetailScreen(), "Lines 1-") {
		t.Fatalf("scroll footer rendered for short detail content")
	}
}

func TestDetailViewportUpdateHandlesMouseWheelMsgs(t *testing.T) {
	// Mouse mode is not enabled app-wide, so real terminals do not deliver
	// wheel events yet; this only verifies the Update routing that help and
	// shell-output viewports share.
	m := newDetailScrollTestModel(t, detailContent(80))
	openDetail(t, &m)

	mv, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("mouse wheel did not scroll the detail viewport")
	}
}

func detailNeedleContent() []task.Task {
	desc := ""
	for i := 0; i < 40; i++ {
		desc += "filler text to push content down the screen "
		if i >= 20 && i%5 == 0 {
			desc += "needle "
		}
		desc += "and some more prose words to wrap across lines. "
	}
	return []task.Task{{ID: 1, UUID: "one", Description: desc, Status: "pending"}}
}

// detailScrollContent builds a task whose detail content overflows the
// viewport: a long description followed by many annotations, so the
// Annotations field (and the tail of the description) sit below the fold.
func detailScrollContent() []task.Task {
	tsk := detailContent(60)[0]
	for i := 0; i < 30; i++ {
		tsk.Annotations = append(tsk.Annotations, task.Annotation{
			Description: fmt.Sprintf("annotation note %d with some extra length", i+1),
		})
	}
	return []task.Task{tsk}
}

func TestDetailFieldNavigationScrollsFieldIntoView(t *testing.T) {
	m := newDetailScrollTestModel(t, detailScrollContent())
	openDetail(t, &m)

	// G selects the last field (Annotations), which sits below the fold, so
	// the viewport must scroll down to its section end.
	mv, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("G did not scroll the last detail field into view")
	}

	// g returns to the first field, which must be visible again.
	mv, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = *mv.(*Model)
	if sel := m.detailFieldLines[m.detailFieldIndex]; sel < m.detailViewport.YOffset() ||
		sel > m.detailViewport.YOffset()+m.detailViewport.Height()-1 {
		t.Fatalf("g did not bring the first field (line %d) into view: YOffset = %d",
			sel, m.detailViewport.YOffset())
	}

	// j moves through fields and keeps the selected one visible.
	for i := 0; i < 20; i++ {
		mv, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = *mv.(*Model)
	}
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("j navigation did not scroll with the field highlight")
	}
}

func TestDetailBlinkScrollsFieldIntoView(t *testing.T) {
	m := newDetailScrollTestModel(t, detailScrollContent())
	openDetail(t, &m)

	// Blink the last detail field (the Annotations section, deep in the
	// content) while the viewport is at the top. The scroll-into-view is
	// deferred to the next render, which recomputes line offsets.
	lastField := m.getDetailFieldCount() - 1
	m.startDetailBlink(lastField)
	m.renderDetailScreen()
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("startDetailBlink did not scroll the blinking field into view")
	}
}

func TestDetailSearchMatchNavigation(t *testing.T) {
	m := newDetailScrollTestModel(t, detailNeedleContent())
	openDetail(t, &m)
	m.detailSearching = true
	m.detailSearchInput.SetValue("needle")
	m.detailSearchInput.Focus()

	// While typing, the search input renders below the viewport.
	if got := m.renderDetailScreen(); !strings.Contains(got, "Search:") {
		t.Fatalf("search input not rendered below the viewport while searching")
	}

	// Confirm the search; the next render computes match offsets and
	// scrolls the first match into view.
	mv, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	m.renderDetailScreen()
	if len(m.detailSearchMatchLine) == 0 {
		t.Fatalf("no search match lines recorded from render")
	}
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("search confirm did not scroll the first match into view")
	}

	// n moves to the next match.
	first := m.detailSearchMatchIdx
	mv, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = *mv.(*Model)
	if m.detailSearchMatchIdx != first+1 {
		t.Fatalf("detailSearchMatchIdx = %d after n, want %d", m.detailSearchMatchIdx, first+1)
	}

	// N steps backwards and wraps at the start.
	mv, _ = m.Update(tea.KeyPressMsg{Code: 'N', Mod: 0, Text: "N"})
	m = *mv.(*Model)
	if m.detailSearchMatchIdx != first {
		t.Fatalf("detailSearchMatchIdx = %d after N, want %d", m.detailSearchMatchIdx, first)
	}
	mv, _ = m.Update(tea.KeyPressMsg{Code: 'N', Mod: 0, Text: "N"})
	m = *mv.(*Model)
	if m.detailSearchMatchIdx != len(m.detailSearchMatchLine)-1 {
		t.Fatalf("detailSearchMatchIdx = %d after N wrap, want last match", m.detailSearchMatchIdx)
	}
}

func TestDetailKeepsSelectedFieldVisibleAfterReload(t *testing.T) {
	m := newDetailScrollTestModel(t, detailScrollContent())
	openDetail(t, &m)

	// Scroll to the bottom field (Annotations) via G, then re-render with a
	// description that grew, which shifts the Annotations section down.
	mv, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = *mv.(*Model)
	if m.detailViewport.YOffset() == 0 {
		t.Fatalf("G did not scroll to the last field")
	}

	// Double the description so every line below it moves.
	shifted := detailScrollContent()
	shifted[0].Description += shifted[0].Description
	data := reloadData{tasks: shifted}
	m.processTasks(&data)
	m.renderDetailScreen()

	// The selected field's fresh position must be visible.
	lastField := m.getDetailFieldCount() - 1
	top := m.detailViewport.YOffset()
	bottom := top + m.detailViewport.Height() - 1
	sel := m.detailFieldLines[lastField]
	if sel < top || sel > bottom {
		t.Fatalf("selected field line %d outside visible range %d-%d after reload", sel, top, bottom)
	}
}

func TestHelpSectionsIncludeDetailViewScrolling(t *testing.T) {
	m := newDetailScrollTestModel(t, []task.Task{{ID: 1, UUID: "one", Description: "d", Status: "pending"}})
	content := m.buildHelpContent()
	for _, want := range []string{"Task Detail View", "half page up/down", "next/previous match"} {
		if !strings.Contains(content, want) {
			t.Fatalf("help content missing %q", want)
		}
	}
}
