package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// newShellTestModel builds a Model wired to a fake taskwarrior so the shell
// prompt (shellInput) is initialized, then marks the prompt active.
func newShellTestModel(t *testing.T) *Model {
	t.Helper()
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "fake-1", Description: "buy milk", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.shellActive = true
	m.shellInput.Focus()
	return &m
}

// ctrlOKey builds a real tea.KeyPressMsg for Ctrl+O, matching how bubbletea
// delivers the key to Update.
func ctrlOKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
}

// TestShellPromptCtrlOSchedulesEditor verifies that pressing Ctrl+O inside
// the : prompt returns a command (the temp-file preparation step) rather
// than forwarding the key to the text input.
func TestShellPromptCtrlOSchedulesEditor(t *testing.T) {
	m := newShellTestModel(t)
	m.shellInput.SetValue("add Buy milk")

	mv, cmd := m.handleShellMode(ctrlOKey())
	m = mv.(*Model)
	if cmd == nil {
		t.Fatalf("expected a command from Ctrl+O, got nil")
	}
	// The prompt should stay active so it reappears after the editor closes.
	if !m.shellActive {
		t.Fatalf("expected shell prompt to remain active while editor is open")
	}
}

// TestPrepareShellEditCmdWritesTempFile verifies the prepare step writes the
// prompt content to a temp file and returns a launch message with the path.
func TestPrepareShellEditCmdWritesTempFile(t *testing.T) {
	cmd := prepareShellEditCmd("add Buy milk")
	msg := cmd()
	launch, ok := msg.(shellEditLaunchMsg)
	if !ok {
		t.Fatalf("expected shellEditLaunchMsg, got %T", msg)
	}
	if launch.err != nil {
		t.Fatalf("unexpected prepare error: %v", launch.err)
	}
	if launch.tempFile == "" {
		t.Fatalf("expected a temp file path")
	}
	defer os.Remove(launch.tempFile)

	data, err := os.ReadFile(launch.tempFile)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(data) != "add Buy milk" {
		t.Fatalf("temp file content = %q, want %q", string(data), "add Buy milk")
	}
}

// TestHandleShellEditDoneLoadsEditedContent verifies that when the editor
// finishes, the edited file content is loaded back into the prompt
// (single-line collapsed) and the temp file is removed.
func TestHandleShellEditDoneLoadsEditedContent(t *testing.T) {
	m := newShellTestModel(t)

	// Simulate an editor save: write a temp file with a trailing newline and
	// an extra blank line, which should be collapsed to a single command line.
	dir := t.TempDir()
	tmp := filepath.Join(dir, "edited.txt")
	content := []byte("add Buy cheese\n\n")
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	mv, cmd := m.handleShellEditDone(shellEditDoneMsg{tempFile: tmp})
	m = mv.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command after edit done, got %v", cmd)
	}
	if got := m.shellInput.Value(); got != "add Buy cheese" {
		t.Fatalf("shell input = %q, want %q", got, "add Buy cheese")
	}
	if !strings.Contains(m.statusMsg, "Enter to run") {
		t.Fatalf("expected status hint about Enter to run, got %q", m.statusMsg)
	}
	// The temp file should have been removed by the handler.
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed, got stat err=%v", err)
	}
}

// TestHandleShellEditDoneEmptyContent verifies that an editor saving an empty
// file clears the prompt and reports an informative status message.
func TestHandleShellEditDoneEmptyContent(t *testing.T) {
	m := newShellTestModel(t)
	m.shellInput.SetValue("leftover")

	dir := t.TempDir()
	tmp := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(tmp, []byte("   \n\n"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	mv, _ := m.handleShellEditDone(shellEditDoneMsg{tempFile: tmp})
	m = mv.(*Model)
	if got := m.shellInput.Value(); got != "" {
		t.Fatalf("shell input = %q, want empty", got)
	}
	if !strings.Contains(m.statusMsg, "empty") {
		t.Fatalf("expected status message about empty prompt, got %q", m.statusMsg)
	}
}

// TestHandleShellEditDoneReportsReadError verifies a read failure is surfaced
// via the status bar rather than crashing.
func TestHandleShellEditDoneReportsReadError(t *testing.T) {
	m := newShellTestModel(t)
	mv, cmd := m.handleShellEditDone(shellEditDoneMsg{tempFile: "/nonexistent/path/to/file.txt"})
	m = mv.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command on read error")
	}
	if !strings.Contains(m.statusMsg, "Error") {
		t.Fatalf("expected an error status, got %q", m.statusMsg)
	}
}

// TestHandleShellEditDoneRemovesTempFileOnEditorError verifies that when the
// editor process itself fails (shellEditDoneMsg.err set), the temp file is
// still removed and the error is surfaced via the status bar.
func TestHandleShellEditDoneRemovesTempFileOnEditorError(t *testing.T) {
	m := newShellTestModel(t)

	dir := t.TempDir()
	tmp := filepath.Join(dir, "edited.txt")
	if err := os.WriteFile(tmp, []byte("add Buy milk"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	mv, cmd := m.handleShellEditDone(shellEditDoneMsg{err: errEditorFailed, tempFile: tmp})
	m = mv.(*Model)
	if cmd != nil {
		t.Fatalf("expected no command on editor error")
	}
	if !strings.Contains(m.statusMsg, "Error") {
		t.Fatalf("expected an error status, got %q", m.statusMsg)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected temp file removed on editor error, got stat err=%v", err)
	}
}

var errEditorFailed = fmt.Errorf("editor exited with status 1")
