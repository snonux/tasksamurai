package task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/shlex"
)

// Add creates a new task with the given description and tags.
func Add(description string, tags []string) error {
	return AddContext(context.Background(), description, tags)
}

// AddContext creates a new task with the given description and tags using ctx
// for the underlying Taskwarrior command.
func AddContext(ctx context.Context, description string, tags []string) error {
	var args []string
	for _, t := range tags {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	args = append(args, description)
	return AddArgsContext(ctx, args)
}

// AddArgs runs "task add" with the provided arguments. Each element in args
// is passed as a separate command-line argument, allowing the caller to
// specify additional modifiers like due dates or tags.
func AddArgs(args []string) error {
	return AddArgsContext(context.Background(), args)
}

// AddArgsContext runs "task add" with the provided arguments using ctx for the
// underlying Taskwarrior command.
func AddArgsContext(ctx context.Context, args []string) error {
	return runContext(ctx, append([]string{"add"}, args...)...)
}

// AddLine splits the given line into shell words and runs "task add" with the
// resulting arguments. This allows users to pass raw Taskwarrior parameters
// such as "due:today" directly.
func AddLine(line string) error {
	return AddLineContext(context.Background(), line)
}

// AddLineContext splits the given line into shell words and runs "task add"
// with the resulting arguments using ctx for the underlying Taskwarrior
// command.
func AddLineContext(ctx context.Context, line string) error {
	fields, err := shlex.Split(line)
	if err != nil {
		return err
	}
	return AddArgsContext(ctx, fields)
}

// SetStatusUUID changes the status of the task with the given UUID.
func SetStatusUUID(uuid, status string) error {
	return SetStatusUUIDContext(context.Background(), uuid, status)
}

// SetStatusUUIDContext changes the status of the task with the given UUID
// using ctx for the underlying Taskwarrior command.
func SetStatusUUIDContext(ctx context.Context, uuid, status string) error {
	return runContext(ctx, uuid, "modify", "status:"+status)
}

// StartContext begins the task with the given address using ctx for the
// underlying Taskwarrior command.
func StartContext(ctx context.Context, addr string) error {
	return simpleTaskCommandContext(ctx, addr, "start")
}

// StopContext stops the task with the given address using ctx for the
// underlying Taskwarrior command.
func StopContext(ctx context.Context, addr string) error {
	return simpleTaskCommandContext(ctx, addr, "stop")
}

// DoneContext marks the task with the given address as completed using ctx
// for the underlying Taskwarrior command.
func DoneContext(ctx context.Context, addr string) error {
	return simpleTaskCommandContext(ctx, addr, "done")
}

// DeleteContext removes the task with the given address using ctx for the
// underlying Taskwarrior command.
func DeleteContext(ctx context.Context, addr string) error {
	return simpleTaskCommandContext(ctx, addr, "delete")
}

// SetPriorityContext changes the priority of the task with the given address
// using ctx for the underlying Taskwarrior command.
func SetPriorityContext(ctx context.Context, addr, priority string) error {
	return modifyTaskContext(ctx, addr, "priority:"+priority)
}

// AddTagsContext adds tags to the task with the given address using ctx for
// the underlying Taskwarrior command.
func AddTagsContext(ctx context.Context, addr string, tags []string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	args := []string{addr, "modify"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	return runContext(ctx, args...)
}

// RemoveTagsContext removes tags from the task with the given address using
// ctx for the underlying Taskwarrior command.
func RemoveTagsContext(ctx context.Context, addr string, tags []string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	args := []string{addr, "modify"}
	for _, t := range tags {
		if len(t) > 0 && t[0] != '-' {
			t = "-" + t
		}
		args = append(args, t)
	}
	return runContext(ctx, args...)
}

// SetTags sets the tags of the task with the given address to exactly the
// provided set. Tags not present will be removed and new tags added as needed.
func SetTags(ctx context.Context, addr string, tags []string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	tasks, err := Export(ctx, addr)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %s not found", addr)
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

	args := tagModifyArgs(adds, removes)
	if len(args) > 0 {
		if err := modifyTaskContext(ctx, addr, args...); err != nil {
			return err
		}
	}
	return nil
}

func tagModifyArgs(adds, removes []string) []string {
	sort.Strings(adds)
	sort.Strings(removes)

	args := make([]string, 0, len(adds)+len(removes))
	for _, t := range adds {
		if len(t) > 0 && t[0] != '+' {
			t = "+" + t
		}
		args = append(args, t)
	}
	for _, t := range removes {
		if len(t) > 0 && t[0] != '-' {
			t = "-" + t
		}
		args = append(args, t)
	}
	return args
}

// SetRecurrenceContext sets the recurrence for the task with the given
// address using ctx for the underlying Taskwarrior command.
func SetRecurrenceContext(ctx context.Context, addr, rec string) error {
	return modifyTaskContext(ctx, addr, "recur:"+rec)
}

// SetRecurringSeriesRecurrenceContext sets the recurrence for every known task
// in a recurring series identified by rootUUID.
func SetRecurringSeriesRecurrenceContext(ctx context.Context, rootUUID, rec string) error {
	tasks, err := RecurringSeries(ctx, rootUUID)
	if err != nil {
		return err
	}
	tasks = recurringSeriesUpdateOrder(tasks, rootUUID)
	if len(tasks) == 0 {
		return fmt.Errorf("recurring series %s not found", rootUUID)
	}

	completed := make([]Task, 0, len(tasks))
	for _, tsk := range tasks {
		if tsk.UUID == "" {
			continue
		}
		if err := setRecurrenceUUIDContext(ctx, tsk.UUID, rec); err != nil {
			if rollbackErr := restoreRecurringSeriesRecurrences(completed); rollbackErr != nil {
				return fmt.Errorf("set recurrence for %s: %w; rollback failed: %w", tsk.UUID, err, rollbackErr)
			}
			return fmt.Errorf("set recurrence for %s: %w", tsk.UUID, err)
		}
		completed = append(completed, tsk)
	}
	if len(completed) == 0 {
		return fmt.Errorf("recurring series %s has no task UUIDs", rootUUID)
	}
	return nil
}

func recurringSeriesUpdateOrder(tasks []Task, rootUUID string) []Task {
	ordered := make([]Task, 0, len(tasks))
	var root []Task
	for _, tsk := range tasks {
		if tsk.UUID == "" {
			continue
		}
		if tsk.UUID == rootUUID {
			root = append(root, tsk)
			continue
		}
		ordered = append(ordered, tsk)
	}
	return append(ordered, root...)
}

func restoreRecurringSeriesRecurrences(tasks []Task) error {
	ctx, cancel := rollbackContext()
	defer cancel()

	for i := len(tasks) - 1; i >= 0; i-- {
		if err := setRecurrenceUUIDContext(ctx, tasks[i].UUID, tasks[i].Recur); err != nil {
			return fmt.Errorf("restore recurrence for %s: %w", tasks[i].UUID, err)
		}
	}
	return nil
}

func setRecurrenceUUIDContext(ctx context.Context, uuid, rec string) error {
	if uuid == "" {
		return fmt.Errorf("empty task UUID")
	}
	return runContext(ctx, "rc.recurrence.confirmation=no", uuid, "modify", "recur:"+rec)
}

// SetDueDateContext sets the due date for the task with the given address
// using ctx for the underlying Taskwarrior command.
func SetDueDateContext(ctx context.Context, addr, due string) error {
	return modifyTaskContext(ctx, addr, "due:"+due)
}

// SetDescriptionContext changes the description of the task with the given
// address using ctx for the underlying Taskwarrior command.
func SetDescriptionContext(ctx context.Context, addr, desc string) error {
	return modifyTaskContext(ctx, addr, "description:"+desc)
}

// SetProjectContext changes the project of the task with the given address
// using ctx for the underlying Taskwarrior command.
func SetProjectContext(ctx context.Context, addr, project string) error {
	return modifyTaskContext(ctx, addr, "project:"+project)
}

// AnnotateContext adds an annotation to the task with the given address using
// ctx for the underlying Taskwarrior command.
func AnnotateContext(ctx context.Context, addr, text string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	return runContext(ctx, addr, "annotate", text)
}

// DenotateContext removes an annotation from the task with the given address
// using ctx for the underlying Taskwarrior command. The annotation text is
// matched exactly when provided. If text is empty, the oldest annotation is
// removed.
func DenotateContext(ctx context.Context, addr, text string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	args := []string{addr, "denotate"}
	if text != "" {
		args = append(args, text)
	}
	return runContext(ctx, args...)
}

// ReplaceAnnotations removes all existing annotations from the task with the
// given address and sets a single annotation with the provided text. If text
// is empty, all annotations are simply removed.
func ReplaceAnnotations(ctx context.Context, addr, text string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	tasks, err := Export(ctx, addr)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %s not found", addr)
	}
	anns := tasks[0].Annotations
	for i := len(anns) - 1; i >= 0; i-- {
		if err := DenotateContext(ctx, addr, anns[i].Description); err != nil {
			return replaceAnnotationsError(addr, anns, err)
		}
	}
	if text == "" {
		return nil
	}
	if err := AnnotateContext(ctx, addr, text); err != nil {
		return replaceAnnotationsError(addr, anns, err)
	}
	return nil
}

func replaceAnnotationsError(addr string, anns []Annotation, err error) error {
	rollbackCtx, cancel := rollbackContext()
	defer cancel()

	if rollbackErr := restoreAnnotations(rollbackCtx, addr, anns); rollbackErr != nil {
		return fmt.Errorf("replace annotations failed: %w; rollback failed: %w", err, rollbackErr)
	}
	return err
}

func rollbackContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func restoreAnnotations(ctx context.Context, addr string, anns []Annotation) error {
	tasks, err := Export(ctx, addr)
	if err != nil {
		return fmt.Errorf("snapshot current annotations: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %s not found", addr)
	}
	current := tasks[0].Annotations
	for i := len(current) - 1; i >= 0; i-- {
		if err := DenotateContext(ctx, addr, current[i].Description); err != nil {
			return fmt.Errorf("remove current annotation %q: %w", current[i].Description, err)
		}
	}
	for _, ann := range anns {
		if err := AnnotateContext(ctx, addr, ann.Description); err != nil {
			return fmt.Errorf("restore annotation %q: %w", ann.Description, err)
		}
	}
	return nil
}

// EditCmd returns an exec.Cmd that edits the task with the given address.
// The caller is responsible for running the command, typically via
// tea.ExecProcess so that the terminal state is properly managed.
func EditCmd(addr string) *exec.Cmd {
	if !validTaskAddr(addr) {
		// Return a command that will fail with an appropriate error
		cmd := exec.Command("sh", "-c", "echo 'invalid task address' >&2; exit 1")
		return cmd
	}
	cmd := exec.Command("task", addr, "edit")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// validTaskAddr validates a CLI task address: a non-empty UUID string, or a
// numeric ID greater than zero (the old int-based API rejected id <= 0;
// pure-digit addresses like "0" or "-1" stay invalid for parity).
func validTaskAddr(addr string) bool {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return false
	}
	if id, err := strconv.Atoi(trimmed); err == nil {
		return id > 0
	}
	return true
}

func invalidAddrError(addr string) error {
	return fmt.Errorf("invalid task address: %q", addr)
}

// modifyTask runs a modify command with validation
func modifyTask(addr string, args ...string) error {
	return modifyTaskContext(context.Background(), addr, args...)
}

// modifyTaskContext runs "task <addr> modify ...". addr is the task's UUID
// (preferred) or its legacy numeric ID: Taskwarrior renumbers pending IDs
// whenever the working set changes, so mutations must not depend on an
// in-memory ID staying valid; UUIDs are stable.
func modifyTaskContext(ctx context.Context, addr string, args ...string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	return runContext(ctx, append([]string{addr, "modify"}, args...)...)
}

// simpleTaskCommand runs a simple command on a task with validation
func simpleTaskCommand(addr string, command string) error {
	return simpleTaskCommandContext(context.Background(), addr, command)
}

// simpleTaskCommandContext runs "task <addr> <command>". See
// modifyTaskContext for what addr may contain.
func simpleTaskCommandContext(ctx context.Context, addr string, command string) error {
	if !validTaskAddr(addr) {
		return invalidAddrError(addr)
	}
	return runContext(ctx, addr, command)
}
