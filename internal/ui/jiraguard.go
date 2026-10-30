package ui

import "strings"

// controlRoute describes how a control acts on its target. Field controls
// delegate to the selected column/detail field; activation helpers check that
// field's control again before opening any editor.
type controlRoute uint8

const (
	routeGlobal controlRoute = iota
	routeTaskwarrior
	routeJira
	routeBlocked
	routeField
)

type controlPolicy struct {
	route  controlRoute
	reason string
}

// jiraControlPolicy is the source-specific dispatch policy. Navigation and
// presentation controls are global by default. Keep row operations explicit:
// Phase-2 implementations replace their planned block here and route the
// corresponding activation/commit boundaries to Jira too. Merely allowing a
// control here must never send Jira addresses through a TW-only commit,
// description-editor callback or delayed-done pipeline.
var jiraControlPolicy = map[string]controlPolicy{
	"a":     {route: routeJira},
	"o":     {route: routeJira},
	"i":     {route: routeField},
	"enter": {route: routeField},
	":":     {route: routeGlobal},
	"+":     {route: routeGlobal},
	"A":     {routeBlocked, "comments can't be replaced wholesale — use a to add"},
	"D":     {routeBlocked, "Jira issues can't be deleted from here"},
	"U":     {routeBlocked, "no undo for Jira issues"},
	"R":     {routeBlocked, "Jira issues have no recurrence"},
	"J":     {routeBlocked, "moving Jira issues not supported (yet)"},
	";":     {routeBlocked, "task commands don't work on Jira issues"},
	"d":     {route: routeJira},
	"s":     {route: routeJira},
	"w":     {route: routeJira},
	"p":     {route: routeJira},
	"t":     {route: routeJira},
	"e":     {route: routeJira},
}

// Aliases share the canonical control's policy and reason. In particular E
// must never fall through to the external Taskwarrior editor on a Jira row.
var controlAliases = map[string]string{
	"E": "e", "r": "w", "W": "w", "ctrl+r": "R", "T": "J",
}

func routeControl(addr, key string) controlPolicy {
	if canonical, ok := controlAliases[key]; ok {
		key = canonical
	}
	policy := jiraControlPolicy[key]
	if !strings.HasPrefix(addr, "jira:") && (policy.route == routeJira || policy.route == routeBlocked) {
		return controlPolicy{route: routeTaskwarrior}
	}
	return policy
}

func jiraBlockedReason(key string) string {
	return routeControl("jira:", key).reason
}

func (m *Model) blockJiraAction(key string) bool {
	t := m.getTaskForOpenURL()
	if !isJiraTask(t) {
		return false
	}
	return m.blockJiraAddress(t.UUID, key)
}

func (m *Model) blockJiraAddress(addr, key string) bool {
	if policy := routeControl(addr, key); policy.route == routeBlocked {
		m.statusMsg = policy.reason
		return true
	}
	return false
}
