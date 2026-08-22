// Package debug provides runtime diagnostics for troubleshooting a stuck or
// unresponsive Task Samurai session: goroutine/profile dumps (signals.go,
// build tag debugsignals) and a snapshot of the UI's own state (this file).
//
// This file has no build constraint because internal/ui always needs
// DumpStateMsg and SetSender to exist, even in production builds where
// signal handling itself is compiled out (signals_disabled.go) or
// unsupported (signals_windows.go). In those builds nothing ever calls
// RequestStateDump, so the machinery here simply goes unused.
package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// debugDir is the directory where debug output files are written by both
// the goroutine/profile dumper and the UI state dumper, so all diagnostic
// output for a session lands next to each other.
var debugDir string

// SetDebugDir sets the directory where debug output files will be written.
// If empty, the current working directory is used.
func SetDebugDir(dir string) {
	debugDir = dir
}

// DumpStateMsg is sent into the running Bubble Tea program to ask it to
// write a snapshot of its own UI state to a file. It is delivered through
// the normal Update() loop (via Sender.Send), so the dump runs on the same
// goroutine that owns the model - no locking, and no risk of racing the
// very state being inspected.
type DumpStateMsg struct{}

// Sender is the subset of *tea.Program's API needed to deliver DumpStateMsg.
// Defined locally, rather than requiring callers to pass a *tea.Program
// directly, so a future test double doesn't need to spin up a whole
// bubbletea program.
type Sender interface {
	Send(msg tea.Msg)
}

var sender Sender

// SetSender registers the running program so a signal handler can ask it to
// dump its own state. Safe to call even when signal handling is compiled
// out (production builds) - the sender then simply goes unused.
func SetSender(s Sender) {
	sender = s
}

// RequestStateDump asks the registered program to dump its UI state. It
// returns false if no program has been registered yet (e.g. called before
// tea.NewProgram runs).
func RequestStateDump() bool {
	if sender == nil {
		return false
	}
	sender.Send(DumpStateMsg{})
	return true
}

// NewDumpFile creates a timestamped diagnostic file named
// "tasksamurai-<suffix>-<timestamp>.txt" inside debugDir (or the current
// working directory, if unset) and returns it along with its path.
func NewDumpFile(suffix string) (*os.File, string, error) {
	name := fmt.Sprintf("tasksamurai-%s-%s.txt", suffix, time.Now().Format("20060102-150405"))
	path := name
	if debugDir != "" {
		path = filepath.Join(debugDir, name)
	}
	f, err := os.Create(path)
	return f, path, err
}
