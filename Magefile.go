//go:build mage
// +build mage

// Mage targets for TaskSamurai.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const binaryName = "tasksamurai"

// Default builds the tasksamurai binary.
func Default() {
	mg.Deps(Build)
}

// Build compiles the tasksamurai binary.
func Build() error {
	fmt.Println("Building tasksamurai...")
	if err := sh.RunV("go", "build", "-o", binaryName, "./cmd/tasksamurai"); err != nil {
		return fmt.Errorf("build %s: %w", binaryName, err)
	}
	return nil
}

// Run builds and starts tasksamurai with any provided arguments.
func Run() error {
	mg.Deps(Build)

	fmt.Println("Running tasksamurai...")
	if err := sh.RunV("./"+binaryName, targetArgs("run")...); err != nil {
		return fmt.Errorf("run %s: %w", binaryName, err)
	}
	return nil
}

// Test runs all tests.
func Test() error {
	fmt.Println("Running tests...")
	if err := sh.RunV("go", "test", "./..."); err != nil {
		return fmt.Errorf("test: %w", err)
	}
	return nil
}

// Verify runs formatting and static checks before tests.
func Verify() error {
	fmt.Println("Checking gofmt...")
	out, err := sh.Output("gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("check gofmt: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("gofmt needed for:\n%s", out)
	}

	fmt.Println("Running errcheck...")
	if err := sh.RunV("errcheck", "./..."); err != nil {
		return fmt.Errorf("errcheck: %w", err)
	}

	mg.Deps(Test)
	return nil
}

// Install builds and installs tasksamurai to $GOPATH/bin.
func Install() error {
	mg.Deps(Build)

	fmt.Println("Installing tasksamurai...")
	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		goPath = filepath.Join(home, "go")
	}

	binDir := filepath.Join(goPath, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create install directory %s: %w", binDir, err)
	}

	installPath := filepath.Join(binDir, binaryName)
	if err := sh.RunV("cp", "-v", binaryName, installPath); err != nil {
		return fmt.Errorf("install %s to %s: %w", binaryName, installPath, err)
	}
	return nil
}

// Clean removes the built binary.
func Clean() error {
	fmt.Println("Cleaning...")
	if err := sh.Rm(binaryName); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", binaryName, err)
		}
	}
	return nil
}

func targetArgs(target string) []string {
	for i, arg := range os.Args[1:] {
		if !strings.EqualFold(arg, target) {
			continue
		}

		args := os.Args[i+2:]
		if len(args) > 0 && args[0] == "--" {
			return args[1:]
		}
		return args
	}
	return nil
}
