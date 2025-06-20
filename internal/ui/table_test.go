package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAnnotateHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	annoFile := filepath.Join(tmp, "anno.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0,\"annotations\":[]}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"1\" ] && [ \"$2\" = \"annotate\" ]; then\n" +
		"  echo \"$3\" > " + annoFile + "\n" +
		"  exit 0\n" +
		"fi\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = mv.(Model)
	for _, r := range "note" {
		mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mv.(Model)
	}
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(annoFile)
	if err != nil {
		t.Fatalf("read ann: %v", err)
	}

	if strings.TrimSpace(string(data)) != "note" {
		t.Fatalf("annotation not recorded: %q", data)
	}
}

func TestReplaceAnnotationHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	annoFile := filepath.Join(tmp, "anno.txt")
	logFile := filepath.Join(tmp, "log.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0,\"annotations\":[{\"entry\":\"\",\"description\":\"old\"}]}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" >> " + logFile + "\n" +
		"if [ \"$1\" = \"1\" ] && [ \"$2\" = \"annotate\" ]; then\n" +
		"  echo \"$3\" > " + annoFile + "\n" +
		"  exit 0\n" +
		"fi\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = mv.(Model)
	for _, r := range "new" {
		mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mv.(Model)
	}
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(annoFile)
	if err != nil {
		t.Fatalf("read ann: %v", err)
	}

	if strings.TrimSpace(string(data)) != "new" {
		t.Fatalf("annotation not recorded: %q", data)
	}

	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	if !strings.Contains(string(logData), "denotate") {
		t.Fatalf("denotate not called: %s", logData)
	}
}

func TestDoneHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	doneFile := filepath.Join(tmp, "done.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + doneFile + "\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = mv.(Model)

	data, err := os.ReadFile(doneFile)
	if err != nil {
		t.Fatalf("read done: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 done" {
		t.Fatalf("done not called: %q", data)
	}
}

func TestDueDateHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	dueFile := filepath.Join(tmp, "due.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + dueFile + "\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = mv.(Model)
	for _, r := range "2024-12-31" {
		mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mv.(Model)
	}
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(dueFile)
	if err != nil {
		t.Fatalf("read due: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 modify due:2024-12-31" {
		t.Fatalf("due not set: %q", data)
	}
}

func TestRandomDueDateHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	dueFile := filepath.Join(tmp, "due.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + dueFile + "\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = mv.(Model)

	data, err := os.ReadFile(dueFile)
	if err != nil {
		t.Fatalf("read due: %v", err)
	}

	parts := strings.Split(strings.TrimSpace(string(data)), " ")
	if len(parts) != 3 {
		t.Fatalf("unexpected command: %q", data)
	}
	dueStr := strings.TrimPrefix(parts[2], "due:")
	dueTime, err := time.Parse("2006-01-02", dueStr)
	if err != nil {
		t.Fatalf("parse due: %v", err)
	}
	days := int(time.Until(dueTime).Hours() / 24)
	if days < 7 || days > 37 {
		t.Fatalf("due date out of range: %d", days)
	}
}

func TestSetPriorityHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	priFile := filepath.Join(tmp, "pri.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + priFile + "\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = mv.(Model)
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	m = mv.(Model)
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(priFile)
	if err != nil {
		t.Fatalf("read pri: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 modify priority:H" {
		t.Fatalf("priority not set: %q", data)
	}
}

func TestDeletePriorityHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	priFile := filepath.Join(tmp, "pri.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + priFile + "\n"

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})

	m, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = mv.(Model)

	data, err := os.ReadFile(priFile)
	if err != nil {
		t.Fatalf("read pri: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 modify priority:" {
		t.Fatalf("priority not cleared: %q", data)
	}
}
