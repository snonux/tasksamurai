package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
)

func TestJiraFieldUIHTTPRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		key, field string
		value      any
	}{
		{"e", "summary", "updated summary"}, {"w", "duedate", "2026-12-25"}, {"W", "duedate", nil},
		{"p", "priority", map[string]any{"name": "High"}}, {"t", "labels", []any{"keep", "jira", "new"}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			searches, gets, puts := 0, 0, 0
			fields := map[string]any{"summary": "initial summary", "labels": []string{"keep", "jira"}, "status": map[string]any{"statusCategory": map[string]string{"key": "new"}}}
			issue := map[string]any{"key": "TST-42", "fields": fields}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
					searches++
					if err := json.NewEncoder(w).Encode(map[string]any{"issues": []any{issue}, "isLast": true}); err != nil {
						t.Error(err)
					}
				case r.Method == http.MethodPut && r.URL.Path == "/rest/api/3/issue/TST-42":
					puts++
					var body struct {
						Fields map[string]any `json:"fields"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if !reflect.DeepEqual(body.Fields, map[string]any{tc.field: tc.value}) {
						t.Errorf("payload %v", body.Fields)
					}
					fields[tc.field] = body.Fields[tc.field]
					w.WriteHeader(http.StatusNoContent)
				case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/TST-42":
					gets++
					if err := json.NewEncoder(w).Encode(issue); err != nil {
						t.Error(err)
					}
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			token := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(token, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			client, err := jira.New(context.Background(), config.Jira{Enabled: true, Site: server.URL, Email: "test@example.com", TokenFile: token, Timeout: time.Second, MaxResults: 100}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			tw := &routingTaskwarrior{}
			m := newJiraTestModel(t, tw, client)
			drainCmds(t, &m, m.Init())
			m.blinkEnabled = false
			_, cmd := m.Update(routingKey(tc.key))
			switch tc.key {
			case "e":
				m.descInput.SetValue("updated summary")
				_, cmd = m.Update(routingKey("enter"))
			case "w":
				m.dueDate = time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
				_, cmd = m.Update(routingKey("enter"))
			case "p":
				m.priorityIndex = 0
				_, cmd = m.Update(routingKey("enter"))
			case "t":
				m.tagsInput.SetValue("+new")
				_, cmd = m.Update(routingKey("enter"))
			}
			if cmd == nil {
				t.Fatal("no field write scheduled")
			}
			drainCmds(t, &m, cmd)
			if searches != 1 || puts != 1 || gets != 1 || len(tw.calls) != 0 {
				t.Fatalf("searches %d PUTs %d GETs %d TW %v", searches, puts, gets, tw.calls)
			}
			cached := m.jiraCachedTask("jira:TST-42")
			if cached == nil || cached.IDText != "TST-42" {
				t.Fatal("lost cached identity")
			}
			switch tc.key {
			case "e":
				if cached.Description != "updated summary" {
					t.Fatal("summary not refreshed")
				}
			case "w":
				if cached.Due != "20261225T000000Z" {
					t.Fatal("due not mapped UTC midnight")
				}
			case "W":
				if cached.Due != "" {
					t.Fatal("due not cleared")
				}
			case "p":
				if cached.Priority != "H" {
					t.Fatal("priority not refreshed")
				}
			case "t":
				if !reflect.DeepEqual(cached.JiraLabels, []string{"keep", "jira", "new"}) {
					t.Fatalf("source labels %v", cached.JiraLabels)
				}
			}
		})
	}
}
