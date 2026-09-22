package ui

import (
	"fmt"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// pagingTestTasks builds enough tasks to make the table and ultra mode
// scrollable in a 120x24 window.
func pagingTestTasks(n int) []task.Task {
	tasks := make([]task.Task, 0, n)
	for i := 1; i <= n; i++ {
		tasks = append(tasks, task.Task{
			ID:          i,
			UUID:        fmt.Sprintf("uuid-%d", i),
			Description: "task body for paging tests",
			Status:      "pending",
		})
	}
	return tasks
}

func newPagingTestModel(t *testing.T) Model {
	t.Helper()
	fake := &fakeTaskwarrior{tasks: pagingTestTasks(80)}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return *mv.(*Model)
}

func TestHelpHalfPageKeys(t *testing.T) {
	m := newPagingTestModel(t)

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = *mv.(*Model)
	if !m.showHelp {
		t.Fatalf("H did not open the help screen")
	}
	if m.helpViewport.TotalLineCount() <= m.helpViewport.Height() {
		t.Fatalf("help content not scrollable: %d lines in %d rows",
			m.helpViewport.TotalLineCount(), m.helpViewport.Height())
	}

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	half := m.helpViewport.Height() / 2
	if m.helpViewport.YOffset() != half {
		t.Fatalf("ctrl+d moved help YOffset to %d, want %d", m.helpViewport.YOffset(), half)
	}
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.helpViewport.YOffset() != 0 {
		t.Fatalf("ctrl+u did not return help to the top: YOffset = %d", m.helpViewport.YOffset())
	}
}

func TestShellOutputHalfPageKeys(t *testing.T) {
	m := newPagingTestModel(t)

	content := ""
	for i := 0; i < 60; i++ {
		content += "shell output line\n"
	}
	m.shellOutputVisible = true
	m.shellOutputTitle = "task x"
	m.shellOutputViewport = viewport.New(viewport.WithWidth(100), viewport.WithHeight(18))
	m.shellOutputViewport.SetContent(content)

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	half := m.shellOutputViewport.Height() / 2
	if m.shellOutputViewport.YOffset() != half {
		t.Fatalf("ctrl+d moved shell output YOffset to %d, want %d", m.shellOutputViewport.YOffset(), half)
	}
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.shellOutputViewport.YOffset() != 0 {
		t.Fatalf("ctrl+u did not return shell output to the top: YOffset = %d", m.shellOutputViewport.YOffset())
	}
}

func TestUltraSpaceRefreshesNotPages(t *testing.T) {
	// space is the shared "refresh tasks" binding in table and ultra modes
	// (only the help, shell-output, and detail viewports page with space);
	// verify it does not move the ultra cursor.
	m := newPagingTestModel(t)

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not open ultra mode")
	}
	before := m.ultraCursor

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = *mv.(*Model)
	if m.ultraCursor != before {
		t.Fatalf("space moved the ultra cursor: %d -> %d", before, m.ultraCursor)
	}
}

func TestTableHalfPageKeys(t *testing.T) {
	m := newPagingTestModel(t)

	before := m.tbl.Cursor()
	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	after := m.tbl.Cursor()
	if after <= before {
		t.Fatalf("ctrl+d did not page the table down: cursor %d -> %d", before, after)
	}
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.tbl.Cursor() != before {
		t.Fatalf("ctrl+u did not page the table back: cursor %d, want %d", m.tbl.Cursor(), before)
	}
}

func TestUltraHalfPageKeys(t *testing.T) {
	m := newPagingTestModel(t)

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = *mv.(*Model)
	before := m.ultraCursor

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.ultraCursor <= before {
		t.Fatalf("ctrl+d did not half-page ultra mode: cursor %d -> %d", before, m.ultraCursor)
	}
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = *mv.(*Model)
	if m.ultraCursor != before {
		t.Fatalf("ctrl+u did not half-page ultra mode back: cursor %d, want %d", m.ultraCursor, before)
	}
}
