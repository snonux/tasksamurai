package ui

import (
	"fmt"
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

	m, err := New(nil, "firefox")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mp := &m  // Get pointer to model
	mv, _ := mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mp = mv.(*Model)
	for _, r := range "note" {
		mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mp = mv.(*Model)
	}
	mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = *mv.(*Model)
	for _, r := range "new" {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = *mv.(*Model)
	for i := 0; i < blinkCycles; i++ {
		mp := &m; mv, _ = mp.Update(blinkMsg{})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = *mv.(*Model)
	for i := 0; i < blinkCycles; i++ {
		mp := &m; mv, _ = mp.Update(blinkMsg{})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = *mv.(*Model)
	for i := 0; i < 3; i++ {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = *mv.(*Model)
	for _, r := range "daily" {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = *mv.(*Model)
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	m = *mv.(*Model)
	for _, r := range "foo due:today" {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 1 {
		t.Fatalf("down: got cursor %d", m.tbl.Cursor())
	}

	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 0 {
		t.Fatalf("0 hotkey: expected 0 got %d", m.tbl.Cursor())
	}

	mp = &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = *mv.(*Model)
	if m.tbl.Cursor() != 1 {
		t.Fatalf("G hotkey: expected 1 got %d", m.tbl.Cursor())
	}
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

	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	m = *mv.(*Model)
	if !m.showHelp {
		t.Fatalf("help not shown")
	}

	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = *mv.(*Model)
	if m.showHelp {
		t.Fatalf("esc did not close help")
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
	mv, _ := (&m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = *mv.(*Model)
	for _, r := range "alpha" {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = *mv.(*Model)
	}
	mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *mv.(*Model)
	if m.searchRegex == nil {
		t.Fatalf("search regex not set")
	}

	// escape search results with ESC
	mp = &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = *mv.(*Model)
	if m.searchRegex != nil {
		t.Fatalf("esc did not clear search")
	}

	// search again and exit with q
	mp = &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = *mv.(*Model)
	for _, r := range "beta" {
		mp := &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = *mv.(*Model)
	}
	mp = &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *mv.(*Model)
	if m.searchRegex == nil {
		t.Fatalf("search regex not set for q")
	}

	mp = &m; mv, _ = mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = *mv.(*Model)
	if m.searchRegex != nil {
		t.Fatalf("q did not clear search")
	}
}
