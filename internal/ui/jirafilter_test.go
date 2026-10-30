package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

func TestJiraFilterPickerApplyAndCancel(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		t.Run(fmt.Sprintf("ultra=%t", ultra), func(t *testing.T) {
			var queries []string
			source := &fakeJiraSource{fetch: func(_ context.Context, jql string) ([]task.Task, error) {
				queries = append(queries, jql)
				return jiraRows(), nil
			}}
			tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW row", Status: "pending"}}}
			m := newJiraTestModel(t, tw, source)
			m.showUltra = ultra
			m.jira.settings.Filters = append(m.jira.settings.Filters, config.Filter{Name: "other", JQL: "project=OTHER"})
			drainCmds(t, &m, m.Init())
			exports := len(tw.exportFilters)
			_, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
			originalHeight := m.tbl.Height()
			_, cmd := m.Update(jiraKeyPress("F"))
			if cmd != nil || !m.jira.picker.open || m.jira.picker.cursor != 1 || !m.anyInputActive() {
				t.Fatal("picker did not open at active default filter")
			}
			if m.tbl.Height() >= originalHeight {
				t.Fatal("picker did not reserve table space")
			}
			if got := ansi.Strip(m.View().Content); !strings.Contains(got, "Jira filters") || !strings.Contains(got, "(none) all issues") || !strings.Contains(got, "project=OTHER") {
				t.Fatalf("overlay missing from active view: %s", got)
			}
			for _, key := range []tea.KeyPressMsg{jiraKeyPress("j"), {Code: tea.KeyDown}} {
				_, _ = m.Update(key)
			}
			if m.jira.picker.cursor != 2 {
				t.Fatal("down navigation did not stop at final entry")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			_, _ = m.Update(jiraKeyPress("k"))
			_, _ = m.Update(jiraKeyPress("k"))
			if m.jira.picker.cursor != 0 {
				t.Fatal("up navigation did not stop at none")
			}
			for _, key := range []tea.KeyPressMsg{jiraKeyPress("d"), {Code: ' ', Text: " "}, jiraKeyPress("q"), jiraKeyPress("F")} {
				_, cmd = m.Update(key)
				if cmd != nil || !m.jira.picker.open || m.taskFlightActive() {
					t.Fatalf("key %q escaped picker", key)
				}
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.jira.picker.open || m.jira.filterID != "local:mine" || source.calls.Load() != 1 || len(m.tasks) != 2 {
				t.Fatal("cancel changed selected filter/cache or fetched")
			}
			if m.tbl.Height() != originalHeight {
				t.Fatal("cancel did not restore table space")
			}
			_, _ = m.Update(jiraKeyPress("F"))
			_, _ = m.Update(jiraKeyPress("j"))
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || m.jira.picker.open || m.jira.filterID != "local:other" || m.taskByUUID("jira:TST-42") != nil || m.taskByUUID("tw-1") == nil || m.taskFlightActive() {
				t.Fatal("apply failed to schedule Jira-only request or clear old-filter rows")
			}
			if m.statusMsg != "jira filter: other" || source.calls.Load() != 1 {
				t.Fatal("apply did not report filter or fetched synchronously")
			}
			drainCmds(t, &m, cmd)
			if len(queries) != 2 || queries[1] != "project=OTHER" || len(tw.exportFilters) != exports || len(m.tasks) != 2 {
				t.Fatalf("incorrect fetch scope: queries=%v TW exports=%d", queries, len(tw.exportFilters))
			}
			if !strings.Contains(m.jiraStatusToken(), "jira: other") || !strings.Contains(m.activeHelpContent(), "select Jira filter") {
				t.Fatal("missing persistent filter name or help entry")
			}
		})
	}
}

func TestJiraFilterPickerNoneUnboundedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			JQL string `json:"jql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.JQL != "" {
			t.Errorf("none sent restricted JQL: %q", body.JQL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorMessages":["Unbounded JQL queries are not allowed here"]}`))
	}))
	defer server.Close()
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("test-token"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.Jira{Enabled: true, Site: server.URL, Email: "test@example.org", TokenFile: token, Timeout: time.Second, MaxResults: 100}
	source, err := jira.New(context.Background(), settings, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}}}
	m := newJiraTestModel(t, tw, source)
	m.jira.tasks = jiraRows()
	m.rebuildJiraRows()
	_, _ = m.Update(jiraKeyPress("F"))
	_, _ = m.Update(jiraKeyPress("k"))
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainCmds(t, &m, cmd)
	if m.jira.filterID != "" || m.jira.filter.Name != "none" || len(m.jira.tasks) != 0 || len(m.tasks) != 1 || m.tasks[0].UUID != "tw-1" || !strings.Contains(m.statusMsg, "press F to pick one") {
		t.Fatalf("none failure displayed old rows or lost pick hint: %s %+v", m.statusMsg, m.tasks)
	}
}

func TestJiraFilterPickerOrderScrollAndWidth(t *testing.T) {
	source := &fakeJiraSource{}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	m.jira.settings.Filters = nil
	for i := 0; i < 20; i++ {
		m.jira.settings.Filters = append(m.jira.settings.Filters, config.Filter{Name: fmt.Sprintf("filter-%02d", i), JQL: strings.Repeat("long query ", 20)})
	}
	m.tbl.SetWidth(35)
	m.windowHeight = 24
	_, _ = m.Update(jiraKeyPress("F"))
	if m.jira.picker.cursor != 0 || len(m.jira.picker.entries) != 21 {
		t.Fatal("none missing or wrong start without default")
	}
	for i, entry := range m.jira.picker.entries[1:] {
		if entry.filter.Name != fmt.Sprintf("filter-%02d", i) {
			t.Fatal("config order changed")
		}
	}
	for i := 0; i < 25; i++ {
		_, _ = m.Update(jiraKeyPress("j"))
	}
	view := m.jiraFilterPickerView()
	if !strings.Contains(view, "> filter-19") || strings.Count(view, "\n") > 9 {
		t.Fatalf("selected entry inaccessible or unbounded overlay: %s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 35 {
			t.Fatalf("picker line overflows width: %q", line)
		}
	}
}

func TestJiraFilterPickerDisabledAndReservedKey(t *testing.T) {
	m, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.cancelTaskOperations)
	_, cmd := m.Update(jiraKeyPress("F"))
	if cmd != nil || m.anyInputActive() || m.jira.picker.open {
		t.Fatal("disabled Jira opened picker")
	}
	h := DefaultHotkeys()
	h.Agent = "F"
	if err := m.SetHotkeys(h); err == nil {
		t.Fatal("F allowed to conflict with configurable hotkey")
	}
}
