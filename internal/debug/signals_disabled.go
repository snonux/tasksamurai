//go:build !debugsignals && !windows

package debug

// InitSignalHandlers is a no-op in production builds. SetDebugDir,
// DumpStateMsg, and SetSender remain available (see state.go) but nothing
// ever triggers a dump without the debugsignals build tag.
func InitSignalHandlers() {}
