//go:build integration

package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

type integrationFixture struct{ project, key string }
type integrationRun struct {
	client   *Client
	marker   string
	projects []string
}
type liveSearchCounter struct {
	next  http.RoundTripper
	calls atomic.Int32
}

func (c *liveSearchCounter) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql" {
		c.calls.Add(1)
	}
	return c.next.RoundTrip(r)
}

func (r *integrationRun) guard(ctx context.Context, f integrationFixture) (task.Task, error) {
	if !slices.Contains(r.projects, f.project) || !strings.HasPrefix(f.key, f.project+"-") {
		return task.Task{}, errors.New("fixture project is outside this integration run")
	}
	value, err := r.client.FetchIssue(ctx, f.key)
	if err != nil {
		return task.Task{}, err
	}
	return value, ownedIntegrationIssue(value, f.project, r.marker)
}

func (r *integrationRun) create(ctx context.Context, t *testing.T, project, typeID, label string) integrationFixture {
	t.Helper()
	if !slices.Contains(r.projects, project) || !validIntegrationProject(project) {
		t.Fatal("unsafe integration create project")
	}
	key, err := r.client.CreateIssue(ctx, project, typeID, r.marker+" "+label)
	if err != nil {
		t.Fatal(err)
	}
	f := integrationFixture{project, key}
	// Register cleanup before validating the response, then refuse cleanup if
	// the fresh issue does not pass the exact project/run fence.
	t.Cleanup(func() { r.cleanup(t, f) })
	if _, err := r.guard(ctx, f); err != nil {
		t.Fatal(err)
	}
	return f
}

func (r *integrationRun) field(ctx context.Context, f integrationFixture, field string, value any) error {
	if _, err := r.guard(ctx, f); err != nil {
		return err
	}
	return r.client.SetField(ctx, f.key, field, value)
}

func (r *integrationRun) comment(ctx context.Context, f integrationFixture, text string) (task.Annotation, error) {
	if _, err := r.guard(ctx, f); err != nil {
		return task.Annotation{}, err
	}
	return r.client.AddComment(ctx, f.key, text)
}

func (r *integrationRun) transition(ctx context.Context, f integrationFixture, category string) (task.Task, error) {
	if _, err := r.guard(ctx, f); err != nil {
		return task.Task{}, err
	}
	available, err := r.client.Transitions(ctx, f.key)
	if err != nil {
		return task.Task{}, err
	}
	selected, fields, err := ChooseTransition(available, category, "", nil)
	if err != nil {
		return task.Task{}, err
	}
	if _, err := r.guard(ctx, f); err != nil {
		return task.Task{}, err
	}
	if err := r.client.DoTransition(ctx, f.key, selected.ID, fields); err != nil {
		return task.Task{}, err
	}
	return r.guard(ctx, f)
}

func (r *integrationRun) cleanup(t *testing.T, f integrationFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	value, err := r.guard(ctx, f)
	if isHTTPStatus(err, 404) {
		return
	}
	if err != nil {
		t.Logf("Cleanup refused for %s: %v", f.key, err)
		return
	}
	err = r.client.request(ctx, http.MethodDelete, "/rest/api/3/issue/"+url.PathEscape(f.key), nil, nil, false)
	if err == nil || isHTTPStatus(err, 404) {
		t.Logf("Cleaned up owned issue %s", f.key)
		return
	}
	t.Logf("Cleanup delete failed for %s: %v; attempting done fallback", f.key, err)
	if value.Status != "completed" {
		if _, fallbackErr := r.transition(ctx, f, "done"); fallbackErr != nil {
			t.Logf("Cleanup fallback failed for %s: %v", f.key, fallbackErr)
		}
	}
}

func (r *integrationRun) jql() string {
	keys := make([]string, len(r.projects))
	for i, key := range r.projects {
		keys[i] = strconv.Quote(key)
	}
	// Exact Lucene phrase plus project restriction; ownership is rechecked
	// locally and again with a fresh issue GET before every mutation.
	return "project in (" + strings.Join(keys, ",") + ") AND summary ~ " + strconv.Quote("\""+r.marker+"\"") + " ORDER BY key ASC"
}

func (r *integrationRun) indexed(ctx context.Context, count int) ([]task.Task, error) {
	for {
		values, err := r.client.FetchIssues(ctx, r.jql())
		if err != nil {
			return nil, err
		}
		var owned []task.Task
		for _, value := range values {
			if ownedIntegrationIssue(value, value.Project, r.marker) == nil && slices.Contains(r.projects, value.Project) {
				owned = append(owned, value)
			}
		}
		if len(owned) >= count {
			return owned, nil
		}
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("waiting for Jira search indexing: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func integrationIssueType(ctx context.Context, c *Client, project string) (string, error) {
	values, err := c.IssueTypes(ctx, project)
	if err != nil {
		return "", err
	}
	wanted := os.Getenv("JIRA_TEST_ISSUE_TYPE_ID")
	for _, value := range values {
		if !value.Subtask && value.ID != "" && ((wanted != "" && value.ID == wanted) || (wanted == "" && value.Name == "Task")) {
			return value.ID, nil
		}
	}
	if wanted == "" {
		for _, value := range values {
			if !value.Subtask && value.ID != "" {
				return value.ID, nil
			}
		}
	}
	return "", errors.New("test project has no matching creatable issue type")
}

func isHTTPStatus(err error, status int) bool {
	var api *APIError
	return errors.As(err, &api) && api.StatusCode == status
}

func TestIntegrationLifecycleAcrossProjects(t *testing.T) {
	c, primary := requireIntegration(t)
	second := os.Getenv("JIRA_TEST_SECOND_PROJECT")
	if !validIntegrationProject(second) || second == primary {
		t.Fatal("JIRA_TEST_SECOND_PROJECT must name a distinct dedicated test project")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	run := &integrationRun{client: c, marker: "ts-integration-" + strconv.FormatInt(time.Now().UnixNano(), 10), projects: []string{primary, second}}
	fixtures := make([]integrationFixture, 0, 4)
	for _, project := range run.projects {
		typeID, err := integrationIssueType(ctx, c, project)
		if err != nil {
			t.Fatal(err)
		}
		for n := 0; n < 2; n++ {
			fixtures = append(fixtures, run.create(ctx, t, project, typeID, fmt.Sprintf("fixture-%s-%d", project, n)))
		}
	}
	t.Logf("Created %d fixtures in %s; exact run marker %s", len(fixtures), strings.Join(run.projects, ","), run.marker)
	indexCtx, indexCancel := context.WithTimeout(ctx, 60*time.Second)
	values, err := run.indexed(indexCtx, len(fixtures))
	indexCancel()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	projects := map[string]bool{}
	for _, value := range values {
		if value.UUID != "jira:"+value.IDText || !strings.HasPrefix(value.IDText, value.Project+"-") || value.Urgency <= 0 || value.Status != "pending" {
			t.Fatalf("incorrect mapped integration row %+v", value)
		}
		keys[value.IDText] = true
		projects[value.Project] = true
	}
	if len(keys) != len(fixtures) || len(projects) != 2 {
		t.Fatal("cross-project keys collided or source project was lost")
	}
	// Use real server pages of one issue while keeping the disposable volume low.
	next := c.http.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	counter := &liveSearchCounter{next: next}
	c.http.Transport = counter
	c.searchPageSize = 1
	values, err = c.FetchIssues(ctx, run.jql())
	if err != nil || len(values) != len(fixtures) || counter.calls.Load() < int32(len(fixtures)) {
		t.Fatalf("real pagination returned %d rows in %d requests: %v", len(values), counter.calls.Load(), err)
	}
	c.searchPageSize = 100
	for _, fixture := range []integrationFixture{fixtures[0], fixtures[2]} {
		t.Run(fixture.project, func(t *testing.T) { run.lifecycle(ctx, t, fixture) })
	}
	t.Run("negative paths", func(t *testing.T) { run.negatives(ctx, t, fixtures[0]) })
}

func (r *integrationRun) lifecycle(ctx context.Context, t *testing.T, f integrationFixture) {
	t.Helper()
	value, err := r.guard(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != "pending" || value.Start != "" {
		t.Fatal("created fixture was not in a todo category")
	}
	summary := r.marker + " updated summary " + f.project
	if err := r.field(ctx, f, "summary", summary); err != nil {
		t.Fatal(err)
	}
	if value, err = r.guard(ctx, f); err != nil || value.Description != summary || value.IDText != f.key {
		t.Fatalf("summary/key mapping failed: %v", err)
	}
	if err := r.field(ctx, f, "priority", map[string]string{"name": "High"}); err != nil {
		t.Fatal(err)
	}
	if value, err = r.guard(ctx, f); err != nil || value.Priority != "H" {
		t.Fatalf("priority mapping failed: %v", err)
	}
	labels := []string{"jira", "integration", r.marker}
	if err := r.field(ctx, f, "labels", labels); err != nil {
		t.Fatal(err)
	}
	if value, err = r.guard(ctx, f); err != nil || !slices.Contains(value.JiraLabels, "jira") || len(value.JiraLabels) != 3 {
		t.Fatalf("source labels lost: %v", err)
	}
	due := time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02")
	if err := r.field(ctx, f, "duedate", due); err != nil {
		t.Fatal(err)
	}
	if value, err = r.guard(ctx, f); err != nil || value.Due != strings.ReplaceAll(due, "-", "")+"T000000Z" {
		t.Fatalf("due mapping failed: %v", err)
	}
	if err := r.field(ctx, f, "duedate", nil); err != nil {
		t.Fatal(err)
	}
	if value, err = r.guard(ctx, f); err != nil || value.Due != "" {
		t.Fatalf("due clear failed: %v", err)
	}
	text := "Integration comment 日本語 \"quotes\"\nSecond paragraph"
	annotation, err := r.comment(ctx, f, text)
	if err != nil || annotation.Description != text || annotation.Entry == "" {
		t.Fatalf("ADF comment failed: %v", err)
	}
	page, err := r.client.FetchComments(ctx, f.key)
	if err != nil || len(page.Annotations) != 1 || page.Annotations[0].Description != text || page.Remaining != 0 {
		t.Fatalf("comment fetch failed: %v", err)
	}
	for _, category := range []string{"indeterminate", "new", "indeterminate", "done"} {
		value, err = r.transition(ctx, f, category)
		if err != nil {
			t.Fatal(err)
		}
		if category == "done" && (value.Status != "completed" || value.End == "") {
			t.Fatal("done mapping failed")
		}
		if category == "indeterminate" && (value.Status != "pending" || value.Start == "") {
			t.Fatal("in-progress mapping failed")
		}
		if category == "new" && (value.Status != "pending" || value.Start != "") {
			t.Fatal("stop mapping failed")
		}
	}
	if _, err := r.comment(ctx, f, "Comment on resolved issue"); err != nil {
		t.Fatal(err)
	}
	page, err = r.client.FetchComments(ctx, f.key)
	if err != nil || len(page.Annotations) != 2 {
		t.Fatalf("resolved comments missing: %v", err)
	}
	t.Logf("Verified %s: fields, multiline comments, start/stop/done and resolved comment", f.key)
}

func (r *integrationRun) negatives(ctx context.Context, t *testing.T, f integrationFixture) {
	t.Helper()
	if _, err := r.comment(ctx, f, " \n\t"); err == nil {
		t.Fatal("blank comment accepted")
	}
	if err := r.field(ctx, f, "summary", " "); err == nil {
		t.Fatal("blank summary accepted")
	}
	if _, err := r.client.FetchIssue(ctx, f.project+"-2147483647"); !isHTTPStatus(err, 404) {
		t.Fatalf("missing issue did not yield404: %v", err)
	}
	if _, err := r.client.FetchIssues(ctx, "project = "+strconv.Quote(f.project)+" AND summary ~"); !isHTTPStatus(err, 400) {
		t.Fatalf("invalid bounded JQL did not yield400: %v", err)
	}
}

func TestIntegrationInvalidCredentials(t *testing.T) {
	c, _ := requireIntegration(t)
	badToken := filepath.Join(t.TempDir(), "invalid-token")
	if err := os.WriteFile(badToken, []byte("deliberately-invalid-integration-token=TEST"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := c.settings
	settings.TokenFile = badToken
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	bad, err := New(ctx, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Issue endpoints deliberately mask authentication failures as 404.
	// /myself requires authentication and is a faithful 401 oracle.
	var account map[string]any
	if err := bad.request(ctx, http.MethodGet, "/rest/api/3/myself", nil, &account, true); !isHTTPStatus(err, 401) {
		t.Fatalf("invalid credential did not yield401: %v", err)
	}
}

func TestIntegrationUnboundedJQLRestriction(t *testing.T) {
	c, _ := requireIntegration(t)
	if os.Getenv("JIRA_TEST_UNBOUNDED") != "1" {
		t.Skip("set JIRA_TEST_UNBOUNDED=1 for the site-specific read-only probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.FetchIssues(ctx, ""); !isHTTPStatus(err, 400) || !strings.Contains(err.Error(), "restricted filter") {
		t.Fatalf("unbounded site restriction changed: %v", err)
	}
}
