package task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
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

// SetStatus changes the status of the task with the given id.
func SetStatus(id int, status string) error {
	return SetStatusContext(context.Background(), id, status)
}

// SetStatusContext changes the status of the task with the given id using ctx
// for the underlying Taskwarrior command.
func SetStatusContext(ctx context.Context, id int, status string) error {
	return modifyTaskContext(ctx, id, "status:"+status)
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

// Start begins the task with the given id.
func Start(id int) error {
	return StartContext(context.Background(), id)
}

// StartContext begins the task with the given id using ctx for the underlying
// Taskwarrior command.
func StartContext(ctx context.Context, id int) error {
	return simpleTaskCommandContext(ctx, id, "start")
}

// Stop stops the task with the given id.
func Stop(id int) error {
	return StopContext(context.Background(), id)
}

// StopContext stops the task with the given id using ctx for the underlying
// Taskwarrior command.
func StopContext(ctx context.Context, id int) error {
	return simpleTaskCommandContext(ctx, id, "stop")
}

// Done marks the task with the given id as completed.
func Done(id int) error {
	return DoneContext(context.Background(), id)
}

// DoneContext marks the task with the given id as completed using ctx for the
// underlying Taskwarrior command.
func DoneContext(ctx context.Context, id int) error {
	return simpleTaskCommandContext(ctx, id, "done")
}

// Delete removes the task with the given id.
func Delete(id int) error {
	return DeleteContext(context.Background(), id)
}

// DeleteContext removes the task with the given id using ctx for the
// underlying Taskwarrior command.
func DeleteContext(ctx context.Context, id int) error {
	return simpleTaskCommandContext(ctx, id, "delete")
}

// SetPriority changes the priority of the task with the given id.
func SetPriority(id int, priority string) error {
	return SetPriorityContext(context.Background(), id, priority)
}

// SetPriorityContext changes the priority of the task with the given id using
// ctx for the underlying Taskwarrior command.
func SetPriorityContext(ctx context.Context, id int, priority string) error {
	return modifyTaskContext(ctx, id, "priority:"+priority)
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

	args := tagModifyArgs(adds, removes)
	if len(args) > 0 {
		if err := modifyTaskContext(ctx, id, args...); err != nil {
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

// SetRecurrence sets the recurrence for the task with the given id.
func SetRecurrence(id int, rec string) error {
	return SetRecurrenceContext(context.Background(), id, rec)
}

// SetRecurrenceContext sets the recurrence for the task with the given id
// using ctx for the underlying Taskwarrior command.
func SetRecurrenceContext(ctx context.Context, id int, rec string) error {
	return modifyTaskContext(ctx, id, "recur:"+rec)
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

// SetDueDate sets the due date for the task with the given id.
func SetDueDate(id int, due string) error {
	return SetDueDateContext(context.Background(), id, due)
}

// SetDueDateContext sets the due date for the task with the given id using ctx
// for the underlying Taskwarrior command.
func SetDueDateContext(ctx context.Context, id int, due string) error {
	return modifyTaskContext(ctx, id, "due:"+due)
}

// SetDescription changes the description of the task with the given id.
func SetDescription(id int, desc string) error {
	return SetDescriptionContext(context.Background(), id, desc)
}

// SetDescriptionContext changes the description of the task with the given id
// using ctx for the underlying Taskwarrior command.
func SetDescriptionContext(ctx context.Context, id int, desc string) error {
	return modifyTaskContext(ctx, id, "description:"+desc)
}

// SetProject changes the project of the task with the given id.
func SetProject(id int, project string) error {
	return SetProjectContext(context.Background(), id, project)
}

// SetProjectContext changes the project of the task with the given id using
// ctx for the underlying Taskwarrior command.
func SetProjectContext(ctx context.Context, id int, project string) error {
	return modifyTaskContext(ctx, id, "project:"+project)
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
			return replaceAnnotationsError(id, anns, err)
		}
	}
	if text == "" {
		return nil
	}
	if err := AnnotateContext(ctx, id, text); err != nil {
		return replaceAnnotationsError(id, anns, err)
	}
	return nil
}

func replaceAnnotationsError(id int, anns []Annotation, err error) error {
	rollbackCtx, cancel := rollbackContext()
	defer cancel()

	if rollbackErr := restoreAnnotations(rollbackCtx, id, anns); rollbackErr != nil {
		return fmt.Errorf("replace annotations failed: %w; rollback failed: %w", err, rollbackErr)
	}
	return err
}

func rollbackContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func restoreAnnotations(ctx context.Context, id int, anns []Annotation) error {
	tasks, err := Export(ctx, strconv.Itoa(id))
	if err != nil {
		return fmt.Errorf("snapshot current annotations: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	current := tasks[0].Annotations
	for i := len(current) - 1; i >= 0; i-- {
		if err := DenotateContext(ctx, id, current[i].Description); err != nil {
			return fmt.Errorf("remove current annotation %q: %w", current[i].Description, err)
		}
	}
	for _, ann := range anns {
		if err := AnnotateContext(ctx, id, ann.Description); err != nil {
			return fmt.Errorf("restore annotation %q: %w", ann.Description, err)
		}
	}
	return nil
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

// modifyTask runs a modify command with validation
func modifyTask(id int, args ...string) error {
	return modifyTaskContext(context.Background(), id, args...)
}

func modifyTaskContext(ctx context.Context, id int, args ...string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return runContext(ctx, append([]string{strconv.Itoa(id), "modify"}, args...)...)
}

// simpleTaskCommand runs a simple command on a task with validation
func simpleTaskCommand(id int, command string) error {
	return simpleTaskCommandContext(context.Background(), id, command)
}

func simpleTaskCommandContext(ctx context.Context, id int, command string) error {
	if id <= 0 {
		return fmt.Errorf("invalid task ID: %d", id)
	}
	return runContext(ctx, strconv.Itoa(id), command)
}
