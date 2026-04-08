package task

import (
	"sort"
	"strings"
	"time"
)

// SortTasks orders tasks by start status, priority, due date, tag names and id.
// Started tasks are always placed before non-started ones. Tasks without a due
// date are placed after tasks with a due date. Overdue tasks are placed at the
// very top regardless of other properties.
//
// The sort order is:
// 1. Overdue tasks (oldest due date first)
// 2. Started tasks (not completed)
// 3. High priority tasks
// 4. Tasks with earlier due dates
// 5. Tasks sorted alphabetically by tags
// 6. Tasks sorted by ID (oldest first)
func SortTasks(tasks []Task) {
	now := time.Now()

	sort.Slice(tasks, func(i, j int) bool {
		ti, tj := tasks[i], tasks[j]

		if oi, oj := isOverdue(ti, now), isOverdue(tj, now); oi != oj {
			return oi
		}

		startedI := ti.Start != "" && ti.Status != "completed"
		startedJ := tj.Start != "" && tj.Status != "completed"
		if startedI != startedJ {
			return startedI
		}

		pi, pj := priorityRank(ti.Priority), priorityRank(tj.Priority)
		if pi != pj {
			return pi > pj
		}

		di, iok := parseDueDate(ti.Due)
		dj, jok := parseDueDate(tj.Due)
		if iok && !jok {
			return true
		}
		if !iok && jok {
			return false
		}
		if iok && jok && !di.Equal(dj) {
			return di.Before(dj)
		}

		tgI, tgJ := joinTags(ti.Tags), joinTags(tj.Tags)
		if tgI != tgJ {
			return tgI < tgJ
		}

		return ti.ID < tj.ID
	})
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cpy := append([]string(nil), tags...)
	sort.Strings(cpy)
	return strings.Join(cpy, " ")
}

func priorityRank(priority string) int {
	switch priority {
	case "H":
		return 3
	case "M":
		return 2
	case "L":
		return 1
	default:
		return 0
	}
}

func parseDueDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(DateFormat, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func isOverdue(task Task, now time.Time) bool {
	due, ok := parseDueDate(task.Due)
	return ok && now.After(due)
}
