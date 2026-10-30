package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

func TestParseWorklogInput(t *testing.T) {
	for _, tc := range []struct {
		input   string
		seconds int64
		note    string
	}{
		{"1h 30m; review; notes", 5400, "review; notes"}, {"45m", 2700, ""}, {"1.5h; 日本語", 5400, "日本語"}, {"1s", 1, ""},
	} {
		seconds, note, err := parseWorklogInput(tc.input)
		if err != nil || seconds != tc.seconds || note != tc.note {
			t.Fatalf("%q = %d %q %v", tc.input, seconds, note, err)
		}
	}
	for _, input := range []string{"", " ; note", "1", "1d", "-1h", "0m", "1ms", "1.1s", "999999999999h"} {
		if _, _, err := parseWorklogInput(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestJiraWorklogUIHTTP(t *testing.T) {
	for _, mode := range []string{"table", "ultra", "detail"} {
		for _, outcome := range []string{"success", "write forbidden", "write unavailable", "refresh failure"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				posts, gets, searches := 0, 0, 0
				spent := int64(60)
				issue := func() map[string]any {
					return map[string]any{"key": "TST-42", "fields": map[string]any{"summary": "Issue", "timespent": spent, "status": map[string]any{"name": "In Review", "statusCategory": map[string]string{"key": "indeterminate"}}}}
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
						searches++
						if err := json.NewEncoder(w).Encode(map[string]any{"issues": []any{issue()}, "isLast": true}); err != nil {
							t.Error(err)
						}
					case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/TST-42/worklog":
						posts++
						var body struct {
							Seconds int64          `json:"timeSpentSeconds"`
							Started string         `json:"started"`
							Comment map[string]any `json:"comment"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						started, err := time.Parse("2006-01-02T15:04:05.000-0700", body.Started)
						if err != nil || time.Since(started) < 5390*time.Second || time.Since(started) > 5410*time.Second || body.Seconds != 5400 || body.Comment["type"] != "doc" || r.URL.Query().Get("adjustEstimate") != "leave" {
							t.Errorf("payload %+v", body)
						}
						if outcome == "write forbidden" {
							w.WriteHeader(403)
							return
						}
						if outcome == "write unavailable" {
							w.WriteHeader(500)
							return
						}
						spent += body.Seconds
						w.WriteHeader(201)
						if err := json.NewEncoder(w).Encode(map[string]any{"id": "123", "timeSpentSeconds": body.Seconds}); err != nil {
							t.Error(err)
						}
					case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/TST-42":
						gets++
						if outcome == "refresh failure" {
							w.WriteHeader(500)
							return
						}
						if err := json.NewEncoder(w).Encode(issue()); err != nil {
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
				_, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
				m.showUltra = mode == "ultra"
				if mode == "detail" {
					m.showTaskDetail = true
					m.setCurrentTaskDetail(&m.tasks[0])
					m.initDetailViewport()
				}
				activation := "ctrl+w"
				if mode == "table" {
					m.tbl.SetColumnCursor(m.logicalToDisplay(colAnnotations))
					activation = "i"
				}
				_, _ = m.Update(routingKey(activation))
				if !m.worklog.editing || !m.anyInputActive() {
					t.Fatal("work input did not open")
				}
				_, _ = m.Update(routingKey("esc"))
				if m.worklog.editing || posts != 0 {
					t.Fatal("cancel submitted")
				}
				_, _ = m.Update(routingKey("ctrl+w"))
				_, _ = m.Update(routingKey("enter"))
				if !m.worklog.editing || posts != 0 {
					t.Fatal("blank duration submitted")
				}
				m.statusMsg = ""
				const input = "1h 30m; work details 日本語"
				for _, r := range input {
					_, _ = m.Update(routingKey(string(r)))
				}
				if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Log work") || !strings.Contains(view, input) {
					t.Fatalf("input invisible: %s", view)
				}
				if posts != 0 || len(tw.calls) != 0 {
					t.Fatal("typing triggered mutation")
				}
				m.jira.commenting = true
				_, blocked := m.Update(routingKey("enter"))
				if blocked != nil || !m.worklog.editing || posts != 0 {
					t.Fatal("overlapping write allowed")
				}
				m.jira.commenting = false
				_, cmd := m.Update(routingKey("enter"))
				if cmd == nil || m.worklog.editing {
					t.Fatal("submission not scheduled")
				}
				drainCmds(t, &m, cmd)
				if posts != 1 || searches != 1 || len(tw.calls) != 0 || m.jira.mutating {
					t.Fatalf("posts %d searches %d TW %v", posts, searches, tw.calls)
				}
				cached := m.jiraCachedTask("jira:TST-42")
				switch outcome {
				case "success":
					if gets != 1 || cached.JiraTimeSpent != 5460 || !strings.Contains(m.statusMsg, "Logged 1h 30m") {
						t.Fatalf("refresh %d cached %+v status %s", gets, cached, m.statusMsg)
					}
					row := m.taskToRowSearch(*cached, nil, m.tblStyles, -1)
					if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(colAnnotations)])); got != "1h 31m" {
						t.Fatalf("logged cell %q", got)
					}
					for _, re := range []*regexp.Regexp{nil, regexp.MustCompile(`logged: 1h`)} {
						if ultra := ansi.Strip(m.renderUltraStatusWithRegex(*cached, 160, re, "")); !strings.Contains(ultra, "logged: 1h 31m") {
							t.Fatalf("logged total missing in ultra: %s", ultra)
						}
					}
					if title, _ := m.columnSpec(colAnnotations); title != "Ann / Logged" {
						t.Fatalf("mixed header %q", title)
					}
					m.jira.view = "jira"
					if title, _ := m.columnSpec(colAnnotations); title != "Logged" {
						t.Fatalf("Jira header %q", title)
					}
					m.jira.view = "tw"
					if title, _ := m.columnSpec(colAnnotations); title != "Annotations" {
						t.Fatalf("TW header %q", title)
					}
					m.showTaskDetail = true
					m.setCurrentTaskDetail(cached)
					if detail := ansi.Strip(m.renderTaskDetail()); !strings.Contains(detail, "Time spent:") || !strings.Contains(detail, "1h 31m") {
						t.Fatalf("total missing: %s", detail)
					}
				case "refresh failure":
					if gets != 2 || cached.JiraTimeSpent != 60 || !strings.Contains(m.statusMsg, "write succeeded") {
						t.Fatal("applied write incorrectly handled")
					}
				default:
					if gets != 0 || cached.JiraTimeSpent != 60 || !strings.Contains(m.statusMsg, "Error:") {
						t.Fatal("failed write incorrectly handled")
					}
				}
			})
		}
	}
}

func TestWorklogCancelAndUnavailable(t *testing.T) {
	m, _, tw := fieldFixture(t)
	_, _ = m.Update(routingKey("ctrl+w"))
	if m.worklog.editing || !strings.Contains(m.statusMsg, "unavailable") || len(tw.calls) != 0 {
		t.Fatal("missing capability bypassed")
	}
	plain, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := plain.Update(routingKey("ctrl+w"))
	if cmd != nil || plain.worklog.editing || !strings.Contains(plain.statusMsg, "Jira issues") {
		t.Fatal("Taskwarrior work log opened")
	}

	if got := formatWorkTime(0); got != "0m" {
		t.Fatal(got)
	}
	if got := formatWorkTime(3661); got != "1h 1m 1s" {
		t.Fatal(got)
	}
}
