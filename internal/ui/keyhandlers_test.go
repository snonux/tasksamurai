package ui

import "testing"

func TestSharedKeyBindingsHaveUsableMetadata(t *testing.T) {
	seen := map[string]struct{}{}
	for _, binding := range sharedKeyBindings {
		if len(binding.keys) == 0 {
			t.Fatalf("binding has no keys: %#v", binding)
		}
		if binding.modes == 0 {
			t.Fatalf("binding %v has no modes", binding.keys)
		}
		if binding.desc == "" {
			t.Fatalf("binding %v has no description", binding.keys)
		}
		if binding.action == nil {
			t.Fatalf("binding %v has no action", binding.keys)
		}

		for _, key := range binding.keys {
			if key == "" {
				t.Fatalf("binding %v contains an empty key", binding.keys)
			}
			if _, ok := seen[key]; ok {
				t.Fatalf("duplicate shared key binding for %q", key)
			}
			seen[key] = struct{}{}
		}
	}

	for _, key := range []string{"p", "+", "H", "space"} {
		if _, ok := seen[key]; !ok {
			t.Fatalf("shared key registry is missing %q", key)
		}
	}
}

func TestAgentFilterHotkeyValidationRejectsSharedKeyBindings(t *testing.T) {
	for _, binding := range sharedKeyBindings {
		for _, key := range binding.keys {
			if err := validateAgentFilterHotkey(key); err == nil {
				t.Fatalf("shared key binding %q can be set as the agent filter hotkey", key)
			}
		}
	}
}

func TestAgentFilterHotkeyRejectsCtrlRCaseVariants(t *testing.T) {
	for _, key := range []string{"ctrl+r", "Ctrl+R", "CTRL+R"} {
		t.Run(key, func(t *testing.T) {
			var m Model
			if err := m.SetAgentFilterHotkey(key); err == nil {
				t.Fatalf("expected collision for agent hotkey %q", key)
			}
			if err := validateAgentFilterHotkey(key); err == nil {
				t.Fatalf("expected direct validation collision for agent hotkey %q", key)
			}
			if got := m.agentFilterHotkeyLabel(); got != "3" {
				t.Fatalf("colliding hotkey changed label: got %q want %q", got, "3")
			}
		})
	}
}
