package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

type cloudFilterSource struct {
	*fakeJiraSource
	filters []jira.SavedFilter
	err     error
	calls   int
}

func (s *cloudFilterSource) FetchFilters(ctx context.Context) ([]jira.SavedFilter, error) {
	s.calls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]jira.SavedFilter(nil), s.filters...), s.err
}

func newCloudFilterModel(t *testing.T) (Model, *cloudFilterSource) {
	t.Helper()
	s := &cloudFilterSource{fakeJiraSource: &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}, filters: []jira.SavedFilter{{ID: "1", Name: "mine", JQL: "project=CLOUD"}, {ID: "2", Name: "mine", JQL: "project=OTHER"}, {ID: "3", Name: "hidden"}}}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, s)
	m.jira.settings.SavedFilters = true
	m.tbl.SetWidth(160)
	return m, s
}

func TestCloudFiltersLazyCacheOriginAndQueryRefresh(t *testing.T) {
	m, s := newCloudFilterModel(t)
	drainCmds(t, &m, m.Init())
	if s.calls != 0 {
		t.Fatal("startup fetched filter list")
	}
	issueStamp := m.jira.lastFetched
	_, cmd := m.Update(jiraKeyPress("F"))
	if s.calls != 0 || cmd == nil || len(m.jira.picker.entries) != 2 {
		t.Fatal("first open was not lazy async")
	}
	drainCmds(t, &m, cmd)
	if s.calls != 1 || len(m.jira.picker.entries) != 4 || m.jira.picker.entries[1].identity != "local:mine" || m.jira.picker.entries[2].identity != "cloud:1" || m.jira.picker.entries[3].identity != "cloud:2" {
		t.Fatal("wrong group order or identity")
	}
	if view := ansi.Strip(m.jiraFilterPickerView()); !strings.Contains(view, "mine (local)") || !strings.Contains(view, "mine (cloud)") {
		t.Fatalf("missing origins: %s", view)
	}
	for _, r := range "OTHER" {
		_, _ = m.Update(jiraKeyPress(string(r)))
	}
	if len(m.jira.picker.entries) != 2 || m.jira.picker.entries[1].identity != "cloud:2" {
		t.Fatal("cloud JQL fuzzy search failed")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	s.filters = append([]jira.SavedFilter{{ID: "0", Name: "new", JQL: "project=OTHER"}}, s.filters...)
	drainCmds(t, &m, cmd)
	if m.jira.picker.query.Value() != "OTHER" || m.jira.picker.entries[m.jira.picker.cursor].identity != "cloud:2" {
		t.Fatal("refresh lost query/selection")
	}
	if !m.jira.lastFetched.Equal(issueStamp) || m.jira.filtersFetched.IsZero() {
		t.Fatal("filter refresh changed issue freshness")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainCmds(t, &m, cmd)
	if m.jira.filterID != "cloud:2" || m.jira.filter.JQL != "project=OTHER" || !strings.Contains(m.jiraStatusToken(), "mine (cloud)") {
		t.Fatal("cloud apply/status incorrect")
	}
	_, cmd = m.Update(jiraKeyPress("F"))
	if cmd != nil {
		t.Fatal("cached reopen fetched")
	}
	_, _ = m.Update(jiraKeyPress("r"))
	if m.jira.picker.query.Value() != "r" {
		t.Fatal("printable r did not enter query")
	}
}

func TestCloudFiltersFailureRetainsCacheAndActiveJQL(t *testing.T) {
	m, s := newCloudFilterModel(t)
	s.err = errors.New("permission denied")
	_, cmd := m.Update(jiraKeyPress("F"))
	drainCmds(t, &m, cmd)
	if len(m.jira.picker.entries) != 2 || !strings.Contains(m.statusMsg, "Jira saved filters") {
		t.Fatal("first failure lost local list or note")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, cmd = m.Update(jiraKeyPress("F"))
	if cmd != nil || s.calls != 1 {
		t.Fatal("failure caused reopen request loop")
	}
	s.err = nil
	drainCmds(t, &m, m.scheduleJiraFilters(false))
	m.jira.filterID = "cloud:1"
	m.jira.filter.JQL = "project=ORIGINAL"
	s.err = errors.New("offline")
	drainCmds(t, &m, m.scheduleJiraFilters(false))
	if len(m.jira.cloudFilters) != 3 || len(m.jira.picker.entries) != 4 || m.jira.filter.JQL != "project=ORIGINAL" {
		t.Fatal("refresh failure changed last-good cache or active JQL")
	}
	s.err = nil
	s.filters = nil
	drainCmds(t, &m, m.scheduleJiraFilters(false))
	if len(m.jira.cloudFilters) != 0 || m.jira.filter.JQL != "project=ORIGINAL" {
		t.Fatal("deleted filter changed active query")
	}
}

func TestCloudFiltersGenerationCancellationAndDisable(t *testing.T) {
	m, s := newCloudFilterModel(t)
	old := m.scheduleJiraFilters(false)
	current := m.scheduleJiraFilters(false)
	drainCmds(t, &m, current)
	stamp := m.jira.filtersFetched
	drainCmds(t, &m, old)
	if !m.jira.filtersFetched.Equal(stamp) || !strings.Contains(m.jira.cloudFilters[0].JQL, "CLOUD") {
		t.Fatal("stale response replaced cache")
	}
	m.jira.settings.SavedFilters = false
	if cmd := m.scheduleJiraFilters(false); cmd != nil {
		t.Fatal("disabled filters fetched")
	}
	m.jira.settings.SavedFilters = true
	cmd := m.scheduleJiraFilters(false)
	m.cancelTaskOperations()
	drainCmds(t, &m, cmd)
	if !m.jira.filtersFetched.Equal(stamp) {
		t.Fatal("closed model accepted result")
	}
	if s.calls != 3 {
		t.Fatalf("calls = %d", s.calls)
	}
}

func TestCloudFiltersExplicitAndPeriodicRefresh(t *testing.T) {
	m, s := newCloudFilterModel(t)
	_, cmd := m.handleRefresh()
	drainCmds(t, &m, cmd)
	if s.calls != 1 {
		t.Fatal("space refresh did not warm filters")
	}
	m.autoRefresh = true
	_, cmd = m.Update(autoRefreshMsg{gen: m.autoRefreshGen})
	m.autoRefresh = false
	drainCmds(t, &m, cmd)
	if s.calls != 2 {
		t.Fatal("periodic refresh did not fetch filters")
	}
}

func TestCloudFiltersCredentialFailureSkipsRequest(t *testing.T) {
	m, s := newCloudFilterModel(t)
	drainCmds(t, &m, m.scheduleJiraFilters(false))
	stamp := m.jira.filtersFetched
	m.jira.reloadToken = func(context.Context) error { return errors.New("token file missing") }
	drainCmds(t, &m, m.scheduleJiraFilters(true))
	if s.calls != 1 || !m.jira.filtersFetched.Equal(stamp) || !strings.Contains(m.statusMsg, "token file missing") {
		t.Fatal("credential failure fetched with stale credentials or lost cache")
	}
}
