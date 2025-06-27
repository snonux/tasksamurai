package task

import (
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	now := time.Now()
	tasks := []Task{
		{Description: "t1"},
		{Description: "t2", Start: "20240101T000000Z"},
		{Description: "t3", Due: now.Add(-time.Hour).UTC().Format("20060102T150405Z")},
		{Description: "t4", Start: "20240101T000000Z", Due: now.Add(-time.Hour).UTC().Format("20060102T150405Z")},
	}

	if TotalTasks(tasks) != 4 {
		t.Errorf("total tasks wrong: %d", TotalTasks(tasks))
	}

	if InProgressTasks(tasks) != 2 {
		t.Errorf("in progress wrong: %d", InProgressTasks(tasks))
	}

	if DueTasks(tasks, now) != 2 {
		t.Errorf("due tasks wrong: %d", DueTasks(tasks, now))
	}
}
