package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/task"
)

func testJiraSettings(filters ...config.Filter) config.Jira {
	s := config.Jira{Enabled: true, Site: "https://a.example", DefaultView: "both", MaxResults: 100, Timeout: time.Second, Filters: filters}
	if len(filters) > 0 {
		s.DefaultFilter = filters[0].Name
	}
	return s
}

// runCmdMsgs runs cmd (expanding batches) and returns every produced message.
func runCmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmdMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// applyConfigReload drives editor exit -> reload Cmd -> apply -> Jira fetch.
func applyConfigReload(t *testing.T, m *Model) {
	t.Helper()
	_, cmd := m.Update(configEditDoneMsg{})
	msgs := runCmdMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("reload produced %d messages", len(msgs))
	}
	if _, ok := msgs[0].(configReloadDoneMsg); !ok {
		t.Fatalf("unexpected reload message %T", msgs[0])
	}
	_, cmd = m.Update(msgs[0])
	for _, msg := range runCmdMsgs(cmd) {
		if done, ok := msg.(jiraReloadDoneMsg); ok {
			_, _ = m.Update(done)
		}
	}
}

func TestConfigEditUnavailableWithoutOption(t *testing.T) {
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW", Status: "pending"}}}
	m, err := NewWithTaskwarrior(nil, "firefox", tw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.cancelTaskOperations)
	_, _ = m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if !strings.Contains(m.statusMsg, "not available") {
		t.Fatalf("status %q", m.statusMsg)
	}
}

func TestConfigEditRefusedDuringJiraWrite(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	_ = WithConfigEditor(t.TempDir()+"/config.toml", func(context.Context) (ConfigReload, error) { return ConfigReload{}, nil })(&m)
	m.jira.mutating = true
	_, cmd := m.handleEditConfig()
	if !strings.Contains(m.statusMsg, "Busy") {
		t.Fatalf("status %q", m.statusMsg)
	}
	for _, msg := range runCmdMsgs(cmd) {
		if _, ok := msg.(configEditLaunchMsg); ok {
			t.Fatal("editor launched during a Jira write")
		}
	}
}

func TestConfigEditCreatesConfigDirectory(t *testing.T) {
	path := t.TempDir() + "/nested/dir/config.toml"
	msg, ok := prepareConfigEditCmd(path)().(configEditLaunchMsg)
	if !ok || msg.err != nil || msg.path != path {
		t.Fatalf("launch message %+v", msg)
	}
}

func TestConfigReloadEnablesJira(t *testing.T) {
	tw := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "tw-1", Description: "TW", Status: "pending"}}}
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	reload := func(context.Context) (ConfigReload, error) {
		return ConfigReload{Jira: testJiraSettings(), Source: source}, nil
	}
	m, err := NewWithTaskwarrior(nil, "firefox", tw, WithConfigEditor("/unused", reload))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.cancelTaskOperations)
	applyConfigReload(t, &m)
	if m.jira.source == nil || m.taskByUUID("jira:TST-42") == nil {
		t.Fatalf("Jira not enabled by reload: rows %+v", m.tasks)
	}
}

func TestConfigReloadDisablesJiraAndKeepsOnError(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	for _, msg := range runCmdMsgs(m.Init()) {
		_, _ = m.Update(msg)
	}
	result, resultErr := ConfigReload{}, errors.New("bad TOML")
	_ = WithConfigEditor("/unused", func(context.Context) (ConfigReload, error) { return result, resultErr })(&m)

	applyConfigReload(t, &m)
	if m.jira.source == nil || m.taskByUUID("jira:TST-42") == nil {
		t.Fatal("invalid config dropped the running Jira session")
	}
	resultErr = nil
	applyConfigReload(t, &m)
	if m.jira.source != nil || m.taskByUUID("jira:TST-42") != nil {
		t.Fatal("Jira rows remain after a reload disabled Jira")
	}
	if !strings.Contains(m.statusMsg, "Jira disabled") {
		t.Fatalf("status %q", m.statusMsg)
	}
}

func TestConfigReloadRejectsStaleFetchAndKeepsSession(t *testing.T) {
	source := &fakeJiraSource{fetch: func(_ context.Context, jql string) ([]task.Task, error) {
		return jiraRows(), nil
	}}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	m.jira.settings.Site = "https://a.example"
	m.jira.view = "jira"
	staleMsgs := runCmdMsgs(m.Init()) // in flight against the old client
	newSettings := testJiraSettings(config.Filter{Name: "other", JQL: "project=X"}, config.Filter{Name: "mine", JQL: "project=TST2"})
	next, err := newJiraState(newSettings, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.replaceJiraState(next)
	if m.jira.view != "jira" || m.jira.filterID != "local:mine" || m.jira.filter.JQL != "project=TST2" {
		t.Fatalf("session choice not carried: view %q filter %q %q", m.jira.view, m.jira.filterID, m.jira.filter.JQL)
	}
	_ = m.scheduleJiraReload(false)
	for _, msg := range staleMsgs {
		_, _ = m.Update(msg)
	}
	if !m.jira.loading || len(m.jira.tasks) != 0 {
		t.Fatal("stale fetch from the replaced client was applied")
	}
}

func TestConfigReloadNotAppliedDuringWriteOrInvalidView(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	prevSource := m.jira.source
	m.jira.mutating = true
	_, _ = m.Update(configReloadDoneMsg{})
	if m.jira.source != prevSource || !strings.Contains(m.statusMsg, "write is in progress") {
		t.Fatalf("reload applied during a Jira write: status %q", m.statusMsg)
	}
	m.jira.mutating = false
	m.worklog.editing = true
	_, _ = m.Update(configReloadDoneMsg{})
	if m.jira.source != prevSource || !strings.Contains(m.statusMsg, "input is open") {
		t.Fatalf("reload applied with an input open: status %q", m.statusMsg)
	}
	m.worklog.editing = false
	bad := testJiraSettings()
	bad.DefaultView = "sideways"
	_, _ = m.Update(configReloadDoneMsg{reload: ConfigReload{Jira: bad, Source: source}})
	if m.jira.source != prevSource || !strings.Contains(m.statusMsg, "not reloaded") {
		t.Fatalf("invalid view applied: status %q", m.statusMsg)
	}
}

func TestConfigReloadSiteChangeDropsSessionCaches(t *testing.T) {
	source := &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return jiraRows(), nil }}
	m := newJiraTestModel(t, &fakeTaskwarrior{}, source)
	for _, msg := range runCmdMsgs(m.Init()) {
		_, _ = m.Update(msg)
	}
	m.jira.settings.Site = "https://a.example"
	m.jira.filter, m.jira.filterID = config.Filter{Name: "saved", JQL: "project=C"}, "cloud:1"
	other := testJiraSettings()
	other.Site = "https://b.example"
	next, err := newJiraState(other, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.replaceJiraState(next)
	if m.jira.filterID == "cloud:1" || len(m.jira.tasks) != 0 || len(m.jira.displayIDs) != 0 || m.taskByUUID("jira:TST-42") != nil {
		t.Fatalf("old site's filter %q, rows or display IDs carried over", m.jira.filterID)
	}
}

func TestConfigEditorFailureReportsError(t *testing.T) {
	m := newJiraTestModel(t, &fakeTaskwarrior{}, &fakeJiraSource{fetch: func(context.Context, string) ([]task.Task, error) { return nil, nil }})
	_, cmd := m.Update(configEditDoneMsg{err: errors.New("exit status 1")})
	if !strings.Contains(m.statusMsg, "config editor") {
		t.Fatalf("status %q", m.statusMsg)
	}
	for _, msg := range runCmdMsgs(cmd) {
		if _, ok := msg.(configReloadDoneMsg); ok {
			t.Fatal("reloaded after a failed editor run")
		}
	}
}
