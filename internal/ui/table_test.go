package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestTagHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "log.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0,\"tags\":[\"old\"],\"annotations\":[]}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" >> " + logFile + "\n"

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

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = mv.(Model)
	for _, r := range "+foo -old" {
		mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mv.(Model)
	}
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "+foo") || !strings.Contains(out, "-old") {
		t.Fatalf("tag commands not recorded: %s", out)
	}
}

func TestReplaceTagsHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "log.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0,\"tags\":[\"old\"],\"annotations\":[]}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" >> " + logFile + "\n"

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

	mv, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = mv.(Model)
	for _, r := range "foo bar" {
		mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mv.(Model)
	}
	mv, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mv.(Model)

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+foo") {
		t.Fatalf("replace tag commands not recorded: %s", out)
	}
}
