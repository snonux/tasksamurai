package ui

import (
	"context"

	"github.com/snonux/tasksamurai/internal/jira"
	"github.com/snonux/tasksamurai/internal/task"
)

// IssueSource is the Jira capability used by phase-1 UI operations.
// Optional cloud-filter and mutation capabilities are added independently.
type IssueSource interface {
	FetchIssues(context.Context, string) ([]task.Task, error)
	FetchIssue(context.Context, string) (task.Task, error)
	FetchComments(context.Context, string) (jira.CommentPage, error)
	AddComment(context.Context, string, string) (task.Annotation, error)
}

// FilterSource is the optional saved-filter capability used by the picker.
type FilterSource interface {
	FetchFilters(context.Context) ([]jira.SavedFilter, error)
}

// TransitionSource is the optional phase-2 workflow capability.
type TransitionSource interface {
	Transitions(context.Context, string) ([]jira.Transition, error)
	DoTransition(context.Context, string, string, map[string]any) error
}

// FieldSource is the optional Jira issue-field editing capability.
type FieldSource interface {
	SetField(context.Context, string, string, any) error
}

var _ FieldSource = (*jira.Client)(nil)

var _ TransitionSource = (*jira.Client)(nil)

var _ FilterSource = (*jira.Client)(nil)

var _ IssueSource = (*jira.Client)(nil)
