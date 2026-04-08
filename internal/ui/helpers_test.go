package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestParseTaskDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid date",
			input:   "20250627T150405Z",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "2025-06-27",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTaskDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTaskDate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatDueText(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "today",
			input:    now.UTC().Format("20060102T150405Z"),
			expected: "today",
		},
		{
			name:     "tomorrow",
			input:    now.Add(24 * time.Hour).UTC().Format("20060102T150405Z"),
			expected: "tomorrow",
		},
		{
			name:     "yesterday",
			input:    now.Add(-24 * time.Hour).UTC().Format("20060102T150405Z"),
			expected: "yesterday",
		},
		{
			name:     "future",
			input:    now.Add(5 * 24 * time.Hour).UTC().Format("20060102T150405Z"),
			expected: "5d",
		},
		{
			name:     "past",
			input:    now.Add(-3 * 24 * time.Hour).UTC().Format("20060102T150405Z"),
			expected: "-3d",
		},
		{
			name:     "invalid",
			input:    "invalid",
			expected: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDueText(tt.input)
			if got != tt.expected {
				t.Errorf("formatDueText() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSearchRegexCacheConcurrentAccess(t *testing.T) {
	searchRegexMu.Lock()
	searchRegexCache = make(map[string]*regexp.Regexp)
	searchRegexMu.Unlock()
	t.Cleanup(func() {
		searchRegexMu.Lock()
		searchRegexCache = make(map[string]*regexp.Regexp)
		searchRegexMu.Unlock()
	})

	patterns := []string{`alpha`, `beta`}
	var wg sync.WaitGroup
	errCh := make(chan error, 128)

	for i := 0; i < 16; i++ {
		for _, pattern := range patterns {
			wg.Add(1)
			go func(pattern string) {
				defer wg.Done()

				re, err := compileAndCacheRegex(pattern)
				if err != nil {
					errCh <- err
					return
				}
				if re == nil || !re.MatchString(pattern) {
					errCh <- fmt.Errorf("compiled regex for %q did not match", pattern)
					return
				}
				if cached, ok := cachedSearchRegex(pattern); !ok || cached == nil {
					errCh <- fmt.Errorf("missing cached regex for %q", pattern)
				}
			}(pattern)
		}
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateTagName(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{
			name:    "valid tag",
			tag:     "work",
			wantErr: false,
		},
		{
			name:    "valid with plus",
			tag:     "+work",
			wantErr: false,
		},
		{
			name:    "valid with minus",
			tag:     "-work",
			wantErr: false,
		},
		{
			name:    "empty tag",
			tag:     "",
			wantErr: true,
		},
		{
			name:    "tag with space",
			tag:     "my tag",
			wantErr: true,
		},
		{
			name:    "tag with tab",
			tag:     "my\ttag",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTagName(tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTagName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePriority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		wantErr  bool
	}{
		{
			name:     "high",
			priority: "H",
			wantErr:  false,
		},
		{
			name:     "medium",
			priority: "M",
			wantErr:  false,
		},
		{
			name:     "low",
			priority: "L",
			wantErr:  false,
		},
		{
			name:     "empty",
			priority: "",
			wantErr:  false,
		},
		{
			name:     "invalid",
			priority: "X",
			wantErr:  true,
		},
		{
			name:     "lowercase",
			priority: "h",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePriority(tt.priority)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePriority() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name    string
		desc    string
		wantErr bool
	}{
		{
			name:    "valid description",
			desc:    "Fix the bug",
			wantErr: false,
		},
		{
			name:    "empty description",
			desc:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			desc:    "   ",
			wantErr: true,
		},
		{
			name:    "description with whitespace",
			desc:    "  Fix the bug  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDescription(tt.desc)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDescription() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleTextInputKeepsStateOnEnterError(t *testing.T) {
	input := textinput.New()
	input.SetValue("value")
	input.Focus()

	m := Model{windowHeight: 20}
	called := false

	mv, cmd := (&m).handleTextInput(tea.KeyPressMsg{Code: tea.KeyEnter}, &input, func(string) error {
		return fmt.Errorf("boom")
	}, func() {
		called = true
	})
	m = *mv.(*Model)

	if cmd == nil {
		t.Fatalf("expected clear-status command on enter error")
	}
	if called {
		t.Fatalf("onExit should not run on enter error")
	}
	if !input.Focused() {
		t.Fatalf("input should stay focused on enter error")
	}
	if !strings.Contains(m.statusMsg, "boom") {
		t.Fatalf("unexpected status message: %q", m.statusMsg)
	}
}

func TestActivateDueEditFallsBackToNowOnInvalidDate(t *testing.T) {
	m := Model{windowHeight: 20}
	before := time.Now().Add(-time.Second)

	m.activateDueEdit(7, "not-a-date")

	if !m.dueEditing {
		t.Fatalf("due editing was not enabled")
	}
	if m.dueID != 7 {
		t.Fatalf("due ID = %d, want 7", m.dueID)
	}
	after := time.Now().Add(time.Second)
	if m.dueDate.Before(before) || m.dueDate.After(after) {
		t.Fatalf("due date fallback was not based on now: %v", m.dueDate)
	}
}

func TestValidateDueDate(t *testing.T) {
	tests := []struct {
		name    string
		due     string
		wantErr bool
	}{
		{
			name:    "empty",
			due:     "",
			wantErr: false,
		},
		{
			name:    "ISO date",
			due:     "2025-06-27",
			wantErr: false,
		},
		{
			name:    "ISO datetime",
			due:     "2025-06-27T15:04:05",
			wantErr: false,
		},
		{
			name:    "ISO datetime with Z",
			due:     "2025-06-27T15:04:05Z",
			wantErr: false,
		},
		{
			name:    "taskwarrior format",
			due:     "20250627T150405Z",
			wantErr: false,
		},
		{
			name:    "relative - today",
			due:     "today",
			wantErr: false,
		},
		{
			name:    "relative - tomorrow",
			due:     "tomorrow",
			wantErr: false,
		},
		{
			name:    "relative - monday",
			due:     "monday",
			wantErr: false,
		},
		{
			name:    "relative - eod",
			due:     "eod",
			wantErr: false,
		},
		{
			name:    "relative - tomorrow+2d",
			due:     "tomorrow+2d",
			wantErr: false,
		},
		{
			name:    "invalid format",
			due:     "27/06/2025",
			wantErr: true,
		},
		{
			name:    "invalid relative",
			due:     "someday",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDueDate(tt.due)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDueDate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRecurrence(t *testing.T) {
	tests := []struct {
		name    string
		recur   string
		wantErr bool
	}{
		{
			name:    "empty",
			recur:   "",
			wantErr: false,
		},
		{
			name:    "daily",
			recur:   "daily",
			wantErr: false,
		},
		{
			name:    "weekly",
			recur:   "weekly",
			wantErr: false,
		},
		{
			name:    "3 days",
			recur:   "3d",
			wantErr: false,
		},
		{
			name:    "2 weeks",
			recur:   "2w",
			wantErr: false,
		},
		{
			name:    "1 month",
			recur:   "1m",
			wantErr: false,
		},
		{
			name:    "too short",
			recur:   "d",
			wantErr: true,
		},
		{
			name:    "single char",
			recur:   "x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecurrence(tt.recur)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRecurrence() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseFilterInput verifies that parseFilterInput correctly handles
// taskwarrior filter expressions, including attribute filters (proj:xxx),
// tag filters (+tag), quoted values (description:"some text"), and empty input.
func TestParseFilterInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty input clears filter",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "project attribute filter",
			input:   "proj:dtail",
			want:    []string{"proj:dtail"},
			wantErr: false,
		},
		{
			name:    "tag filter",
			input:   "+urgent",
			want:    []string{"+urgent"},
			wantErr: false,
		},
		{
			name:    "multiple filters",
			input:   "proj:dtail +urgent",
			want:    []string{"proj:dtail", "+urgent"},
			wantErr: false,
		},
		{
			name:  "quoted description filter keeps value as single token",
			input: `description:"my task"`,
			// shlex strips the quotes and keeps the value as one argument
			want:    []string{"description:my task"},
			wantErr: false,
		},
		{
			name:  "project filter with multiple words via quoting",
			input: `project:"my project"`,
			want:  []string{"project:my project"},
		},
		{
			name:    "status filter",
			input:   "status:pending",
			want:    []string{"status:pending"},
			wantErr: false,
		},
		{
			name:    "whitespace-only input clears filter",
			input:   "   ",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "unclosed quote returns error",
			input:   `proj:"unclosed`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilterInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFilterInput(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFilterInput(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
