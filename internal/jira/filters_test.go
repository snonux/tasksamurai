package jira

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFetchFiltersPaginationAuthAndAccessibleJQL(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		email, token, ok := r.BasicAuth()
		if !ok || email != "person@example.com" || token != fixtureToken {
			t.Error("incorrect credentials")
		}
		if r.Method != "GET" || r.URL.Path != "/rest/api/3/filter/search" || r.URL.Query().Get("expand") != "jql" || r.URL.Query().Get("maxResults") != "100" || r.URL.Query().Get("nextPageToken") != "" {
			t.Errorf("incorrect request: %s", r.URL)
		}
		switch r.URL.Query().Get("startAt") {
		case "0":
			_, _ = fmt.Fprint(w, `{"startAt":0,"total":5,"values":[{"id":"1","name":"same","jql":"project=A"},{"id":"private","name":"hidden"}]}`)
		case "2":
			_, _ = fmt.Fprint(w, `{"startAt":2,"total":5,"values":[{"id":"1","name":"same","jql":"project=A"},{"id":"2","name":"same","jql":"project=B"},{"id":"3","name":"blank","jql":"  "}]}`)
		default:
			t.Error("unexpected page")
		}
	})
	got, err := c.FetchFilters(context.Background())
	if err != nil || calls != 2 || len(got) != 2 || got[1].ID != "2" || got[0].Origin != "cloud" {
		t.Fatalf("got %+v, %v, calls %d", got, err, calls)
	}
}

func TestFetchFiltersErrorsNeverReturnPartialList(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"auth", `{"errorMessages":["denied"]}`, 401},
		{"permission", `{}`, 403},
		{"html", `<html>error</html>`, 200},
		{"missing-array", `{}`, 200},
		{"empty-page", `{"startAt":1,"total":2,"values":[]}`, 200},
		{"repeated-page", `{"startAt":0,"total":3,"values":[{"id":"1","name":"one","jql":"x"}]}`, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("startAt") == "0" {
					_, _ = fmt.Fprint(w, `{"startAt":0,"total":2,"values":[{"id":"1","name":"one","jql":"x"}]}`)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, tt.body)
			})
			got, err := c.FetchFilters(context.Background())
			if err == nil || got != nil || strings.Contains(err.Error(), fixtureToken) {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestFetchFiltersRetryAndCancellation(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			_, _ = fmt.Fprint(w, `{}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"startAt":0,"isLast":true,"values":[]}`)
	})
	got, err := c.FetchFilters(context.Background())
	if err != nil || len(got) != 0 || calls != 2 {
		t.Fatalf("retry: %+v %v %d", got, err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.FetchFilters(ctx); err == nil || calls != 2 {
		t.Fatal("cancelled fetch made request or succeeded")
	}
}
