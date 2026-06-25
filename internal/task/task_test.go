package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestSetDebugLog exercises the lifecycle of the debug logger: enable,
// disable, and re-enable. It also covers the negative case where the
// target directory does not exist.
func TestSetDebugLog(t *testing.T) {
	// Ensure the package-level state is clean before and after.
	t.Cleanup(func() { _ = SetDebugLog("") })

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
	if _, err := fmt.Fprintln(dbg.writer, "test-entry"); err != nil {
		t.Fatalf("write debug log: %v", err)
	}
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
		_ = os.Unsetenv("TASKDATA")
		_ = os.Unsetenv("TASKRC")
	})

	if err := Add("hello world", []string{"tag", "anothertag", "tasksamuraitesting"}); err != nil {
		t.Fatalf("add task 1: %v", err)
	}
	if err := Add("hello universe", []string{"foo", "tasksamuraitesting"}); err != nil {
		t.Fatalf("add task 2: %v", err)
	}

	tasks, err := Export(context.Background())
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
	_ = os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

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
	_ = os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

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
	_ = os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	result, err := RunLine(context.Background(), "bad command")
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stderr != "bad-output\n" {
		t.Fatalf("stderr = %q", result.Stderr)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want wrapped *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), "bad-output") {
		t.Fatalf("error did not include stderr: %v", err)
	}
}

func TestExportHonorsContextCancellation(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\nsleep 5\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Export(ctx)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Export error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Export took %s, expected prompt context cancellation", elapsed)
	}
}

func TestExportReturnsCapturedErrorOutput(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"echo export-failed >&2\n" +
		"exit 2\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	_, err := Export(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want wrapped *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), "export-failed") {
		t.Fatalf("error did not include stderr: %v", err)
	}
}

func TestMutationHelpersHonorContextCancellation(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\nsleep 5\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	tests := []struct {
		name string
		run  func(context.Context) error
	}{
		{"AddContext", func(ctx context.Context) error { return AddContext(ctx, "new task", []string{"tag"}) }},
		{"AddArgsContext", func(ctx context.Context) error { return AddArgsContext(ctx, []string{"new task", "+tag"}) }},
		{"AddLineContext", func(ctx context.Context) error { return AddLineContext(ctx, `"new task" +tag`) }},
		{"SetStatusContext", func(ctx context.Context) error { return SetStatusContext(ctx, 1, "pending") }},
		{"SetStatusUUIDContext", func(ctx context.Context) error { return SetStatusUUIDContext(ctx, "task-uuid", "pending") }},
		{"StartContext", func(ctx context.Context) error { return StartContext(ctx, 1) }},
		{"StopContext", func(ctx context.Context) error { return StopContext(ctx, 1) }},
		{"DoneContext", func(ctx context.Context) error { return DoneContext(ctx, 1) }},
		{"DeleteContext", func(ctx context.Context) error { return DeleteContext(ctx, 1) }},
		{"SetPriorityContext", func(ctx context.Context) error { return SetPriorityContext(ctx, 1, "H") }},
		{"SetRecurrenceContext", func(ctx context.Context) error { return SetRecurrenceContext(ctx, 1, "daily") }},
		{"SetDueDateContext", func(ctx context.Context) error { return SetDueDateContext(ctx, 1, "tomorrow") }},
		{"SetDescriptionContext", func(ctx context.Context) error { return SetDescriptionContext(ctx, 1, "new description") }},
		{"SetProjectContext", func(ctx context.Context) error { return SetProjectContext(ctx, 1, "home") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := tt.run(ctx)
			elapsed := time.Since(start)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s error = %v, want context deadline exceeded", tt.name, err)
			}
			if elapsed > time.Second {
				t.Fatalf("%s took %s, expected prompt context cancellation", tt.name, elapsed)
			}
		})
	}
}

func TestSetTagsHonorsContextDuringMutations(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = modify ]; then\n" +
		"    sleep 5\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"printf '%s\\n' '{\"id\":1,\"tags\":[\"old\"]}'\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := SetTags(ctx, 1, []string{"new"})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SetTags error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("SetTags took %s, expected prompt context cancellation", elapsed)
	}
}

func TestSetTagsHonorsContextDuringRemovals(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = modify ]; then\n" +
		"    sleep 5\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"printf '%s\\n' '{\"id\":1,\"tags\":[\"old\"]}'\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := SetTags(ctx, 1, nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SetTags error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("SetTags took %s, expected prompt context cancellation", elapsed)
	}
}

func TestSetTagsFailureDoesNotPartiallyApply(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "tags.txt")
	logPath := filepath.Join(tmp, "commands.log")
	if err := os.WriteFile(statePath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fmt.Sprintf(`#!/bin/sh
state=%q
log=%q
printf '%%s\n' "$*" >> "$log"
if [ "$2" = export ]; then
  tags=$(cat "$state")
  printf '{"id":1,"tags":['
  sep=
  for tag in $tags; do
    printf '%%s"%%s"' "$sep" "$tag"
    sep=,
  done
  printf ']}\n'
  exit 0
fi
if [ "$2" = modify ]; then
  for arg in "$@"; do
    if [ "$arg" = "-old" ]; then
      echo remove failed >&2
      exit 2
    fi
  done
  tags=$(cat "$state")
  shift 2
  for arg in "$@"; do
    case "$arg" in
      +*)
        tag=${arg#+}
        case " $tags " in
          *" $tag "*) ;;
          *) tags="$tags $tag" ;;
        esac
        ;;
      -*)
        tag=${arg#-}
        next=
        for current in $tags; do
          if [ "$current" != "$tag" ]; then
            next="$next $current"
          fi
        done
        tags=$next
        ;;
    esac
  done
  set -- $tags
  printf '%%s' "$*" > "$state"
  exit 0
fi
exit 1
`, statePath, logPath)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	err := SetTags(context.Background(), 1, []string{"new"})
	if err == nil {
		t.Fatal("expected SetTags error")
	}
	if !strings.Contains(err.Error(), "remove failed") {
		t.Fatalf("SetTags error = %v, want remove failure", err)
	}
	if got := readSpaceSeparatedFile(t, statePath); strings.Join(got, ",") != "old" {
		t.Fatalf("tags after failed SetTags = %#v, want original old tag only", got)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	if got := strings.Count(string(logData), "modify"); got != 1 {
		t.Fatalf("modify command count = %d, want 1; log:\n%s", got, logData)
	}
}

func TestReplaceAnnotationsHonorsContextDuringMutations(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	failPath := filepath.Join(tmp, "deadline-once")
	script := "#!/bin/sh\n" +
		"fail=" + shellQuote(failPath) + "\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = denotate ] || [ \"$arg\" = annotate ]; then\n" +
		"    if [ ! -e \"$fail\" ]; then\n" +
		"      touch \"$fail\"\n" +
		"      sleep 5\n" +
		"    fi\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"printf '%s\\n' '{\"id\":1,\"annotations\":[{\"entry\":\"20260622T000000Z\",\"description\":\"old note\"}]}'\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := ReplaceAnnotations(ctx, 1, "new note")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReplaceAnnotations error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("ReplaceAnnotations took %s, expected prompt context cancellation", elapsed)
	}
}

func TestReplaceAnnotationsHonorsContextDuringAnnotate(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = annotate ]; then\n" +
		"    sleep 5\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"printf '%s\\n' '{\"id\":1,\"annotations\":[]}'\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := ReplaceAnnotations(ctx, 1, "new note")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReplaceAnnotations error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("ReplaceAnnotations took %s, expected prompt context cancellation", elapsed)
	}
}

func TestReplaceAnnotationsRestoresSnapshotAfterDenotateDeadline(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "annotations.txt")
	failPath := filepath.Join(tmp, "deadline-once")
	if err := os.WriteFile(statePath, []byte("first note\nsecond note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fakeAnnotationTaskScript(statePath, `
if [ "$2" = denotate ] && [ "$3" = "first note" ] && [ ! -e `+shellQuote(failPath)+` ]; then
  touch `+shellQuote(failPath)+`
  sleep 5
  exit 0
fi
`)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := ReplaceAnnotations(ctx, 1, "replacement note")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReplaceAnnotations error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("ReplaceAnnotations took %s, expected prompt context cancellation plus rollback", elapsed)
	}
	if got := readLinesFile(t, statePath); strings.Join(got, "|") != "first note|second note" {
		t.Fatalf("annotations after rollback = %#v, want original annotations", got)
	}
}

func TestReplaceAnnotationsRestoresSnapshotAfterAnnotateDeadline(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "annotations.txt")
	failPath := filepath.Join(tmp, "deadline-once")
	if err := os.WriteFile(statePath, []byte("first note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fakeAnnotationTaskScript(statePath, `
if [ "$2" = annotate ] && [ "$3" = "replacement note" ] && [ ! -e `+shellQuote(failPath)+` ]; then
  touch `+shellQuote(failPath)+`
  sleep 5
  exit 0
fi
`)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := ReplaceAnnotations(ctx, 1, "replacement note")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReplaceAnnotations error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("ReplaceAnnotations took %s, expected prompt context cancellation plus rollback", elapsed)
	}
	if got := readLinesFile(t, statePath); strings.Join(got, "|") != "first note" {
		t.Fatalf("annotations after rollback = %#v, want original annotations", got)
	}
}

func TestReplaceAnnotationsRestoresSnapshotAfterDenotateFailure(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "annotations.txt")
	failPath := filepath.Join(tmp, "failed-once")
	if err := os.WriteFile(statePath, []byte("first note\nsecond note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fakeAnnotationTaskScript(statePath, `
if [ "$2" = denotate ] && [ "$3" = "first note" ] && [ ! -e `+shellQuote(failPath)+` ]; then
  touch `+shellQuote(failPath)+`
  echo denotate first failed >&2
  exit 2
fi
`)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	err := ReplaceAnnotations(context.Background(), 1, "replacement note")
	if err == nil {
		t.Fatal("expected ReplaceAnnotations error")
	}
	if !strings.Contains(err.Error(), "denotate first failed") {
		t.Fatalf("ReplaceAnnotations error = %v, want denotate failure", err)
	}
	if got := readLinesFile(t, statePath); strings.Join(got, "|") != "first note|second note" {
		t.Fatalf("annotations after rollback = %#v, want original annotations", got)
	}
}

func TestReplaceAnnotationsRestoresSnapshotAfterAnnotateFailure(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "annotations.txt")
	if err := os.WriteFile(statePath, []byte("first note\nsecond note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fakeAnnotationTaskScript(statePath, `
if [ "$2" = annotate ] && [ "$3" = "replacement note" ]; then
  echo annotate replacement failed >&2
  exit 2
fi
`)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	err := ReplaceAnnotations(context.Background(), 1, "replacement note")
	if err == nil {
		t.Fatal("expected ReplaceAnnotations error")
	}
	if !strings.Contains(err.Error(), "annotate replacement failed") {
		t.Fatalf("ReplaceAnnotations error = %v, want annotate failure", err)
	}
	if got := readLinesFile(t, statePath); strings.Join(got, "|") != "first note|second note" {
		t.Fatalf("annotations after rollback = %#v, want original annotations", got)
	}
}

func TestReplaceAnnotationsReportsRollbackFailure(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	statePath := filepath.Join(tmp, "annotations.txt")
	if err := os.WriteFile(statePath, []byte("first note\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fakeAnnotationTaskScript(statePath, `
if [ "$2" = annotate ] && [ "$3" = "replacement note" ]; then
  echo annotate replacement failed >&2
  exit 2
fi
if [ "$2" = annotate ] && [ "$3" = "first note" ]; then
  echo rollback annotate failed >&2
  exit 3
fi
`)
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+":"+origPath)

	err := ReplaceAnnotations(context.Background(), 1, "replacement note")
	if err == nil {
		t.Fatal("expected ReplaceAnnotations error")
	}
	if !strings.Contains(err.Error(), "annotate replacement failed") {
		t.Fatalf("ReplaceAnnotations error = %v, want original failure", err)
	}
	if !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("ReplaceAnnotations error = %v, want rollback failure", err)
	}
	if !strings.Contains(err.Error(), "rollback annotate failed") {
		t.Fatalf("ReplaceAnnotations error = %v, want rollback detail", err)
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
	_ = os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

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
	_ = os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	tasks, err := RecurringSeries(context.Background(), "parent-uuid")
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

func TestSetRecurringSeriesRecurrenceContext(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "commands.txt")

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "(root or parent:root)" ] && [ "$2" = "status.any:" ] && [ "$3" = "export" ]; then
  echo '{"id":0,"uuid":"root","description":"template","status":"recurring","recur":"daily"}'
  echo '{"id":1,"uuid":"child-1","parent":"root","description":"child 1","status":"pending","recur":"daily"}'
  echo '{"id":2,"uuid":"child-2","parent":"root","description":"child 2","status":"pending","recur":"daily"}'
  exit 0
fi
echo "$@" >> %s
`, shellQuote(logFile))
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmp+":"+origPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Setenv("PATH", origPath); err != nil {
			t.Errorf("restore PATH: %v", err)
		}
	})

	if err := SetRecurringSeriesRecurrenceContext(context.Background(), "root", "weekly"); err != nil {
		t.Fatalf("SetRecurringSeriesRecurrenceContext: %v", err)
	}

	got := readLinesFile(t, logFile)
	want := []string{
		"rc.recurrence.confirmation=no child-1 modify recur:weekly",
		"rc.recurrence.confirmation=no child-2 modify recur:weekly",
		"rc.recurrence.confirmation=no root modify recur:weekly",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestSetRecurringSeriesRecurrenceContextRollsBackCompletedUpdates(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "commands.txt")

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "(root or parent:root)" ] && [ "$2" = "status.any:" ] && [ "$3" = "export" ]; then
  echo '{"id":0,"uuid":"root","description":"template","status":"recurring","recur":"daily"}'
  echo '{"id":1,"uuid":"child-1","parent":"root","description":"child 1","status":"pending","recur":"daily"}'
  echo '{"id":2,"uuid":"child-2","parent":"root","description":"child 2","status":"pending","recur":"monthly"}'
  exit 0
fi
echo "$@" >> %s
if [ "$2" = "child-2" ] && [ "$4" = "recur:weekly" ]; then
  echo "child-2 failed" >&2
  exit 1
fi
`, shellQuote(logFile))
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmp+":"+origPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Setenv("PATH", origPath); err != nil {
			t.Errorf("restore PATH: %v", err)
		}
	})

	err := SetRecurringSeriesRecurrenceContext(context.Background(), "root", "weekly")
	if err == nil {
		t.Fatal("expected SetRecurringSeriesRecurrenceContext error")
	}
	if !strings.Contains(err.Error(), "set recurrence for child-2") {
		t.Fatalf("error = %v, want child-2 context", err)
	}

	got := readLinesFile(t, logFile)
	want := []string{
		"rc.recurrence.confirmation=no child-1 modify recur:weekly",
		"rc.recurrence.confirmation=no child-2 modify recur:weekly",
		"rc.recurrence.confirmation=no child-1 modify recur:daily",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands:\ngot  %#v\nwant %#v", got, want)
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
		_ = os.Unsetenv("TASKDATA")
		_ = os.Unsetenv("TASKRC")
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

	tasks, err := Export(context.Background())
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

func readSpaceSeparatedFile(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Fields(string(data))
}

func readLinesFile(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func fakeAnnotationTaskScript(statePath, failureBlock string) string {
	return `#!/bin/sh
state=` + shellQuote(statePath) + `
` + failureBlock + `
if [ "$2" = export ]; then
  printf '{"id":1,"annotations":['
  sep=
  while IFS= read -r ann; do
    if [ -n "$ann" ]; then
      printf '%s{"entry":"20260622T000000Z","description":"%s"}' "$sep" "$ann"
      sep=,
    fi
  done < "$state"
  printf ']}\n'
  exit 0
fi
if [ "$2" = denotate ]; then
  tmp="$state.tmp"
  : > "$tmp"
  removed=0
  while IFS= read -r ann; do
    if [ "$removed" = 0 ] && [ "$ann" = "$3" ]; then
      removed=1
    else
      printf '%s\n' "$ann" >> "$tmp"
    fi
  done < "$state"
  mv "$tmp" "$state"
  exit 0
fi
if [ "$2" = annotate ]; then
  printf '%s\n' "$3" >> "$state"
  exit 0
fi
exit 1
`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
