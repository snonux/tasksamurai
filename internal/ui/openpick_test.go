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
	if st := m.statusLine(); !strings.Contains(st, "1: https://one.example") || !strings.Contains(st, "2: https://two.example") {
		t.Fatalf("status does not list numbered targets: %q", st)
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

// TestOpenPickPromptSurvivesStatusChanges: a status clear (timed tick or
// reload message) must not hide a pending pick, and auto-refresh must pause
// while it waits, so the next key is never swallowed by an invisible prompt.
func TestOpenPickPromptSurvivesStatusChanges(t *testing.T) {
	m, _ := newOpenPickModel(t, "a https://one.example b https://two.example")
	pressKey(&m, 'O')

	mv, _ := m.Update(clearStatusMsg{})
	m = *mv.(*Model)
	m.statusMsg = "Reloading…"
	if st := m.statusLine(); !strings.Contains(st, "Open which?") {
		t.Fatalf("pick prompt hidden by status change: %q", st)
	}
	if !m.anyInputActive() {
		t.Fatalf("auto-refresh is not paused while a pick is pending")
	}
}

// TestOpenPickFileTargetAndDetailView: O works in the detail view and a
// chosen @file target opens in the editor rather than the browser.
func TestOpenPickFileTargetAndDetailView(t *testing.T) {
	m, _ := newOpenPickModel(t, "see https://one.example and @notes/x.txt")
	mv, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if !m.showTaskDetail {
		t.Fatalf("detail view did not open")
	}
	pressKey(&m, 'O')
	if !m.openPick.active() {
		t.Fatalf("O in detail view did not start a pick")
	}
	if !strings.Contains(m.renderDetailScreen(), "2: @notes/x.txt") {
		t.Fatalf("detail overlay does not show the pick prompt")
	}
	if got := m.openPick.targets[1]; got.kind != openTargetFile || got.value != "notes/x.txt" {
		t.Fatalf("second target = %#v, want @file notes/x.txt", got)
	}
	if cmd := pressKey(&m, '2'); cmd == nil {
		t.Fatalf("O2 on a file target returned no command")
	}
	if !m.showTaskDetail || m.openPick.active() {
		t.Fatalf("pick did not finish cleanly inside the detail view")
	}
}

// TestOpenPickPromptInUltraView: ultra draws its own status lines, so the
// pick prompt must be added there too or the pick would be invisible.
func TestOpenPickPromptInUltraView(t *testing.T) {
	m, _ := newOpenPickModel(t, "a https://one.example b https://two.example")
	pressKey(&m, 'u')
	if !m.showUltra {
		t.Fatalf("u did not open ultra view")
	}
	mv, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = *mv.(*Model)
	pressKey(&m, 'O')
	if !m.openPick.active() {
		t.Fatalf("O in ultra view did not start a pick")
	}
	if !strings.Contains(m.renderUltraModus(), "Open which?") {
		t.Fatalf("ultra view does not show the pick prompt")
	}
}
