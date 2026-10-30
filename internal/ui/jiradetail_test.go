package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

func newJiraDetailFixture(t *testing.T) (Model, *fakeJiraSource) {
	t.Helper()
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	drainCmds(t, &m, m.Init())
	m.jira.settings.Site = "https://example.atlassian.net"
	return m, source
}

func TestJiraFreshDetailBothViews(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		t.Run(map[bool]string{false: "table", true: "ultra"}[ultra], func(t *testing.T) {
			m, source := newJiraDetailFixture(t)
			m.showUltra = ultra
			issueCalls, commentCalls := 0, 0
			source.issue = func(_ context.Context, key string) (task.Task, error) {
				issueCalls++
				if key != "TST-42" {
					t.Fatalf("wrong key %q", key)
				}
				issue := jiraRows()[0]
				issue.Description = "Fresh summary"
				issue.Assignee = "Jira Person"
				return issue, nil
			}
			source.comments = func(_ context.Context, key string) (jira.CommentPage, error) {
				commentCalls++
				return jira.CommentPage{Annotations: []task.Annotation{{Entry: "20261030T120000Z", Description: "A current comment"}}, Remaining: 7}, nil
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if !m.showTaskDetail || cmd == nil {
				t.Fatal("detail did not open")
			}
			drainCmds(t, &m, cmd)
			got := ansi.Strip(m.renderTaskDetail())
			for _, want := range []string{"Fresh summary", "Jira Person", "https://example.atlassian.net/browse/TST-42", "A current comment", "+7 more", "2026-10-30"} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q in %s", want, got)
				}
			}
			if issueCalls != 1 || commentCalls != 1 || source.calls.Load() != 1 || m.jira.tasks[0].Description != "Fresh summary" || len(m.jira.tasks[0].Annotations) != 1 {
				t.Fatal("fresh detail did not patch cache with exactly per-issue reads")
			}
			if cell := ansi.Strip(m.tbl.Rows()[0][m.logicalToDisplay(colAnnotations)]); strings.TrimSpace(cell) != "0m" {
				t.Fatalf("Jira logged time %q", cell)
			}
			_, _ = m.Update(jiraKeyPress("esc"))
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			if source.calls.Load() != 1 {
				t.Fatal("navigation searched Jira")
			}
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			drainCmds(t, &m, cmd)
			if issueCalls != 2 || commentCalls != 2 {
				t.Fatal("reopen used stale cache")
			}
		})
	}
}

func TestJiraDetailRejectsOldReadsAndKeepsCacheOnErrors(t *testing.T) {
	for _, scenario := range []string{"closed", "reopened", "filter", "wrong identity", "issue error", "comments error", "completed"} {
		t.Run(scenario, func(t *testing.T) {
			m, source := newJiraDetailFixture(t)
			source.issue = func(context.Context, string) (task.Task, error) {
				issue := jiraRows()[0]
				issue.Description = "Fresh"
				if scenario == "issue error" {
					return issue, errors.New("denied")
				}
				if scenario == "wrong identity" {
					issue.UUID = "jira:OTHER-1"
				}
				if scenario == "completed" {
					issue.Status = "completed"
					issue.End = "20200101T000000Z"
				}
				return issue, nil
			}
			source.comments = func(context.Context, string) (jira.CommentPage, error) {
				if scenario == "comments error" {
					return jira.CommentPage{}, errors.New("denied")
				}
				return jira.CommentPage{}, nil
			}
			_, cmd := m.handleShowTaskDetail()
			switch scenario {
			case "closed":
				m.showTaskDetail = false
				m.clearCurrentTaskDetail()
			case "reopened":
				_, _ = m.handleShowTaskDetail()
			case "filter":
				_ = m.setJiraFilter(config.Filter{Name: "new", JQL: "project=OTHER"}, "local:new")
			}
			_, _ = m.Update(cmd())
			if scenario == "completed" {
				if !m.showTaskDetail || m.currentDetailTask() == nil || m.currentDetailTask().Description != "Fresh" {
					t.Fatal("fresh completed detail closed")
				}
				return
			}
			if cached := m.jiraCachedTask("jira:TST-42"); cached != nil && cached.Description != "Jira issue" {
				t.Fatal("stale/failed detail changed cache")
			}
		})
	}
}

func TestJiraCommentRoutesAndRejectsEmpty(t *testing.T) {
	for _, mode := range []string{"table", "ultra", "detail"} {
		t.Run(mode, func(t *testing.T) {
			m, source := newJiraDetailFixture(t)
			if mode == "ultra" {
				m.showUltra = true
			}
			if mode == "detail" {
				m.showTaskDetail = true
				m.setCurrentTaskDetail(&m.tasks[0])
			}
			calls := 0
			confirmed := task.Annotation{Entry: "20261030T123456Z", Description: "server created 日本語"}
			source.comment = func(_ context.Context, key, value string) (task.Annotation, error) {
				calls++
				if key != "TST-42" || value != "日本語 \"comment\"" {
					t.Fatalf("wrong comment %s %q", key, value)
				}
				return confirmed, nil
			}
			_, _ = m.Update(jiraKeyPress("a"))
			if !m.annotating || m.annotateInput.Prompt != "comment: " {
				t.Fatal("comment input missing")
			}
			m.annotateInput.SetValue("  ")
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if calls != 0 || !m.annotating || m.jira.commenting {
				t.Fatal("empty comment submitted")
			}
			m.annotateInput.SetValue("日本語 \"comment\"")
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || m.annotating || !m.jira.commenting {
				t.Fatal("submit did not schedule")
			}
			if len(m.jira.tasks[0].Annotations) != 0 {
				t.Fatal("unconfirmed append")
			}
			// Disable blink ticks in this test; route still uses the existing machinery.
			m.blinkEnabled = false
			drainCmds(t, &m, cmd)
			if calls != 1 || source.calls.Load() != 1 || len(m.jira.tasks[0].Annotations) != 1 || m.jira.tasks[0].Annotations[0] != confirmed {
				t.Fatal("confirmed annotation missing or bulk refetch")
			}
		})
	}
}

func TestJiraCommentInvalidatesEarlierReads(t *testing.T) {
	m, source := newJiraDetailFixture(t)
	source.issue = func(context.Context, string) (task.Task, error) { return jiraRows()[0], nil }
	source.comments = func(context.Context, string) (jira.CommentPage, error) { return jira.CommentPage{}, nil }
	source.comment = func(context.Context, string, string) (task.Annotation, error) {
		return task.Annotation{Description: "confirmed"}, nil
	}
	oldSearch := m.scheduleJiraReload(false)
	oldSearchMsg := oldSearch()
	_, oldDetail := m.handleShowTaskDetail()
	oldDetailMsg := oldDetail()
	m.annotateAddr = "jira:TST-42"
	cmd, err := m.scheduleJiraComment("new")
	if err != nil {
		t.Fatal(err)
	}
	// A search can also start while the comment POST is pending.
	duringSearch := m.scheduleJiraReload(false)
	duringMsg := duringSearch()
	m.blinkEnabled = false
	drainCmds(t, &m, cmd)
	for _, msg := range []tea.Msg{oldSearchMsg, oldDetailMsg, duringMsg} {
		_, _ = m.Update(msg)
	}
	if got := m.jira.tasks[0].Annotations; len(got) != 1 || got[0].Description != "confirmed" {
		t.Fatalf("old reads overwrote write: %+v", got)
	}
}

func TestJiraCommentFailuresAndFilterChange(t *testing.T) {
	for _, scenario := range []string{"error", "filter", "quit"} {
		t.Run(scenario, func(t *testing.T) {
			m, source := newJiraDetailFixture(t)
			source.comment = func(context.Context, string, string) (task.Annotation, error) {
				if scenario == "error" {
					return task.Annotation{}, errors.New("permission denied")
				}
				return task.Annotation{Description: "confirmed"}, nil
			}
			m.annotateAddr = "jira:TST-42"
			cmd, err := m.scheduleJiraComment("new")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = m.scheduleJiraComment("double"); err == nil {
				t.Fatal("concurrent write allowed")
			}
			if scenario == "filter" {
				_ = m.setJiraFilter(config.Filter{Name: "new", JQL: "project=OTHER"}, "local:new")
			}
			if scenario == "quit" {
				m.cancelTaskOperations()
			}
			_, _ = m.Update(cmd())
			if cached := m.jiraCachedTask("jira:TST-42"); cached != nil && len(cached.Annotations) != 0 {
				t.Fatal("failed/stale write patched cache")
			}
			if scenario == "error" && !strings.Contains(m.statusMsg, "permission denied") {
				t.Fatal("error not shown")
			}
		})
	}
}

func TestJiraCommentFilterChangePreservesCurrentFetch(t *testing.T) {
	for _, path := range []string{"A-B", "A-B-A"} {
		for _, outcome := range []string{"success", "failure"} {
			for _, order := range []string{"comment-first", "fetch-first"} {
				t.Run(path+"/"+outcome+"/"+order, func(t *testing.T) {
					m, source := newJiraDetailFixture(t)
					original, identity := m.jira.filter, m.jira.filterID
					started, release := make(chan struct{}), make(chan struct{})
					t.Cleanup(func() {
						select {
						case <-release:
						default:
							close(release)
						}
					})
					writes := 0
					source.comment = func(context.Context, string, string) (task.Annotation, error) {
						writes++
						close(started)
						<-release
						if outcome == "failure" {
							return task.Annotation{}, errors.New("permission denied")
						}
						return task.Annotation{Description: "old filter comment"}, nil
					}
					m.annotateAddr = "jira:TST-42"
					write, err := m.scheduleJiraComment("new")
					if err != nil {
						t.Fatal(err)
					}
					result := make(chan tea.Msg, 1)
					go func() { result <- write() }()
					select {
					case <-started:
					case <-time.After(time.Second):
						t.Fatal("comment POST did not start")
					}
					current := m.setJiraFilter(config.Filter{Name: "other", JQL: "project=OTHER"}, "local:other")
					if path == "A-B-A" {
						current = m.setJiraFilter(original, identity)
					}
					source.fetch = func(ctx context.Context, jql string) ([]task.Task, error) {
						if ctx.Err() != nil {
							return nil, ctx.Err()
						}
						row := jiraRows()[0] // Same issue can belong to both filters.
						row.Description = "current " + jql
						return []task.Task{row}, nil
					}
					if order == "fetch-first" {
						drainCmds(t, &m, current)
					}
					gen := m.jira.gen
					close(release)
					select {
					case msg := <-result:
						if _, cmd := m.Update(msg); cmd != nil {
							t.Fatal("old filter comment scheduled extra work")
						}
					case <-time.After(time.Second):
						t.Fatal("comment POST did not complete")
					}
					if m.jira.gen != gen || m.jira.commenting || (order == "comment-first" && !m.jira.loading) {
						t.Fatal("old comment completion invalidated current filter fetch or kept write busy")
					}
					if order == "comment-first" {
						drainCmds(t, &m, current)
					}
					cached := m.jiraCachedTask("jira:TST-42")
					if cached == nil || cached.Description != "current "+m.jira.filter.JQL || len(cached.Annotations) != 0 || m.jira.loading {
						t.Fatalf("current filter view lost or patched by old comment: %+v", cached)
					}
					if writes != 1 || source.calls.Load() != 2 {
						t.Fatalf("unexpected writes/searches: %d/%d", writes, source.calls.Load())
					}
					if outcome == "failure" && !strings.Contains(m.statusMsg, "permission denied") {
						t.Fatal("comment failure not shown")
					}
				})
			}
		}
	}
}

func TestJiraCommentCancelsConcurrentDetail(t *testing.T) {
	m, source := newJiraDetailFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var readContext context.Context
	source.issue = func(ctx context.Context, _ string) (task.Task, error) {
		readContext = ctx
		close(started)
		<-release // Simulate a transport that completes despite cancellation.
		issue := jiraRows()[0]
		issue.Description = "stale response"
		return issue, nil
	}
	source.comments = func(context.Context, string) (jira.CommentPage, error) { return jira.CommentPage{}, nil }
	source.comment = func(context.Context, string, string) (task.Annotation, error) {
		return task.Annotation{Description: "confirmed"}, nil
	}
	_, read := m.handleShowTaskDetail()
	result := make(chan tea.Msg, 1)
	go func() { result <- read() }()
	<-started
	m.annotateAddr = "jira:TST-42"
	write, err := m.scheduleJiraComment("new")
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(readContext.Err(), context.Canceled) {
		t.Fatal("write did not cancel detail read")
	}
	m.blinkEnabled = false
	drainCmds(t, &m, write)
	close(release)
	_, _ = m.Update(<-result)
	if got := m.jira.tasks[0]; got.Description != "Jira issue" || len(got.Annotations) != 1 {
		t.Fatalf("concurrent read replaced comment: %+v", got)
	}
}

func TestJiraRequestsSurviveTaskwarriorContextRecreation(t *testing.T) {
	for _, operation := range []string{"detail", "comment"} {
		for _, quit := range []bool{false, true} {
			name := operation + "/recreate"
			if quit {
				name = operation + "/quit"
			}
			t.Run(name, func(t *testing.T) {
				m, source := newJiraDetailFixture(t)
				m.initTaskContext()
				oldTaskContext := m.taskContext
				calls := 0
				checkContext := func(ctx context.Context) error {
					t.Helper()
					calls++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("Jira request has no deadline")
					}
					if quit {
						if !errors.Is(ctx.Err(), context.Canceled) {
							t.Fatalf("quit did not cancel Jira request: %v", ctx.Err())
						}
					} else if ctx.Err() != nil {
						t.Fatalf("Taskwarrior gate recreation canceled Jira request: %v", ctx.Err())
					}
					return ctx.Err()
				}
				source.issue = func(ctx context.Context, _ string) (task.Task, error) {
					return jiraRows()[0], checkContext(ctx)
				}
				source.comments = func(ctx context.Context, _ string) (jira.CommentPage, error) {
					return jira.CommentPage{}, checkContext(ctx)
				}
				source.comment = func(ctx context.Context, _, _ string) (task.Annotation, error) {
					return task.Annotation{Description: "confirmed"}, checkContext(ctx)
				}
				var cmd tea.Cmd
				if operation == "detail" {
					_, cmd = m.handleShowTaskDetail()
				} else {
					m.annotateAddr = "jira:TST-42"
					var err error
					cmd, err = m.scheduleJiraComment("new")
					if err != nil {
						t.Fatal(err)
					}
				}
				m.recreateTaskContext()
				if !errors.Is(oldTaskContext.Err(), context.Canceled) {
					t.Fatal("fixture did not cancel old Taskwarrior context")
				}
				if quit {
					m.cancelTaskOperations()
				}
				m.blinkEnabled = false
				drainCmds(t, &m, cmd)
				wantCalls := 1
				if operation == "detail" && !quit {
					wantCalls = 2
				}
				if calls != wantCalls {
					t.Fatalf("Jira calls = %d, want %d", calls, wantCalls)
				}
				if !quit && operation == "comment" && len(m.jira.tasks[0].Annotations) != 1 {
					t.Fatal("comment did not complete after Taskwarrior gate recreation")
				}
			})
		}
	}
}

func TestJiraDetailCommentInputRendered(t *testing.T) {
	for _, mode := range []string{"table", "ultra"} {
		for _, existing := range []bool{false, true} {
			name := mode + "/empty"
			if existing {
				name = mode + "/existing"
			}
			t.Run(name, func(t *testing.T) {
				m, source := newJiraDetailFixture(t)
				_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
				m.showUltra = mode == "ultra"
				source.issue = func(context.Context, string) (task.Task, error) {
					issue := jiraRows()[0]
					issue.Description = strings.Repeat("Long detail content for scrolling. ", 100)
					return issue, nil
				}
				source.comments = func(context.Context, string) (jira.CommentPage, error) {
					if existing {
						return jira.CommentPage{Annotations: []task.Annotation{{Description: "Existing server comment"}}}, nil
					}
					return jira.CommentPage{}, nil
				}
				_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				drainCmds(t, &m, cmd)
				_ = m.View()
				originalHeight := m.detailViewport.Height()
				_, _ = m.Update(jiraKeyPress("a"))
				for _, r := range "Visible typed comment" {
					_, _ = m.Update(jiraKeyPress(string(r)))
				}
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(view, "comment: Visible typed comment") {
					t.Fatalf("comment prompt and typed value missing from View: %s", view)
				}
				if m.detailViewport.Height() != originalHeight-1 {
					t.Fatal("comment input did not reserve a viewport line")
				}
				if line := strings.Index(view, "comment: "); line < 0 || strings.Count(view[:line], "\n") < m.detailViewport.Height() {
					t.Fatal("comment input rendered inside the scrolling viewport")
				}
				m.detailViewport.GotoBottom()
				if view = ansi.Strip(m.View().Content); !strings.Contains(view, "comment: Visible typed comment") {
					t.Fatal("scrolling hid the comment input")
				}
				_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
				view = ansi.Strip(m.View().Content)
				if m.annotating || !m.showTaskDetail || strings.Contains(view, "comment: ") || m.detailViewport.Height() != originalHeight {
					t.Fatal("cancel did not remove input and restore detail viewport")
				}
				if source.calls.Load() != 1 {
					t.Fatal("editing or rendering refetched the Jira list")
				}
			})
		}
	}
}
