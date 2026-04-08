//go:build !debugsignals && !windows

package debug

// SetDebugDir sets the directory where debug output files will be written.
// In production builds, runtime signal diagnostics are disabled.
func SetDebugDir(dir string) {}

// InitSignalHandlers is a no-op in production builds.
func InitSignalHandlers() {}
