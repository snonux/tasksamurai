package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// TestFindTaskOpenTargets checks the numbered target order used by O: every
// URL (description, then annotations) before every @file reference, with
// duplicates removed, so target 1 matches what plain o opens.
func TestFindTaskOpenTargets(t *testing.T) {
	tsk := &task.Task{
		Description: "read @notes/a.txt and https://one.example then https://two.example",
		Annotations: []task.Annotation{{Description: "also https://one.example and @notes/b.md"}},
	}
	got := findTaskOpenTargets(tsk)
	want := []openTarget{
		{kind: openTargetURL, value: "https://one.example"},
		{kind: openTargetURL, value: "https://two.example"},
		{kind: openTargetFile, value: "notes/a.txt"},
		{kind: openTargetFile, value: "notes/b.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findTaskOpenTargets = %#v, want %#v", got, want)
	}
	if got := findTaskOpenTargets(&task.Task{Description: "nothing here"}); len(got) != 0 {
		t.Fatalf("expected no targets, got %#v", got)
	}
}

// newOpenPickModel builds a model whose browser command records the opened
// URL into a file, so tests can assert which target O<n> picked.
func newOpenPickModel(t *testing.T, desc string) (Model, string) {
	t.Helper()
	tmp := t.TempDir()
	out := filepath.Join(tmp, "opened.txt")
	browser := filepath.Join(tmp, "browser")
	if err := os.WriteFile(browser, []byte("#!/bin/sh\necho \"$1\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "u1", Description: desc, Status: "pending"}}}
	m, err := NewWithTaskwarrior(nil, browser, fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	m.youtubeBrowserCmd = ""
	return m, out
}

func pressKey(m *Model, r rune) tea.Cmd {
	mv, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	*m = *mv.(*Model)
	return cmd
}

// TestOpenPickOpensNthTarget presses O then 2 and expects the second URL.
func TestOpenPickOpensNthTarget(t *testing.T) {
	m, out := newOpenPickModel(t, "a https://one.example b https://two.example")

	if cmd := pressKey(&m, 'O'); cmd != nil {
		t.Fatalf("O with several targets should only prompt, got a command")
	}
	if !strings.Contains(m.statusMsg, "1:") || !strings.Contains(m.statusMsg, "2:") {
		t.Fatalf("status does not list numbered targets: %q", m.statusMsg)
	}
	cmd := pressKey(&m, '2')
	if cmd == nil {
		t.Fatalf("O2 returned no command")
	}
	if done, ok := cmd().(openURLDoneMsg); !ok || done.err != nil {
		t.Fatalf("O2 command result = %#v", done)
	}
	if !waitForFile(out, 2*time.Second) {
		t.Fatalf("browser was not invoked")
	}
	data, _ := os.ReadFile(out)
	if got := strings.TrimSpace(string(data)); got != "https://two.example" {
		t.Fatalf("opened %q, want second URL", got)
	}
	if m.openPick.active() {
		t.Fatalf("pick state not cleared after choosing")
	}
}

// TestOpenPickCancelAndOutOfRange checks that ESC and an out-of-range digit
// cancel the pick without opening anything or leaking the key elsewhere.
func TestOpenPickCancelAndOutOfRange(t *testing.T) {
	m, _ := newOpenPickModel(t, "a https://one.example b https://two.example")

	pressKey(&m, 'O')
	if cmd := pressKey(&m, '9'); cmd != nil {
		t.Fatalf("out-of-range digit returned a command")
	}
	if m.openPick.active() {
		t.Fatalf("out-of-range digit did not end the pick")
	}

	pressKey(&m, 'O')
	mv, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = *mv.(*Model)
	if cmd != nil || m.openPick.active() {
		t.Fatalf("ESC did not cancel the pick cleanly")
	}
}

// TestOpenPickSingleTargetOpensDirectly: with one target O behaves like o.
func TestOpenPickSingleTargetOpensDirectly(t *testing.T) {
	m, _ := newOpenPickModel(t, "only https://one.example")
	if cmd := pressKey(&m, 'O'); cmd == nil {
		t.Fatalf("O with a single target should open it directly")
	}
	if m.openPick.active() {
		t.Fatalf("single target should not enter pick mode")
	}

	m2, _ := newOpenPickModel(t, "no links")
	pressKey(&m2, 'O')
	if m2.openPick.active() || !strings.Contains(m2.statusMsg, "No URL") {
		t.Fatalf("no-target O status = %q", m2.statusMsg)
	}
}
