package task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestExportCompletedWindow verifies the export filter used by the
// completed-task display window (V key in the UI): completed tasks within
// the window are exported alongside the pending ones, older completed tasks
// are excluded, and pending tasks always stay included.
func TestExportCompletedWindow(t *testing.T) {
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("task command not available")
	}
	tmp := t.TempDir()
	if err := os.Setenv("TASKDATA", tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("TASKRC", "/dev/null"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("TASKDATA")
		_ = os.Unsetenv("TASKRC")
	})

	ctx := context.Background()
	add := func(desc string) {
		t.Helper()
		if _, err := RunLine(ctx, "add "+desc); err != nil {
			t.Fatalf("add %q: %v", desc, err)
		}
	}
	add("tsu-window pending")
	add("tsu-window recent done")
	add("tsu-window old done")

	// Resolve addresses by description via export: the working-set renumbering
	// after each mutation makes numeric IDs unreliable across commands.
	uuidFor := func(needle string) string {
		t.Helper()
		tasks, err := Export(ctx)
		if err != nil {
			t.Fatalf("export for uuid lookup: %v", err)
		}
		for _, tsk := range tasks {
			if strings.Contains(tsk.Description, needle) {
				return tsk.UUID
			}
		}
		t.Fatalf("task %q not found", needle)
		return ""
	}

	if _, err := RunLine(ctx, uuidFor("recent")+" done"); err != nil {
		t.Fatalf("done recent: %v", err)
	}
	if _, err := RunLine(ctx, uuidFor("old")+" done"); err != nil {
		t.Fatalf("done old: %v", err)
	}
	// The "old done" task was completed now but its end date is moved three
	// days back, placing it outside a 4-hour window.
	old := time.Now().AddDate(0, 0, -3).Format("2006-01-02T15:04:05")
	if _, err := RunLine(ctx, uuidFor("old")+" modify end:"+old); err != nil {
		t.Fatalf("modify end: %v", err)
	}

	window := 4 * time.Hour
	cutoff := time.Now().Add(-window)
	filter := fmt.Sprintf("(status:pending or (status:completed and end.after:%s))",
		cutoff.Format("2006-01-02T15:04:05"))
	tasks, err := Export(ctx, filter)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	var gotPending, gotRecent, gotOld bool
	for _, tsk := range tasks {
		if !strings.Contains(tsk.Description, "tsu-window") {
			continue
		}
		switch {
		case strings.Contains(tsk.Description, "pending") && tsk.Status == "pending":
			gotPending = true
		case strings.Contains(tsk.Description, "recent") && tsk.Status == "completed":
			gotRecent = true
		case strings.Contains(tsk.Description, "old") && tsk.Status == "completed":
			gotOld = true
		}
	}
	if !gotPending {
		t.Error("pending task missing from windowed export")
	}
	if !gotRecent {
		t.Error("recently completed task missing from windowed export")
	}
	if gotOld {
		t.Error("old completed task leaked into windowed export")
	}
}
