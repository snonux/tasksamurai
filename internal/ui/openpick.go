package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

// The "O" key opens the Nth link of a task. Plain "o" only ever opens the
// first URL (or, without one, the first @file reference). A vim-style count
// prefix ("2o") is not possible because digits 0-6 are configurable hotkeys,
// so "O" instead lists every target numbered in the status bar and the next
// key picks one: 1-9 opens that target, anything else cancels. The order
// matches "o" (URLs before @files), so "O1" opens what "o" opens.

// maxOpenTargets caps the list at the single-digit keys 1-9.
const maxOpenTargets = 9

type openTargetKind uint8

const (
	openTargetURL openTargetKind = iota
	openTargetFile
)

// openTarget is one openable item: a browser URL or an @file path.
type openTarget struct {
	kind  openTargetKind
	value string
}

// openPickState holds the targets offered by "O" while waiting for a digit.
type openPickState struct {
	targets []openTarget
	taskID  int
}

func (s openPickState) active() bool { return len(s.targets) > 0 }

// findTaskOpenTargets returns every URL (description first, then
// annotations) followed by every @file reference, without duplicates and
// capped at maxOpenTargets.
func findTaskOpenTargets(t *task.Task) []openTarget {
	texts := make([]string, 0, 1+len(t.Annotations))
	texts = append(texts, t.Description)
	for _, ann := range t.Annotations {
		texts = append(texts, ann.Description)
	}
	var targets []openTarget
	seen := map[openTarget]bool{}
	add := func(tg openTarget) {
		if tg.value != "" && !seen[tg] && len(targets) < maxOpenTargets {
			seen[tg] = true
			targets = append(targets, tg)
		}
	}
	for _, text := range texts {
		for _, u := range urlRegex.FindAllString(text, -1) {
			add(openTarget{kind: openTargetURL, value: u})
		}
	}
	for _, text := range texts {
		for _, p := range extractFileRefs(text) {
			add(openTarget{kind: openTargetFile, value: p})
		}
	}
	return targets
}

// extractFileRefs returns all file references in text, applying the same
// rules as extractFileRef: "@path" is taken as-is, "@ path" only when the
// path exists on disk.
func extractFileRefs(text string) []string {
	var paths []string
	for _, match := range fileRefRegex.FindAllStringSubmatch(text, -1) {
		if path := resolveFileRefPath(match[2]); path != "" {
			paths = append(paths, path)
		}
	}
	for _, match := range fileRefSpacedRegex.FindAllStringSubmatch(text, -1) {
		if path := resolveFileRefPath(match[2]); path != "" && fileExists(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// handleOpenPick implements the "O" key. Jira issues keep their single
// target (the issue page) like "o". With one target it opens immediately;
// with several it lists them and waits for a digit (handleOpenPickKey).
func (m *Model) handleOpenPick() (tea.Model, tea.Cmd) {
	tsk := m.getTaskForOpenURL()
	if tsk == nil {
		return m, nil
	}
	if routeControl(tsk.UUID, "o").route == routeJira {
		return m.handleOpenURL()
	}
	targets := findTaskOpenTargets(tsk)
	switch len(targets) {
	case 0:
		return m, m.showStatusTimed("No URL or @file reference in this task")
	case 1:
		return m, m.openTargetCmd(targets[0], tsk.ID)
	}
	m.openPick = openPickState{targets: targets, taskID: tsk.ID}
	m.statusMsg = openPickPrompt(targets)
	return m, nil
}

// openPickPrompt renders the numbered target list for the status bar.
// Long entries are shortened so several fit on one line.
func openPickPrompt(targets []openTarget) string {
	parts := make([]string, len(targets))
	for i, tg := range targets {
		v := tg.value
		if tg.kind == openTargetFile {
			v = "@" + v
		}
		if r := []rune(v); len(r) > 40 {
			v = string(r[:39]) + "…"
		}
		parts[i] = fmt.Sprintf("%d: %s", i+1, v)
	}
	return "Open which? " + strings.Join(parts, "  ") + "  (ESC cancels)"
}

// handleOpenPickKey consumes the key after "O": a digit within range opens
// that target; any other key cancels the pick without further effect.
func (m *Model) handleOpenPickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	pick := m.openPick
	m.openPick = openPickState{}
	m.statusMsg = ""
	key := msg.String()
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		if n := int(key[0] - '0'); n <= len(pick.targets) {
			return m, m.openTargetCmd(pick.targets[n-1], pick.taskID)
		}
	}
	return m, nil
}

// openTargetCmd opens a URL in the browser or an @file in $EDITOR.
func (m *Model) openTargetCmd(tg openTarget, taskID int) tea.Cmd {
	if tg.kind == openTargetFile {
		return openFileInEditorCmd(tg.value, taskID)
	}
	return openURLCmd(m.browserForURL(tg.value), tg.value, taskID)
}
