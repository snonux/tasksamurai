package task

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Task represents a taskwarrior task as returned by `task export`.
type Task struct {
	ID          int          `json:"id"`
	UUID        string       `json:"uuid"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	Status      string       `json:"status"`
	Priority    string       `json:"priority,omitempty"`
	Due         string       `json:"due,omitempty"`
	Recur       string       `json:"recur,omitempty"`
	Start       string       `json:"start,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Annotation represents a taskwarrior annotation entry.
type Annotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

// Add creates a new task with the given description and tags.
func Add(description string, tags []string) error {
	args := []string{"add"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	args = append(args, description)

	cmd := exec.Command("task", args...)
	return cmd.Run()
}

// Export retrieves all tasks using `task export rc.json.array=off` and parses
// the JSON output into a slice of Task structs.
func Export() ([]Task, error) {
	cmd := exec.Command("task", "export", "rc.json.array=off")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var tasks []Task
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var t Task
		if err := json.Unmarshal(line, &t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// SetActive sets the task active (start) or inactive (stop).
func SetActive(id int, active bool) error {
	action := "stop"
	if active {
		action = "start"
	}
	cmd := exec.Command("task", fmt.Sprint(id), action)
	return cmd.Run()
}

// SetPriority changes the priority of a task. Use empty string to clear.
func SetPriority(id int, priority string) error {
	arg := fmt.Sprintf("priority:%s", priority)
	cmd := exec.Command("task", fmt.Sprint(id), "modify", arg)
	return cmd.Run()
}

// ChangeTags adds and removes tags on a task.
func ChangeTags(id int, add []string, remove []string) error {
	args := []string{fmt.Sprint(id), "modify"}
	for _, t := range add {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	for _, t := range remove {
		if len(t) > 0 && t[0] != '-' {
			t = "-" + t
		}
		args = append(args, t)
	}
	cmd := exec.Command("task", args...)
	return cmd.Run()
}

// SetRecurrence updates the recurrence rule for a task. Empty string clears it.
func SetRecurrence(id int, recur string) error {
	arg := fmt.Sprintf("recur:%s", recur)
	cmd := exec.Command("task", fmt.Sprint(id), "modify", arg)
	return cmd.Run()
}

// SetDue updates the due date of a task. Empty string clears it.
func SetDue(id int, due string) error {
	arg := fmt.Sprintf("due:%s", due)
	cmd := exec.Command("task", fmt.Sprint(id), "modify", arg)
	return cmd.Run()
}

// SetDescription updates the description of a task.
func SetDescription(id int, desc string) error {
	cmd := exec.Command("task", fmt.Sprint(id), "modify", desc)
	return cmd.Run()
}

// Annotate adds an annotation to a task.
func Annotate(id int, note string) error {
	cmd := exec.Command("task", fmt.Sprint(id), "annotate", note)
	return cmd.Run()
}

// Edit opens the task in an external editor.
func Edit(id int) error {
	cmd := exec.Command("task", fmt.Sprint(id), "edit")
	return cmd.Run()
}
