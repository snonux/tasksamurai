// +build !windows

package debug

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetDebugDir(t *testing.T) {
	testDir := "/tmp/test-debug"
	SetDebugDir(testDir)
	if debugDir != testDir {
		t.Errorf("SetDebugDir failed: expected %s, got %s", testDir, debugDir)
	}
}

func TestInitSignalHandlers(t *testing.T) {
	// This test just verifies InitSignalHandlers doesn't panic
	// We can't easily test actual signal handling without more complex setup
	InitSignalHandlers()
}

func TestDumpGoroutines(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	SetDebugDir(tmpDir)

	// Call dumpGoroutines
	dumpGoroutines()

	// Check that a file was created
	files, err := filepath.Glob(filepath.Join(tmpDir, "tasksamurai-goroutines-*.txt"))
	if err != nil {
		t.Fatalf("failed to glob for goroutine files: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("no goroutine dump file was created")
	}

	// Verify the file is not empty
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("failed to stat goroutine dump file: %v", err)
	}

	if info.Size() == 0 {
		t.Error("goroutine dump file is empty")
	}

	t.Logf("Goroutine dump created: %s (%d bytes)", files[0], info.Size())
}

func TestWriteProfile(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Test goroutine profile (text format)
	goroutineFile := filepath.Join(tmpDir, "test-goroutine.txt")
	if err := writeProfile(goroutineFile, "goroutine", 1); err != nil {
		t.Errorf("failed to write goroutine profile: %v", err)
	}

	// Test heap profile (binary format)
	heapFile := filepath.Join(tmpDir, "test-heap.pprof")
	if err := writeProfile(heapFile, "heap", 0); err != nil {
		t.Errorf("failed to write heap profile: %v", err)
	}

	// Verify files exist and are not empty
	for _, file := range []string{goroutineFile, heapFile} {
		info, err := os.Stat(file)
		if err != nil {
			t.Errorf("profile file does not exist: %s", file)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("profile file is empty: %s", file)
		}
		t.Logf("Profile created: %s (%d bytes)", file, info.Size())
	}
}
