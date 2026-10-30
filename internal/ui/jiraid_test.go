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
