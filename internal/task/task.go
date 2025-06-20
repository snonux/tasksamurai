package task

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
)

// Task represents a taskwarrior task as returned by `task export`.
type Annotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type Task struct {
	ID          int          `json:"id"`
	UUID        string       `json:"uuid"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	Status      string       `json:"status"`
	Start       string       `json:"start"`
	Entry       string       `json:"entry"`
	Due         string       `json:"due"`
	Priority    string       `json:"priority"`
	Recur       string       `json:"recur"`
	Urgency     float64      `json:"urgency"`
	Annotations []Annotation `json:"annotations"`
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
// Export retrieves tasks using `task <filter> export rc.json.array=off` and parses
// the JSON output into a slice of Task structs. Optional filter arguments are
// passed directly to the `task` command before `export`.
func Export(filters ...string) ([]Task, error) {
	args := append(filters, "export", "rc.json.array=off")
	cmd := exec.Command("task", args...)
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

func run(args ...string) error {
	cmd := exec.Command("task", args...)
	return cmd.Run()
}

// SetStatus changes the status of the task with the given id.
func SetStatus(id int, status string) error {
	return run(strconv.Itoa(id), "modify", "status:"+status)
}

// SetPriority changes the priority of the task with the given id.
func SetPriority(id int, priority string) error {
	return run(strconv.Itoa(id), "modify", "priority:"+priority)
}

// AddTags adds tags to the task with the given id.
func AddTags(id int, tags []string) error {
	args := []string{strconv.Itoa(id), "modify"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	return run(args...)
}

// RemoveTags removes tags from the task with the given id.
func RemoveTags(id int, tags []string) error {
	args := []string{strconv.Itoa(id), "modify"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '-' {
			t = "-" + t
		}
		args = append(args, t)
	}
	return run(args...)
}

// SetRecurrence sets the recurrence for the task with the given id.
func SetRecurrence(id int, rec string) error {
	return run(strconv.Itoa(id), "modify", "recur:"+rec)
}

// SetDueDate sets the due date for the task with the given id.
func SetDueDate(id int, due string) error {
	return run(strconv.Itoa(id), "modify", "due:"+due)
}

// SetDescription changes the description of the task with the given id.
func SetDescription(id int, desc string) error {
	return run(strconv.Itoa(id), "modify", "description:"+desc)
}

// Annotate adds an annotation to the task with the given id.
func Annotate(id int, text string) error {
	return run(strconv.Itoa(id), "annotate", text)
}

// Denotate removes an annotation from the task with the given id.
func Denotate(id int, annoID int) error {
	return run(strconv.Itoa(id), "denotate", strconv.Itoa(annoID))
}

// Edit opens the task in an editor for manual modification.
// EditCmd returns an exec.Cmd that edits the task with the given id.
// The caller is responsible for running the command, typically via
// tea.ExecProcess so that the terminal state is properly managed.
func EditCmd(id int) *exec.Cmd {
	cmd := exec.Command("task", strconv.Itoa(id), "edit")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// Edit opens the task in an editor for manual modification.
// This is a convenience wrapper around EditCmd.
func Edit(id int) error {
	return EditCmd(id).Run()
}
