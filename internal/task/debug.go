package task

import (
	"io"
	"os"
)

// debugConfig groups the optional debug-logging state for the task package.
// Collecting related vars into a struct makes the mutable state explicit and
// allows the logger to be swapped or reset cleanly without touching unrelated
// package globals.
type debugConfig struct {
	writer io.Writer
	file   *os.File // tracked separately so it can be closed on reconfiguration
}

// dbg holds the active debug-logging configuration for this package.
// It is written only via SetDebugLog and read only in run().
var dbg debugConfig

// SetDebugLog enables logging of executed commands to the given file.
// Passing an empty path disables logging and closes any previously opened file.
func SetDebugLog(path string) error {
	// Close existing debug file if open before re-configuring.
	if dbg.file != nil {
		_ = dbg.file.Close()
		dbg.file = nil
		dbg.writer = nil
	}

	if path == "" {
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	dbg.file = f
	dbg.writer = f
	return nil
}
