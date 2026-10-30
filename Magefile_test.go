//go:build mage
// +build mage

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJiraPrerequisitesAreExplicit(t *testing.T) {
	for _, name := range []string{"JIRA_TEST_SITE", "JIRA_TEST_EMAIL", "JIRA_TEST_PROJECT", "JIRA_TEST_TOKEN_FILE", "JIRA_INTEGRATION"} {
		t.Setenv(name, "")
	}
	if err := Jiratest(); err == nil || !strings.Contains(err.Error(), "JIRA_INTEGRATION") {
		t.Fatalf("missing opt-in: %v", err)
	}
	if err := Jiracheck(); err == nil || !strings.Contains(err.Error(), "JIRA_TEST_SITE") {
		t.Fatalf("missing site: %v", err)
	}
	t.Setenv("JIRA_TEST_SITE", "https://example.atlassian.net")
	t.Setenv("JIRA_TEST_EMAIL", "person@example.com")
	if err := jiraPrerequisites(); err == nil || !strings.Contains(err.Error(), "JIRA_TEST_PROJECT") {
		t.Fatalf("missing project: %v", err)
	}
	t.Setenv("JIRA_TEST_PROJECT", "TST")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := jiraPrerequisites(); err == nil {
		t.Fatal("missing default token accepted")
	}
	path := filepath.Join(home, ".jira_api_token")
	if err := os.WriteFile(path, []byte(" \n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := jiraPrerequisites(); err == nil {
		t.Fatal("empty token accepted")
	}
	if err := os.WriteFile(path, []byte("fixture=SUFFIX\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := jiraPrerequisites(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JIRA_TEST_TOKEN_FILE", filepath.Join(home, "absent"))
	if err := jiraPrerequisites(); err == nil {
		t.Fatal("missing explicit token accepted")
	}
}

func TestJiraLiveSuiteRequiresTwoProjects(t *testing.T) {
	t.Setenv("JIRA_INTEGRATION", "1")
	t.Setenv("JIRA_TEST_PROJECT", "TST")
	for _, second := range []string{"", "TST"} {
		t.Setenv("JIRA_TEST_SECOND_PROJECT", second)
		if err := Jiratest(); err == nil || !strings.Contains(err.Error(), "JIRA_TEST_SECOND_PROJECT") {
			t.Fatalf("unsafe second-project setup accepted: %v", err)
		}
	}
}

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
