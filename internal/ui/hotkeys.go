package ui

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Hotkeys configures the actions assigned to number keys by default.
// Empty fields select their defaults. Random jumps apply in table mode only.
type Hotkeys struct {
	First       string
	Random      string
	RandomNoDue string
	Agent       string
	Auto        string
}

// DefaultHotkeys returns the default number-key assignments.
func DefaultHotkeys() Hotkeys {
	return Hotkeys{First: "0", Random: "1", RandomNoDue: "2", Agent: "4", Auto: "5"}
}

func (h Hotkeys) normalized() Hotkeys {
	defaults := DefaultHotkeys()
	keys := []*string{&h.First, &h.Random, &h.RandomNoDue, &h.Agent, &h.Auto}
	fallbacks := []string{defaults.First, defaults.Random, defaults.RandomNoDue, defaults.Agent, defaults.Auto}
	for i, key := range keys {
		*key = normalizeAgentFilterHotkey(*key)
		if *key == "" {
			*key = fallbacks[i]
		}
	}
	return h
}

// SetHotkeys validates all assignments together so keys can be swapped.
// On error the current configuration is left unchanged.
func (m *Model) SetHotkeys(h Hotkeys) error {
	h = h.normalized()
	seen := make(map[string]string)
	for _, binding := range []struct{ name, key string }{
		{"first", h.First}, {"random", h.Random}, {"random-no-due", h.RandomNoDue},
		{"agent", h.Agent}, {"auto", h.Auto},
	} {
		if err := validateConfigurableHotkey(binding.key); err != nil {
			return fmt.Errorf("%s hotkey: %w", binding.name, err)
		}
		if other, ok := seen[binding.key]; ok {
			return fmt.Errorf("%s hotkey %q conflicts with %s hotkey", binding.name, binding.key, other)
		}
		seen[binding.key] = binding.name
	}
	m.hotkeys = h
	m.tbl.KeyMap.GotoTop.SetKeys("home", "g", h.First)
	return nil
}

// SetAgentFilterHotkey changes the agent binding, preserving other bindings.
func (m *Model) SetAgentFilterHotkey(key string) error {
	if normalizeAgentFilterHotkey(key) == "" {
		return nil
	}
	h := m.configuredHotkeys()
	h.Agent = key
	return m.SetHotkeys(h)
}

func (m *Model) configuredHotkeys() Hotkeys { return m.hotkeys.normalized() }

func (m *Model) agentFilterHotkeyLabel() string { return m.configuredHotkeys().Agent }

func validateAgentFilterHotkey(key string) error {
	m := Model{}
	return m.SetAgentFilterHotkey(key)
}

func validateConfigurableHotkey(key string) error {
	if sharedKeyBindingContains(key) || reservedAgentHotkeyContains(key) {
		return fmt.Errorf("key %q conflicts with an existing command", key)
	}
	if utf8.ValidString(key) && utf8.RuneCountInString(key) == 1 {
		r, _ := utf8.DecodeRuneInString(key)
		if unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return nil
		}
	}
	if key == "tab" || key == "shift+tab" {
		return nil
	}
	// Bubble Tea reports these Ctrl combinations using lowercase names.
	if len(key) == len("ctrl+a") && key[:5] == "ctrl+" && key[5] >= 'a' && key[5] <= 'z' {
		return nil
	}
	return fmt.Errorf("unsupported key %q; use a character, tab, shift+tab, or ctrl+letter", key)
}
