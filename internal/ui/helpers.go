package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/shlex"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// taskDateFormat aliases task.DateFormat for use within this package.
// It is kept as a package-level constant so internal helpers don't need
// to qualify every parse/format call with the package name.
const taskDateFormat = task.DateFormat

// parseTaskDate parses a date string in Taskwarrior format
func parseTaskDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}
	return time.Parse(taskDateFormat, dateStr)
}

// daysUntil returns the number of days until the given time
func daysUntil(t time.Time) int {
	now := time.Now()
	// Normalize both times to midnight UTC to avoid timezone and fractional day issues
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	target := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return int(target.Sub(today).Hours() / 24)
}

// formatDueText returns a human-readable due date string
func formatDueText(dueStr string) string {
	if dueStr == "" {
		return ""
	}

	ts, err := parseTaskDate(dueStr)
	if err != nil {
		return dueStr
	}

	days := daysUntil(ts)
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	case -1:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd", days)
	}
}

// compileAndCacheRegex compiles a regex and adds it to the cache
func compileAndCacheRegex(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	storeSearchRegex(pattern, re)
	return re, nil
}

// cachedSearchRegex returns a compiled regex from the cache if present.
func cachedSearchRegex(pattern string) (*regexp.Regexp, bool) {
	searchRegexMu.RLock()
	re, ok := searchRegexCache[pattern]
	searchRegexMu.RUnlock()
	return re, ok
}

// storeSearchRegex records a compiled regex in the cache.
func storeSearchRegex(pattern string, re *regexp.Regexp) {
	searchRegexMu.Lock()
	defer searchRegexMu.Unlock()

	// Limit cache size to prevent memory leak.
	if len(searchRegexCache) > 100 {
		searchRegexCache = make(map[string]*regexp.Regexp)
	}
	searchRegexCache[pattern] = re
}

// parseFilterInput splits a raw filter string typed by the user into the
// individual filter tokens that are passed to taskwarrior. Shell-quoting
// rules are applied via shlex so that expressions like
//
//	description:"my task"
//	proj:dtail +urgent
//
// are handled correctly: quoted values are kept as a single argument (with the
// quotes stripped) rather than being split on whitespace. An empty input
// returns a nil slice, which clears the current filter.
func parseFilterInput(input string) ([]string, error) {
	fields, err := shlex.Split(input)
	if err != nil {
		return nil, fmt.Errorf("invalid filter expression: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}

// Validation functions

// validateTagName validates a tag name
func validateTagName(tag string) error {
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}

	// Remove leading + or - for validation
	tag = strings.TrimPrefix(strings.TrimPrefix(tag, "+"), "-")

	// Check for invalid characters
	if strings.ContainsAny(tag, " \t\n\r") {
		return fmt.Errorf("tag cannot contain whitespace")
	}

	return nil
}

// validateDueDate validates a due date string
func validateDueDate(due string) error {
	if due == "" {
		return nil // Empty due date is valid
	}

	// Try common formats
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		taskDateFormat,
	}

	for _, format := range formats {
		if _, err := time.Parse(format, due); err == nil {
			return nil
		}
	}

	// Check for relative dates that taskwarrior understands
	relatives := []string{"now", "today", "tomorrow", "yesterday", "monday", "tuesday",
		"wednesday", "thursday", "friday", "saturday", "sunday", "eod", "eow", "eom", "eoy"}

	due = strings.ToLower(due)
	for _, rel := range relatives {
		if due == rel || strings.HasPrefix(due, rel+"+") || strings.HasPrefix(due, rel+"-") {
			return nil
		}
	}

	return fmt.Errorf("invalid due date format: %s", due)
}

// validatePriority validates a priority value
func validatePriority(priority string) error {
	switch priority {
	case "", "H", "M", "L":
		return nil
	default:
		return fmt.Errorf("invalid priority: %s (must be H, M, L, or empty)", priority)
	}
}

// validateRecurrence validates a recurrence string
func validateRecurrence(recur string) error {
	if recur == "" {
		return nil // Empty recurrence is valid
	}

	// Basic validation - taskwarrior will do the full validation
	if len(recur) < 2 {
		return fmt.Errorf("recurrence too short")
	}

	// Check for common patterns
	validPrefixes := []string{"daily", "weekly", "monthly", "yearly", "biweekly", "bimonthly"}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(strings.ToLower(recur), prefix) {
			return nil
		}
	}

	// Check for duration format (e.g., "3d", "2w", "1m")
	if len(recur) >= 2 {
		last := recur[len(recur)-1]
		if (last == 'd' || last == 'w' || last == 'm' || last == 'y') &&
			recur[:len(recur)-1] != "" {
			return nil
		}
	}

	return nil // Let taskwarrior handle complex validation
}

// validateDescription validates a task description
func validateDescription(desc string) error {
	if strings.TrimSpace(desc) == "" {
		return fmt.Errorf("description cannot be empty")
	}
	return nil
}
