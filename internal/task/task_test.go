package task

import (
	"os"
	"testing"
)

func TestAddAndExport(t *testing.T) {
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
