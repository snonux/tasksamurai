package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

// Any unexpected API call panics through the nil embedded interface.
type browserOnlyJiraSource struct{ IssueSource }

func TestJiraOpenURLHotkey(t *testing.T) {
	for _, mode := range []string{"table", "detail", "ultra"} {
		for _, description := range []string{"issue summary", "see https://youtu.be/other", "open @notes.txt"} {
			t.Run(mode+"/"+description, func(t *testing.T) {
				dir := t.TempDir()
				logPath := filepath.Join(dir, "browser.log")
				browserPath := filepath.Join(dir, "browser")
				t.Setenv("JIRA_BROWSER_LOG", logPath)
				if err := os.WriteFile(browserPath, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$JIRA_BROWSER_LOG.tmp\"\nmv \"$JIRA_BROWSER_LOG.tmp\" \"$JIRA_BROWSER_LOG\"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				m := newJiraTestModel(t, &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "https://example.com/decoy", Status: "pending"}}}, browserOnlyJiraSource{})
				m.browserCmd = browserPath
				m.youtubeBrowserCmd = filepath.Join(dir, "must-not-run")
				m.jira.settings.Site = "https://example.atlassian.net/"
				m.jira.tasks = []task.Task{{UUID: "jira:TST-42", IDText: "wrong-display-key", Description: description, Status: "pending", Annotations: []task.Annotation{{Description: "https://example.com/annotation @other.txt"}}}}
				m.rebuildJiraRows()
				selected := m.taskByUUID("jira:TST-42")
				if selected == nil || !m.selectTaskByID(selected.ID) {
					t.Fatal("Jira issue unavailable for selection")
				}
				switch mode {
				case "detail":
					m.showTaskDetail = true
					m.setCurrentTaskDetail(selected)
					m.tbl.SetCursor(m.taskIndexByID(1))
				case "ultra":
					m.showUltra = true
					m.ultraCursor = m.ultraTaskIndexByID(selected.ID)
					m.tbl.SetCursor(m.taskIndexByID(1))
				}
				_, cmd := m.Update(jiraKeyPress("o"))
				if cmd == nil {
					t.Fatal("missing browser command")
				}
				done, ok := cmd().(openURLDoneMsg)
				if !ok || done.err != nil || done.taskID != selected.ID {
					t.Fatalf("browser result: %+v", done)
				}
				if !waitForFile(logPath, 2*time.Second) {
					t.Fatal("browser did not write URL")
				}
				data, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := strings.TrimSpace(string(data)), "https://example.atlassian.net/browse/TST-42"; got != want {
					t.Fatalf("URL = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestJiraIssueURL(t *testing.T) {
	m := Model{}
	for _, tc := range []struct{ name, site, uuid, want string }{
		{"identity", "https://example.atlassian.net", "jira:TST-42", "https://example.atlassian.net/browse/TST-42"},
		{"escaped segment", "https://example.atlassian.net/", "jira:TST/42?#", "https://example.atlassian.net/browse/TST%2F42%3F%23"},
		{"TW display key", "https://example.atlassian.net", "tw-1", ""},
		{"missing site", "", "jira:TST-42", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.jira.settings.Site = tc.site
			if got := m.jiraIssueURL(&task.Task{UUID: tc.uuid, IDText: "TST-99"}); got != tc.want {
				t.Fatalf("URL = %q, want %q", got, tc.want)
			}
		})
	}
	if got := m.jiraIssueURL(nil); got != "" {
		t.Fatalf("nil task URL = %q", got)
	}
}
