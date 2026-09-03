package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snonux/tasksamurai/internal/debug"
	"github.com/snonux/tasksamurai/internal/task"
)

// TestDumpStateWritesModeFlagsAndTaskSummary exercises the debug.DumpStateMsg
// path exactly as a SIGUSR1 handler would trigger it: through Update(), not
// by calling dumpState's helpers directly. This is the important part to
// cover, since the whole point of routing the dump through a tea.Msg is that
// it runs on the same goroutine as every other Update call.
func TestDumpStateWritesModeFlagsAndTaskSummary(t *testing.T) {
	tmp := t.TempDir()
	debug.SetDebugDir(tmp)
	t.Cleanup(func() { debug.SetDebugDir("") })

	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "fake-1", Description: "existing", Status: "pending"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.tagsEditing = true // simulate a stuck editing-mode flag

	mp := &m
	mv, cmd := mp.Update(debug.DumpStateMsg{})
	mp = mv.(*Model)
	if cmd == nil {
		t.Fatal("dumpState returned a nil cmd; expected a status-clear tick")
	}
	if !strings.Contains(mp.statusMsg, "UI state dumped to") {
		t.Fatalf("statusMsg = %q, want mention of the dump path", mp.statusMsg)
	}

	matches, err := filepath.Glob(filepath.Join(tmp, "tasksamurai-state-*.txt"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("state dump files = %d, want 1", len(matches))
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	content := string(data)

	for _, want := range []string{"tagsEditing=true", "tasks=1", "UI State Dump", "taskFlight=idle", "taskOpGen="} {
		if !strings.Contains(content, want) {
			t.Errorf("dump missing %q; got:\n%s", want, content)
		}
	}
}
