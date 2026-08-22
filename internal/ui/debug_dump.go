package ui

import (
	"fmt"
	"io"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal"
	"github.com/snonux/tasksamurai/internal/debug"
)

// dumpState handles debug.DumpStateMsg by writing a snapshot of the model's
// own fields to a file. Most of what makes hotkeys stop responding lives
// here rather than in a goroutine dump: Update() gates key handling behind
// a chain of mutually-exclusive mode flags (see handleEditingModes and the
// blink/help/ultra/detail checks at the top of Update), so a single flag
// left stuck true after an error path is enough to make every keypress a
// no-op. See docs/debugging.md for the full triage workflow.
func (m *Model) dumpState() (tea.Model, tea.Cmd) {
	f, path, err := debug.NewDumpFile("state")
	if err != nil {
		return m, m.showErrorTimed(fmt.Errorf("dumping UI state: %w", err))
	}
	defer f.Close()

	m.writeStateHeader(f)
	m.writeModeFlags(f)
	m.writeTaskSummary(f)
	m.writeWindowInfo(f)

	return m, m.showStatusTimed(fmt.Sprintf("UI state dumped to %s", path))
}

func (m *Model) writeStateHeader(w io.Writer) {
	fmt.Fprintf(w, "Task Samurai %s - UI State Dump\n", internal.Version)
	fmt.Fprintf(w, "Timestamp: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "NumGoroutine: %d\n\n", runtime.NumGoroutine())
}

// writeModeFlags lists every flag that gates key handling in Update(). If
// hotkeys stopped working, look here first: whichever of these is
// unexpectedly true is almost always why - it's either intercepting every
// keypress itself (an editing/search mode never cleared) or blocking the
// normal-mode dispatch entirely (blinkID, showTaskDetail, showHelp,
// showUltra checked ahead of everything else in Update).
func (m *Model) writeModeFlags(w io.Writer) {
	fmt.Fprintln(w, "-- Mode flags (checked in this order by Update) --")
	fmt.Fprintf(w, "blinkID=%d blinkOn=%v blinkCount=%d blinkEnabled=%v\n", m.blinkID, m.blinkOn, m.blinkCount, m.blinkEnabled)
	fmt.Fprintf(w, "shellOutputVisible=%v shellActive=%v shellCompletionLoad=%v\n", m.shellOutputVisible, m.shellActive, m.shellCompletionLoad)
	fmt.Fprintf(w, "showTaskDetail=%v detailSearching=%v detailDescEditing=%v\n", m.showTaskDetail, m.detailSearching, m.detailDescEditing)
	fmt.Fprintf(w, "showHelp=%v helpSearching=%v\n", m.showHelp, m.helpSearching)
	fmt.Fprintf(w, "showUltra=%v ultraSearching=%v ultraStartup=%v\n", m.showUltra, m.ultraSearching, m.ultraStartup)
	fmt.Fprintf(w, "annotating=%v descEditing=%v tagsEditing=%v dueEditing=%v recurEditing=%v projEditing=%v\n",
		m.annotating, m.descEditing, m.tagsEditing, m.dueEditing, m.recurEditing, m.projEditing)
	fmt.Fprintf(w, "prioritySelecting=%v filterEditing=%v addingTask=%v searching=%v\n",
		m.prioritySelecting, m.filterEditing, m.addingTask, m.searching)
	fmt.Fprintf(w, "cellExpanded=%v disco=%v compactView=%v\n", m.cellExpanded, m.disco, m.compactView)
	fmt.Fprintf(w, "autoRefresh=%v autoRefreshInterval=%s autoRefreshGen=%d\n\n", m.autoRefresh, m.autoRefreshInterval, m.autoRefreshGen)
}

func (m *Model) writeTaskSummary(w io.Writer) {
	fmt.Fprintln(w, "-- Task data --")
	fmt.Fprintf(w, "tasks=%d total=%d inProgress=%d due=%d undoStack=%d\n", len(m.tasks), m.total, m.inProgress, m.due, len(m.undoStack))
	fmt.Fprintf(w, "filters=%q\n", m.filters)
	fmt.Fprintf(w, "table cursor=%d columnCursor=%d rows=%d\n", m.tbl.Cursor(), m.tbl.ColumnCursor(), len(m.tbl.Rows()))
	fmt.Fprintf(w, "statusMsg=%q\n", m.statusMsg)
	fmt.Fprintf(w, "taskContext.Err()=%v\n\n", m.taskContext.Err())
}

func (m *Model) writeWindowInfo(w io.Writer) {
	fmt.Fprintln(w, "-- Window / config --")
	fmt.Fprintf(w, "windowHeight=%d\n", m.windowHeight)
	fmt.Fprintf(w, "browserCmd=%q youtubeBrowserCmd=%q agentFilterHotkey=%q\n", m.browserCmd, m.youtubeBrowserCmd, m.agentFilterHotkey)
}
