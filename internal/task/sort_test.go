package task

import (
	"reflect"
	"testing"
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
