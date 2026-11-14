package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/config"
)

// configReloadTimeout bounds the reload callback: it rereads the TOML file and,
// when Jira is enabled, the token file. It performs no network requests.
const configReloadTimeout = 10 * time.Second

// ConfigReload is the runtime configuration produced by a ConfigReloader.
// Source and ReloadToken are nil when the reloaded config disables Jira.
type ConfigReload struct {
	Jira        config.Jira
	Source      IssueSource
	ReloadToken func(context.Context) error
}

// ConfigReloader rereads the config file with the startup options (so flags
// such as --no-jira keep applying) and builds a new Jira client when enabled.
// It runs inside a tea.Cmd and must not touch the Model.
type ConfigReloader func(context.Context) (ConfigReload, error)

// configEditState tracks the ctrl+e config editor flow.
type configEditState struct {
	configPath   string
	configReload ConfigReloader
}

// configEditLaunchMsg is emitted once the config directory exists, so Update
// can suspend the TUI and start $EDITOR on the file.
type configEditLaunchMsg struct {
	path string
	err  error
}

// configEditDoneMsg is emitted when the config editor exits.
type configEditDoneMsg struct{ err error }

// configReloadDoneMsg carries the reloaded configuration back to Update, the
// only place that may swap the Jira state.
type configReloadDoneMsg struct {
	reload ConfigReload
	err    error
}

// WithConfigEditor enables ctrl+e: edit path in $EDITOR, then apply the result
// of reload. Without this option ctrl+e only reports that editing is
// unavailable (tests and embedders that have no config file).
func WithConfigEditor(path string, reload ConfigReloader) ModelOption {
	return func(m *Model) error {
		m.configEditState = configEditState{configPath: path, configReload: reload}
		return nil
	}
}

// handleEditConfig opens the config file in $EDITOR. Jira writes are never
// cancelled for a reload, so the editor is refused while one is in flight.
func (m *Model) handleEditConfig() (tea.Model, tea.Cmd) {
	if m.configPath == "" || m.configReload == nil {
		return m, m.showStatusTimed("Config editing is not available")
	}
	if m.jiraWriteInFlight() {
		return m, m.showStatusTimed("Busy: wait for the Jira write before editing the config")
	}
	return m, prepareConfigEditCmd(m.configPath)
}

// prepareConfigEditCmd creates the config directory so the editor can save a
// default config that does not exist yet. The file itself is left to the
// editor: an unsaved new file stays absent and Jira stays disabled.
func prepareConfigEditCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return configEditLaunchMsg{err: fmt.Errorf("create config directory: %w", err)}
		}
		return configEditLaunchMsg{path: path}
	}
}

// launchConfigEditorCmd suspends the TUI and runs $EDITOR (vi by default) on
// the config file, mirroring the : prompt's ctrl+o editor flow.
func launchConfigEditorCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return configEditDoneMsg{err: err}
	})
}

func (m *Model) handleConfigEditLaunch(msg configEditLaunchMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.showErrorTimed(msg.err)
	}
	return m, launchConfigEditorCmd(msg.path)
}

// handleConfigEditDone reloads the config after the editor exits. The reload
// runs as a Cmd because it reads files; its result is applied in Update.
func (m *Model) handleConfigEditDone(msg configEditDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.showErrorTimed(fmt.Errorf("config editor: %w", msg.err))
	}
	reload := m.configReload
	m.statusMsg = "Reloading config…"
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), configReloadTimeout)
		defer cancel()
		result, err := reload(ctx)
		return configReloadDoneMsg{reload: result, err: err}
	}
}

// handleConfigReloadDone applies a successfully reloaded config. An invalid
// config keeps the running settings so a typo never drops the Jira session.
func (m *Model) handleConfigReloadDone(msg configReloadDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.showErrorTimed(fmt.Errorf("config not reloaded: %w", msg.err))
	}
	if m.jira.closed {
		return m, nil
	}
	if m.jiraWriteInFlight() {
		return m, m.showErrorTimed(errors.New("config not reloaded: a Jira write is in progress; press ctrl+e again"))
	}
	// An input opened while the reload ran (e.g. ctrl+w worklog or a Jira field
	// editor) would commit through the new client, possibly on another site.
	if m.anyInputActive() || m.worklog.editing {
		return m, m.showErrorTimed(errors.New("config not reloaded: an input is open; press ctrl+e again"))
	}
	next, err := newJiraState(msg.reload.Jira, msg.reload.Source, msg.reload.ReloadToken)
	if err != nil {
		return m, m.showErrorTimed(fmt.Errorf("config not reloaded: %w", err))
	}
	m.replaceJiraState(next)
	return m, tea.Batch(m.showStatusTimed(m.configReloadedStatus()), m.scheduleJiraReload(false))
}

func (m *Model) configReloadedStatus() string {
	if m.jira.source == nil {
		return "Config reloaded (Jira disabled)"
	}
	return "Config reloaded"
}

// jiraWriteInFlight reports a Jira mutation, worklog or comment write. These
// are never cancelled or retried automatically, so a reload waits for them.
func (m *Model) jiraWriteInFlight() bool {
	return m.jira.mutating || m.jira.commenting
}

// replaceJiraState swaps in the reloaded Jira state. Outstanding reads are
// cancelled and every generation counter keeps increasing, so a late message
// from the old client can never match the new state's generations.
func (m *Model) replaceJiraState(next jiraState) {
	prev := m.jira
	m.cancelJiraRequests()
	m.closeJiraDetailView()
	if next.source != nil {
		carryJiraSession(&next, prev)
	}
	next.gen, next.detailGen = prev.gen, prev.detailGen
	next.mutationGen, next.filtersGen = prev.mutationGen, prev.filtersGen
	next.filterEpoch = prev.filterEpoch + 1
	m.jira = next
	m.rebuildJiraRows()
}

// carryJiraSession keeps what the user chose during the session when it is
// still valid for the new config: the source view, a still-defined local or
// same-site cloud filter, and (same site and account only) the synthetic
// display IDs and last-good rows of an unchanged filter, which the scheduled
// fetch replaces. A changed account never shows the old account's rows.
func carryJiraSession(next *jiraState, prev jiraState) {
	if prev.source == nil {
		return
	}
	next.view = prev.view
	sameSite := prev.settings.Site == next.settings.Site
	if filter, ok := carriedJiraFilter(next.settings, prev, sameSite); ok {
		next.filter, next.filterID = filter, prev.filterID
	}
	sameAccount := sameSite && prev.settings.Email == next.settings.Email && prev.settings.TokenFile == next.settings.TokenFile
	if !sameAccount {
		return
	}
	next.displayIDs, next.nextID = prev.displayIDs, prev.nextID
	if next.filterID == prev.filterID && next.filter.JQL == prev.filter.JQL {
		next.tasks, next.lastFetched = prev.tasks, prev.lastFetched
	}
}

// carriedJiraFilter returns the previously selected filter as defined by the
// new settings. Cloud filters are only meaningful on the same site. With no
// filter selected ("none") the new config's default_filter applies instead.
func carriedJiraFilter(settings config.Jira, prev jiraState, sameSite bool) (config.Filter, bool) {
	switch {
	case prev.filterID == "":
		return config.Filter{}, false
	case strings.HasPrefix(prev.filterID, "cloud:"):
		return prev.filter, sameSite
	}
	name := strings.TrimPrefix(prev.filterID, "local:")
	for _, filter := range settings.Filters {
		if filter.Name == name {
			return filter, true
		}
	}
	return config.Filter{}, false
}

// closeJiraDetailView leaves a Jira issue's detail view: its cached issue and
// comment requests belong to the client being replaced.
func (m *Model) closeJiraDetailView() {
	if !m.showTaskDetail || !strings.HasPrefix(m.currentTaskDetailUUID, "jira:") {
		return
	}
	m.showTaskDetail = false
	m.clearCurrentTaskDetail()
	m.resetDetailViewport()
}
