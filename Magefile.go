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

// Default is the target Mage runs when no target is specified.
var Default = Build

// Build compiles the tasksamurai binary.
func Build() error {
	fmt.Println("Building tasksamurai...")
	if err := sh.RunV("go", "build", "-buildvcs=false", "-o", binaryName, "./cmd/tasksamurai"); err != nil {
		return fmt.Errorf("build %s: %w", binaryName, err)
	}
	return nil
}

// Run builds and starts tasksamurai.
func Run() error {
	mg.Deps(Build)

	fmt.Println("Running tasksamurai...")
	if err := sh.RunV("./" + binaryName); err != nil {
		return fmt.Errorf("run %s: %w", binaryName, err)
	}
	return nil
}

// Test runs all tests.
func Test() error {
	fmt.Println("Running tests...")
	return runTests()
}

// Jiratest runs the explicitly opted-in live Jira integration tests.
func Jiratest() error {
	if os.Getenv("JIRA_INTEGRATION") != "1" {
		return fmt.Errorf("set JIRA_INTEGRATION=1 to opt into live Jira tests")
	}
	if strings.TrimSpace(os.Getenv("JIRA_TEST_SECOND_PROJECT")) == "" || os.Getenv("JIRA_TEST_SECOND_PROJECT") == os.Getenv("JIRA_TEST_PROJECT") {
		return fmt.Errorf("JIRA_TEST_SECOND_PROJECT must name a distinct second test project")
	}
	if err := jiraPrerequisites(); err != nil {
		return err
	}
	return sh.RunV("go", "test", "-tags", "integration", "-count=1", "-v", "./internal/jira/...")
}

// Jiracheck checks live Jira prerequisites without creating or modifying issues.
func Jiracheck() error {
	if err := jiraPrerequisites(); err != nil {
		return err
	}
	return sh.RunWithV(map[string]string{"JIRA_INTEGRATION": "1"}, "go", "test", "-tags", "integration", "-count=1", "-v", "./internal/jira/...", "-run", "^TestIntegrationPrerequisites$")
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
	binDir, err := installBinDir()
	if err != nil {
		return err
	}
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

func installBinDir() (string, error) {
	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		goPath = filepath.Join(home, "go")
	} else {
		goPath = filepath.SplitList(goPath)[0]
		if goPath == "" {
			return "", fmt.Errorf("resolve GOPATH: first path entry is empty")
		}
	}

	return filepath.Join(goPath, "bin"), nil
}

func runTests() error {
	if err := sh.RunV("go", "test", "./..."); err != nil {
		return fmt.Errorf("test packages: %w", err)
	}
	if err := sh.RunV("go", "test", "-tags", "mage", "."); err != nil {
		return fmt.Errorf("test Magefile: %w", err)
	}
	return nil
}

func jiraPrerequisites() error {
	for _, name := range []string{"JIRA_TEST_SITE", "JIRA_TEST_EMAIL", "JIRA_TEST_PROJECT"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return fmt.Errorf("%s must be set for Jira integration checks", name)
		}
	}
	path := os.Getenv("JIRA_TEST_TOKEN_FILE")
	if path == "" {
		path = "~/.jira_api_token"
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve token home: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Jira token file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return fmt.Errorf("Jira token file is empty")
	}
	return nil
}
