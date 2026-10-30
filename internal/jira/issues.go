package jira

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/snonux/tasksamurai/internal/task"
)

// IssueType is a creatable type returned by project create metadata.
type IssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

type issue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary        string   `json:"summary"`
		Created        string   `json:"created"`
		Updated        string   `json:"updated"`
		ResolutionDate string   `json:"resolutiondate"`
		DueDate        string   `json:"duedate"`
		Labels         []string `json:"labels"`
		Project        struct {
			Key string `json:"key"`
		} `json:"project"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		Assignee struct {
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
		Status struct {
			Category struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

// FetchIssues follows search tokens up to the configured result cap. Partial
// failures return an error instead of a partial list masquerading as fresh data.
// len(result)==MaxResults lets the UI display that its configured cap was hit.
func (c *Client) FetchIssues(ctx context.Context, jql string) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, c.settings.Timeout)
	defer cancel()
	tasks := make([]task.Task, 0)
	seen := make(map[string]bool)
	tokens := make(map[string]bool)
	token := ""
	pageSize := c.searchPageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	for {
		body := map[string]any{"jql": jql, "fields": mappedFields(), "maxResults": min(pageSize, c.settings.MaxResults-len(tasks))}
		if token != "" {
			body["nextPageToken"] = token
		}
		var page struct {
			Issues []issue `json:"issues"`
			Next   string  `json:"nextPageToken"`
			Last   bool    `json:"isLast"`
		}
		if err := c.request(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &page, true); err != nil {
			return nil, err
		}
		if page.Issues == nil {
			return nil, errors.New("Jira: search response has no issues array")
		}
		for _, value := range page.Issues {
			if value.Key == "" {
				return nil, errors.New("Jira: issue response has no key")
			}
			if !seen[value.Key] {
				seen[value.Key] = true
				tasks = append(tasks, c.mapIssue(value, time.Now()))
			}
			if len(tasks) == c.settings.MaxResults {
				return tasks, nil
			}
		}
		if page.Last || page.Next == "" {
			return tasks, nil
		}
		if tokens[page.Next] {
			return nil, errors.New("Jira: repeated search page token")
		}
		tokens[page.Next] = true
		token = page.Next
	}
}

// FetchIssue fetches fresh mapped fields for one issue (comments are separate).
func (c *Client) FetchIssue(ctx context.Context, key string) (task.Task, error) {
	path, err := issuePath(key)
	if err != nil {
		return task.Task{}, err
	}
	var value issue
	err = c.request(ctx, http.MethodGet, path+"?fields="+url.QueryEscape(strings.Join(mappedFields(), ",")), nil, &value, true)
	if err != nil {
		return task.Task{}, err
	}
	if value.Key == "" {
		return task.Task{}, errors.New("Jira: issue response has no key")
	}
	return c.mapIssue(value, time.Now()), nil
}

// CreateIssue creates a test issue and returns its key. Summary is a string,
// unlike an ADF description. No write is automatically retried.
func (c *Client) CreateIssue(ctx context.Context, projectKey, issueTypeID, summary string) (string, error) {
	if strings.TrimSpace(projectKey) == "" || strings.TrimSpace(issueTypeID) == "" || strings.TrimSpace(summary) == "" {
		return "", errors.New("Jira: project, issue type and summary required")
	}
	body := map[string]any{"fields": map[string]any{"project": map[string]string{"key": projectKey}, "issuetype": map[string]string{"id": issueTypeID}, "summary": summary}}
	var created struct {
		Key string `json:"key"`
	}
	if err := c.request(ctx, http.MethodPost, "/rest/api/3/issue", body, &created, false); err != nil {
		return "", err
	}
	if created.Key == "" {
		return "", errors.New("Jira: create response has no key")
	}
	return created.Key, nil
}

// IssueTypes discovers creatable types without hardcoding a workflow's names.
func (c *Client) IssueTypes(ctx context.Context, project string) ([]IssueType, error) {
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("Jira: project required")
	}
	ctx, cancel := context.WithTimeout(ctx, c.settings.Timeout)
	defer cancel()
	var result []IssueType
	for start := 0; ; {
		var page struct {
			Types []IssueType `json:"issueTypes"`
			Total int         `json:"total"`
		}
		path := "/rest/api/3/issue/createmeta/" + url.PathEscape(project) + "/issuetypes?maxResults=50&startAt=" + strconv.Itoa(start)
		if err := c.request(ctx, http.MethodGet, path, nil, &page, true); err != nil {
			return nil, err
		}
		if page.Types == nil {
			return nil, errors.New("Jira: metadata response has no issueTypes array")
		}
		result = append(result, page.Types...)
		start += len(page.Types)
		if start >= page.Total {
			return result, nil
		}
		if len(page.Types) == 0 {
			return nil, errors.New("Jira: empty issue type page before total")
		}
	}
}

func (c *Client) mapIssue(value issue, now time.Time) task.Task {
	f := value.Fields
	t := task.Task{IDText: value.Key, UUID: "jira:" + value.Key, Description: f.Summary, Project: f.Project.Key, Status: "pending", Assignee: f.Assignee.DisplayName, Entry: taskDate(f.Created), Priority: mapPriority(f.Priority.Name)}
	t.JiraLabels = append([]string{}, f.Labels...)
	t.Tags = []string{"jira"}
	for _, label := range f.Labels {
		if label != "jira" {
			t.Tags = append(t.Tags, label)
		}
	}
	switch f.Status.Category.Key {
	case "new":
	case "indeterminate":
		t.Start = taskDate(f.Updated)
	case "done":
		t.Status = "completed"
		t.End = taskDate(f.ResolutionDate)
		if t.End == "" {
			t.End = taskDate(f.Updated)
		}
	default:
		c.logger.Debug("unknown Jira status category; treating issue as pending")
	}
	if due, err := time.Parse("2006-01-02", f.DueDate); err == nil {
		t.Due = due.UTC().Format(task.DateFormat)
	}
	t.Urgency = urgency(t, now)
	return t
}

func mappedFields() []string {
	return []string{"summary", "status", "priority", "duedate", "project", "labels", "updated", "created", "assignee", "resolutiondate"}
}

func issuePath(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("Jira: issue key required")
	}
	return "/rest/api/3/issue/" + url.PathEscape(key), nil
}

func taskDate(value string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(task.DateFormat)
		}
	}
	return ""
}

func mapPriority(name string) string {
	switch strings.ToLower(name) {
	case "highest", "high", "h":
		return "H"
	case "lowest", "low", "l":
		return "L"
	default:
		return "M"
	}
}

func urgency(t task.Task, now time.Time) float64 {
	base := map[string]float64{"H": 6, "M": 3, "L": 1}[t.Priority]
	if due, err := time.Parse(task.DateFormat, t.Due); err == nil {
		base += math.Max(0, math.Min(12, (14-due.Sub(now).Hours()/24)*12/21))
	}
	return base
}
