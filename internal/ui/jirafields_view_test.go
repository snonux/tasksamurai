package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type typingJiraSource struct {
	*fieldTestSource
	transitionCalls int
}

func (s *typingJiraSource) Transitions(context.Context, string) ([]jira.Transition, error) {
	s.transitionCalls++
	return nil, nil
}
func (s *typingJiraSource) DoTransition(context.Context, string, string, map[string]any) error {
	s.transitionCalls++
	return nil
}

func scrolledJiraDetailFixture(t *testing.T, ultra bool) (Model, *typingJiraSource, *routingTaskwarrior) {
	t.Helper()
	m, fields, tw := fieldFixture(t)
	source := &typingJiraSource{fieldTestSource: fields}
	m.jira.source = source
	source.comments = func(context.Context, string) (jira.CommentPage, error) {
		return jira.CommentPage{Annotations: []task.Annotation{{Description: strings.Repeat("Long comment body for scrolling. ", 300)}}}, nil
	}
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.showUltra = ultra
	_, cmd := m.Update(routingKey("enter"))
	drainCmds(t, &m, cmd)
	if !m.showTaskDetail {
		t.Fatal("detail did not open")
	}
	_ = m.View()
	m.detailViewport.GotoBottom()
	if m.detailViewport.YOffset() == 0 {
		t.Fatal("fixture did not scroll")
	}
	return m, source, tw
}

func TestJiraDetailSummaryTypingVisibleAndRouted(t *testing.T) {
	for _, mode := range []string{"table", "ultra"} {
		for _, activation := range []string{"e", "E", "i", "enter"} {
			for _, submit := range []bool{false, true} {
				ending := "cancel"
				if submit {
					ending = "submit"
				}
				t.Run(mode+"/"+activation+"/"+ending, func(t *testing.T) {
					m, source, tw := scrolledJiraDetailFixture(t, mode == "ultra")
					beforeReads := source.reads
					beforeSummary := m.currentDetailTask().Description
					m.detailFieldIndex = m.detailDescriptionFieldIndex()
					_, cmd := m.Update(routingKey(activation))
					if cmd != nil || !m.descEditing {
						t.Fatal("summary did not open inline")
					}
					m.descInput.SetValue("")
					text := "describe details safely"
					for _, r := range text {
						_, _ = m.Update(routingKey(string(r)))
						view := ansi.Strip(m.View().Content)
						if !strings.Contains(view, m.descInput.Value()) || !strings.Contains(view, "description: ") {
							t.Fatalf("active summary input invisible: %s", view)
						}
						if source.transitionCalls != 0 || source.writes != 0 || source.reads != beforeReads || m.jira.mutating || m.statusMsg != "" {
							t.Fatalf("typing triggered action: transitions %d writes %d status %q", source.transitionCalls, source.writes, m.statusMsg)
						}
					}
					if m.descInput.Value() != text {
						t.Fatalf("typed value %q", m.descInput.Value())
					}
					// Mouse/viewport movement cannot hide an active editor on the next frame.
					m.detailViewport.GotoBottom()
					if view := ansi.Strip(m.View().Content); !strings.Contains(view, "description: "+text) {
						t.Fatalf("scrolling hid input: %s", view)
					}
					key := "esc"
					if submit {
						key = "enter"
					}
					_, cmd = m.Update(routingKey(key))
					drainCmds(t, &m, cmd)
					view := ansi.Strip(m.View().Content)
					if m.descEditing || !m.showTaskDetail || strings.Contains(view, "description: ") {
						t.Fatal("editor did not close while retaining detail")
					}
					if source.transitionCalls != 0 || len(tw.calls) != 0 {
						t.Fatal("summary typed into transitions or Taskwarrior")
					}
					if submit {
						if source.writes != 1 || source.field != "summary" || source.value != text || m.currentDetailTask().Description != text {
							t.Fatal("typed summary not saved")
						}
					} else if source.writes != 0 || m.currentDetailTask().Description != beforeSummary {
						t.Fatal("cancel saved summary")
					}
				})
			}
		}
	}
}

func TestJiraDetailFieldHotkeysKeepEditorsVisible(t *testing.T) {
	for _, tc := range []struct {
		key   string
		field int
	}{{"w", fieldDue}, {"p", fieldPriority}, {"t", fieldTags}} {
		t.Run(tc.key, func(t *testing.T) {
			m, source, tw := scrolledJiraDetailFixture(t, false)
			_, _ = m.Update(routingKey(tc.key))
			if !m.anyInputActive() {
				t.Fatal("field editor missing")
			}
			_ = m.View()
			line := detailFieldLine(m.detailFieldLines, tc.field)
			top := m.detailViewport.YOffset()
			if line < top || line >= top+m.detailViewport.Height() {
				t.Fatalf("editor row %d outside viewport starting at %d", line, top)
			}
			if source.writes != 0 || source.transitionCalls != 0 || len(tw.calls) != 0 {
				t.Fatal("opening editor wrote data")
			}
		})
	}
}
