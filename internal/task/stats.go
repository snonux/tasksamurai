package task

import "time"

// TotalTasks returns the number of tasks provided.
func TotalTasks(tasks []Task) int {
	return len(tasks)
}

// InProgressTasks returns the number of tasks that have been started and are not completed.
func InProgressTasks(tasks []Task) int {
	count := 0
	for _, t := range tasks {
		if t.Status == "completed" {
			continue
		}
		if t.Start != "" {
			count++
		}
	}
	return count
}

// DueTasks returns the number of tasks with a due date that is not in the future.
func DueTasks(tasks []Task, now time.Time) int {
	count := 0
	for _, t := range tasks {
		if t.Status == "completed" || t.Due == "" {
			continue
		}
		ts, err := time.Parse("20060102T150405Z", t.Due)
		if err != nil {
			continue
		}
		if !ts.After(now) {
			count++
		}
	}
	return count
}
