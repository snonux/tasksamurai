package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/tasksamurai/internal/task"
)

func TestHotkeysDefaultsAndRebinding(t *testing.T) {
	var m Model
	if got := m.configuredHotkeys(); got != (Hotkeys{"0", "1", "2", "4", "5", "3"}) {
		t.Fatalf("defaults = %+v", got)
	}
	// Track gets a fixed unassigned key ("Q") so reassigning Agent to "3"
	// does not collide with the track default.
	for _, key := range "0123456789" {
		h := Hotkeys{First: "F", Random: "P", RandomNoDue: "L", Agent: string(key), Auto: "Y", Track: "Q"}
		if err := m.SetHotkeys(h); err != nil {
			t.Fatalf("digit %c: %v", key, err)
		}
		if got := m.configuredHotkeys(); got != h {
			t.Fatalf("got %+v, want %+v", got, h)
		}
	}
	if err := m.SetHotkeys(Hotkeys{First: "9", Random: "2", RandomNoDue: "1", Agent: "5", Auto: "4", Track: "6"}); err != nil {
		t.Fatalf("swap: %v", err)
	}
	if err := m.SetHotkeys(Hotkeys{Agent: " Tab ", Auto: "SHIFT+TAB"}); err != nil {
		t.Fatalf("named keys: %v", err)
	}
	if got := m.configuredHotkeys(); got.Agent != "tab" || got.Auto != "shift+tab" {
		t.Fatalf("named keys not canonicalized: %+v", got)
	}
	if err := m.SetHotkeys(Hotkeys{}); err != nil || m.configuredHotkeys() != DefaultHotkeys() {
		t.Fatalf("reset defaults: %v, %+v", err, m.configuredHotkeys())
	}
}

func TestHotkeysInvalidConfigurationIsAtomic(t *testing.T) {
	for _, h := range []Hotkeys{
		{Agent: "5"}, {Auto: "4"}, {Random: "0"}, {RandomNoDue: "1"},
		{First: "q"}, {Agent: "Enter"}, {Auto: "Ctrl+R"}, {Auto: "ctrl+c"},
		{Agent: "nonsense"}, {Auto: "ctrl+ab"}, {Auto: "\x01"}, {Agent: string([]byte{0xff})},
		{Agent: "tab", Auto: "CTRL+I"}, {Auto: "CTRL+M"},
		{Agent: "3"}, {Track: "q"}, {Track: "3", Auto: "3"},
	} {
		t.Run(strings.TrimSpace(h.First+h.Random+h.RandomNoDue+h.Agent+h.Auto+h.Track), func(t *testing.T) {
			var m Model
			if err := m.SetHotkeys(Hotkeys{First: "9", Agent: "7", Auto: "8"}); err != nil {
				t.Fatal(err)
			}
			before := m.configuredHotkeys()
			if err := m.SetHotkeys(h); err == nil {
				t.Fatalf("accepted invalid configuration: %+v", h)
			}
			if got := m.configuredHotkeys(); got != before {
				t.Fatalf("failed validation changed bindings: %+v", got)
			}
		})
	}
}

func TestTagFilterTogglePreservesOtherFilters(t *testing.T) {
	for _, tag := range []string{"agent", "auto", "track"} {
		other := "agent"
		if tag == other {
			other = "auto"
		}
		for _, sign := range []string{"", "+", "-"} {
			in := []string{"(", "project:home", "or", "project:work", ")", "+" + other}
			want := append([]string(nil), in...)
			if sign == "" {
				want = append(want, "+"+tag)
			} else {
				in = append(in, sign+tag)
				next := "+"
				if sign == "+" {
					next = "-"
				}
				want = append(want, next+tag)
			}
			before := append([]string(nil), in...)
			if got := toggleTagFilter(in, tag); !reflect.DeepEqual(got, want) {
				t.Fatalf("toggle %v: got %v, want %v", in, got, want)
			}
			if !reflect.DeepEqual(in, before) {
				t.Fatalf("toggle mutated input: %v", in)
			}
		}
	}
}

func TestTagHotkeysInBothViews(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		for _, swapped := range []bool{false, true} {
			fake := &countingExportTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{
				tasks: []task.Task{{ID: 1, UUID: "u1", Description: "work", Status: "pending"}},
			}}
			m, err := NewWithTaskwarrior([]string{"project:home"}, "firefox", fake)
			if err != nil {
				t.Fatal(err)
			}
			m.SetUltra(ultra)
			h := DefaultHotkeys()
			if swapped {
				h.Agent, h.Auto = h.Auto, h.Agent
			}
			if err := m.SetHotkeys(h); err != nil {
				t.Fatal(err)
			}
			for _, binding := range []struct{ key, tag string }{{h.Agent, "agent"}, {h.Auto, "auto"}, {h.Track, "track"}} {
				for _, sign := range []string{"+", "-", "+"} {
					baseline := fake.exports
					_, cmd := m.Update(tea.KeyPressMsg{Code: rune(binding.key[0]), Text: binding.key})
					if cmd == nil || fake.exports != baseline {
						t.Fatal("toggle must schedule an asynchronous export")
					}
					if !strings.Contains(strings.Join(m.filters, " "), sign+binding.tag) {
						t.Fatalf("filters = %v, want %s%s", m.filters, sign, binding.tag)
					}
					drainCmds(t, &m, cmd)
					last := fake.exportFilters[len(fake.exportFilters)-1]
					if !reflect.DeepEqual(last, append(append([]string(nil), m.filters...), "status:pending")) {
						t.Fatalf("export filters = %v; model = %v", last, m.filters)
					}
				}
			}
		}
	}
}

func TestReboundNavigationAndHelp(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: []task.Task{
		{ID: 1, UUID: "u1", Description: "completed", Status: "completed"},
		{ID: 2, UUID: "u2", Description: "pending", Status: "pending"},
		{ID: 3, UUID: "u3", Description: "due", Status: "pending", Due: "20300101T070000Z"},
	}}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatal(err)
	}
	h := Hotkeys{First: "9", Random: "6", RandomNoDue: "7", Agent: "8", Auto: "3", Track: "5"}
	if err := m.SetHotkeys(h); err != nil {
		t.Fatal(err)
	}
	m.blinkEnabled = false
	for _, ultra := range []bool{false, true} {
		m.SetUltra(ultra)
		m.tbl.SetCursor(1)
		m.ultraCursor = 1
		_, _ = m.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
		if m.tbl.Cursor() != 1 || m.ultraCursor != 1 {
			t.Fatal("old first-task binding still navigates")
		}
		_, _ = m.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
		if ultra && m.ultraCursor != 0 || !ultra && m.tbl.Cursor() != 0 {
			t.Fatal("rebound first-task key did not navigate")
		}
		sections := m.helpSections()
		if ultra {
			sections = m.ultraHelpSections()
		}
		found := map[string]bool{}
		for _, section := range sections {
			for _, item := range section.Items {
				if item.Desc == "toggle +agent/-agent filter" || item.Desc == "toggle +auto/-auto filter" || item.Desc == "toggle +track/-track filter" {
					found[item.Key] = true
				}
			}
		}
		if !found["8"] || !found["3"] || !found["5"] {
			t.Fatalf("help does not reflect rebound filters: %v", found)
		}
	}
	m.SetUltra(false)
	for _, key := range []rune{'6', '7'} {
		for i, tsk := range m.tasks {
			if tsk.Status == "completed" {
				m.tbl.SetCursor(i)
			}
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		drainCmds(t, &m, cmd)
		selected := m.tasks[m.tbl.Cursor()]
		if key == '6' && selected.Status == "completed" {
			t.Fatal("rebound random key selected a completed task")
		}
		if key == '7' && selected.ID != 2 {
			t.Fatal("rebound random-no-due key did not select the only pending task without a due date")
		}
	}
}

func TestAutoFilterReloadFailureAndTyping(t *testing.T) {
	fake := &errorExportTaskwarrior{fakeTaskwarrior: fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "work", Status: "pending"}},
	}}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatal(err)
	}
	m.handleFilter()
	_, cmd := m.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	if len(m.filters) != 0 || m.filterInput.Value() != "5" || len(fake.exportFilters) != 1 {
		t.Fatalf("typing 5 triggered a hotkey: %v, %q", m.filters, m.filterInput.Value())
	}
	m.filterEditing = false
	fake.err = errors.New("export failed")
	_, cmd = m.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	if cmd == nil {
		t.Fatal("no reload scheduled")
	}
	drainCmds(t, &m, cmd)
	if !strings.Contains(m.statusMsg, "export failed") || m.taskFlightBlocks() {
		t.Fatalf("failed reload did not release the UI and display an error: %q", m.statusMsg)
	}
}

// Exercise Bubble Tea events, including terminal-specific Ctrl+I encoding.
func TestNamedHotkeysDispatchKeyEvents(t *testing.T) {
	for _, ultra := range []bool{false, true} {
		for _, tc := range []struct {
			key   string
			event tea.KeyPressMsg
		}{
			{"CTRL+I", tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl}},
			{"ctrl+i", tea.KeyPressMsg{Code: tea.KeyTab}},
			{"Tab", tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl}},
			{"Tab", tea.KeyPressMsg{Code: tea.KeyTab}},
			{"Ctrl+O", tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}},
			{"Shift+Tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}},
		} {
			fake := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "u1", Status: "pending"}}}
			m, err := NewWithTaskwarrior(nil, "firefox", fake)
			if err != nil {
				t.Fatal(err)
			}
			m.SetUltra(ultra)
			if err := m.SetHotkeys(Hotkeys{Auto: tc.key}); err != nil {
				t.Fatal(err)
			}
			_, cmd := m.Update(tc.event)
			if cmd == nil || !reflect.DeepEqual(m.filters, []string{"+auto"}) {
				t.Fatalf("key %q, event %q, ultra=%v did not toggle auto: %v", tc.key, tc.event.String(), ultra, m.filters)
			}
			drainCmds(t, &m, cmd)
		}
	}
}

func TestReboundRandomNoDueWithNoCandidate(t *testing.T) {
	fake := &fakeTaskwarrior{tasks: []task.Task{{ID: 1, UUID: "u1", Status: "pending", Due: "20300101T070000Z"}}}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetHotkeys(Hotkeys{RandomNoDue: "7"}); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: '7', Text: "7"})
	if cmd != nil || m.statusMsg != "No tasks without due date to jump to" {
		t.Fatalf("no-candidate jump: cmd=%v, status=%q", cmd != nil, m.statusMsg)
	}
}
