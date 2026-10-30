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
			for _, key := range []tea.KeyPressMsg{tea.KeyPressMsg{Code: tea.KeyDown}, {Code: tea.KeyDown}} {
				_, _ = m.Update(key)
			}
			if m.jira.picker.cursor != 2 {
				t.Fatal("down navigation did not stop at final entry")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			if m.jira.picker.cursor != 0 {
				t.Fatal("up navigation did not stop at none")
			}
			for _, key := range []tea.KeyPressMsg{jiraKeyPress("d"), {Code: ' ', Text: " "}, jiraKeyPress("q"), jiraKeyPress("F")} {
				_, cmd = m.Update(key)
				if !m.jira.picker.open || m.taskFlightActive() {
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
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
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
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
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
		_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view := m.jiraFilterPickerView()
	if !strings.Contains(view, "> filter-19") || strings.Count(view, "\n") > 10 {
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

func TestFuzzyFilterScore(t *testing.T) {
	for _, tt := range []struct {
		text, query string
		want        int
		match       bool
	}{
		{"anything", "", 0, true},
		{"Jira", "JIR", 0, true},
		{"Été 東京", "É東", 4, true},
		{"abc", "ac", 2, true},
		{"axxb abc", "abc", 5, true},
		{"jira", "ij", 0, false},
		{"abc", "aa", 0, false},
		{"", "a", 0, false},
	} {
		t.Run(tt.text+"/"+tt.query, func(t *testing.T) {
			got, ok := fuzzyFilterScore(tt.text, tt.query)
			if ok != tt.match || ok && got != tt.want {
				t.Fatalf("score = %d, %t; want %d, %t", got, ok, tt.want, tt.match)
			}
		})
	}
}

func TestJiraFilterPickerLiveQuery(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		t.Run(fmt.Sprintf("ultra=%t", ultra), func(t *testing.T) {
			source := &fakeJiraSource{}
			m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
			m.showUltra = ultra
			m.jira.settings.Filters = []config.Filter{
				{Name: "j--k--r", JQL: "project=SLOW"},
				{Name: "jkr", JQL: "project=EXACT"},
				{Name: "equal", JQL: "jkr"},
				{Name: "equal", JQL: "jkr other"},
				{Name: "Été 東京", JQL: "project=UNICODE"},
			}
			_, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
			_, _ = m.Update(jiraKeyPress("F"))
			originalID := m.jira.filterID
			for _, ch := range "jkr" {
				_, _ = m.Update(jiraKeyPress(string(ch)))
			}
			p := &m.jira.picker
			if p.query.Value() != "jkr" || len(p.entries) != 5 || p.entries[0].identity != "" {
				t.Fatalf("query or filtered entries incorrect: %q %+v", p.query.Value(), p.entries)
			}
			for i, want := range []string{"", "local:jkr", "local:equal", "local:equal", "local:j--k--r"} {
				if p.entries[i].identity != want {
					t.Fatalf("ranking[%d] = %s; want %s", i, p.entries[i].identity, want)
				}
			}
			if p.entries[2].filter.JQL != "jkr" || p.entries[3].filter.JQL != "jkr other" {
				t.Fatal("tied results lost source order")
			}
			if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Search: jkr") || strings.Contains(view, "Été") {
				t.Fatalf("live query not rendered: %s", view)
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
			if p.cursor != 1 {
				t.Fatal("Ctrl+n did not navigate")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
			if p.cursor != 0 {
				t.Fatal("Ctrl+p did not reach none")
			}
			_, _ = m.Update(jiraKeyPress("z"))
			if len(p.entries) != 1 || p.entries[0].identity != "" {
				t.Fatal("none unreachable for unmatched query")
			}
			for range 4 {
				_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			}
			if p.query.Value() != "" || len(p.entries) != 6 || p.entries[1].filter.Name != "j--k--r" {
				t.Fatal("clearing failed to restore local order")
			}
			for _, ch := range "é東" {
				_, _ = m.Update(jiraKeyPress(string(ch)))
			}
			if len(p.entries) != 2 || p.entries[1].filter.Name != "Été 東京" {
				t.Fatal("Unicode name not matched")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if p.open || m.jira.filterID != originalID || source.calls.Load() != 0 {
				t.Fatal("cancel with query changed filter or fetched")
			}
			_, _ = m.Update(jiraKeyPress("F"))
			if m.jira.picker.query.Value() != "" {
				t.Fatal("reopen retained query")
			}
			for _, ch := range "EXACT" {
				_, _ = m.Update(jiraKeyPress(string(ch)))
			}
			if len(m.jira.picker.entries) != 2 || m.jira.picker.entries[1].identity != "local:jkr" {
				t.Fatal("JQL query did not filter")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || m.jira.picker.open || m.jira.filterID != "local:jkr" || m.jira.filter.JQL != "project=EXACT" {
				t.Fatal("apply with query selected wrong filter")
			}
		})
	}
}
