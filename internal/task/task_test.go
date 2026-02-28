package task

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetDebugLog exercises the lifecycle of the debug logger: enable,
// disable, and re-enable. It also covers the negative case where the
// target directory does not exist.
func TestSetDebugLog(t *testing.T) {
	// Ensure the package-level state is clean before and after.
	t.Cleanup(func() { SetDebugLog("") }) //nolint:errcheck

	// Negative: directory does not exist — must return an error.
	if err := SetDebugLog("/nonexistent-dir-xyz/debug.log"); err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
	// After a failed open the package state must remain empty.
	if dbg.writer != nil || dbg.file != nil {
		t.Error("dbg fields must be nil after a failed SetDebugLog call")
	}

	// Positive: enable logging to a temp file.
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "debug.log")
	if err := SetDebugLog(logPath); err != nil {
		t.Fatalf("SetDebugLog failed: %v", err)
	}
	if dbg.writer == nil || dbg.file == nil {
		t.Fatal("dbg fields must be set after a successful SetDebugLog call")
	}

	// The run helper uses dbg.writer — verify that a write actually reaches
	// the log file. We fake a task invocation by writing directly.
	fmt.Fprintln(dbg.writer, "test-entry")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(content), "test-entry") {
		t.Errorf("expected log entry in file, got: %q", string(content))
	}

	// Re-enable with a different path — old file must be closed, new one opened.
	logPath2 := filepath.Join(tmp, "debug2.log")
	if err := SetDebugLog(logPath2); err != nil {
		t.Fatalf("second SetDebugLog failed: %v", err)
	}
	if dbg.file == nil {
		t.Error("dbg.file must be non-nil after re-enabling debug log")
	}

	// Disable — both fields must return to nil, no error expected.
	if err := SetDebugLog(""); err != nil {
		t.Fatalf("disable SetDebugLog failed: %v", err)
	}
	if dbg.writer != nil || dbg.file != nil {
		t.Error("dbg fields must be nil after disabling SetDebugLog")
	}
}

func TestAddAndExport(t *testing.T) {
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
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	if err := Add("hello world", []string{"tag", "anothertag", "tasksamuraitesting"}); err != nil {
		t.Fatalf("add task 1: %v", err)
	}
	if err := Add("hello universe", []string{"foo", "tasksamuraitesting"}); err != nil {
		t.Fatalf("add task 2: %v", err)
	}

	tasks, err := Export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	found := make(map[string]bool)
	for _, task := range tasks {
		hasTag := false
		for _, tag := range task.Tags {
			if tag == "tasksamuraitesting" {
				hasTag = true
				break
			}
		}
		if hasTag {
			found[task.Description] = true
		}
	}

	if len(found) != 2 {
		t.Fatalf("expected 2 tasks with tag, got %d", len(found))
	}
	if !found["hello world"] {
		t.Errorf("missing task 'hello world'")
	}
	if !found["hello universe"] {
		t.Errorf("missing task 'hello universe'")
	}
}

func TestModifyHelpers(t *testing.T) {
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
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	if err := Add("hello", nil); err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := SetPriority(1, "H"); err != nil {
		t.Fatalf("set priority: %v", err)
	}
	if err := AddTags(1, []string{"foo"}); err != nil {
		t.Fatalf("add tags: %v", err)
	}
	if err := SetDescription(1, "hello there"); err != nil {
		t.Fatalf("set description: %v", err)
	}
	if err := Annotate(1, "note"); err != nil {
		t.Fatalf("annotate: %v", err)
	}

	tasks, err := Export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	t1 := tasks[0]
	if t1.Priority != "H" {
		t.Errorf("priority not set: %v", t1.Priority)
	}
	found := false
	for _, tag := range t1.Tags {
		if tag == "foo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tag not added")
	}
	if t1.Description != "hello there" {
		t.Errorf("description not set: %v", t1.Description)
	}
	annFound := false
	for _, a := range t1.Annotations {
		if a.Description == "note" {
			annFound = true
			break
		}
	}
	if !annFound {
		t.Errorf("annotation not added")
	}
}
