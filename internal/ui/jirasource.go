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

var _ IssueSource = (*jira.Client)(nil)
