package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type transitionTestSource struct {
	*fakeJiraSource
	available                        []jira.Transition
	transitionErr, writeErr, readErr error
	transitions, writes, reads       int
	chosen                           string
	fields                           map[string]any
	after                            task.Task
}

func transitionFixture(t *testing.T) (Model, *transitionTestSource, *routingTaskwarrior) {
	t.Helper()
	source := &transitionTestSource{fakeJiraSource: &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}, after: jiraRows()[0]}
	for _, v := range []struct{ id, name, category string }{{"d", "Custom close", "done"}, {"s", "Custom work", "indeterminate"}, {"q", "Custom queue", "new"}} {
		transition := jira.Transition{ID: v.id, Name: v.name}
		transition.To.Category.Key = v.category
		source.available = append(source.available, transition)
	}
	source.issue = func(context.Context, string) (task.Task, error) { source.reads++; return source.after, source.readErr }
	tw := &routingTaskwarrior{}
	m := newJiraTestModel(t, tw, source)
	drainCmds(t, &m, m.Init())
	m.blinkEnabled = false
	m.jira.tasks[0].Annotations = []task.Annotation{{Entry: "20261030T120000Z", Description: "Existing comment"}}
	m.rebuildJiraRows()
	return m, source, tw
}

func (s *transitionTestSource) Transitions(context.Context, string) ([]jira.Transition, error) {
	s.transitions++
	return s.available, s.transitionErr
}
func (s *transitionTestSource) DoTransition(_ context.Context, key, id string, fields map[string]any) error {
	s.writes++
	s.chosen = id
	s.fields = fields
	if key != "TST-42" {
		return errors.New("wrong issue key")
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	s.after.Status = "pending"
	s.after.Start = ""
	s.after.End = ""
	for _, v := range s.available {
		if v.ID == id {
			switch v.To.Category.Key {
			case "done":
				s.after.Status = "completed"
				s.after.End = time.Now().UTC().Format(task.DateFormat)
			case "indeterminate":
				s.after.Start = "20261030T120000Z"
			}
		}
	}
	return nil
}

func TestJiraTransitionsRouteInEveryView(t *testing.T) {
	for _, mode := range []string{"table", "ultra", "detail"} {
		for _, key := range []string{"d", "s"} {
			t.Run(mode+"/"+key, func(t *testing.T) {
				m, source, tw := transitionFixture(t)
				m.showUltra = mode == "ultra"
				if mode == "detail" {
					m.showTaskDetail = true
					m.setCurrentTaskDetail(&m.tasks[0])
				}
				_, cmd := m.Update(routingKey(key))
				if cmd == nil || source.transitions != 0 || source.writes != 0 {
					t.Fatal("transition was synchronous or not scheduled")
				}
				drainCmds(t, &m, cmd)
				cached := m.jiraCachedTask("jira:TST-42")
				if source.transitions != 1 || source.writes != 1 || source.reads != 1 || source.calls.Load() != 1 || len(tw.calls) != 0 || len(cached.Annotations) != 1 {
					t.Fatalf("wrong workflow counts/cache: %+v, TW %v", source, tw.calls)
				}
				if key == "d" && cached.Status != "completed" || key == "s" && cached.Start == "" {
					t.Fatalf("cache not patched: %+v", cached)
				}
				if !strings.Contains(m.statusMsg, "transitioned via 'Custom ") {
					t.Fatalf("transition note %q", m.statusMsg)
				}
				if mode == "detail" && !m.showTaskDetail {
					t.Fatal("completion closed fresh cached detail")
				}
			})
		}
	}
}

func TestJiraStartThenStopAndDoneNoop(t *testing.T) {
	m, source, tw := transitionFixture(t)
	_, cmd := m.handleToggleStart()
	drainCmds(t, &m, cmd)
	if source.chosen != "s" || m.jiraCachedTask("jira:TST-42").Start == "" {
		t.Fatal("start did not use indeterminate")
	}
	_, cmd = m.handleToggleStart()
	drainCmds(t, &m, cmd)
	if source.chosen != "q" || m.jiraCachedTask("jira:TST-42").Start != "" {
		t.Fatal("stop did not use new category")
	}
	_, cmd = m.handleMarkDone()
	drainCmds(t, &m, cmd)
	m.showTaskDetail = true
	m.setCurrentTaskDetail(m.jiraCachedTask("jira:TST-42"))
	before := source.transitions
	_, cmd = m.handleDetailMarkDone()
	if cmd != nil || source.transitions != before || m.statusMsg != "Jira issue is already completed" || len(tw.calls) != 0 {
		t.Fatal("done on completed issue sent a request")
	}
}

func TestJiraTransitionErrorsAndRequiredFieldDefaults(t *testing.T) {
	for _, scenario := range []string{"zero", "metadata", "write", "read", "required", "default", "override"} {
		t.Run(scenario, func(t *testing.T) {
			m, source, tw := transitionFixture(t)
			switch scenario {
			case "zero":
				source.available = nil
			case "metadata":
				source.transitionErr = errors.New("metadata denied")
			case "write":
				source.writeErr = errors.New("write denied")
			case "read":
				source.readErr = errors.New("fresh issue denied")
			case "required", "default":
				meta := jira.TransitionField{Name: "Resolution", Required: true}
				meta.Schema.Type = "resolution"
				source.available[0].Fields = map[string]jira.TransitionField{"resolution": meta}
				if scenario == "default" {
					m.jira.settings.TransitionFields = map[string]any{"resolution": map[string]any{"name": "Done"}}
				}
			case "override":
				other := source.available[0]
				other.ID = "override"
				other.Name = "Selected close"
				source.available = append(source.available, other)
				m.jira.settings.Transitions = map[string]string{"done": "Selected close"}
			}
			_, cmd := m.handleMarkDone()
			drainCmds(t, &m, cmd)
			if len(tw.calls) != 0 || source.calls.Load() != 1 {
				t.Fatal("transition invoked TW or blanket search")
			}
			if scenario == "default" || scenario == "override" {
				if source.writes != 1 || m.jiraCachedTask("jira:TST-42").Status != "completed" {
					t.Fatal("configured transition did not apply")
				}
				if scenario == "default" && source.fields["resolution"] == nil {
					t.Fatal("structured required default missing")
				}
				if scenario == "override" && source.chosen != "override" {
					t.Fatal("override ignored")
				}
			} else {
				if !strings.Contains(m.statusMsg, "Jira") || m.jiraCachedTask("jira:TST-42").Status != "pending" {
					t.Fatalf("failure lost old cache/status: %s", m.statusMsg)
				}
				if scenario == "read" && (source.writes != 1 || source.reads != 1 || !strings.Contains(m.statusMsg, "write succeeded")) {
					t.Fatal("post-write refresh failure indistinguishable from failed write")
				}
				if (scenario == "zero" || scenario == "metadata" || scenario == "required") && source.writes != 0 {
					t.Fatal("invalid transition sent POST")
				}
			}
		})
	}
}

func TestJiraMutationInvalidatesReadsSerializesWritesAndQuits(t *testing.T) {
	m, source, tw := transitionFixture(t)
	stale := m.scheduleJiraReload(false)
	_, cmd := m.handleToggleStart()
	if cmd == nil {
		t.Fatal("missing transition command")
	}
	if _, err := m.scheduleJiraMutation("jira:TST-42", func(context.Context) (string, error) { t.Fatal("overlapping write ran"); return "", nil }); err == nil {
		t.Fatal("overlapping writes accepted")
	}
	m.annotateAddr = "jira:TST-42"
	if _, err := m.scheduleJiraComment("comment"); err == nil {
		t.Fatal("comment overlapped transition")
	}
	m.recreateTaskContext()
	drainCmds(t, &m, cmd)
	_, _ = m.Update(stale())
	if m.jiraCachedTask("jira:TST-42").Start == "" || source.writes != 1 || len(tw.calls) != 0 {
		t.Fatal("stale read overwrote write or TW context canceled it")
	}
	if err := doneOp("jira:TST-42")(context.Background(), tw); err == nil || len(tw.calls) != 0 {
		t.Fatal("generic TW done accepted Jira after policy translation")
	}
	cmd = m.startBlink(m.tasks[0].ID, true)
	if cmd == nil || m.blinkMarkDone {
		t.Fatal("delayed Jira done entered TW blink pipeline")
	}
	m.cancelTaskOperations()
	_, _ = m.Update(cmd())
	if source.writes != 2 { // the fake ignores cancellation, but its result must be ignored
		t.Fatalf("unexpected fake write count %d", source.writes)
	}
	if m.jiraCachedTask("jira:TST-42").Status != "pending" {
		t.Fatal("late result applied after quit")
	}
}

func TestJiraTransitionFilterRoundtripKeepsNewFetch(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "post-write read failure"}[readFailure], func(t *testing.T) {
			m, source, _ := transitionFixture(t)
			original, identity := m.jira.filter, m.jira.filterID
			source.fetch = func(ctx context.Context, _ string) ([]task.Task, error) {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return []task.Task{source.after}, nil
			}
			if readFailure {
				source.readErr = errors.New("per-issue read failed")
			}
			_, write := m.handleToggleStart()
			_ = m.setJiraFilter(config.Filter{Name: "other", JQL: "project=OTHER"}, "local:other")
			current := m.setJiraFilter(original, identity)
			if len(m.jira.tasks) != 0 || !m.jira.loading {
				t.Fatal("roundtrip fixture didn't clear cache/start new fetch")
			}
			_, _ = m.Update(write())
			if !m.jira.loading {
				t.Fatal("old filter epoch canceled newly selected A fetch")
			}
			if readFailure && !strings.Contains(m.statusMsg, "write succeeded") {
				t.Fatal("write/read failure distinction lost")
			}
			drainCmds(t, &m, current)
			if len(m.tasks) != 1 || m.tasks[0].Start == "" || m.jira.loading || source.writes != 1 {
				t.Fatalf("new A filter fetch did not populate view: %+v, %s", m.tasks, m.statusMsg)
			}
		})
	}
}
