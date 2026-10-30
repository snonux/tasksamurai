package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Worklog is a confirmed Jira work entry.
type Worklog struct {
	ID               string `json:"id"`
	TimeSpentSeconds int64  `json:"timeSpentSeconds"`
}

// AddWorklog logs elapsed seconds without changing the remaining estimate.
// Writes are never retried, since a timeout can follow a successful creation.
func (c *Client) AddWorklog(ctx context.Context, key string, started time.Time, seconds int64, comment string) (Worklog, error) {
	path, err := issuePath(key)
	if err != nil {
		return Worklog{}, err
	}
	if started.IsZero() || seconds <= 0 || seconds > 2147483647 {
		return Worklog{}, errors.New("Jira: work log needs a start time and positive duration up to 2147483647 seconds")
	}
	body := map[string]any{"started": started.Format("2006-01-02T15:04:05.000-0700"), "timeSpentSeconds": seconds}
	if strings.TrimSpace(comment) != "" {
		body["comment"] = commentDocument(comment)
	}
	var result Worklog
	if err := c.request(ctx, http.MethodPost, path+"/worklog?adjustEstimate=leave", body, &result, false); err != nil {
		return Worklog{}, err
	}
	if result.ID == "" || result.TimeSpentSeconds != seconds {
		return Worklog{}, errors.New("Jira: invalid work log response; work may have been logged, check Jira before resubmitting")
	}
	return result, nil
}
