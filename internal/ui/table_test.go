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

func TestUltraExitHotkeysClearUltraState(t *testing.T) {
	tmp := t.TempDir()
	taskPath := setupBasicTask(t, tmp)
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

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatalf("q in ultra mode unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("q did not exit ultra mode")
	}
	if m.ultraSearchRegex != nil {
		t.Fatalf("q did not clear ultraSearchRegex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("q did not clear ultraFiltered")
	}
	if got := m.ultraSearchInput.Value(); got != "" {
		t.Fatalf("q did not clear ultraSearchInput, got %q", got)
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

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc in ultra mode unexpectedly returned a command")
	}
	m = *mv.(*Model)
	if m.showUltra {
		t.Fatalf("esc did not exit ultra mode")
	}
	if m.ultraSearchRegex != nil {
		t.Fatalf("esc did not clear ultraSearchRegex")
	}
	if m.ultraFiltered != nil {
		t.Fatalf("esc did not clear ultraFiltered")
	}
	if got := m.ultraSearchInput.Value(); got != "" {
		t.Fatalf("esc did not clear ultraSearchInput, got %q", got)
	}
	if m.ultraFocusedID != 0 {
		t.Fatalf("esc did not clear ultraFocusedID, got %d", m.ultraFocusedID)
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
	if m.ultraOffset != 1 {
		t.Fatalf("u: offset = %d, want 1", m.ultraOffset)
	}
	if got := m.ultraVisibleCount(); got != 2 {
		t.Fatalf("u: visible count = %d, want 2", got)
	}
	if start := m.ultraVisibleStart(len(m.ultraTaskList())); m.ultraCursor < start || m.ultraCursor >= start+m.ultraVisibleCount() {
		t.Fatalf("u: cursor %d not visible at offset %d", m.ultraCursor, m.ultraOffset)
	}

	resize(60, 7)
	if m.ultraOffset != 2 {
		t.Fatalf("resize: offset = %d, want 2", m.ultraOffset)
	}
	if got := m.ultraVisibleCount(); got != 1 {
		t.Fatalf("resize: visible count = %d, want 1", got)
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

	step(tea.KeyPressMsg{Code: tea.KeyPgDown, Text: "pgdn"})
	if m.ultraCursor != 2 {
		t.Fatalf("pgdn: cursor = %d, want 2", m.ultraCursor)
	}
	if m.ultraOffset != 2 {
		t.Fatalf("pgdn: offset = %d, want 2", m.ultraOffset)
	}

	step(tea.KeyPressMsg{Code: tea.KeyPgUp, Text: "pgup"})
	if m.ultraCursor != 1 {
		t.Fatalf("pgup: cursor = %d, want 1", m.ultraCursor)
	}
	if m.ultraOffset != 1 {
		t.Fatalf("pgup: offset = %d, want 1", m.ultraOffset)
	}

	step(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.ultraCursor != 0 {
		t.Fatalf("g: cursor = %d, want 0", m.ultraCursor)
	}
	if m.ultraOffset != 0 {
		t.Fatalf("g: offset = %d, want 0", m.ultraOffset)
	}

	step(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if m.ultraCursor != 2 {
		t.Fatalf("G: cursor = %d, want 2", m.ultraCursor)
	}
	if m.ultraOffset != 2 {
		t.Fatalf("G: offset = %d, want 2", m.ultraOffset)
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
