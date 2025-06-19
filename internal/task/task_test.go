package task

import (
        "os"
        "os/exec"
        "testing"
)

func requireTask(t *testing.T) {
        t.Helper()
        if _, err := exec.LookPath("task"); err != nil {
                t.Skip("task command not available")
        }
}

func setupTaskEnv(t *testing.T) {
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
}

func TestAddAndExport(t *testing.T) {
        requireTask(t)
        setupTaskEnv(t)

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

func TestHelperFunctions(t *testing.T) {
        requireTask(t)
        setupTaskEnv(t)

	if err := Add("sample", []string{"one"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	tasks, err := Export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	id := tasks[0].ID

	// Description
	if err := SetDescription(id, "changed"); err != nil {
		t.Fatalf("set description: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Description != "changed" {
		t.Errorf("description not updated: %v", tasks[0].Description)
	}

	// Priority
	if err := SetPriority(id, "H"); err != nil {
		t.Fatalf("set priority: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Priority != "H" {
		t.Errorf("priority not updated: %v", tasks[0].Priority)
	}

	if err := SetPriority(id, ""); err != nil {
		t.Fatalf("clear priority: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Priority != "" {
		t.Errorf("priority not cleared: %v", tasks[0].Priority)
	}

	// Tags
	if err := ChangeTags(id, []string{"two"}, []string{"one"}); err != nil {
		t.Fatalf("change tags: %v", err)
	}
	tasks, _ = Export()
	foundTwo, foundOne := false, false
	for _, tag := range tasks[0].Tags {
		if tag == "two" {
			foundTwo = true
		}
		if tag == "one" {
			foundOne = true
		}
	}
	if !foundTwo || foundOne {
		t.Errorf("tags not modified correctly: %v", tasks[0].Tags)
	}

	// Due date (required for recurrence)
	if err := SetDue(id, "2030-01-02"); err != nil {
		t.Fatalf("set due: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Due == "" {
		t.Errorf("due not set")
	}

	// Recurrence
	if err := SetRecurrence(id, "daily"); err != nil {
		t.Fatalf("set recur: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Recur != "daily" {
		t.Errorf("recur not set: %v", tasks[0].Recur)
	}

	// Annotation
	if err := Annotate(id, "note"); err != nil {
		t.Fatalf("annotate: %v", err)
	}
	tasks, _ = Export()
	if len(tasks[0].Annotations) == 0 || tasks[0].Annotations[0].Description != "note" {
		t.Errorf("annotation missing: %#v", tasks[0].Annotations)
	}

	// Active state
	if err := SetActive(id, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Start == "" {
		t.Errorf("expected start timestamp")
	}
	if err := SetActive(id, false); err != nil {
		t.Fatalf("stop: %v", err)
	}
	tasks, _ = Export()
	if tasks[0].Start != "" {
		t.Errorf("expected start cleared")
	}

	// Edit command (ensure it runs)
	if err := os.Setenv("EDITOR", "true"); err != nil {
		t.Fatal(err)
	}
	if err := Edit(id); err != nil {
		t.Fatalf("edit: %v", err)
	}
	os.Unsetenv("EDITOR")
}
