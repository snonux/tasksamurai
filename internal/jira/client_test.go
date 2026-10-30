package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snonux/tasksamurai/internal/config"
)

const fixtureToken = "test-only-secret=TOKEN-END"

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(" \n"+fixtureToken+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := New(context.Background(), config.Jira{Enabled: true, Site: server.URL, Email: "person@example.com", TokenFile: path, Timeout: time.Second, MaxResults: 100}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func reply(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func TestSearchAuthPaginationAndCap(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("bad search endpoint: %s %s", r.Method, r.URL.Path)
		}
		email, token, ok := r.BasicAuth()
		if !ok || email != "person@example.com" || token != fixtureToken {
			t.Error("incorrect complete Basic credentials")
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing JSON headers")
		}
		var body struct {
			JQL    string   `json:"jql"`
			Fields []string `json:"fields"`
			Next   string   `json:"nextPageToken"`
			Max    int      `json:"maxResults"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.JQL != "project = TST" || !reflect.DeepEqual(body.Fields, []string{"summary", "status", "priority", "duedate", "project", "labels", "updated", "created", "assignee", "resolutiondate"}) {
			t.Errorf("unexpected query/fields: %+v", body)
		}
		if n == 1 {
			if body.Max != 2 || body.Next != "" {
				t.Errorf("first pagination %+v", body)
			}
			reply(t, w, map[string]any{"issues": []any{map[string]any{"key": "TST-1", "fields": map[string]any{"summary": "one"}}}, "nextPageToken": "next"})
		} else {
			if body.Max != 1 || body.Next != "next" {
				t.Errorf("second pagination %+v", body)
			}
			reply(t, w, map[string]any{"issues": []any{map[string]any{"key": "TST-1"}, map[string]any{"key": "TST-2"}, map[string]any{"key": "TST-3"}}, "nextPageToken": "unused"})
		}
	})
	c.settings.MaxResults = 2
	got, err := c.FetchIssues(context.Background(), "project = TST")
	if err != nil || len(got) != 2 || got[0].UUID != "jira:TST-1" || got[1].UUID != "jira:TST-2" || calls.Load() != 2 {
		t.Fatalf("pagination result %+v, %v, calls %d", got, err, calls.Load())
	}
}

func TestSearchPageFailures(t *testing.T) {
	for _, mode := range []string{"repeat", "partial error", "missing key", "malformed", "null", "missing array"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				switch mode {
				case "missing key":
					reply(t, w, map[string]any{"issues": []any{map[string]any{"fields": map[string]any{"summary": "missing"}}}})
				case "malformed":
					_, _ = w.Write([]byte("{broken"))
				case "null":
					_, _ = w.Write([]byte("null"))
				case "missing array":
					reply(t, w, map[string]any{})
				default:
					if mode == "partial error" && n > 1 {
						w.WriteHeader(403)
						reply(t, w, map[string]any{})
						return
					}
					reply(t, w, map[string]any{"issues": []any{map[string]any{"key": "TST-1"}}, "nextPageToken": "same"})
				}
			})
			got, err := c.FetchIssues(context.Background(), "project=TST")
			if err == nil || got != nil {
				t.Fatalf("partial/invalid search succeeded: %+v %v", got, err)
			}
			if calls.Load() > 2 {
				t.Fatal("repeated page token did not terminate")
			}
		})
	}
}

func TestErrorMappingAndReadRetry(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body, want string
		calls      int
	}{
		{"unauthorized", 401, `{}`, "wrong email or truncated token", 1},
		{"forbidden", 403, `{}`, "no permission", 1},
		{"missing", 404, `{}`, "issue not found", 1},
		{"unbounded", 400, `{"errorMessages":["Unbounded JQL queries are not allowed here"]}`, "press F", 1},
		{"JQL", 400, `{"errorMessages":["Field frobnitz does not exist"]}`, "Field frobnitz does not exist", 1},
		{"field error", 400, `{"errors":{"summary":"Summary is required"}}`, "Summary is required", 1},
		{"throttle", 429, `{"errorMessages":["slow down"]}`, "slow down", 2},
		{"server", 503, `{"errorMessages":["unavailable"]}`, "unavailable", 2},
		{"HTML error", 502, `<html>proxy</html>`, "unexpected response from Jira (HTTP 502)", 2},
		{"HTML success", 200, `<html>proxy</html>`, "unexpected response from Jira (HTTP 200)", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.FetchIssues(context.Background(), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) || int(calls.Load()) != tt.calls {
				t.Fatalf("got %v (%d calls), want %q (%d)", err, calls.Load(), tt.want, tt.calls)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
				t.Fatalf("missing HTTP status: %v", err)
			}
		})
	}
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			reply(t, w, map[string]any{})
			return
		}
		reply(t, w, map[string]any{"issues": []any{}, "isLast": true})
	})
	if got, err := c.FetchIssues(context.Background(), ""); err != nil || len(got) != 0 || calls.Load() != 2 {
		t.Fatalf("retry recovery: %+v %v", got, err)
	}
}

func TestCredentialRotationRedactionAndRedirect(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("person@example.com:" + fixtureToken))
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		reply(t, w, map[string]any{"errorMessages": []string{fixtureToken + " " + basic}})
	})
	_, err := c.FetchIssues(context.Background(), "")
	if err == nil || strings.Contains(err.Error(), fixtureToken) || strings.Contains(err.Error(), basic) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("unsafe error: %v", err)
	}
	for _, value := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(value, fixtureToken) || strings.Contains(value, basic) {
			t.Fatal("client formatting leaks credentials")
		}
	}
	if err := os.WriteFile(c.settings.TokenFile, []byte("rotated=FULL"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.ReloadToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, token, _ := r.BasicAuth()
		if token != "rotated=FULL" {
			t.Error("rotation not used")
		}
		return jsonResponse(200, `{"issues":[]}`), nil
	})
	if _, err := c.FetchIssues(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(c.settings.TokenFile); err != nil {
		t.Fatal(err)
	}
	if err := c.ReloadToken(context.Background()); err == nil {
		t.Fatal("missing rotated token accepted")
	}
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1); reply(t, w, map[string]any{}) }))
	defer target.Close()
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) })
	if _, err := c.FetchIssues(context.Background(), ""); err == nil || redirected.Load() != 0 {
		t.Fatal("redirect followed or accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: ioString(body)}
}
func ioString(value string) io.ReadCloser { return io.NopCloser(strings.NewReader(value)) }

func TestCancellationBoundsAndNoWriteRetry(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(503)
		reply(t, w, map[string]any{})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.FetchIssues(ctx, ""); !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatalf("backoff cancellation %v calls %d", err, calls.Load())
	}
	calls.Store(0)
	if _, err := c.AddComment(context.Background(), "TST-1", "comment"); err == nil || calls.Load() != 1 {
		t.Fatalf("write retried: %v calls %d", err, calls.Load())
	}
	calls.Store(0)
	if _, err := c.CreateIssue(context.Background(), "TST", "1", "summary"); err == nil || calls.Load() != 1 {
		t.Fatalf("create retried: %v calls %d", err, calls.Load())
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, strings.Repeat("x", maxResponseBytes+1)), nil
	})
	if _, err := c.FetchIssues(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized response: %v", err)
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport contains %s", fixtureToken)
	})
	if _, err := c.FetchIssues(context.Background(), ""); err == nil || strings.Contains(err.Error(), fixtureToken) {
		t.Fatalf("unsafe transport error: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := c.FetchIssues(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request: %v", err)
	}
}

func TestConstructorAndMetadataErrors(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, map[string]any{}) })
	for _, change := range []func(*config.Jira){
		func(s *config.Jira) { s.Site = "https://user:secret@example.com" },
		func(s *config.Jira) { s.Site = "https://example.com/path" },
		func(s *config.Jira) { s.Site = "https://example.com?" },
		func(s *config.Jira) { s.Site = "https://example.com#" },
		func(s *config.Jira) { s.MaxResults = 0 },
		func(s *config.Jira) { s.Timeout = 0 },
		func(s *config.Jira) { s.Email = "" },
		func(s *config.Jira) { s.TokenFile = "/nonexistent/token" },
		func(s *config.Jira) { s.Enabled = false },
	} {
		settings := c.settings
		change(&settings)
		if _, err := New(context.Background(), settings, c.http); err == nil {
			t.Fatal("invalid constructor settings accepted")
		}
	}
	if _, err := New(context.Background(), c.settings, nil); err == nil {
		t.Fatal("production HTTP accepted")
	}
	settings := c.settings
	settings.Site = "https://example.atlassian.net"
	if _, err := New(context.Background(), settings, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.IssueTypes(context.Background(), "TST"); err == nil {
		t.Fatal("missing metadata array accepted")
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"issueTypes":[],"total":3}`), nil
	})
	if _, err := c.IssueTypes(context.Background(), "TST"); err == nil {
		t.Fatal("non-progressing metadata page accepted")
	}
}

type failingBody struct{ closed bool }

func (b *failingBody) Read([]byte) (int, error) { return 0, errors.New("read failure " + fixtureToken) }
func (b *failingBody) Close() error             { b.closed = true; return nil }

func TestResponseReadFailureAndDeadline(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected real request") })
	body := &failingBody{}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	})
	if _, err := c.FetchIssues(context.Background(), ""); err == nil || strings.Contains(err.Error(), fixtureToken) || !body.closed {
		t.Fatalf("response read/close %v closed %v", err, body.closed)
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	c.settings.Timeout = 10 * time.Millisecond
	if _, err := c.FetchIssues(context.Background(), ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline not enforced: %v", err)
	}
}
