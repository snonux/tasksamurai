//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/sh"
)

// Default target builds the tasksamurai binary
func Default() error {
	return Build()
}

// Build compiles the tasksamurai binary
func Build() error {
	fmt.Println("Building tasksamurai...")
	return sh.Run("go", "build", "-o", "tasksamurai", "./cmd/tasksamurai")
}

// Run starts tasksamurai with any provided arguments
func Run() error {
	fmt.Println("Running tasksamurai...")
	args := append([]string{"run", "./cmd/tasksamurai"}, os.Args[1:]...)
	return sh.Run("go", args...)
}

// Test runs all tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.Run("go", "test", "./...")
}

// Verify runs formatting and static checks before tests.
func Verify() error {
	fmt.Println("Checking gofmt...")
	out, err := sh.Output("gofmt", "-l", ".")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("gofmt needed for:\n%s", out)
	}

	fmt.Println("Running errcheck...")
	if err := sh.Run("errcheck", "./..."); err != nil {
		return err
	}

	return Test()
}

// Install installs tasksamurai to $GOPATH/bin
func Install() error {
	fmt.Println("Installing tasksamurai...")
	return sh.Run("go", "install", "./cmd/tasksamurai")
}

// Clean removes the built binary
func Clean() error {
	fmt.Println("Cleaning...")
	if err := sh.Rm("tasksamurai"); err != nil {
		// Ignore error if file doesn't exist
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
