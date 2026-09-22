package ui

import (
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
