//go:build integration

package jira

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/config"
)

// requireIntegration gates all live checks even with the build tag enabled.
// No implicit project is allowed, even when the build tag is enabled.
func requireIntegration(t *testing.T) (*Client, string) {
	t.Helper()
	if os.Getenv("JIRA_INTEGRATION") != "1" {
		t.Skip("set JIRA_INTEGRATION=1 to opt into live Jira checks")
	}
	project := strings.TrimSpace(os.Getenv("JIRA_TEST_PROJECT"))
	if !validIntegrationProject(project) {
		t.Fatal("JIRA_TEST_PROJECT is mandatory; use a dedicated test project")
	}
	path := os.Getenv("JIRA_TEST_TOKEN_FILE")
	if path == "" {
		path = "~/.jira_api_token"
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal("cannot resolve token home directory")
		}
		path = filepath.Join(home, path[2:])
	}
	settings := config.Jira{Enabled: true, Site: os.Getenv("JIRA_TEST_SITE"), Email: os.Getenv("JIRA_TEST_EMAIL"), TokenFile: path, MaxResults: 100, Timeout: 15 * time.Second}
	c, err := New(context.Background(), settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, project
}

// TestIntegrationPrerequisites is read-only and is also used by mage jiracheck.
func TestIntegrationPrerequisites(t *testing.T) {
	c, project := requireIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var account struct {
		AccountID string `json:"accountId"`
	}
	if err := c.request(ctx, http.MethodGet, "/rest/api/3/myself", nil, &account, true); err != nil {
		t.Fatal(err)
	}
	if account.AccountID == "" {
		t.Fatal("authenticated account response has no accountId")
	}
	projects := []string{project}
	if second := os.Getenv("JIRA_TEST_SECOND_PROJECT"); second != "" {
		if !validIntegrationProject(second) || second == project {
			t.Fatal("second project must be a distinct valid project key")
		}
		projects = append(projects, second)
	}
	for _, project := range projects {
		var found struct {
			Key string `json:"key"`
		}
		if err := c.request(ctx, http.MethodGet, "/rest/api/3/project/"+url.PathEscape(project), nil, &found, true); err != nil {
			t.Fatal(err)
		}
		if found.Key != project {
			t.Fatal("test project response does not match configured project")
		}
		types, err := c.IssueTypes(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		usable := false
		for _, value := range types {
			if value.ID != "" && !value.Subtask {
				usable = true
			}
		}
		if !usable {
			t.Fatal("test project has no creatable non-subtask issue type")
		}
	}
	t.Log("Read-only prerequisites passed; full suite creates uniquely marked issues, searches, comments, transitions, edits and cleans up only this run")
}
