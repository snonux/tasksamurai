package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

func TestIssueMapping(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) { t.Error("mapping must not make a request") })
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct{ category, status, start, end string }{
		{"new", "pending", "", ""},
		{"indeterminate", "pending", "20261029T103000Z", ""},
		{"done", "completed", "", "20261028T100000Z"},
		{"undefined", "pending", "", ""},
		{"custom", "pending", "", ""},
	} {
		t.Run(tt.category, func(t *testing.T) {
			var value issue
			fixture := `{"key":"TST-42","fields":{"summary":"Review 日本語","project":{"key":"TST"},"priority":{"name":"High"},"status":{"name":"Ignore workflow name","statusCategory":{"key":"` + tt.category + `"}},"duedate":"2026-10-29","created":"2026-10-01T10:00:00.000+0200","updated":"2026-10-29T12:30:00.000+0200","resolutiondate":"2026-10-28T10:00:00Z","assignee":{"displayName":"Person"},"labels":["work","jira"]}}`
			if err := json.Unmarshal([]byte(fixture), &value); err != nil {
				t.Fatal(err)
			}
			got := c.mapIssue(value, now)
			if got.ID != 0 || got.IDText != "TST-42" || got.UUID != "jira:TST-42" || got.Description != "Review 日本語" || got.Project != "TST" || got.Priority != "H" || got.Assignee != "Person" || got.Entry != "20261001T080000Z" || got.Due != "20261029T000000Z" || got.Status != tt.status || got.Start != tt.start || got.End != tt.end || strings.Join(got.Tags, ",") != "jira,work" || got.Urgency <= 6 {
				t.Fatalf("incorrect mapping: %+v", got)
			}
		})
	}
	var minimal issue
	if err := json.Unmarshal([]byte(`{"key":"TST-1","fields":{"priority":null,"assignee":null,"duedate":"nonsense","status":null}}`), &minimal); err != nil {
		t.Fatal(err)
	}
	got := c.mapIssue(minimal, now)
	if got.Status != "pending" || got.Priority != "M" || got.Due != "" || got.Entry != "" || got.Start != "" || got.Urgency != 3 {
		t.Fatalf("minimal mapping: %+v", got)
	}
	minimal.Fields.Status.Category.Key = "done"
	minimal.Fields.Updated = "2026-10-30T00:00:00Z"
	if got := c.mapIssue(minimal, now); got.End != "20261030T000000Z" {
		t.Fatalf("End fallback: %+v", got)
	}
	for _, tt := range []struct{ name, want string }{{"Highest", "H"}, {"H", "H"}, {"Low", "L"}, {"Lowest", "L"}, {"Medium", "M"}, {"unknown", "M"}} {
		if got := mapPriority(tt.name); got != tt.want {
			t.Errorf("priority %q=%s", tt.name, got)
		}
	}
}

func TestFetchIssueCreateAndMetadata(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/createmeta/"):
			if r.URL.Query().Get("startAt") == "0" {
				reply(t, w, map[string]any{"issueTypes": []any{map[string]any{"id": "1", "name": "Task"}}, "total": 2})
			} else {
				reply(t, w, map[string]any{"issueTypes": []any{map[string]any{"id": "2", "name": "Subtask", "subtask": true}}, "total": 2})
			}
		case r.Method == "GET":
			if !strings.Contains(r.URL.Query().Get("fields"), "resolutiondate") || r.URL.EscapedPath() != "/rest/api/3/issue/TST%2F42" {
				t.Errorf("issue fetch path/fields: %s", r.URL)
			}
			reply(t, w, map[string]any{"key": "TST-42", "fields": map[string]any{"summary": "fresh"}})
		case r.Method == "POST":
			if r.URL.Path != "/rest/api/3/issue" {
				t.Error("wrong create endpoint")
			}
			var body struct {
				Fields struct {
					Summary string            `json:"summary"`
					Project map[string]string `json:"project"`
					Type    map[string]string `json:"issuetype"`
				} `json:"fields"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Fields.Summary != "plain summary" || body.Fields.Project["key"] != "TST" || body.Fields.Type["id"] != "1" {
				t.Errorf("incorrect create: %+v", body)
			}
			w.WriteHeader(201)
			reply(t, w, map[string]any{"key": "TST-99"})
		}
	})
	got, err := c.FetchIssue(context.Background(), "TST/42")
	if err != nil || got.Description != "fresh" {
		t.Fatalf("fetch issue %+v %v", got, err)
	}
	key, err := c.CreateIssue(context.Background(), "TST", "1", "plain summary")
	if err != nil || key != "TST-99" {
		t.Fatalf("create %s %v", key, err)
	}
	types, err := c.IssueTypes(context.Background(), "TST")
	if err != nil || len(types) != 2 || types[0].ID != "1" || !types[1].Subtask {
		t.Fatalf("metadata %+v %v", types, err)
	}
	before := calls.Load()
	if _, err := c.FetchIssue(context.Background(), ""); err == nil {
		t.Fatal("empty issue key accepted")
	}
	if _, err := c.CreateIssue(context.Background(), "TST", "1", " \n"); err == nil {
		t.Fatal("empty summary accepted")
	}
	if _, err := c.IssueTypes(context.Background(), ""); err == nil {
		t.Fatal("empty project accepted")
	}
	if calls.Load() != before {
		t.Fatal("invalid input caused API call")
	}
}

func TestMalformedIssueAndCreateResponses(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `<html>bad</html>`} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		if _, err := c.FetchIssue(context.Background(), "TST-1"); err == nil {
			t.Fatalf("fetch accepted %s", body)
		}
		if _, err := c.CreateIssue(context.Background(), "TST", "1", "summary"); err == nil {
			t.Fatalf("create accepted %s", body)
		}
	}
}

func TestUrgencyDueContribution(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		days int
		want float64
	}{{-30, 18}, {-7, 18}, {0, 14}, {14, 6}, {30, 6}} {
		got := urgency(task.Task{Priority: "H", Due: now.AddDate(0, 0, tt.days).Format(task.DateFormat)}, now)
		if got != tt.want {
			t.Errorf("due %+d days: got %v, want %v", tt.days, got, tt.want)
		}
	}
	if got := urgency(task.Task{Priority: "L"}, now); got != 1 {
		t.Errorf("no due urgency %v", got)
	}
}
