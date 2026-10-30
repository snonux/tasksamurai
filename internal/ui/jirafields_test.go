package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

type fieldTestSource struct {
	*fakeJiraSource
	field             string
	value             any
	writes, reads     int
	writeErr, readErr error
	after             task.Task
}

func fieldFixture(t *testing.T) (Model, *fieldTestSource, *routingTaskwarrior) {
	t.Helper()
	source := &fieldTestSource{fakeJiraSource: &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}, after: jiraRows()[0]}
	source.issue = func(context.Context, string) (task.Task, error) { source.reads++; return source.after, source.readErr }
	tw := &routingTaskwarrior{}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	m.blinkEnabled = false
	m.jira.tasks[0].Tags = []string{"jira", "keep", "remove"}
	m.jira.tasks[0].Annotations = []task.Annotation{{Description: "existing comment"}}
	m.rebuildJiraRows()
	return m, source, tw
}

func (s *fieldTestSource) SetField(_ context.Context, key, field string, value any) error {
	s.writes++
	s.field = field
	s.value = value
	if key != "TST-42" {
		return errors.New("wrong key")
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	switch field {
	case "summary":
		s.after.Description = value.(string)
	case "duedate":
		s.after.Due = ""
		if value != nil {
			date, err := time.Parse("2006-01-02", value.(string))
			if err != nil {
				return err
			}
			s.after.Due = date.UTC().Format(task.DateFormat)
		}
	case "priority":
		s.after.Priority = map[string]string{"High": "H", "Medium": "M", "Low": "L"}[value.(map[string]string)["name"]]
	case "labels":
		s.after.Tags = append([]string{"jira"}, value.([]string)...)
	}
	return nil
}

func TestJiraFieldHotkeysAndAliases(t *testing.T) {
	for _, mode := range []string{"table", "ultra", "detail"} {
		for _, key := range []string{"e", "E", "w", "W", "r", "p", "t"} {
			t.Run(mode+"/"+key, func(t *testing.T) {
				m, s, tw := fieldFixture(t)
				m.showUltra = mode == "ultra"
				if mode == "detail" {
					m.showTaskDetail = true
					m.setCurrentTaskDetail(&m.tasks[0])
				}
				_, cmd := m.Update(routingKey(key))
				switch key {
				case "e", "E":
					if !m.descEditing || m.descInput.Value() != m.tasks[0].Description {
						t.Fatal("summary editor not prefilled")
					}
					m.descInput.SetValue("new summary")
					_, cmd = m.Update(routingKey("enter"))
				case "w":
					if !m.dueEditing {
						t.Fatal("due editor missing")
					}
					m.dueDate = time.Date(2026, 12, 25, 14, 0, 0, 0, time.FixedZone("offset", 3600))
					_, cmd = m.Update(routingKey("enter"))
				case "p":
					if !m.prioritySelecting || len(m.priorityChoices()) != 3 {
						t.Fatal("Jira priority picker must have H/M/L only")
					}
					m.priorityIndex = 2
					_, cmd = m.Update(routingKey("enter"))
				case "t":
					if !m.tagsEditing {
						t.Fatal("labels editor missing")
					}
					m.tagsInput.SetValue("+new -remove +jira")
					_, cmd = m.Update(routingKey("enter"))
				}
				if cmd == nil || s.writes != 0 {
					t.Fatal("write missing or synchronous")
				}
				drainCmds(t, &m, cmd)
				cached := m.jiraCachedTask("jira:TST-42")
				if s.writes != 1 || s.reads != 1 || s.calls.Load() != 1 || len(tw.calls) != 0 || len(cached.Annotations) != 1 {
					t.Fatalf("write/read/search/TW/cache counts: %+v %v", s, tw.calls)
				}
				switch key {
				case "e", "E":
					if s.field != "summary" || cached.Description != "new summary" || cached.IDText != "TST-42" {
						t.Fatalf("summary/key %+v", cached)
					}
				case "w":
					if s.field != "duedate" || s.value != "2026-12-25" || cached.Due != "20261225T000000Z" {
						t.Fatalf("due %v %+v", s.value, cached)
					}
				case "W":
					if s.field != "duedate" || s.value != nil || cached.Due != "" {
						t.Fatal("due not cleared with null")
					}
				case "r":
					if s.field != "duedate" || cached.Due == "" {
						t.Fatal("random date missing")
					}
				case "p":
					if s.field != "priority" || !reflect.DeepEqual(s.value, map[string]string{"name": "Low"}) || cached.Priority != "L" {
						t.Fatal("priority object missing")
					}
				case "t":
					if s.field != "labels" || !reflect.DeepEqual(s.value, []string{"keep", "new"}) {
						t.Fatalf("labels %v", s.value)
					}
				}
			})
		}
	}
}

func TestJiraFieldEnterPaths(t *testing.T) {
	for _, mode := range []string{"column", "detail"} {
		for _, tc := range []struct {
			col, field int
			name       string
		}{
			{colPri, fieldPriority, "priority"}, {colDue, fieldDue, "duedate"}, {colTags, fieldTags, "labels"}, {colDescription, fieldEntry + 1, "summary"},
		} {
			for _, key := range []string{"i", "enter"} {
				if mode == "column" && key == "enter" {
					continue
				}
				t.Run(mode+"/"+tc.name+"/"+key, func(t *testing.T) {
					m, s, tw := fieldFixture(t)
					m.tbl.SetColumnCursor(m.logicalToDisplay(tc.col))
					if mode == "detail" {
						m.showTaskDetail = true
						m.setCurrentTaskDetail(&m.tasks[0])
						m.detailFieldIndex = tc.field
					}
					_, cmd := m.Update(routingKey(key))
					if cmd != nil || !m.anyInputActive() {
						t.Fatal("editor missing or external editor launched")
					}
					m.descInput.SetValue("edited summary")
					m.tagsInput.SetValue("+new -remove")
					_, cmd = m.Update(routingKey("enter"))
					drainCmds(t, &m, cmd)
					if s.field != tc.name || s.writes != 1 || s.reads != 1 || len(tw.calls) != 0 {
						t.Fatalf("wrong source %s writes %d reads %d TW %v", s.field, s.writes, s.reads, tw.calls)
					}
				})
			}
		}
	}
}

func TestJiraFieldFailuresKeepCacheAndNeverRepeatWrite(t *testing.T) {
	for _, field := range []string{"summary", "duedate", "priority", "labels"} {
		for _, readFailure := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/write", true: "/read"}[readFailure], func(t *testing.T) {
				m, s, tw := fieldFixture(t)
				before := cloneTasks(m.jira.tasks)
				if readFailure {
					s.readErr = errors.New("read failed")
				} else {
					s.writeErr = errors.New("write denied")
				}
				value := map[string]any{"summary": "updated", "duedate": "2026-12-25", "priority": map[string]string{"name": "High"}, "labels": []string{"new"}}[field]
				cmd, err := m.scheduleJiraField("jira:TST-42", field, value)
				if err != nil {
					t.Fatal(err)
				}
				drainCmds(t, &m, cmd)
				reads := 0
				if readFailure {
					reads = 1
				}
				if s.writes != 1 || s.reads != reads || s.calls.Load() != 1 || len(tw.calls) != 0 || !reflect.DeepEqual(before, m.jira.tasks) {
					t.Fatal("failed edit altered cache, repeated write, searched, or called TW")
				}
				want := "write denied"
				if readFailure {
					want = "write succeeded, but issue refresh failed"
				}
				if !strings.Contains(m.statusMsg, want) {
					t.Fatalf("status %q", m.statusMsg)
				}
			})
		}
	}
}

func TestJiraFieldLocalValidationAndPriorityNavigation(t *testing.T) {
	m, s, tw := fieldFixture(t)
	_, _ = m.handleEditTask()
	m.descInput.SetValue(" \n")
	_, _ = m.handleDescriptionMode(routingKey("enter"))
	if !m.descEditing || s.writes != 0 {
		t.Fatal("blank summary accepted")
	}
	_, _ = m.handleSetPriority()
	m.priorityIndex = 3
	_, _ = m.handlePriorityMode(routingKey("enter"))
	if !m.prioritySelecting || s.writes != 0 {
		t.Fatal("TW unset priority accepted")
	}
	m.priorityIndex = 2
	_, _ = m.handlePriorityMode(routingKey("right"))
	if m.priorityIndex != 0 || strings.Contains(m.priorityView(true), "none") {
		t.Fatal("Jira offers unset priority")
	}
	if len(tw.calls) != 0 {
		t.Fatal("validation called TW")
	}
}

func TestJiraSummaryExternalCallbackRoutesToJira(t *testing.T) {
	m, s, tw := fieldFixture(t)
	m.showTaskDetail = true
	m.setCurrentTaskDetail(&m.tasks[0])
	path := filepath.Join(t.TempDir(), "summary")
	if err := os.WriteFile(path, []byte("edited externally"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.handleDescEditDone(descEditDoneMsg{tempFile: path})
	drainCmds(t, &m, cmd)
	if s.field != "summary" || s.value != "edited externally" || s.writes != 1 || len(tw.calls) != 0 {
		t.Fatal("callback used wrong source")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("editor file retained")
	}
}

var _ FieldSource = (*fieldTestSource)(nil)

func TestJiraLabelsPreserveRealJiraLabel(t *testing.T) {
	for _, real := range []bool{false, true} {
		t.Run(map[bool]string{false: "virtual only", true: "real and virtual"}[real], func(t *testing.T) {
			m, s, tw := fieldFixture(t)
			m.jira.tasks[0].JiraLabels = []string{"keep", "remove"}
			if real {
				m.jira.tasks[0].JiraLabels = append(m.jira.tasks[0].JiraLabels, "jira")
			}
			snapshot := cloneTasks(m.jira.tasks)
			m.jira.tasks[0].JiraLabels[0] = "changed"
			if snapshot[0].JiraLabels[0] != "keep" {
				t.Fatal("source labels alias snapshot")
			}
			m.jira.tasks = snapshot
			m.rebuildJiraRows()
			_, _ = m.handleEditTags()
			m.tagsInput.SetValue("+new -remove +jira")
			_, cmd := m.handleTagsMode(routingKey("enter"))
			drainCmds(t, &m, cmd)
			want := []string{"keep", "new"}
			if real {
				want = []string{"keep", "jira", "new"}
			}
			if !reflect.DeepEqual(s.value, want) || len(tw.calls) != 0 {
				t.Fatalf("labels %v want %v TW %v", s.value, want, tw.calls)
			}
		})
	}
}

func TestJiraFieldBusyPreservesEditorAndInput(t *testing.T) {
	for _, field := range []string{"summary", "duedate", "priority", "labels"} {
		t.Run(field, func(t *testing.T) {
			m, s, tw := fieldFixture(t)
			switch field {
			case "summary":
				_, _ = m.handleEditTask()
				m.descInput.SetValue("keep input")
			case "duedate":
				_, _ = m.handleSetDueDate()
			case "priority":
				_, _ = m.handleSetPriority()
			case "labels":
				_, _ = m.handleEditTags()
				m.tagsInput.SetValue("+keep")
			}
			m.jira.mutating = true
			_, _ = m.Update(routingKey("enter"))
			if !m.anyInputActive() || s.writes != 0 || len(tw.calls) != 0 || !strings.Contains(m.statusMsg, "still being submitted") {
				t.Fatalf("busy input lost or wrong status %q", m.statusMsg)
			}
			if field == "summary" && m.descInput.Value() != "keep input" || field == "labels" && m.tagsInput.Value() != "+keep" {
				t.Fatal("busy lost input")
			}
		})
	}
}

func TestJiraFieldPayloadSnapshots(t *testing.T) {
	for _, field := range []string{"labels", "priority"} {
		t.Run(field, func(t *testing.T) {
			m, s, _ := fieldFixture(t)
			labels := []string{"keep"}
			priority := map[string]string{"name": "High"}
			var value any = labels
			if field == "priority" {
				value = priority
			}
			cmd, err := m.scheduleJiraField("jira:TST-42", field, value)
			if err != nil {
				t.Fatal(err)
			}
			labels[0] = "changed"
			priority["name"] = "Low"
			drainCmds(t, &m, cmd)
			want := any([]string{"keep"})
			if field == "priority" {
				want = map[string]string{"name": "High"}
			}
			if !reflect.DeepEqual(s.value, want) {
				t.Fatalf("mutable payload %v", s.value)
			}
		})
	}
}

func TestJiraFieldDetailBlinkMatchesEditedField(t *testing.T) {
	for _, tc := range []struct {
		field string
		index int
		value any
	}{
		{"summary", fieldEntry + 1, "summary"}, {"duedate", fieldDue, "2026-12-25"},
		{"priority", fieldPriority, map[string]string{"name": "High"}}, {"labels", fieldTags, []string{"keep"}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			m, _, _ := fieldFixture(t)
			m.showTaskDetail = true
			m.setCurrentTaskDetail(&m.tasks[0])
			m.blinkEnabled = true
			cmd, err := m.scheduleJiraField("jira:TST-42", tc.field, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			msg := cmd().(jiraMutationDoneMsg)
			_, _ = m.handleJiraMutationDone(msg)
			if m.detailBlinkField != tc.index || !m.detailBlinkOn {
				t.Fatalf("blink %d want %d", m.detailBlinkField, tc.index)
			}
		})
	}
}
