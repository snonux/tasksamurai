package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAddWorklog(t *testing.T) {
	started := time.Date(2026, 10, 30, 12, 15, 0, 0, time.UTC)
	const note = "Review 日本語\nSecond line"
	posts := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		posts++
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issue/TST-1/worklog" || r.URL.Query().Get("adjustEstimate") != "leave" {
			t.Errorf("worklog request %s %s", r.Method, r.URL)
		}
		var body struct {
			Started string  `json:"started"`
			Seconds int64   `json:"timeSpentSeconds"`
			Comment adfNode `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Started != "2026-10-30T12:15:00.000+0000" || body.Seconds != 5400 || adfText(body.Comment) != note+"\n" {
			t.Errorf("worklog body %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		reply(t, w, map[string]any{"id": "10001", "timeSpentSeconds": 5400})
	})
	got, err := c.AddWorklog(context.Background(), "TST-1", started, 5400, note)
	if err != nil || got.ID != "10001" || got.TimeSpentSeconds != 5400 {
		t.Fatalf("created %+v: %v", got, err)
	}
	for _, seconds := range []int64{0, -1, 2147483648} {
		if _, err := c.AddWorklog(context.Background(), "TST-1", started, seconds, ""); err == nil {
			t.Fatalf("accepted seconds %d", seconds)
		}
	}
	if _, err := c.AddWorklog(context.Background(), "", started, 60, ""); err == nil {
		t.Fatal("accepted missing issue")
	}
	if _, err := c.AddWorklog(context.Background(), "TST-1", time.Time{}, 60, ""); err == nil {
		t.Fatal("accepted missing start")
	}
	if posts != 1 {
		t.Fatalf("invalid input caused writes: %d", posts)
	}
}

func TestAddWorklogFailuresNeverRetry(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 201} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				reply(t, w, map[string]any{})
			})
			_, err := c.AddWorklog(context.Background(), "TST-1", time.Now(), 60, "")
			if err == nil || calls != 1 {
				t.Fatalf("calls %d, err %v", calls, err)
			}
			if status == 201 && !strings.Contains(err.Error(), "may have been logged") {
				t.Fatalf("ambiguous response: %v", err)
			}
		})
	}
}
