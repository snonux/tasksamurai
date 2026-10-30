package ui

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/snonux/tasksamurai/internal/task"
)

func TestExternalIDRendering(t *testing.T) {
	for _, tt := range []struct{ name, key, status, want, ultra string }{
		{"jira pending", "TST-42", "pending", "TST-42", "TST-42"},
		{"jira completed", "TST-42", "completed", "TST-42", "TST-42"},
		{"TW pending", "", "pending", "42", "#42"},
		{"TW completed", "", "completed", "✓", "✓"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tsk := task.Task{ID: 42, IDText: tt.key, UUID: "row", Status: tt.status, Description: "description"}
			m, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{tasks: []task.Task{tsk}})
			if err != nil {
				t.Fatal(err)
			}
			m.tbl.SetWidth(120)
			row := m.taskToRowSearch(tsk, nil, m.tblStyles, -1)
			if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(colID)])); got != tt.want {
				t.Fatalf("ID cell %q, want %q", got, tt.want)
			}
			if got := m.taskIDLabel(&tsk); got != tt.want {
				t.Fatalf("detail label %q", got)
			}
			m.showTaskDetail = true
			m.setCurrentTaskDetail(&m.tasks[0])
			if detail := ansi.Strip(m.renderTaskDetail()); !strings.Contains(detail, "Task "+tt.want+" Details") {
				t.Fatalf("detail missing issue identity: %s", detail)
			}
			m.tbl.SetColumnCursor(m.logicalToDisplay(colID))
			if expanded := ansi.Strip(m.expandedCellView()); !strings.Contains(expanded, tt.want) {
				t.Fatalf("expanded ID %q", expanded)
			}
			if got := m.ultraIDText(tsk); got != tt.ultra {
				t.Fatalf("ultra label %q", got)
			}
			if got := ansi.Strip(m.renderUltraStatusWithRegex(tsk, 120, nil, "")); !strings.HasPrefix(got, tt.ultra) {
				t.Fatalf("ultra render %q", got)
			}
			if tt.status == "completed" && !regexp.MustCompile(`\x1b\[(?:[0-9]+;)*9(?:;[0-9]+)*m`).MatchString(row[m.logicalToDisplay(colDescription)]) {
				t.Fatal("completed row lost strikethrough")
			}
		})
	}
}

func TestExternalIDWidthsAndSearch(t *testing.T) {
	for _, key := range []string{"TST-12345", "日本語-42", strings.Repeat("長", maxColumnWidth)} {
		t.Run(key, func(t *testing.T) {
			tsk := task.Task{ID: -42, IDText: key, UUID: "jira:" + key, Status: "pending", Description: "unrelated"}
			m, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{tasks: []task.Task{tsk}})
			if err != nil {
				t.Fatal(err)
			}
			if want := min(ansi.StringWidth(key), maxColumnWidth); m.idWidth != want {
				t.Fatalf("width %d, want %d", m.idWidth, want)
			}
			m.searchRegex = regexp.MustCompile(regexp.QuoteMeta(key))
			rows := m.buildTaskRows(m.tasks)
			if len(m.searchMatches) != 1 || m.searchMatches[0].row != 0 || m.searchMatches[0].col != m.logicalToDisplay(colID) {
				t.Fatalf("key search matches %+v", m.searchMatches)
			}
			if got := ansi.Strip(rows[0][m.logicalToDisplay(colID)]); !strings.Contains(got, key) {
				t.Fatalf("search render lost key: %q", got)
			}
			if got := m.ultraFilteredIndexes(m.searchRegex); len(got) != 1 || got[0] != 0 {
				t.Fatalf("ultra key search %+v", got)
			}
			m.searchRegex = regexp.MustCompile("NO-MATCH")
			m.buildTaskRows(m.tasks)
			if len(m.searchMatches) != 0 || len(m.ultraFilteredIndexes(m.searchRegex)) != 0 {
				t.Fatal("unmatched key incorrectly found")
			}
		})
	}
}

func TestTaskwarriorJSONLeavesExternalIDEmpty(t *testing.T) {
	var value task.Task
	if err := json.Unmarshal([]byte(`{"id":42,"uuid":"tw-uuid","description":"plain task","status":"pending"}`), &value); err != nil {
		t.Fatal(err)
	}
	if value.ID != 42 || value.IDText != "" || value.Assignee != "" {
		t.Fatalf("TW parsing changed: %+v", value)
	}
}

func TestJiraDisplayOmitsRedundantMetadata(t *testing.T) {
	for _, tc := range []struct {
		name          string
		value         task.Task
		project, tags string
	}{
		{"synthetic tag", task.Task{UUID: "jira:FOO-123", IDText: "FOO-123", Project: "FOO", Tags: []string{"jira"}}, "", ""},
		{"real labels", task.Task{UUID: "jira:FOO-123", IDText: "FOO-123", Project: "FOO", Tags: []string{"jira", "backend"}, JiraLabels: []string{"backend"}}, "", "backend"},
		{"real jira label", task.Task{UUID: "jira:FOO-123", IDText: "FOO-123", Project: "FOO", Tags: []string{"jira", "backend"}, JiraLabels: []string{"jira", "backend"}}, "", "jira backend"},
		{"Taskwarrior", task.Task{UUID: "tw-1", Project: "FOO", Tags: []string{"jira", "backend"}}, "FOO", "jira backend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.value.Status = "pending"
			m, err := NewWithTaskwarrior(nil, "firefox", &fakeTaskwarrior{tasks: []task.Task{tc.value}})
			if err != nil {
				t.Fatal(err)
			}
			row := m.taskToRowSearch(tc.value, nil, m.tblStyles, -1)
			for col, want := range map[int]string{colProject: tc.project, colTags: tc.tags} {
				if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(col)])); got != want {
					t.Fatalf("column %d = %q, want %q", col, got, want)
				}
			}
			if isJiraTask(&tc.value) {
				for _, re := range []*regexp.Regexp{nil, regexp.MustCompile(`pending \|`)} {
					for _, text := range []string{m.ultraStatusText(tc.value), ansi.Strip(m.renderUltraStatusWithRegex(tc.value, 120, re, ""))} {
						if !strings.Contains(text, tc.value.IDText) || strings.Contains(text, "proj:") {
							t.Fatalf("redundant project or missing issue key: %q", text)
						}
						if tc.tags == "" && strings.Contains(text, "tags:") {
							t.Fatalf("empty synthetic tags: %q", text)
						}
						if tc.tags != "" && !strings.Contains(text, "tags: "+tc.tags) {
							t.Fatalf("lost real labels: %q", text)
						}
					}
				}
			}
			if tc.value.Project != "FOO" || tc.value.Tags[0] != "jira" {
				t.Fatal("display mutated source metadata")
			}
		})
	}
}

func TestJiraStatusAndAssigneeColumns(t *testing.T) {
	m := newJiraTestModel(t, &fakeTaskwarrior{}, &fakeJiraSource{})
	jiraTask := task.Task{UUID: "jira:FOO-123", IDText: "FOO-123", Status: "pending", JiraStatus: "In Review", Assignee: "Paul", Project: "FOO"}
	twTask := task.Task{ID: 1, UUID: "tw-1", Status: "pending", Project: "home", Recur: "weekly"}
	for _, tc := range []struct {
		view, projectHeader, recurHeader string
	}{
		{"tw", "Project", "Recur"},
		{"both", "Project / Status", "Recur / Assignee"},
		{"jira", "Status", "Assignee"},
	} {
		t.Run(tc.view, func(t *testing.T) {
			m.jira.view = tc.view
			for col, want := range map[int]string{colProject: tc.projectHeader, colRecur: tc.recurHeader} {
				title, width := m.columnSpec(col)
				if title != want || (tc.view != "tw" && width < ansi.StringWidth(want)) {
					t.Fatalf("column %d: %q, width %d", col, title, width)
				}
			}
			for _, value := range []task.Task{jiraTask, twTask} {
				row := m.taskToRowSearch(value, nil, m.tblStyles, -1)
				project, recur := "home", "weekly"
				if isJiraTask(&value) {
					project, recur = "In Review", "Paul"
				}
				for col, want := range map[int]string{colProject: project, colRecur: recur} {
					if got := strings.TrimSpace(ansi.Strip(row[m.logicalToDisplay(col)])); got != want {
						t.Fatalf("column %d = %q, want %q", col, got, want)
					}
				}
			}
		})
	}
	m.tasks = []task.Task{jiraTask}
	m.tbl.SetRows(m.buildTaskRows(m.tasks))
	m.tbl.SetCursor(0)
	for query, col := range map[string]int{"In Review": colProject, "Paul": colRecur} {
		m.searchRegex = regexp.MustCompile(query)
		m.buildTaskRows(m.tasks)
		if len(m.searchMatches) != 1 || m.searchMatches[0].col != m.logicalToDisplay(col) {
			t.Fatalf("search %q: %+v", query, m.searchMatches)
		}
		m.tbl.SetColumnCursor(m.logicalToDisplay(col))
		if got := ansi.Strip(m.expandedCellView()); !strings.Contains(got, query) {
			t.Fatalf("expanded cell lost %q: %s", query, got)
		}
	}
	jiraTask.Assignee = ""
	if got := taskDisplayRecur(jiraTask); got != "Unassigned" {
		t.Fatalf("missing assignee: %q", got)
	}
	if jiraTask.Project != "FOO" || jiraTask.Status != "pending" || jiraTask.Recur != "" {
		t.Fatal("display changed source fields")
	}
}
