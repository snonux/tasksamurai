package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/tasksamurai/internal/config"
	"github.com/snonux/tasksamurai/internal/jira"
)

type jiraFiltersDoneMsg struct {
	gen     int
	filters []jira.SavedFilter
	fetched time.Time
	err     error
}

func (m *Model) scheduleJiraFilters(rotateToken bool) tea.Cmd {
	source, ok := m.jira.source.(FilterSource)
	if !ok || !m.jira.settings.SavedFilters || m.jira.closed {
		return nil
	}
	if m.jira.filtersCancel != nil {
		m.jira.filtersCancel()
	}
	timeout := m.jira.settings.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	m.jira.filtersCancel = cancel
	m.jira.filtersGen++
	m.jira.filtersAttempted, m.jira.filtersLoading = true, true
	gen, reloadToken := m.jira.filtersGen, m.jira.reloadToken
	return func() tea.Msg {
		defer cancel()
		if rotateToken && reloadToken != nil {
			if err := reloadToken(ctx); err != nil {
				return jiraFiltersDoneMsg{gen: gen, err: err}
			}
		}
		filters, err := source.FetchFilters(ctx)
		return jiraFiltersDoneMsg{gen: gen, filters: filters, err: err, fetched: time.Now()}
	}
}

func (m *Model) handleJiraFiltersDone(msg jiraFiltersDoneMsg) (tea.Model, tea.Cmd) {
	if m.jira.closed || msg.gen != m.jira.filtersGen || !m.jira.filtersLoading {
		return m, nil
	}
	m.jira.filtersLoading, m.jira.filtersCancel = false, nil
	if msg.err != nil {
		m.showError(fmt.Errorf("Jira saved filters: %w", msg.err))
		return m, nil
	}
	m.jira.cloudFilters = append([]jira.SavedFilter(nil), msg.filters...)
	m.jira.filtersFetched = msg.fetched
	if m.jira.picker.open {
		picker := &m.jira.picker
		identity := ""
		if picker.cursor >= 0 && picker.cursor < len(picker.entries) {
			identity = picker.entries[picker.cursor].identity
		}
		picker.allEntries = m.jiraFilterEntries()
		picker.refilter()
		for i, entry := range picker.entries {
			if entry.identity == identity {
				picker.cursor = i
				break
			}
		}
		m.updateTableHeight()
	}
	return m, nil
}

func (m *Model) jiraFilterEntries() []jiraFilterEntry {
	entries := []jiraFilterEntry{{filter: config.Filter{Name: "none"}}}
	for _, filter := range m.jira.settings.Filters {
		entries = append(entries, jiraFilterEntry{filter: filter, identity: "local:" + filter.Name, origin: "local"})
	}
	seen := make(map[string]bool)
	if m.jira.settings.SavedFilters {
		for _, filter := range m.jira.cloudFilters {
			if strings.TrimSpace(filter.ID) == "" || strings.TrimSpace(filter.Name) == "" || strings.TrimSpace(filter.JQL) == "" || seen[filter.ID] {
				continue
			}
			seen[filter.ID] = true
			entries = append(entries, jiraFilterEntry{filter: config.Filter{Name: filter.Name, JQL: filter.JQL}, identity: "cloud:" + filter.ID, origin: "cloud"})
		}
	}
	return entries
}
