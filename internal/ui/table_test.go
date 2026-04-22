package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"codeberg.org/snonux/tasksamurai/internal/task"
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mp := &m // Get pointer to model
	mv, _ := mp.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	mp = mv.(*Model)
	for _, r := range "note" {
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		mp = mv.(*Model)
	}
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mp = mv.(*Model)

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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = *mv.(*Model)
	for _, r := range "new" {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

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

func TestHandleDescEditDoneUpdatesDescriptionAndRemovesTempFile(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "log.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"old description\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' \"$@\" >> " + logFile + "\n"

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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.currentTaskDetail = &task.Task{ID: 1, Description: "old description"}
	m.showTaskDetail = true
	m.detailDescEditing = true

	tempFile := filepath.Join(tmp, "desc.txt")
	if err := os.WriteFile(tempFile, []byte("updated description\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mv, cmd := (&m).handleDescEditDone(descEditDoneMsg{tempFile: tempFile})
	m = *mv.(*Model)

	if m.detailDescEditing {
		t.Fatalf("description editor state was not cleared")
	}
	if cmd == nil {
		t.Fatalf("handleDescEditDone did not return a reload command")
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists after handler: %v", err)
	}

	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logData), "1\nmodify\ndescription:updated description") {
		t.Fatalf("description update did not reach task command: %s", logData)
	}
}

func TestPrepareDescriptionTempFileRemovesTempFileOnWriteError(t *testing.T) {
	tmp := t.TempDir()
	tempFile := filepath.Join(tmp, "desc.txt")
	if err := os.WriteFile(tempFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := prepareDescriptionTempFile("updated description", func() (descriptionTempFile, error) {
		return &failingDescriptionTempFile{path: tempFile}, nil
	})
	if err == nil {
		t.Fatalf("prepareDescriptionTempFile succeeded unexpectedly: %q", path)
	}
	if path != "" {
		t.Fatalf("expected empty path on error, got %q", path)
	}
	if _, statErr := os.Stat(tempFile); !os.IsNotExist(statErr) {
		t.Fatalf("temp file still exists after write error: %v", statErr)
	}
}

func TestHandleDescEditDoneRemovesTempFileOnEditorError(t *testing.T) {
	tmp := t.TempDir()
	tempFile := filepath.Join(tmp, "desc.txt")
	if err := os.WriteFile(tempFile, []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Model{detailEditState: detailEditState{detailDescEditing: true}}
	mv, cmd := (&m).handleDescEditDone(descEditDoneMsg{
		err:      fmt.Errorf("editor failed"),
		tempFile: tempFile,
	})
	m = *mv.(*Model)

	if m.detailDescEditing {
		t.Fatalf("description editor state was not cleared after error")
	}
	if cmd == nil {
		t.Fatalf("error path did not return a status-clear command")
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists after error path: %v", err)
	}
	if !strings.Contains(m.statusMsg, "Edit error: editor failed") {
		t.Fatalf("unexpected status message: %q", m.statusMsg)
	}
}

type failingDescriptionTempFile struct {
	path string
}

func (f *failingDescriptionTempFile) Name() string {
	return f.path
}

func (f *failingDescriptionTempFile) WriteString(string) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func (f *failingDescriptionTempFile) Close() error {
	return nil
}

func TestHandleFilterModeReportsReloadError(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	failFlag := filepath.Join(tmp, "fail-reload")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  if [ -f " + failFlag + " ]; then\n" +
		"    echo 'reload failed' >&2\n" +
		"    exit 42\n" +
		"  fi\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0,\"annotations\":[]}'\n" +
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := os.WriteFile(failFlag, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.filterEditing = true
	m.filterInput.SetValue("project:alpha")

	mv, cmd := (&m).handleFilterMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	if !m.filterEditing {
		t.Fatalf("filter editing was cleared after reload failure")
	}
	if cmd == nil {
		t.Fatalf("reload failure did not return a clear-status command")
	}
	if got := len(m.filters); got != 0 {
		t.Fatalf("filters were not rolled back after reload failure: %#v", m.filters)
	}
	if got := m.statusMsg; got != "Error: filter error: exit status 42: reload failed" {
		t.Fatalf("unexpected status message: %q", got)
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = *mv.(*Model)
	for i := 0; i < blinkCycles; i++ {
		mp := &m
		mv, _ = mp.Update(blinkMsg{})
		m = *mv.(*Model)
	}

	data, err := os.ReadFile(doneFile)
	if err != nil {
		t.Fatalf("read done: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 done" {
		t.Fatalf("done not called: %q", data)
	}
}

func TestUndoHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "log.txt")

	script := fmt.Sprintf("#!/bin/sh\n"+
		"if echo \"$@\" | grep -q export; then\n"+
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n"+
		"  exit 0\n"+
		"fi\n"+
		"echo \"$@\" >> %s\n", logFile)

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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = *mv.(*Model)
	for i := 0; i < blinkCycles; i++ {
		mp := &m
		mv, _ = mp.Update(blinkMsg{})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = *mv.(*Model)

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two commands, got %d", len(lines))
	}
	if lines[0] != "1 done" {
		t.Fatalf("done not called: %q", lines[0])
	}
	if lines[1] != "x modify status:pending" {
		t.Fatalf("undo not called: %q", lines[1])
	}
}

func TestOpenURLHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	openFile := filepath.Join(tmp, "open.txt")
	browserPath := filepath.Join(tmp, "browser")

	taskScript := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"see https://example.com\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(taskScript), 0o755); err != nil {
		t.Fatal(err)
	}

	browserScript := "#!/bin/sh\n" +
		"echo $1 > " + openFile + "\n"
	if err := os.WriteFile(browserPath, []byte(browserScript), 0o755); err != nil {
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

	m, err := New(nil, browserPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = *mv.(*Model)

	data, err := os.ReadFile(openFile)
	if err != nil {
		t.Fatalf("read open: %v", err)
	}
	if strings.TrimSpace(string(data)) != "https://example.com" {
		t.Fatalf("browser not called with url: %q", data)
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = *mv.(*Model)
	for i := 0; i < 3; i++ {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	data, err := os.ReadFile(dueFile)
	if err != nil {
		t.Fatalf("read due: %v", err)
	}

	want := "1 modify due:" + time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	if strings.TrimSpace(string(data)) != want {
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = *mv.(*Model)

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

func TestRecurrenceHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	recFile := filepath.Join(tmp, "recur.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + recFile + "\n"

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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = *mv.(*Model)
	for _, r := range "daily" {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	data, err := os.ReadFile(recFile)
	if err != nil {
		t.Fatalf("read recur: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 modify recur:daily" {
		t.Fatalf("recur not set: %q", data)
	}
}

func TestPriorityHotkey(t *testing.T) {
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = *mv.(*Model)
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	data, err := os.ReadFile(priFile)
	if err != nil {
		t.Fatalf("read pri: %v", err)
	}

	if strings.TrimSpace(string(data)) != "1 modify priority:H" {
		t.Fatalf("priority not set: %q", data)
	}
}

func TestAddHotkey(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	addFile := filepath.Join(tmp, "add.txt")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"$@\" > " + addFile + "\n"

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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	m = *mv.(*Model)
	for _, r := range "foo due:today" {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)

	data, err := os.ReadFile(addFile)
	if err != nil {
		t.Fatalf("read add: %v", err)
	}

	if strings.TrimSpace(string(data)) != "add foo due:today" {
		t.Fatalf("add not called: %q", data)
	}
}

func TestNavigationHotkeys(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d1\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"y\",\"description\":\"d2\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 1 {
		t.Fatalf("down: got cursor %d", m.tbl.Cursor())
	}

	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 0 {
		t.Fatalf("0 hotkey: expected 0 got %d", m.tbl.Cursor())
	}

	mp = &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 1 {
		t.Fatalf("G hotkey: expected 1 got %d", m.tbl.Cursor())
	}
}

func TestToggleAgentFilter(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect []string
	}{
		{
			name:   "nil filter starts with agent",
			input:  nil,
			expect: []string{"+agent"},
		},
		{
			name:   "preserves unrelated filters",
			input:  []string{"project:home"},
			expect: []string{"project:home", "+agent"},
		},
		{
			name:   "switches +agent to -agent",
			input:  []string{"project:home", "+agent"},
			expect: []string{"project:home", "-agent"},
		},
		{
			name:   "switches -agent to +agent",
			input:  []string{"project:home", "-agent"},
			expect: []string{"project:home", "+agent"},
		},
		{
			name:   "normalizes contradictory agent filters",
			input:  []string{"+agent", "-agent", "status:pending"},
			expect: []string{"status:pending", "+agent"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toggleAgentFilter(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Fatalf("toggleAgentFilter(%v) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestAgentFilterHotkeyDefaultsToThree(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")

	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = *mv.(*Model)
	if !reflect.DeepEqual(m.filters, []string{"+agent"}) {
		t.Fatalf("3 did not toggle agent filter: %#v", m.filters)
	}
}

func TestAgentFilterHotkeyCanBeRebound(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	logFile := filepath.Join(tmp, "log.txt")

	script := "#!/bin/sh\n" +
		"printf '%s ' \"$@\" >> " + logFile + "\n" +
		"printf '\\n' >> " + logFile + "\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"d\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
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

	m, err := New([]string{"project:home"}, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.SetAgentFilterHotkey("7")

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = *mv.(*Model)
	if !reflect.DeepEqual(m.filters, []string{"project:home"}) {
		t.Fatalf("3 unexpectedly changed filters: %#v", m.filters)
	}

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: '7', Text: "7"})
	m = *mv.(*Model)
	if !reflect.DeepEqual(m.filters, []string{"project:home", "+agent"}) {
		t.Fatalf("7 did not toggle agent filter: %#v", m.filters)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "project:home +agent status:pending export") {
		t.Fatalf("toggle did not reload with +agent filter: %s", data)
	}
}

func setupUltraTaskSet(t *testing.T, tmp string) string {
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"alpha\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"beta bravo\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":3,\"uuid\":\"3\",\"description\":\"charlie delta\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

func setupUltraSearchTaskSet(t *testing.T, tmp string) string {
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"alpha\",\"project\":\"home\",\"tags\":[\"blue\"],\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":0,\"annotations\":[{\"entry\":\"\",\"description\":\"project note\"}]}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"beta bravo\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":3,\"uuid\":\"3\",\"description\":\"charlie delta\",\"tags\":[\"home\"],\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

func setupUltraReloadTaskSet(t *testing.T, tmp string) (string, string) {
	taskPath := filepath.Join(tmp, "task")
	phaseFile := filepath.Join(tmp, "phase")
	if err := os.WriteFile(phaseFile, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fmt.Sprintf("#!/bin/sh\n"+
		"phase=$(cat %q)\n"+
		"if echo \"$@\" | grep -q export; then\n"+
		"  if [ \"$phase\" = \"1\" ]; then\n"+
		"    echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"one\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"L\",\"urgency\":0}'\n"+
		"    echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"two\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"M\",\"urgency\":0}'\n"+
		"    echo '{\"id\":3,\"uuid\":\"3\",\"description\":\"three\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":0}'\n"+
		"  else\n"+
		"    echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"one\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":0}'\n"+
		"    echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"two\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"L\",\"urgency\":0}'\n"+
		"    echo '{\"id\":3,\"uuid\":\"3\",\"description\":\"three\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"M\",\"urgency\":0}'\n"+
		"  fi\n"+
		"  exit 0\n"+
		"fi\n", phaseFile)

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return taskPath, phaseFile
}

func setupUltraSearchReloadTaskSet(t *testing.T, tmp string) (string, string) {
	taskPath := filepath.Join(tmp, "task")
	phaseFile := filepath.Join(tmp, "phase")
	if err := os.WriteFile(phaseFile, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := fmt.Sprintf("#!/bin/sh\n"+
		"phase=$(cat %q)\n"+
		"if echo \"$@\" | grep -q export; then\n"+
		"  if [ \"$phase\" = \"1\" ]; then\n"+
		"    echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"alpha current\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":2}'\n"+
		"    echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"beta current\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"L\",\"urgency\":1}'\n"+
		"  else\n"+
		"    echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"omega current\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"H\",\"urgency\":2}'\n"+
		"    echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"alpha moved\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"L\",\"urgency\":1}'\n"+
		"  fi\n"+
		"  exit 0\n"+
		"fi\n", phaseFile)

	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return taskPath, phaseFile
}

func setupBasicTask(t *testing.T, tmp string) string {
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"alpha\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"y\",\"description\":\"beta\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

func setupSharedSearchTaskSet(t *testing.T, tmp string) string {
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"x\",\"description\":\"shared alpha\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"y\",\"description\":\"shared beta\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

func setupEnv(t *testing.T, taskPath string) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", filepath.Dir(taskPath)+":"+origPath)
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	tmp := filepath.Dir(taskPath)
	os.Setenv("TASKDATA", tmp)
	os.Setenv("TASKRC", "/dev/null")
	t.Cleanup(func() {
		os.Unsetenv("TASKDATA")
		os.Unsetenv("TASKRC")
	})
}

func TestEscClosesHelp(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	m = *mv.(*Model)
	if !m.showHelp {
		t.Fatalf("help not shown")
	}

	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = *mv.(*Model)
	if m.showHelp {
		t.Fatalf("esc did not close help")
	}
}

func TestEscDoesNotQuitFromTable(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc in table mode unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showHelp || m.showTaskDetail || m.showUltra {
		t.Fatalf("esc changed mode unexpectedly: help=%v detail=%v ultra=%v", m.showHelp, m.showTaskDetail, m.showUltra)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in table mode did not return a quit command")
	}
	m = *mv.(*Model)
	if m.showHelp || m.showTaskDetail || m.showUltra {
		t.Fatalf("q changed mode unexpectedly: help=%v detail=%v ultra=%v", m.showHelp, m.showTaskDetail, m.showUltra)
	}
}

func TestEscDoesNotQuitUltraStartup(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.SetUltra(true)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc in ultra startup unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("esc in ultra startup exited ultra mode")
	}

	m.ultraSearchRegex = regexp.MustCompile("alpha")
	m.ultraFiltered = []int{0}
	m.ultraSearchInput.SetValue("alpha")

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc in ultra startup with search unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("esc in ultra startup with search exited ultra mode")
	}
	if m.ultraSearchRegex != nil {
		t.Fatalf("esc in ultra startup with search did not clear ultraSearchRegex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("esc in ultra startup with search did not clear ultraFiltered")
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in ultra startup did not return a quit command")
	}
}

func TestUltraHelpUsesUltraBindingsAndClosesBeforeLeavingUltra(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = *mv.(*Model)

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	if cmd != nil {
		t.Fatalf("H unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showHelp {
		t.Fatalf("H did not open help in ultra mode")
	}
	if !m.showUltra {
		t.Fatalf("opening help unexpectedly exited ultra mode")
	}

	view := ansi.Strip(m.activeHelpContent())
	if !strings.Contains(view, "search ultra cards") {
		t.Fatalf("ultra help content missing ultra search binding: %q", view)
	}
	if !strings.Contains(view, "exit ultra mode") {
		t.Fatalf("ultra help content missing ultra exit binding: %q", view)
	}
	if !strings.Contains(view, "open URL from description") {
		t.Fatalf("ultra help content missing open-url binding: %q", view)
	}
	if strings.Contains(view, "edit current field") {
		t.Fatalf("ultra help rendered normal-only inline edit binding: %q", view)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc while ultra help is open unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showHelp {
		t.Fatalf("esc did not close ultra help")
	}
	if !m.showUltra {
		t.Fatalf("esc while ultra help is open unexpectedly exited ultra mode")
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("second esc in ultra mode unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("second esc did not exit ultra mode")
	}
}

func TestUltraHelpSearchUsesUltraHelpLines(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step := func(msg tea.Msg) {
		t.Helper()
		mv, _ := (&m).Update(msg)
		m = *mv.(*Model)
	}

	step(tea.WindowSizeMsg{Width: 120, Height: 24})
	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	step(tea.KeyPressMsg{Code: 'H', Text: "H"})
	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "URL" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := len(m.helpSearchMatches); got == 0 {
		t.Fatalf("ultra help search for 'URL' should match, got 0 matches")
	}

	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "ultra" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := len(m.helpSearchMatches); got == 0 {
		t.Fatalf("ultra help search did not match ultra-specific help content")
	}
}

func TestUltraExitHotkeysClearUltraState(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	m.ultraFocusedID = 42
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("u did not clear ultraFocusedID, got %d", m.ultraFocusedID)
	}

	m.ultraSearchRegex = regexp.MustCompile("alpha")
	m.ultraFiltered = []int{0, 1}
	m.ultraSearchInput.SetValue("ultra needle")
	m.ultraFocusedID = 17

	// First q clears the active search but stays in ultra mode.
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatalf("q with active search unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("q with active search should stay in ultra mode")
	}
	if m.ultraSearchRegex != nil {
		t.Fatalf("first q did not clear ultraSearchRegex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("first q did not clear ultraFiltered")
	}

	// Second q (no active search) exits ultra mode.
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatalf("q in ultra mode (no search) unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("second q did not exit ultra mode")
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("q did not clear ultraFocusedID, got %d", m.ultraFocusedID)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command on re-entry")
	}
	m = *mv.(*Model)
	m.ultraFocusedID = 23
	m.ultraSearchRegex = regexp.MustCompile("beta")
	m.ultraFiltered = []int{2}
	m.ultraSearchInput.SetValue("second needle")

	// First esc clears the active search, stays in ultra.
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc with active search unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("esc with active search should stay in ultra mode")
	}
	if m.ultraSearchRegex != nil {
		t.Fatalf("first esc did not clear ultraSearchRegex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("first esc did not clear ultraFiltered")
	}

	// Second esc (no active search) exits ultra mode.
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc in ultra mode (no search) unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("second esc did not exit ultra mode")
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("esc did not clear ultraFocusedID, got %d", m.ultraFocusedID)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not re-enter ultra mode")
	}
	m.ultraCursor = 2
	m.ultraFocusedID = 31

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u in ultra mode unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("u did not exit ultra mode")
	}
	if got := m.tbl.Cursor(); got != 2 {
		t.Fatalf("u did not preserve table cursor: got %d want 2", got)
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("u did not clear ultraFocusedID, got %d", m.ultraFocusedID)
	}
}

func TestUltraSearchFiltersNavigatesAndHighlights(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraSearchTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var expected []int
	for i, tsk := range m.tasks {
		if tsk.Project == "home" || strings.Contains(strings.Join(tsk.Tags, " "), "home") {
			expected = append(expected, i)
		}
	}
	if len(expected) != 2 {
		t.Fatalf("test setup failed: expected 2 matching tasks, got %d", len(expected))
	}

	step := func(msg tea.KeyPressMsg) {
		t.Helper()
		mv, _ := (&m).Update(msg)
		m = *mv.(*Model)
	}

	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}

	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.ultraSearching {
		t.Fatalf("/ did not start ultra search")
	}
	for _, r := range "home" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.ultraSearching {
		t.Fatalf("enter did not close ultra search")
	}
	if m.ultraSearchRegex == nil {
		t.Fatalf("enter did not compile ultra search regex")
	}
	if !reflect.DeepEqual(m.ultraFiltered, expected) {
		t.Fatalf("unexpected ultraFiltered: got %#v want %#v", m.ultraFiltered, expected)
	}
	if m.ultraCursor != 0 || m.ultraOffset != 0 {
		t.Fatalf("search did not reset cursor/offset, got cursor=%d offset=%d", m.ultraCursor, m.ultraOffset)
	}
	if structured := m.ultraSearchText(m.tasks[0]); !regexp.MustCompile(`proj:`).MatchString(structured) {
		t.Fatalf("ultra search text lost card line structure: %q", structured)
	}

	filtered := m.ultraTaskList()
	if len(filtered) != 2 {
		t.Fatalf("unexpected filtered task count: %d", len(filtered))
	}
	plain := m.renderUltraCard(filtered[0], 80, true, nil)
	highlighted := m.renderUltraCard(filtered[0], 80, true, m.ultraSearchRegex)
	if plain == highlighted {
		t.Fatalf("search highlighting did not change rendered card")
	}
	if ansi.Strip(plain) != ansi.Strip(highlighted) {
		t.Fatalf("search highlighting changed visible text")
	}
	labelHighlighted := m.renderUltraCard(filtered[0], 80, true, regexp.MustCompile(`proj:`))
	if labelHighlighted == plain {
		t.Fatalf("label search did not change rendered card")
	}
	if ansi.Strip(labelHighlighted) != ansi.Strip(plain) {
		t.Fatalf("label search highlighting changed visible text")
	}
	combinedHighlighted := m.renderUltraCard(filtered[0], 80, true, regexp.MustCompile(`proj: home`))
	if combinedHighlighted == plain {
		t.Fatalf("combined meta search did not change rendered card")
	}
	if ansi.Strip(combinedHighlighted) != ansi.Strip(plain) {
		t.Fatalf("combined meta search highlighting changed visible text")
	}
	priorityHighlighted := m.renderUltraCard(filtered[0], 80, true, regexp.MustCompile(`H`))
	if priorityHighlighted == plain {
		t.Fatalf("priority search did not change rendered card")
	}
	if ansi.Strip(priorityHighlighted) != ansi.Strip(plain) {
		t.Fatalf("priority search highlighting changed visible text")
	}

	step(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if got := m.ultraTaskList()[m.ultraCursor].ID; got != filtered[1].ID {
		t.Fatalf("n did not move to next filtered task: got %d want %d", got, filtered[1].ID)
	}
	step(tea.KeyPressMsg{Code: 'N', Text: "N"})
	if got := m.ultraTaskList()[m.ultraCursor].ID; got != filtered[0].ID {
		t.Fatalf("N did not move to previous filtered task: got %d want %d", got, filtered[0].ID)
	}

	prevFiltered := append([]int(nil), m.ultraFiltered...)
	prevRegex := m.ultraSearchRegex
	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	step(tea.KeyPressMsg{Code: '[', Text: "["})
	if !m.ultraSearching {
		t.Fatalf("invalid regex should keep ultra search open")
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !reflect.DeepEqual(m.ultraFiltered, prevFiltered) {
		t.Fatalf("invalid regex changed filtered tasks: got %#v want %#v", m.ultraFiltered, prevFiltered)
	}
	if m.ultraSearchRegex != prevRegex {
		t.Fatalf("invalid regex changed active regex: got %#v want %#v", m.ultraSearchRegex, prevRegex)
	}
	step(tea.KeyPressMsg{Code: tea.KeyEsc})

	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "annotation" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := len(m.ultraTaskList()); got != 0 {
		t.Fatalf("search matched invisible annotation label, got %d tasks", got)
	}

	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "beta bravo" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := len(m.ultraTaskList()); got != 1 {
		t.Fatalf("multi-word ultra search match count = %d, want 1", got)
	}
	if got := m.ultraTaskList()[0].ID; got != 2 {
		t.Fatalf("multi-word ultra search matched task %d, want 2", got)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 24, Height: 24})
	m = *mv.(*Model)
	if got := len(m.ultraTaskList()); got != 1 {
		t.Fatalf("resize changed multi-word ultra search match count = %d, want 1", got)
	}
	if got := m.ultraTaskList()[0].ID; got != 2 {
		t.Fatalf("resize changed multi-word ultra search match to task %d, want 2", got)
	}

	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.ultraSearchRegex != nil {
		t.Fatalf("empty ultra search did not clear regex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("empty ultra search did not clear filtered tasks")
	}
	if got := len(m.ultraTaskList()); got != len(m.tasks) {
		t.Fatalf("empty ultra search did not restore all tasks, got %d want %d", got, len(m.tasks))
	}
}

func TestUltraFocusedIDLifecycleAcrossNormalEditEntryAndReload(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, cmd := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("resize unexpectedly returned a command")
	}
	m = *mv.(*Model)

	m.blinkEnabled = false
	m.editID = 1

	mv, cmd = (&m).Update(editDoneMsg{})
	if cmd != nil {
		t.Fatalf("editDone unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.ultraFocusedID != 0 {
		t.Fatalf("normal edit completion left ultraFocusedID=%d, want 0", m.ultraFocusedID)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if cmd != nil {
		t.Fatalf("j unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if got := m.tbl.Cursor(); got != 1 {
		t.Fatalf("cursor after j = %d, want 1", got)
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("u left ultraFocusedID=%d, want 0", m.ultraFocusedID)
	}
	if got := m.ultraCursor; got != 1 {
		t.Fatalf("ultra cursor after entry = %d, want 1", got)
	}

	if err := m.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("reload left ultraFocusedID=%d, want 0", m.ultraFocusedID)
	}
	if got := m.ultraCursor; got != 1 {
		t.Fatalf("ultra cursor after reload = %d, want 1", got)
	}
	if got := m.ultraTaskList()[m.ultraCursor].ID; got != 2 {
		t.Fatalf("reload snapped to task %d, want 2", got)
	}
	if got := m.tbl.Cursor(); got != 1 {
		t.Fatalf("table cursor after reload = %d, want 1", got)
	}
}

func TestUltraSearchReloadRebuildsMatches(t *testing.T) {
	tmp := t.TempDir()
	taskPath, phaseFile := setupUltraSearchReloadTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step := func(msg tea.KeyPressMsg) {
		t.Helper()
		mv, _ := (&m).Update(msg)
		m = *mv.(*Model)
	}

	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "alpha" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})

	if got := len(m.ultraTaskList()); got != 1 {
		t.Fatalf("initial search match count = %d, want 1", got)
	}
	if got := m.ultraTaskList()[0].ID; got != 1 {
		t.Fatalf("initial search matched task %d, want 1", got)
	}

	if err := os.WriteFile(phaseFile, []byte("2"), 0o644); err != nil {
		t.Fatalf("WriteFile phase: %v", err)
	}
	if err := m.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	expectedRow := m.taskIndexByID(2)
	if expectedRow < 0 {
		t.Fatalf("reloaded tasks missing task 2")
	}
	if !reflect.DeepEqual(m.ultraFiltered, []int{expectedRow}) {
		t.Fatalf("reload did not rebuild search matches: got %#v want %#v", m.ultraFiltered, []int{expectedRow})
	}
	if got := len(m.ultraTaskList()); got != 1 {
		t.Fatalf("reloaded search match count = %d, want 1", got)
	}
	if got := m.ultraTaskList()[m.ultraCursor].ID; got != 2 {
		t.Fatalf("reload kept stale search match %d, want 2", got)
	}
	if got := m.tbl.Cursor(); got != expectedRow {
		t.Fatalf("table cursor after search reload = %d, want %d", got, expectedRow)
	}
}

func TestUltraEntryResizeAndNavigationBindings(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step := func(msg tea.KeyPressMsg) {
		t.Helper()
		mv, cmd := (&m).Update(msg)
		if cmd != nil {
			t.Fatalf("%q unexpectedly returned a command", msg.String())
		}
		m = *mv.(*Model)
	}

	resize := func(width, height int) {
		t.Helper()
		mv, cmd := (&m).Update(tea.WindowSizeMsg{Width: width, Height: height})
		if cmd != nil {
			t.Fatalf("resize %dx%d unexpectedly returned a command", width, height)
		}
		m = *mv.(*Model)
	}

	resize(60, 16)

	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.tbl.Cursor() != 2 {
		t.Fatalf("table cursor = %d, want 2 before ultra entry", m.tbl.Cursor())
	}

	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}
	if m.ultraCursor != 2 {
		t.Fatalf("u: cursor = %d, want 2", m.ultraCursor)
	}
	// 2-line cards (status + description) all fit at offset 0 in a 60×16 window.
	if m.ultraOffset != 0 {
		t.Fatalf("u: offset = %d, want 0", m.ultraOffset)
	}
	if got := m.ultraVisibleCount(); got != 3 {
		t.Fatalf("u: visible count = %d, want 3", got)
	}
	if start := m.ultraVisibleStart(len(m.ultraTaskList())); m.ultraCursor < start || m.ultraCursor >= start+m.ultraVisibleCount() {
		t.Fatalf("u: cursor %d not visible at offset %d", m.ultraCursor, m.ultraOffset)
	}

	// At 60×7 budget=5: 2-line cards with 1-line separator → 2 cards fit (2+1+2=5).
	// cursor=2 → ultraEnsureVisible scrolls offset to 1 so cards 1 and 2 are shown.
	resize(60, 7)
	if m.ultraOffset != 1 {
		t.Fatalf("resize: offset = %d, want 1", m.ultraOffset)
	}
	if got := m.ultraVisibleCount(); got != 2 {
		t.Fatalf("resize: visible count = %d, want 2", got)
	}
	if start := m.ultraVisibleStart(len(m.ultraTaskList())); m.ultraCursor < start || m.ultraCursor >= start+m.ultraVisibleCount() {
		t.Fatalf("resize: cursor %d not visible at offset %d", m.ultraCursor, m.ultraOffset)
	}

	step(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.ultraCursor != 1 {
		t.Fatalf("k: cursor = %d, want 1", m.ultraCursor)
	}
	if m.ultraOffset != 1 {
		t.Fatalf("k: offset = %d, want 1", m.ultraOffset)
	}

	// pgdn += visibleCount(2): cursor 1→3 clamped to 2; still visible at offset 1.
	step(tea.KeyPressMsg{Code: tea.KeyPgDown, Text: "pgdn"})
	if m.ultraCursor != 2 {
		t.Fatalf("pgdn: cursor = %d, want 2", m.ultraCursor)
	}
	if m.ultraOffset != 1 {
		t.Fatalf("pgdn: offset = %d, want 1", m.ultraOffset)
	}

	// pgup -= visibleCount(2): cursor 2→0; ultraEnsureVisible scrolls offset to 0.
	step(tea.KeyPressMsg{Code: tea.KeyPgUp, Text: "pgup"})
	if m.ultraCursor != 0 {
		t.Fatalf("pgup: cursor = %d, want 0", m.ultraCursor)
	}
	if m.ultraOffset != 0 {
		t.Fatalf("pgup: offset = %d, want 0", m.ultraOffset)
	}

	step(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.ultraCursor != 0 {
		t.Fatalf("g: cursor = %d, want 0", m.ultraCursor)
	}
	if m.ultraOffset != 0 {
		t.Fatalf("g: offset = %d, want 0", m.ultraOffset)
	}

	// G goes to last task (2); ultraEnsureVisible scrolls offset to 1.
	step(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if m.ultraCursor != 2 {
		t.Fatalf("G: cursor = %d, want 2", m.ultraCursor)
	}
	if m.ultraOffset != 1 {
		t.Fatalf("G: offset = %d, want 1", m.ultraOffset)
	}
}

func TestUltraBlinkUsesVisibleSelectionAndRendersBlink(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step := func(msg tea.KeyPressMsg) {
		t.Helper()
		mv, cmd := (&m).Update(msg)
		if cmd != nil {
			t.Fatalf("%q unexpectedly returned a command", msg.String())
		}
		m = *mv.(*Model)
	}

	mv, cmd := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("resize unexpectedly returned a command")
	}
	m = *mv.(*Model)

	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}

	m.blinkID = m.tasks[m.ultraCursor].ID
	m.blinkOn = false
	baseView := m.renderUltraModus()
	m.blinkOn = true
	blinkView := m.renderUltraModus()
	if baseView == blinkView {
		t.Fatalf("ultra view did not change when blink state toggled")
	}

	beforeTable := m.tbl.Cursor()
	beforeUltra := m.ultraCursor
	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if cmd != nil {
		t.Fatalf("blink navigation unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.tbl.Cursor() != beforeTable {
		t.Fatalf("blink navigation moved hidden table cursor: got %d want %d", m.tbl.Cursor(), beforeTable)
	}
	if m.ultraCursor != beforeUltra+1 {
		t.Fatalf("blink navigation did not move visible ultra cursor: got %d want %d", m.ultraCursor, beforeUltra+1)
	}
}

func TestUltraPriorityOpUsesUltraSelection(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	m.showUltra = true
	m.tbl.SetCursor(0)
	m.ultraCursor = 1

	hiddenID := m.tasks[m.tbl.Cursor()].ID
	selectedID := m.ultraTaskList()[m.ultraCursor].ID
	if hiddenID == selectedID {
		t.Fatalf("test setup failed: hidden and ultra selections match")
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil {
		t.Fatalf("ultra priority hotkey unexpectedly returned a command")
	}
	m = *mv.(*Model)

	if m.priorityID != selectedID {
		t.Fatalf("priority editor targeted task %d, want ultra-selected task %d", m.priorityID, selectedID)
	}
	if m.priorityID == hiddenID {
		t.Fatalf("priority editor followed hidden table cursor %d instead of ultra selection %d", hiddenID, selectedID)
	}
	if !m.prioritySelecting {
		t.Fatalf("priority editor was not activated")
	}
}

func TestUltraOpenURLHotkeyUsesUltraSelection(t *testing.T) {
	tmp := t.TempDir()
	taskPath := filepath.Join(tmp, "task")
	openLog := filepath.Join(tmp, "open.log")
	browserPath := filepath.Join(tmp, "browser")

	taskScript := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"alpha\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  echo '{\"id\":2,\"uuid\":\"2\",\"description\":\"beta https://example.com\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(taskScript), 0o755); err != nil {
		t.Fatal(err)
	}

	browserScript := "#!/bin/sh\n" +
		"echo \"$1\" >> " + openLog + "\n"
	if err := os.WriteFile(browserPath, []byte(browserScript), 0o755); err != nil {
		t.Fatal(err)
	}

	setupEnv(t, taskPath)

	m, err := New(nil, browserPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	m.showUltra = true
	m.tbl.SetCursor(0)
	m.ultraCursor = 1

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		// Opening a URL starts the blink animation, so a command is expected.
		t.Fatalf("ultra open URL unexpectedly returned no command")
	}
	m = *mv.(*Model)

	data, err := os.ReadFile(openLog)
	if err != nil {
		t.Fatalf("read open log: %v", err)
	}
	if strings.TrimSpace(string(data)) != "https://example.com" {
		t.Fatalf("browser not called with ultra-selected url: %q", data)
	}

	// Clear the blink state so we can test the no-URL path with a second keypress.
	m.blinkID = 0
	m.blinkOn = false
	m.ultraCursor = 0

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("ultra open URL for task without url unexpectedly returned a command")
	}
	m = *mv.(*Model)

	data, err = os.ReadFile(openLog)
	if err != nil {
		t.Fatalf("read open log after no-url task: %v", err)
	}
	if strings.TrimSpace(string(data)) != "https://example.com" {
		t.Fatalf("browser was called for task without url: %q", data)
	}
}

func TestUltraReloadPreservesFilteredSelection(t *testing.T) {
	tmp := t.TempDir()
	taskPath, phaseFile := setupUltraReloadTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got := m.tasks; len(got) != 3 || got[0].ID != 3 || got[1].ID != 2 || got[2].ID != 1 {
		t.Fatalf("unexpected initial sort order: %+v", got)
	}

	m.showUltra = true
	m.ultraFiltered = []int{0, 2}
	m.ultraCursor = 1
	m.ultraOffset = 0
	if got := m.ultraTaskList(); len(got) != 2 || got[0].ID != 3 || got[1].ID != 1 {
		t.Fatalf("unexpected initial ultra filter list: %+v", got)
	}

	if err := os.WriteFile(phaseFile, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if got := m.tasks; len(got) != 3 || got[0].ID != 1 || got[1].ID != 3 || got[2].ID != 2 {
		t.Fatalf("unexpected reloaded sort order: %+v", got)
	}
	if !reflect.DeepEqual(m.ultraFiltered, []int{1, 0}) {
		t.Fatalf("ultraFiltered was not rebuilt from task IDs: %#v", m.ultraFiltered)
	}
	if got := m.ultraTaskList(); len(got) != 2 || got[0].ID != 3 || got[1].ID != 1 {
		t.Fatalf("ultra task list changed selection order: %+v", got)
	}
	if got := m.ultraTaskList()[m.ultraCursor].ID; got != 1 {
		t.Fatalf("ultra cursor drifted after reload: got task %d want 1", got)
	}
}

func TestUltraInlineOverlayRenders(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mv, cmd := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("resize unexpectedly returned a command")
	}
	m = *mv.(*Model)

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd != nil {
		t.Fatalf("u unexpectedly returned a command")
	}
	m = *mv.(*Model)

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if cmd != nil {
		t.Fatalf("f unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if !m.filterEditing {
		t.Fatalf("f did not activate filter editing in ultra mode")
	}

	view := m.View().Content
	if !strings.Contains(view, "filter:") {
		t.Fatalf("ultra view did not render filter overlay: %q", view)
	}
}

// TestExpandedCellViewNoDoubleRender is a regression test for a bug where
// expandedCellView() was appended to the layout unconditionally AND again
// inside the cellExpanded guard, producing a duplicate line when expanded.
// It verifies that:
//   - when cellExpanded is false, expandedCellView content is absent from View()
//   - when cellExpanded is true, expandedCellView content appears exactly once
//   - the expanded content does not appear when expandedCellView returns ""
func TestExpandedCellViewNoDoubleRender(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Give the model a non-zero window size so View() renders real content.
	m.windowHeight = 24
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = *mv.(*Model)

	// Determine what expandedCellView() currently returns so we know what to
	// search for in the full View() output.
	expanded := m.expandedCellView()
	if expanded == "" {
		t.Skip("expandedCellView returned empty string; cannot test placement")
	}

	// With cellExpanded false (the default), the expanded content must be absent.
	m.cellExpanded = false
	viewCollapsed := m.View()
	if strings.Contains(viewCollapsed.Content, expanded) {
		t.Fatalf("cellExpanded=false: expandedCellView content unexpectedly present in View()")
	}

	// With cellExpanded true, the expanded content must appear exactly once.
	m.cellExpanded = true
	viewExpanded := m.View()
	count := strings.Count(viewExpanded.Content, expanded)
	if count != 1 {
		t.Fatalf("cellExpanded=true: expandedCellView content appears %d times in View(), want exactly 1", count)
	}
}

func TestSearchExitHotkeys(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// enter search mode
	mv, _ := (&m).Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = *mv.(*Model)
	for _, r := range "alpha" {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mp := &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if m.searchRegex == nil {
		t.Fatalf("search regex not set")
	}

	// escape search results with ESC
	mp = &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = *mv.(*Model)
	if m.searchRegex != nil {
		t.Fatalf("esc did not clear search")
	}

	// search again and exit with q
	mp = &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = *mv.(*Model)
	for _, r := range "beta" {
		mp := &m
		mv, _ = mp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	mp = &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if m.searchRegex == nil {
		t.Fatalf("search regex not set for q")
	}

	mp = &m
	mv, _ = mp.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = *mv.(*Model)
	if m.searchRegex != nil {
		t.Fatalf("q did not clear search")
	}
}

func TestUltraResizeSyncRefreshesNormalSearchSelection(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupSharedSearchTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step := func(msg tea.Msg) {
		t.Helper()
		mv, _ := (&m).Update(msg)
		m = *mv.(*Model)
	}

	step(tea.WindowSizeMsg{Width: 120, Height: 24})
	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "shared" {
		step(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	step(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.searchRegex == nil {
		t.Fatalf("normal search regex not set")
	}
	if got := m.tbl.Cursor(); got != 0 {
		t.Fatalf("initial search cursor = %d, want 0", got)
	}
	if got := m.tbl.ColumnCursor(); got != 8 {
		t.Fatalf("initial search column = %d, want 8", got)
	}

	step(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}
	step(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := m.ultraCursor; got != 1 {
		t.Fatalf("ultra cursor = %d, want 1", got)
	}

	step(tea.WindowSizeMsg{Width: 100, Height: 24})
	if got := m.tbl.Cursor(); got != 1 {
		t.Fatalf("hidden table cursor after ultra resize = %d, want 1", got)
	}

	rows := m.tbl.Rows()
	wantPrev := m.taskToRowSearch(m.tasks[0], m.searchRegex, m.tblStyles, -1)
	wantNew := m.taskToRowSearch(m.tasks[1], m.searchRegex, m.tblStyles, m.tbl.ColumnCursor())
	if !reflect.DeepEqual(rows[0], wantPrev) {
		t.Fatalf("previous row retained stale search selection after ultra resize")
	}
	if !reflect.DeepEqual(rows[1], wantNew) {
		t.Fatalf("new row did not receive refreshed search selection after ultra resize")
	}
}

// setupSingleTask creates a fake task binary that returns exactly one task.
func setupSingleTask(t *testing.T, tmp string) string {
	t.Helper()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  echo '{\"id\":1,\"uuid\":\"1\",\"description\":\"only task\",\"status\":\"pending\",\"entry\":\"\",\"priority\":\"\",\"urgency\":0}'\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

// setupEmptyTasks creates a fake task binary that returns no tasks.
func setupEmptyTasks(t *testing.T, tmp string) string {
	t.Helper()
	taskPath := filepath.Join(tmp, "task")
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q export; then\n" +
		"  exit 0\n" +
		"fi\n"
	if err := os.WriteFile(taskPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return taskPath
}

// TestUltraSingleTaskCursorStaysAtZero verifies that with one task the cursor
// never advances beyond index 0 when navigating down or jumping to the end.
func TestUltraSingleTaskCursorStaysAtZero(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupSingleTask(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Enter ultra mode.
	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}
	if len(m.ultraTaskList()) != 1 {
		t.Fatalf("expected 1 task, got %d", len(m.ultraTaskList()))
	}

	// Moving down on a single-task list must keep cursor at 0.
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = *mv.(*Model)
	if got := m.ultraCursor; got != 0 {
		t.Fatalf("j on single task: cursor = %d, want 0", got)
	}

	// Jump to end — still must be 0.
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = *mv.(*Model)
	if got := m.ultraCursor; got != 0 {
		t.Fatalf("G on single task: cursor = %d, want 0", got)
	}

	// Verify ultraVisibleCursor agrees.
	if got := m.ultraVisibleCursor(m.ultraTaskList()); got != 0 {
		t.Fatalf("ultraVisibleCursor on single task = %d, want 0", got)
	}
}

// TestUltraNoTasksRender verifies that renderUltraModus does not panic and
// returns a non-empty string containing "No tasks" when the task list is empty.
func TestUltraNoTasksRender(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupEmptyTasks(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Give the model a window size so rendering has a budget.
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = *mv.(*Model)

	// Enter ultra mode.
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = *mv.(*Model)

	// Force ultra mode on in case no tasks means it doesn't activate normally.
	m.showUltra = true

	if len(m.ultraTaskList()) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(m.ultraTaskList()))
	}

	rendered := m.renderUltraModus()
	if rendered == "" {
		t.Fatal("renderUltraModus returned empty string for empty task list")
	}
	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "No tasks") {
		t.Fatalf("renderUltraModus did not contain 'No tasks', got:\n%s", stripped)
	}
}

// TestUltraCursorClampAfterFilterZeroResults verifies that when a search filter
// leaves zero results the cursor is clamped to 0 and remains in-bounds.
func TestUltraCursorClampAfterFilterZeroResults(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupUltraTaskSet(t, tmp)
	setupEnv(t, taskPath)

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Enter ultra mode.
	mv, _ := (&m).Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = *mv.(*Model)
	if !m.showUltra {
		t.Fatalf("u did not enter ultra mode")
	}

	// Move cursor to the last task.
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = *mv.(*Model)
	if got := m.ultraCursor; got != 2 {
		t.Fatalf("G: cursor = %d, want 2", got)
	}

	// Apply a search that matches nothing — filter produces zero results.
	m.ultraSearchRegex = regexp.MustCompile("zzznomatch")
	m.ultraFiltered = m.ultraFilteredIndexes(m.ultraSearchRegex)
	if len(m.ultraFiltered) != 0 {
		t.Fatalf("expected zero filtered results, got %d", len(m.ultraFiltered))
	}

	// ultraEnsureVisible must clamp cursor to 0 for an empty list.
	m.ultraEnsureVisible()
	if got := m.ultraCursor; got != 0 {
		t.Fatalf("cursor after zero-result filter = %d, want 0", got)
	}
	if got := m.ultraOffset; got != 0 {
		t.Fatalf("offset after zero-result filter = %d, want 0", got)
	}

	// ultraVisibleCursor must return -1 for an empty task list.
	if got := m.ultraVisibleCursor(m.ultraTaskList()); got != -1 {
		t.Fatalf("ultraVisibleCursor for empty list = %d, want -1", got)
	}
}
