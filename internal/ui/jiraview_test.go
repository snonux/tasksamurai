package ui

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/task"
)

func TestJiraSourceDefaultView(t *testing.T) {
	for _, view := range []string{"", "tw", "both", "jira"} {
		t.Run(view, func(t *testing.T) {
			source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
			m, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}}}, WithJira(config.Jira{Enabled: true, DefaultView: view}, source, nil))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.cancelTaskOperations)
			drainCmds(t, &m, m.Init())
			want := view
			if want == "" {
				want = "both"
			}
			if m.jira.view != want || (m.taskByUUID("tw-1") != nil) != (want != "jira") || (m.taskByUUID("jira:TST-42") != nil) != (want != "tw") {
				t.Fatalf("default %q: view=%q tasks=%+v", view, m.jira.view, m.tasks)
			}
		})
	}
}

func TestJiraSourceCycleUsesCacheInBothViews(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		for _, key := range []string{"6", "7"} {
			source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
			tw := &countingExportTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW", Status: "pending"}}}}
			m := newJiraTestModel(t, tw, source)
			drainCmds(t, &m, m.Init())
			m.SetUltra(ultra)
			h := DefaultHotkeys()
			h.Source = key
			if err := m.SetHotkeys(h); err != nil {
				t.Fatal(err)
			}
			jiraID := m.taskByUUID("jira:TST-42").ID
			_ = m.selectTaskByID(jiraID)
			stamp, exports := m.jira.lastFetched, tw.exports
			for _, want := range []string{"jira", "tw", "both", "jira"} {
				_, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
				if cmd == nil || m.statusMsg != "Source view: "+want {
					t.Fatalf("cycle status %q", m.statusMsg)
				}
				if m.jira.view != want || len(m.twTasks) != 1 || len(m.jira.tasks) != 1 {
					t.Fatalf("cycle lost caches: %q", m.jira.view)
				}
				if (m.taskByUUID("tw-1") != nil) != (want != "jira") || (m.taskByUUID("jira:TST-42") != nil) != (want != "tw") {
					t.Fatalf("wrong rows in %s: %+v", want, m.tasks)
				}
				if m.total != len(m.tasks) || len(m.ultraTaskList()) != len(m.tasks) {
					t.Fatalf("hidden rows affect stats/ultra in %s", want)
				}
				if want == "jira" && m.getTaskForOpenURL() == nil {
					t.Fatalf("nil selection: ultra=%v key=%s tablecursor=%d ultracursor=%d focused=%d tasks=%+v", ultra, key, m.tbl.Cursor(), m.ultraCursor, m.ultraFocusedID, m.tasks)
				}
				if want == "jira" && (m.getTaskForOpenURL().UUID != "jira:TST-42" || m.taskByUUID("jira:TST-42").ID != jiraID) {
					t.Fatal("visible Jira selection/ID lost")
				}
				if !strings.Contains(m.jiraStatusToken(), "["+want+" · jira: mine · "+stamp.Local().Format("15:04")) {
					t.Fatalf("status token %q", m.jiraStatusToken())
				}
			}
			if source.calls.Load() != 1 || tw.exports != exports || !m.jira.lastFetched.Equal(stamp) {
				t.Fatal("cycle fetched instead of using cache")
			}
			sections := m.helpSections()
			if ultra {
				sections = m.ultraHelpSections()
			}
			found := false
			for _, section := range sections {
				for _, item := range section.Items {
					if item.Key == key && item.Desc == "cycle sources: tw/both/jira" {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("configured source key absent from help")
			}
		}
	}
}

func TestJiraSourceDisabledNoOp(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		tw := &countingExportTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}}}}
		m, err := NewWithTaskwarrior(nil, "firefox", tw)
		if err != nil {
			t.Fatal(err)
		}
		m.SetUltra(ultra)
		exports := tw.exports
		_, cmd := m.Update(tea.KeyPressMsg{Code: '6', Text: "6"})
		if cmd != nil || m.jira.view != "" || m.statusMsg != "" || len(m.tasks) != 1 || tw.exports != exports {
			t.Fatal("disabled Jira cycle changed state")
		}
	}
}

func TestJiraSourceCycleFiltersSearchAndCompletedRows(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) {
		return append(jiraRows(), task.Task{UUID: "jira:TST-43", IDText: "TST-43", Description: "Jira completed", Status: "completed", End: time.Now().UTC().Format(task.DateFormat)}), nil
	}}
	m := newJiraTestModel(t, &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW", Status: "pending"}}}, source)
	drainCmds(t, &m, m.Init())
	m.searchRegex = regexp.MustCompile("Jira")
	m.ultraSearchRegex = m.searchRegex
	for _, want := range []string{"jira", "tw", "both"} {
		_, _ = m.handleCycleSourceView()
		matches := 1
		if want == "tw" {
			matches = 0
		}
		if len(m.searchMatches) != matches || len(m.ultraTaskList()) != matches || m.taskByUUID("jira:TST-43") != nil {
			t.Fatalf("search/window leaks in %s: table=%d ultra=%d", want, len(m.searchMatches), len(m.ultraTaskList()))
		}
	}
}

func TestJiraSourceCyclePreservesVisibleTWSelection(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Status: "pending"}, {ID: 2, UUID: "tw-2", Status: "pending"}}}, source)
	drainCmds(t, &m, m.Init())
	for _, ultra := range []bool{false, true} {
		m.SetUltra(ultra)
		m.jira.view = "tw"
		m.rebuildJiraRows()
		_ = m.selectTaskByID(2)
		_, _ = m.handleCycleSourceView()
		if selected := m.getTaskForOpenURL(); selected == nil || selected.UUID != "tw-2" {
			t.Fatalf("visible TW selection lost in ultra=%v: %+v", ultra, selected)
		}
	}
}
