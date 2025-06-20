package task

import (
	"os"
	"os/exec"
	"testing"
)

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
