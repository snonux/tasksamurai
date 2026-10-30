//go:build integration

package jira

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestIntegrationWorklogsAcrossProjects(t *testing.T) {
	c, primary := requireIntegration(t)
	second := os.Getenv("JIRA_TEST_SECOND_PROJECT")
	if !validIntegrationProject(second) || second == primary {
		t.Fatal("distinct JIRA_TEST_SECOND_PROJECT required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	run := &integrationRun{client: c, marker: "ts-integration-" + strconv.FormatInt(time.Now().UnixNano(), 10), projects: []string{primary, second}}
	for _, project := range run.projects {
		typeID, err := integrationIssueType(ctx, c, project)
		if err != nil {
			t.Fatal(err)
		}
		f := run.create(ctx, t, project, typeID, "worklog fixture")
		before, err := run.guard(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		const note = "TaskSamurai worklog verification 日本語"
		created, err := c.AddWorklog(ctx, f.key, time.Now().Add(-time.Minute), 60, note)
		if err != nil {
			t.Fatal(err)
		}
		after, err := run.guard(ctx, f)
		if err != nil || after.JiraTimeSpent != before.JiraTimeSpent+60 {
			t.Fatalf("work total before %d after %d: %v", before.JiraTimeSpent, after.JiraTimeSpent, err)
		}
		var logged struct {
			Worklog
			Comment adfNode `json:"comment"`
		}
		path, _ := issuePath(f.key) // key was validated by the exact ownership fence
		if err := c.request(ctx, http.MethodGet, path+"/worklog/"+url.PathEscape(created.ID), nil, &logged, true); err != nil {
			t.Fatal(err)
		}
		if logged.ID != created.ID || logged.TimeSpentSeconds != 60 || adfText(logged.Comment) != note+"\n" {
			t.Fatalf("worklog readback mismatch for %s", f.key)
		}
		t.Logf("Confirmed %s: 60s worklog %s, total %d seconds; marker %s", f.key, created.ID, after.JiraTimeSpent, run.marker)
	}
}
