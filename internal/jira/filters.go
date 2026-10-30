package jira

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// SavedFilter is an accessible Jira filter. Identity is Origin plus ID, never Name.
type SavedFilter struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	JQL    string `json:"jql"`
	Origin string `json:"-"`
}

// FetchFilters lists accessible saved filters, expanding JQL on every page.
// Partial failures never return a partial list as a successful cache refresh.
func (c *Client) FetchFilters(ctx context.Context) ([]SavedFilter, error) {
	ctx, cancel := context.WithTimeout(ctx, c.settings.Timeout)
	defer cancel()
	result := make([]SavedFilter, 0)
	seen := make(map[string]bool)
	for start := 0; ; {
		var page struct {
			Values []SavedFilter `json:"values"`
			Start  int           `json:"startAt"`
			Total  *int          `json:"total"`
			Last   bool          `json:"isLast"`
		}
		path := "/rest/api/3/filter/search?expand=jql&maxResults=100&startAt=" + strconv.Itoa(start)
		if err := c.request(ctx, http.MethodGet, path, nil, &page, true); err != nil {
			return nil, err
		}
		if page.Values == nil || page.Start != start {
			return nil, errors.New("Jira: invalid saved-filter page")
		}
		for _, filter := range page.Values {
			if strings.TrimSpace(filter.ID) == "" || strings.TrimSpace(filter.Name) == "" || strings.TrimSpace(filter.JQL) == "" || seen[filter.ID] {
				continue
			}
			filter.Origin = "cloud"
			seen[filter.ID] = true
			result = append(result, filter)
		}
		start += len(page.Values)
		if page.Last || page.Total != nil && start >= *page.Total {
			return result, nil
		}
		if len(page.Values) == 0 {
			return nil, errors.New("Jira: empty saved-filter page before end")
		}
	}
}
