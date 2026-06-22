package task

import (
	"context"
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

func TestRunLineSplitsCapturesAndStripsTaskPrefix(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	argsFile := filepath.Join(tmp, "args.txt")

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"echo stdout-value\n" +
		"echo stderr-value >&2\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	result, err := RunLine(context.Background(), `task add "hello world" project:home`)
	if err != nil {
		t.Fatalf("RunLine: %v", err)
	}
	if result.Stdout != "stdout-value\n" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.Stderr != "stderr-value\n" {
		t.Fatalf("stderr = %q", result.Stderr)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"add", "hello world", "project:home"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestRunShellLineDisablesRecurrencePrompt(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	argsFile := filepath.Join(tmp, "args.txt")

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	if _, err := RunShellLine(context.Background(), `task 260 modify project:foo`); err != nil {
		t.Fatalf("RunShellLine: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"rc.recurrence.confirmation=no", "260", "modify", "project:foo"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestRunLineReturnsCapturedErrorOutput(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"echo bad-output >&2\n" +
		"exit 2\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	result, err := RunLine(context.Background(), "bad command")
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stderr != "bad-output\n" {
		t.Fatalf("stderr = %q", result.Stderr)
	}
	if !strings.Contains(err.Error(), "bad-output") {
		t.Fatalf("error did not include stderr: %v", err)
	}
}

func TestLoadCompletionSources(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  _commands) printf 'add\\nmodify\\n' ;;\n" +
		"  _columns) printf 'project\\ndue\\n' ;;\n" +
		"  _projects) printf 'home\\nwork\\n' ;;\n" +
		"  _tags) printf 'urgent\\n' ;;\n" +
		"  _ids) printf '1\\n2\\n' ;;\n" +
		"  _uuids) printf 'uuid-1\\n' ;;\n" +
		"  _udas) printf 'custom\\n' ;;\n" +
		"esac\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	sources := LoadCompletionSources(context.Background())
	if strings.Join(sources.Commands, ",") != "add,modify" {
		t.Fatalf("commands = %#v", sources.Commands)
	}
	if strings.Join(sources.Columns, ",") != "project,due" {
		t.Fatalf("columns = %#v", sources.Columns)
	}
	if strings.Join(sources.Projects, ",") != "home,work" {
		t.Fatalf("projects = %#v", sources.Projects)
	}
	if strings.Join(sources.Tags, ",") != "urgent" {
		t.Fatalf("tags = %#v", sources.Tags)
	}
	if strings.Join(sources.IDs, ",") != "1,2" {
		t.Fatalf("ids = %#v", sources.IDs)
	}
	if strings.Join(sources.UUIDs, ",") != "uuid-1" {
		t.Fatalf("uuids = %#v", sources.UUIDs)
	}
	if strings.Join(sources.UDAs, ",") != "custom" {
		t.Fatalf("udas = %#v", sources.UDAs)
	}
}

func TestRecurringSeries(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	argsFile := filepath.Join(tmp, "args.txt")

	script := "#!/bin/sh\n" +
		"echo \"$@\" > " + argsFile + "\n" +
		"if [ \"$1\" = \"(parent-uuid or parent:parent-uuid)\" ] && [ \"$2\" = \"status.any:\" ] && [ \"$3\" = \"export\" ]; then\n" +
		"  echo '{\"id\":0,\"uuid\":\"parent-uuid\",\"description\":\"template\",\"status\":\"recurring\",\"recur\":\"daily\"}'\n" +
		"  echo '{\"id\":1,\"uuid\":\"child-uuid\",\"parent\":\"parent-uuid\",\"description\":\"child\",\"status\":\"pending\",\"recur\":\"daily\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	tasks, err := RecurringSeries("parent-uuid")
	if err != nil {
		t.Fatalf("RecurringSeries: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].UUID != "parent-uuid" || tasks[0].Status != "recurring" {
		t.Fatalf("unexpected template task: %#v", tasks[0])
	}
	if tasks[1].UUID != "child-uuid" || tasks[1].Parent != "parent-uuid" {
		t.Fatalf("unexpected child task: %#v", tasks[1])
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "(parent-uuid or parent:parent-uuid) status.any: export rc.json.array=off" {
		t.Fatalf("unexpected args: %q", got)
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
