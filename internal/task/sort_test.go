package task

import (
	"reflect"
	"testing"
	"time"
)

func TestSortTasks(t *testing.T) {
	tasks := []Task{
		{ID: 2, Due: "20240102T000000Z", Priority: "M", Tags: []string{"b", "a"}},
		{ID: 1, Due: "20240101T000000Z", Priority: "H", Tags: []string{"a"}},
		{ID: 3, Due: "", Priority: "", Tags: []string{"c"}},
		{ID: 4, Due: "20240101T000000Z", Priority: "L", Tags: []string{"a"}},
		{ID: 5, Due: "20240101T000000Z", Priority: "H", Tags: []string{"b"}},
		{ID: 6, Due: "20240101T000000Z", Priority: "H", Tags: []string{"b"}},
	}

	SortTasks(tasks)

	var ids []int
	for _, tsk := range tasks {
		ids = append(ids, tsk.ID)
	}
	want := []int{1, 5, 6, 2, 4, 3}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestSortTasksStartedFirst(t *testing.T) {
	tasks := []Task{
		{ID: 1, Priority: "M", Start: "20240101T000000Z"},
		{ID: 2, Priority: "H"},
		{ID: 3, Priority: "H", Start: "20240102T000000Z"},
		{ID: 4, Priority: "L"},
	}

	SortTasks(tasks)

	var ids []int
	for _, tsk := range tasks {
		ids = append(ids, tsk.ID)
	}
	want := []int{3, 1, 2, 4}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestSortTasksOverdueFirst(t *testing.T) {
	tasks := []Task{
		{ID: 1},
		{ID: 2, Due: "20200101T000000Z"},
		{ID: 3},
	}

	SortTasks(tasks)

	var ids []int
	for _, tsk := range tasks {
		ids = append(ids, tsk.ID)
	}
	want := []int{2, 1, 3}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestJoinTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{name: "empty", tags: nil, want: ""},
		{name: "single", tags: []string{"alpha"}, want: "alpha"},
		{name: "sorted copy", tags: []string{"bravo", "alpha"}, want: "alpha bravo"},
		{name: "duplicate values", tags: []string{"b", "a", "b"}, want: "a b b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinTags(tt.tags); got != tt.want {
				t.Fatalf("joinTags(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}

func TestPriorityRank(t *testing.T) {
	tests := []struct {
		priority string
		want     int
	}{
		{priority: "H", want: 3},
		{priority: "M", want: 2},
		{priority: "L", want: 1},
		{priority: "", want: 0},
		{priority: "unexpected", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			if got := priorityRank(tt.priority); got != tt.want {
				t.Fatalf("priorityRank(%q) = %d, want %d", tt.priority, got, tt.want)
			}
		})
	}
}

func TestParseDueDate(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid", value: "20240101T000000Z", want: true},
		{name: "empty", value: "", want: false},
		{name: "invalid", value: "not-a-date", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotOK := parseDueDate(tt.value)
			if gotOK != tt.want {
				t.Fatalf("parseDueDate(%q) ok = %v, want %v", tt.value, gotOK, tt.want)
			}
			if tt.want {
				wantTime, err := time.Parse(DateFormat, tt.value)
				if err != nil {
					t.Fatalf("test setup error parsing %q: %v", tt.value, err)
				}
				if !gotTime.Equal(wantTime) {
					t.Fatalf("parseDueDate(%q) time = %v, want %v", tt.value, gotTime, wantTime)
				}
				return
			}
			if !gotTime.IsZero() {
				t.Fatalf("parseDueDate(%q) returned non-zero time: %v", tt.value, gotTime)
			}
		})
	}
}

func TestIsOverdue(t *testing.T) {
	now := time.Date(2024, time.January, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		task Task
		want bool
	}{
		{name: "past due", task: Task{Due: "20240101T000000Z"}, want: true},
		{name: "future due", task: Task{Due: "20240103T000000Z"}, want: false},
		{name: "invalid due", task: Task{Due: "bad-date"}, want: false},
		{name: "empty due", task: Task{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOverdue(tt.task, now); got != tt.want {
				t.Fatalf("isOverdue(%+v, %v) = %v, want %v", tt.task, now, got, tt.want)
			}
		})
	}
}
