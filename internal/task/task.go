package task

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/shlex"
)

// DateFormat is the date format used by Taskwarrior in all date fields
// (e.g. Entry, Due, Start). All date parsing and formatting in this
// package uses this constant.
const DateFormat = "20060102T150405Z"

// Task represents a taskwarrior task as returned by `task export`.
type Annotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type Task struct {
	ID          int          `json:"id"`
	UUID        string       `json:"uuid"`
	Description string       `json:"description"`
	Project     string       `json:"project"`
	Tags        []string     `json:"tags"`
	Status      string       `json:"status"`
	Start       string       `json:"start"`
	Entry       string       `json:"entry"`
	Due         string       `json:"due"`
	Priority    string       `json:"priority"`
	Recur       string       `json:"recur"`
	Parent      string       `json:"parent"`
	RType       string       `json:"rtype"`
	Urgency     float64      `json:"urgency"`
	Annotations []Annotation `json:"annotations"`
}

// RunResult contains the captured output from a task command invocation.
type RunResult struct {
	Args   []string
	Stdout string
	Stderr string
}

// CompletionSources contains values used for Taskwarrior shell completion.
type CompletionSources struct {
	Commands []string
	Columns  []string
	Projects []string
	Tags     []string
	IDs      []string
	UUIDs    []string
	UDAs     []string
}

func run(args ...string) error {
	return runContext(context.Background(), args...)
}

func runContext(ctx context.Context, args ...string) error {
	_, err := RunArgs(ctx, args)
	return err
}

// modifyTask runs a modify command with validation
func modifyTask(id int, args ...string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(append([]string{strconv.Itoa(id), "modify"}, args...)...)
}

// simpleTaskCommand runs a simple command on a task with validation
func simpleTaskCommand(id int, command string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return run(strconv.Itoa(id), command)
}

// debugConfig groups the optional debug-logging state for the task package.
// Collecting related vars into a struct makes the mutable state explicit and
// allows the logger to be swapped or reset cleanly without touching unrelated
// package globals.
type debugConfig struct {
	writer io.Writer
	file   *os.File // tracked separately so it can be closed on reconfiguration
}

// dbg holds the active debug-logging configuration for this package.
// It is written only via SetDebugLog and read only in run().
var dbg debugConfig

// SetDebugLog enables logging of executed commands to the given file.
// Passing an empty path disables logging and closes any previously opened file.
func SetDebugLog(path string) error {
	// Close existing debug file if open before re-configuring.
	if dbg.file != nil {
		_ = dbg.file.Close()
		dbg.file = nil
		dbg.writer = nil
	}

	if path == "" {
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	dbg.file = f
	dbg.writer = f
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

// RunLine splits line using shell-word rules and runs the resulting task
// arguments. A leading "task" token is ignored so callers may accept either
// "add foo" or "task add foo" from user input.
func RunLine(ctx context.Context, line string) (RunResult, error) {
	fields, err := shlex.Split(line)
	if err != nil {
		return RunResult{}, err
	}
	if len(fields) > 0 && fields[0] == "task" {
		fields = fields[1:]
	}
	return RunArgs(ctx, fields)
}

// RunShellLine runs a user-entered task command in non-interactive mode. It
// avoids Taskwarrior's recurring-task prompt by applying the same behavior as
// answering "no": modify only the addressed recurrence.
func RunShellLine(ctx context.Context, line string) (RunResult, error) {
	fields, err := shlex.Split(line)
	if err != nil {
		return RunResult{}, err
	}
	if len(fields) > 0 && fields[0] == "task" {
		fields = fields[1:]
	}
	fields = append([]string{"rc.recurrence.confirmation=no"}, fields...)
	return RunArgs(ctx, fields)
}

// RunArgs runs "task" with args and captures stdout and stderr.
func RunArgs(ctx context.Context, args []string) (RunResult, error) {
	copied := append([]string(nil), args...)
	result := RunResult{Args: copied}
	if len(copied) == 0 {
		return result, fmt.Errorf("empty task command")
	}

	if dbg.writer != nil {
		if _, err := fmt.Fprintln(dbg.writer, "task "+strings.Join(copied, " ")); err != nil {
			return result, fmt.Errorf("write debug log: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, "task", copied...)
	configureCommandContext(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, fmt.Errorf("task command: %w", ctxErr)
		}
		if strings.TrimSpace(result.Stderr) != "" {
			return result, fmt.Errorf("%v: %s", err, strings.TrimSpace(result.Stderr))
		}
		return result, err
	}
	return result, nil
}

// LoadCompletionSources returns Taskwarrior-provided completion candidates.
func LoadCompletionSources(ctx context.Context) CompletionSources {
	return CompletionSources{
		Commands: completionList(ctx, "_commands"),
		Columns:  completionList(ctx, "_columns"),
		Projects: completionList(ctx, "_projects"),
		Tags:     completionList(ctx, "_tags"),
		IDs:      completionList(ctx, "_ids"),
		UUIDs:    completionList(ctx, "_uuids"),
		UDAs:     completionList(ctx, "_udas"),
	}
}

func completionList(ctx context.Context, command string) []string {
	result, err := RunArgs(ctx, []string{command})
	if err != nil {
		return nil
	}
	return outputLines(result.Stdout)
}

func outputLines(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	seen := make(map[string]struct{})
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	return lines
}

// Export retrieves tasks using `task <filter> export rc.json.array=off` and parses
// the JSON output into a slice of Task structs. Optional filter arguments are
// passed directly to the `task` command before `export`.
func Export(ctx context.Context, filters ...string) ([]Task, error) {
	args := append(filters, "export", "rc.json.array=off")
	cmd := exec.CommandContext(ctx, "task", args...)
	configureCommandContext(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("task export: %w", ctxErr)
		}
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

// SetStatus changes the status of the task with the given id.
func SetStatus(id int, status string) error {
	return modifyTask(id, "status:"+status)
}

// SetStatusUUID changes the status of the task with the given UUID.
func SetStatusUUID(uuid, status string) error {
	return run(uuid, "modify", "status:"+status)
}

// RecurringSeries returns the recurring template and generated instances for
// the recurring task identified by rootUUID.
func RecurringSeries(ctx context.Context, rootUUID string) ([]Task, error) {
	if strings.TrimSpace(rootUUID) == "" {
		return nil, fmt.Errorf("empty recurring task UUID")
	}
	return Export(ctx, fmt.Sprintf("(%s or parent:%s)", rootUUID, rootUUID), "status.any:")
}

// Start begins the task with the given id.
func Start(id int) error {
	return simpleTaskCommand(id, "start")
}

// Stop stops the task with the given id.
func Stop(id int) error {
	return simpleTaskCommand(id, "stop")
}

// Done marks the task with the given id as completed.
func Done(id int) error {
	return simpleTaskCommand(id, "done")
}

// Delete removes the task with the given id.
func Delete(id int) error {
	return simpleTaskCommand(id, "delete")
}

// SetPriority changes the priority of the task with the given id.
func SetPriority(id int, priority string) error {
	return modifyTask(id, "priority:"+priority)
}

// AddTags adds tags to the task with the given id.
func AddTags(id int, tags []string) error {
	return AddTagsContext(context.Background(), id, tags)
}

// AddTagsContext adds tags to the task with the given id using ctx for the
// underlying Taskwarrior command.
func AddTagsContext(ctx context.Context, id int, tags []string) error {
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
	return runContext(ctx, args...)
}

// RemoveTags removes tags from the task with the given id.
func RemoveTags(id int, tags []string) error {
	return RemoveTagsContext(context.Background(), id, tags)
}

// RemoveTagsContext removes tags from the task with the given id using ctx for
// the underlying Taskwarrior command.
func RemoveTagsContext(ctx context.Context, id int, tags []string) error {
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
	return runContext(ctx, args...)
}

// SetTags sets the tags of the task with the given id to exactly the provided set.
// Tags not present will be removed and new tags added as needed.
func SetTags(ctx context.Context, id int, tags []string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	tasks, err := Export(ctx, strconv.Itoa(id))
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
		if err := AddTagsContext(ctx, id, adds); err != nil {
			return err
		}
	}
	if len(removes) > 0 {
		if err := RemoveTagsContext(ctx, id, removes); err != nil {
			return err
		}
	}
	return nil
}

// SetRecurrence sets the recurrence for the task with the given id.
func SetRecurrence(id int, rec string) error {
	return modifyTask(id, "recur:"+rec)
}

// SetDueDate sets the due date for the task with the given id.
func SetDueDate(id int, due string) error {
	return modifyTask(id, "due:"+due)
}

// SetDescription changes the description of the task with the given id.
func SetDescription(id int, desc string) error {
	return modifyTask(id, "description:"+desc)
}

// SetProject changes the project of the task with the given id.
func SetProject(id int, project string) error {
	return modifyTask(id, "project:"+project)
}

// Annotate adds an annotation to the task with the given id.
func Annotate(id int, text string) error {
	return AnnotateContext(context.Background(), id, text)
}

// AnnotateContext adds an annotation to the task with the given id using ctx
// for the underlying Taskwarrior command.
func AnnotateContext(ctx context.Context, id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return runContext(ctx, strconv.Itoa(id), "annotate", text)
}

// Denotate removes an annotation from the task with the given id.
// Denotate removes an annotation from the task with the given id. The
// annotation text is matched exactly when provided. If text is empty, the
// oldest annotation is removed.
func Denotate(id int, text string) error {
	return DenotateContext(context.Background(), id, text)
}

// DenotateContext removes an annotation from the task with the given id using
// ctx for the underlying Taskwarrior command.
func DenotateContext(ctx context.Context, id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	args := []string{strconv.Itoa(id), "denotate"}
	if text != "" {
		args = append(args, text)
	}
	return runContext(ctx, args...)
}

// ReplaceAnnotations removes all existing annotations from the task with the
// given id and sets a single annotation with the provided text. If text is
// empty, all annotations are simply removed.
func ReplaceAnnotations(ctx context.Context, id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	tasks, err := Export(ctx, strconv.Itoa(id))
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	anns := tasks[0].Annotations
	for i := len(anns) - 1; i >= 0; i-- {
		if err := DenotateContext(ctx, id, anns[i].Description); err != nil {
			return err
		}
	}
	if text == "" {
		return nil
	}
	return AnnotateContext(ctx, id, text)
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
