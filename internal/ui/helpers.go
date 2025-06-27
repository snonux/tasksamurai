package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Date format used by Taskwarrior
const taskDateFormat = "20060102T150405Z"

// parseTaskDate parses a date string in Taskwarrior format
func parseTaskDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}
	return time.Parse(taskDateFormat, dateStr)
}

// formatTaskDate formats a time as a Taskwarrior date string
func formatTaskDate(t time.Time) string {
	return t.UTC().Format(taskDateFormat)
}

// daysSince returns the number of days since the given time
func daysSince(t time.Time) int {
	return int(time.Since(t).Hours() / 24)
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
	
	// Limit cache size to prevent memory leak
	if len(searchRegexCache) > 100 {
		// Clear cache when it gets too large
		searchRegexCache = make(map[string]*regexp.Regexp)
	}
	searchRegexCache[pattern] = re
	
	return re, nil
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

// validateTags validates a list of tags
func validateTags(tags []string) error {
	for _, tag := range tags {
		if err := validateTagName(tag); err != nil {
			return fmt.Errorf("invalid tag '%s': %w", tag, err)
		}
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