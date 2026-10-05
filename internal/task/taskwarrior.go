package task

import (
	"context"
	"os/exec"
	"time"
)

// Taskwarrior describes the Taskwarrior operations required by the UI.
type Taskwarrior interface {
	Export(ctx context.Context, filters ...string) ([]Task, error)
	SortTasks(tasks []Task)
	TotalTasks(tasks []Task) int
	InProgressTasks(tasks []Task) int
	CompletedTasks(tasks []Task) int
	DueTasks(tasks []Task, now time.Time) int
	EditCmd(addr string) *exec.Cmd
	RunShellLine(ctx context.Context, line string) (RunResult, error)
	LoadCompletionSources(ctx context.Context) CompletionSources
	AddLineContext(ctx context.Context, line string) error
	AnnotateContext(ctx context.Context, addr string, text string) error
	ReplaceAnnotations(ctx context.Context, addr string, text string) error
	SetDescriptionContext(ctx context.Context, addr string, desc string) error
	AddTagsContext(ctx context.Context, addr string, tags []string) error
	RemoveTagsContext(ctx context.Context, addr string, tags []string) error
	SetDueDateContext(ctx context.Context, addr string, due string) error
	SetRecurrenceContext(ctx context.Context, addr string, rec string) error
	SetRecurringSeriesRecurrenceContext(ctx context.Context, rootUUID, rec string) error
	SetProjectContext(ctx context.Context, addr string, project string) error
	SetPriorityContext(ctx context.Context, addr string, priority string) error
	StartContext(ctx context.Context, addr string) error
	StopContext(ctx context.Context, addr string) error
	DoneContext(ctx context.Context, addr string) error
	SetStatusUUIDContext(ctx context.Context, uuid, status string) error
	RecurringSeries(ctx context.Context, rootUUID string) ([]Task, error)
}

// Client is the production Taskwarrior implementation backed by the task CLI.
type Client struct{}

var _ Taskwarrior = Client{}

// NewTaskwarrior returns a production Taskwarrior Client.
func NewTaskwarrior() Client {
	return Client{}
}

// Export retrieves tasks using Taskwarrior export.
func (Client) Export(ctx context.Context, filters ...string) ([]Task, error) {
	return Export(ctx, filters...)
}

// SortTasks orders tasks using TaskSamurai's default task ordering.
func (Client) SortTasks(tasks []Task) {
	SortTasks(tasks)
}

// TotalTasks returns the number of tasks provided.
func (Client) TotalTasks(tasks []Task) int {
	return TotalTasks(tasks)
}

// InProgressTasks returns the number of started, incomplete tasks.
func (Client) InProgressTasks(tasks []Task) int {
	return InProgressTasks(tasks)
}

// CompletedTasks returns the number of completed tasks.
func (Client) CompletedTasks(tasks []Task) int {
	return CompletedTasks(tasks)
}

// DueTasks returns the number of due tasks.
func (Client) DueTasks(tasks []Task, now time.Time) int {
	return DueTasks(tasks, now)
}

// EditCmd returns an editor command for a task.
func (Client) EditCmd(addr string) *exec.Cmd {
	return EditCmd(addr)
}

// RunShellLine runs a user-entered Taskwarrior shell command.
func (Client) RunShellLine(ctx context.Context, line string) (RunResult, error) {
	return RunShellLine(ctx, line)
}

// LoadCompletionSources returns Taskwarrior-provided completion candidates.
func (Client) LoadCompletionSources(ctx context.Context) CompletionSources {
	return LoadCompletionSources(ctx)
}

// AddLineContext adds a task from a shell-style input line.
func (Client) AddLineContext(ctx context.Context, line string) error {
	return AddLineContext(ctx, line)
}

// AnnotateContext adds an annotation to a task.
func (Client) AnnotateContext(ctx context.Context, addr string, text string) error {
	return AnnotateContext(ctx, addr, text)
}

// ReplaceAnnotations replaces all annotations on a task.
func (Client) ReplaceAnnotations(ctx context.Context, addr string, text string) error {
	return ReplaceAnnotations(ctx, addr, text)
}

// SetDescriptionContext changes a task description.
func (Client) SetDescriptionContext(ctx context.Context, addr string, desc string) error {
	return SetDescriptionContext(ctx, addr, desc)
}

// AddTagsContext adds tags to a task.
func (Client) AddTagsContext(ctx context.Context, addr string, tags []string) error {
	return AddTagsContext(ctx, addr, tags)
}

// RemoveTagsContext removes tags from a task.
func (Client) RemoveTagsContext(ctx context.Context, addr string, tags []string) error {
	return RemoveTagsContext(ctx, addr, tags)
}

// SetDueDateContext changes a task due date.
func (Client) SetDueDateContext(ctx context.Context, addr string, due string) error {
	return SetDueDateContext(ctx, addr, due)
}

// SetRecurrenceContext changes a task recurrence value.
func (Client) SetRecurrenceContext(ctx context.Context, addr string, rec string) error {
	return SetRecurrenceContext(ctx, addr, rec)
}

// SetRecurringSeriesRecurrenceContext changes a recurring series recurrence value.
func (Client) SetRecurringSeriesRecurrenceContext(ctx context.Context, rootUUID, rec string) error {
	return SetRecurringSeriesRecurrenceContext(ctx, rootUUID, rec)
}

// SetProjectContext changes a task project.
func (Client) SetProjectContext(ctx context.Context, addr string, project string) error {
	return SetProjectContext(ctx, addr, project)
}

// SetPriorityContext changes a task priority.
func (Client) SetPriorityContext(ctx context.Context, addr string, priority string) error {
	return SetPriorityContext(ctx, addr, priority)
}

// StartContext starts a task.
func (Client) StartContext(ctx context.Context, addr string) error {
	return StartContext(ctx, addr)
}

// StopContext stops a task.
func (Client) StopContext(ctx context.Context, addr string) error {
	return StopContext(ctx, addr)
}

// DoneContext completes a task.
func (Client) DoneContext(ctx context.Context, addr string) error {
	return DoneContext(ctx, addr)
}

// SetStatusUUIDContext changes a task status by UUID.
func (Client) SetStatusUUIDContext(ctx context.Context, uuid, status string) error {
	return SetStatusUUIDContext(ctx, uuid, status)
}

// RecurringSeries returns a recurring task series.
func (Client) RecurringSeries(ctx context.Context, rootUUID string) ([]Task, error) {
	return RecurringSeries(ctx, rootUUID)
}
