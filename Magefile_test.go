//go:build mage
// +build mage

package main

import (
	"path/filepath"
	"testing"
)

func TestInstallBinDirUsesSingleGOPATH(t *testing.T) {
	goPath := t.TempDir()
	t.Setenv("GOPATH", goPath)

	got, err := installBinDir()
	if err != nil {
		t.Fatalf("installBinDir() error = %v", err)
	}

	want := filepath.Join(goPath, "bin")
	if got != want {
		t.Fatalf("installBinDir() = %q, want %q", got, want)
	}
}

func TestInstallBinDirUsesFirstGOPATHEntry(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	t.Setenv("GOPATH", first+string(filepath.ListSeparator)+second)

	got, err := installBinDir()
	if err != nil {
		t.Fatalf("installBinDir() error = %v", err)
	}

	want := filepath.Join(first, "bin")
	if got != want {
		t.Fatalf("installBinDir() = %q, want %q", got, want)
	}
}
