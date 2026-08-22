//go:build windows

package debug

import (
	"fmt"
	"os"
)

// InitSignalHandlers sets up signal handlers for runtime diagnostics.
// On Windows, SIGUSR1 and SIGUSR2 are not available, so this prints a warning.
func InitSignalHandlers() {
	fmt.Fprintln(os.Stderr, "debug: signal handlers not supported on Windows")
	fmt.Fprintln(os.Stderr, "debug: consider using GODEBUG environment variable or pprof HTTP endpoint")
}
