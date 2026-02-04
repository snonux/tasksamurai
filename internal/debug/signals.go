// +build !windows

package debug

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	// debugDir is the directory where debug output files are written
	debugDir string
	// dumping prevents concurrent dump attempts
	dumping int32
)

// SetDebugDir sets the directory where debug output files will be written.
// If empty, the current working directory is used.
func SetDebugDir(dir string) {
	debugDir = dir
}

// InitSignalHandlers sets up signal handlers for runtime diagnostics.
// SIGUSR1: Dump goroutine stacks
// SIGUSR2: Dump full runtime profiles (goroutines, heap, cpu, block)
func InitSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1, syscall.SIGUSR2)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGUSR1:
				dumpGoroutines()
			case syscall.SIGUSR2:
				dumpFullProfile()
			}
		}
	}()
}

// dumpGoroutines writes all goroutine stacks to a file
func dumpGoroutines() {
	if !atomic.CompareAndSwapInt32(&dumping, 0, 1) {
		fmt.Fprintln(os.Stderr, "debug: dump already in progress, skipping")
		return
	}
	defer atomic.StoreInt32(&dumping, 0)

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("tasksamurai-goroutines-%s.txt", timestamp)
	if debugDir != "" {
		filename = filepath.Join(debugDir, filename)
	}

	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: failed to create goroutine dump file: %v\n", err)
		return
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "TaskSamurai Goroutine Dump\n")
	fmt.Fprintf(f, "Timestamp: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "NumGoroutine: %d\n", runtime.NumGoroutine())
	fmt.Fprintf(f, "NumCPU: %d\n", runtime.NumCPU())
	fmt.Fprintf(f, "\n%s\n\n", strings.Repeat("=", 80))

	// Get stack traces
	buf := make([]byte, 1024*1024) // 1MB buffer
	stackLen := runtime.Stack(buf, true)
	f.Write(buf[:stackLen])

	fmt.Fprintf(os.Stderr, "debug: goroutine stacks written to %s\n", filename)
}

// dumpFullProfile writes comprehensive runtime profiles to files
func dumpFullProfile() {
	if !atomic.CompareAndSwapInt32(&dumping, 0, 1) {
		fmt.Fprintln(os.Stderr, "debug: dump already in progress, skipping")
		return
	}
	defer atomic.StoreInt32(&dumping, 0)

	timestamp := time.Now().Format("20060102-150405")
	prefix := fmt.Sprintf("tasksamurai-%s", timestamp)
	if debugDir != "" {
		prefix = filepath.Join(debugDir, fmt.Sprintf("tasksamurai-%s", timestamp))
	}

	fmt.Fprintf(os.Stderr, "debug: starting full profile dump...\n")

	// Dump goroutines (text format)
	goroutineFile := prefix + "-goroutines.txt"
	if err := writeProfile(goroutineFile, "goroutine", 1); err != nil {
		fmt.Fprintf(os.Stderr, "debug: failed to write goroutine profile: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "debug: goroutine profile written to %s\n", goroutineFile)
	}

	// Dump heap profile
	heapFile := prefix + "-heap.pprof"
	if err := writeProfile(heapFile, "heap", 0); err != nil {
		fmt.Fprintf(os.Stderr, "debug: failed to write heap profile: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "debug: heap profile written to %s\n", heapFile)
	}

	// Dump block profile
	blockFile := prefix + "-block.pprof"
	runtime.SetBlockProfileRate(1)
	if err := writeProfile(blockFile, "block", 0); err != nil {
		fmt.Fprintf(os.Stderr, "debug: failed to write block profile: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "debug: block profile written to %s\n", blockFile)
	}

	// Dump CPU profile (5 second sample)
	cpuFile := prefix + "-cpu.pprof"
	if err := writeCPUProfile(cpuFile, 5*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "debug: failed to write CPU profile: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "debug: CPU profile written to %s\n", cpuFile)
	}

	fmt.Fprintf(os.Stderr, "debug: full profile dump complete\n")
}

// writeProfile writes a pprof profile to a file
func writeProfile(filename, profileName string, debug int) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add header for text format
	if debug > 0 {
		fmt.Fprintf(f, "TaskSamurai %s Profile\n", profileName)
		fmt.Fprintf(f, "Timestamp: %s\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(f, "\n%s\n\n", strings.Repeat("=", 80))
	}

	profile := pprof.Lookup(profileName)
	if profile == nil {
		return fmt.Errorf("profile %s not found", profileName)
	}

	return profile.WriteTo(f, debug)
}

// writeCPUProfile samples CPU usage and writes the profile
func writeCPUProfile(filename string, duration time.Duration) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		return err
	}

	time.Sleep(duration)
	pprof.StopCPUProfile()
	return nil
}
