package task

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/shlex"
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

var debugWriter io.Writer
var debugFile *os.File // Track the file handle to close it properly

// SetDebugLog enables logging of executed commands to the given file.
// Passing an empty path disables logging.
func SetDebugLog(path string) error {
	// Close existing debug file if open
	if debugFile != nil {
		debugFile.Close()
		debugFile = nil
		debugWriter = nil
	}

	if path == "" {
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	debugFile = f
	debugWriter = f
	return nil
}

// Add creates a new task with the given description and tags.
func Add(description string, tags []string) error {
	var args []string
	for _, t := range tags {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	args = append(args, description)
	return AddArgs(args)
}

// AddArgs runs "task add" with the provided arguments. Each element in args
// is passed as a separate command-line argument, allowing the caller to
// specify additional modifiers like due dates or tags.
func AddArgs(args []string) error {
	return run(append([]string{"add"}, args...)...)
}

// AddLine splits the given line into shell words and runs "task add" with the
// resulting arguments. This allows users to pass raw Taskwarrior parameters
// such as "due:today" directly.
func AddLine(line string) error {
	fields, err := shlex.Split(line)
	if err != nil {
		return err
	}
	return AddArgs(fields)
}

// Export retrieves all tasks using `task export rc.json.array=off` and parses
// the JSON output into a slice of Task structs.
// Export retrieves tasks using `task <filter> export rc.json.array=off` and parses
// the JSON output into a slice of Task structs. Optional filter arguments are
// passed directly to the `task` command before `export`.
func Export(filters ...string) ([]Task, error) {
	args := append(filters, "export", "rc.json.array=off")
	cmd := exec.Command("task", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		// Include stderr output in the error message
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
		}
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
	if debugWriter != nil {
		fmt.Fprintln(debugWriter, "task "+strings.Join(args, " "))
	}
	cmd := exec.Command("task", args...)

	// Capture stderr to provide better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr output in the error message
		if stderr.Len() > 0 {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

// SetStatus changes the status of the task with the given id.
func SetStatus(id int, status string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "modify", "status:"+status)
}

// SetStatusUUID changes the status of the task with the given UUID.
func SetStatusUUID(uuid, status string) error {
	return run(uuid, "modify", "status:"+status)
}

// Start begins the task with the given id.
func Start(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "start")
}

// Stop stops the task with the given id.
func Stop(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "stop")
}

// Done marks the task with the given id as completed.
func Done(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "done")
}

// Delete removes the task with the given id.
func Delete(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "delete")
}

// SetPriority changes the priority of the task with the given id.
func SetPriority(id int, priority string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "modify", "priority:"+priority)
}

// AddTags adds tags to the task with the given id.
func AddTags(id int, tags []string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
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
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	args := []string{strconv.Itoa(id), "modify"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '-' {
			t = "-" + t
		}
		args = append(args, t)
	}
	return run(args...)
}

// SetTags sets the tags of the task with the given id to exactly the provided set.
// Tags not present will be removed and new tags added as needed.
func SetTags(id int, tags []string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	tasks, err := Export(strconv.Itoa(id))
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	current := make(map[string]struct{})
	for _, t := range tasks[0].Tags {
		current[t] = struct{}{}
	}
	desired := make(map[string]struct{})
	for _, t := range tags {
		desired[t] = struct{}{}
	}

	var adds, removes []string
	for t := range desired {
		if _, ok := current[t]; !ok {
			adds = append(adds, t)
		}
	}
	for t := range current {
		if _, ok := desired[t]; !ok {
			removes = append(removes, t)
		}
	}

	if len(adds) > 0 {
		if err := AddTags(id, adds); err != nil {
			return err
		}
	}
	if len(removes) > 0 {
		if err := RemoveTags(id, removes); err != nil {
			return err
		}
	}
	return nil
}

// SetRecurrence sets the recurrence for the task with the given id.
func SetRecurrence(id int, rec string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "modify", "recur:"+rec)
}

// SetDueDate sets the due date for the task with the given id.
func SetDueDate(id int, due string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "modify", "due:"+due)
}

// SetDescription changes the description of the task with the given id.
func SetDescription(id int, desc string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "modify", "description:"+desc)
}

// Annotate adds an annotation to the task with the given id.
func Annotate(id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), "annotate", text)
}

// Denotate removes an annotation from the task with the given id.
// Denotate removes an annotation from the task with the given id. The
// annotation text is matched exactly when provided. If text is empty, the
// oldest annotation is removed.
func Denotate(id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	args := []string{strconv.Itoa(id), "denotate"}
	if text != "" {
		args = append(args, text)
	}
	return run(args...)
}

// ReplaceAnnotations removes all existing annotations from the task with the
// given id and sets a single annotation with the provided text. If text is
// empty, all annotations are simply removed.
func ReplaceAnnotations(id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	tasks, err := Export(strconv.Itoa(id))
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	anns := tasks[0].Annotations
	for i := len(anns) - 1; i >= 0; i-- {
		if err := Denotate(id, anns[i].Description); err != nil {
			return err
		}
	}
	if text == "" {
		return nil
	}
	return Annotate(id, text)
}

// Edit opens the task in an editor for manual modification.
// EditCmd returns an exec.Cmd that edits the task with the given id.
// The caller is responsible for running the command, typically via
// tea.ExecProcess so that the terminal state is properly managed.
func EditCmd(id int) *exec.Cmd {
	if id <= 0 {
		// Return a command that will fail with an appropriate error
		cmd := exec.Command("sh", "-c", fmt.Sprintf("echo 'invalid task ID: %d' >&2; exit 1", id))
		return cmd
	}
	cmd := exec.Command("task", strconv.Itoa(id), "edit")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// Edit opens the task in an editor for manual modification.
// This is a convenience wrapper around EditCmd.
func Edit(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return EditCmd(id).Run()
}

// SortTasks orders tasks by start status, priority, due date, tag names and id.
// Started tasks are always placed before non-started ones. Tasks without a due
// date are placed after tasks with a due date. Overdue tasks are placed at the
// very top regardless of other properties.
func SortTasks(tasks []Task) {
	joinTags := func(tags []string) string {
		if len(tags) == 0 {
			return ""
		}
		cpy := append([]string(nil), tags...)
		sort.Strings(cpy)
		return strings.Join(cpy, " ")
	}

	priVal := func(p string) int {
		switch p {
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

	parseDue := func(s string) (time.Time, bool) {
		if s == "" {
			return time.Time{}, false
		}
		t, err := time.Parse("20060102T150405Z", s)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}

	overdue := func(t Task) bool {
		du, ok := parseDue(t.Due)
		return ok && time.Now().After(du)
	}

	sort.Slice(tasks, func(i, j int) bool {
		ti, tj := tasks[i], tasks[j]

		if oi, oj := overdue(ti), overdue(tj); oi != oj {
			return oi
		}

		startedI := ti.Start != "" && ti.Status != "completed"
		startedJ := tj.Start != "" && tj.Status != "completed"
		if startedI != startedJ {
			return startedI
		}

		pi, pj := priVal(ti.Priority), priVal(tj.Priority)
		if pi != pj {
			return pi > pj
		}

		di, iok := parseDue(ti.Due)
		dj, jok := parseDue(tj.Due)
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
