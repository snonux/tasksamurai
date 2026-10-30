package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestCommentRoundTripAndFirstPage(t *testing.T) {
	var posts, gets atomic.Int32
	const text = "Unicode 日本語 and \"quotes\"\n\nlast line"
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/TST-1/comment" {
			t.Errorf("wrong comment URL %s", r.URL)
		}
		if r.Method == "POST" {
			posts.Add(1)
			var body struct {
				Body adfNode `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Body.Type != "doc" || body.Body.Version != 1 || len(body.Body.Content) != 3 || len(body.Body.Content[1].Content) != 0 || mapComment(comment{Body: body.Body}).Description != text {
				t.Errorf("incorrect ADF %+v", body)
			}
			w.WriteHeader(201)
			reply(t, w, map[string]any{"created": "2026-10-30T14:30:00.123+0200", "body": body.Body})
		} else {
			gets.Add(1)
			if r.URL.Query().Get("startAt") != "0" {
				t.Error("not first page")
			}
			reply(t, w, map[string]any{"total": 3, "comments": []any{map[string]any{"created": "2026-10-30T12:30:00Z", "body": commentDocument(text)}}})
		}
	})
	annotation, err := c.AddComment(context.Background(), "TST-1", text)
	if err != nil || annotation.Entry != "20261030T123000Z" || annotation.Description != text {
		t.Fatalf("posted annotation %+v %v", annotation, err)
	}
	page, err := c.FetchComments(context.Background(), "TST-1")
	if err != nil || len(page.Annotations) != 1 || page.Annotations[0] != annotation || page.Remaining != 2 || gets.Load() != 1 {
		t.Fatalf("first page %+v %v", page, err)
	}
	for _, empty := range []string{"", " \n\t"} {
		if _, err := c.AddComment(context.Background(), "TST-1", empty); err == nil {
			t.Fatal("empty comment accepted")
		}
	}
	if posts.Load() != 1 {
		t.Fatal("invalid comment caused POST")
	}
	if _, err := c.FetchComments(context.Background(), ""); err == nil {
		t.Fatal("empty comment issue key accepted")
	}
}

func TestADFToTextNestedNodes(t *testing.T) {
	var node adfNode
	body := `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"},{"type":"hardBreak"},{"type":"mention","attrs":{"text":"@Person"}}]}]}]},{"type":"unknownWrapper","content":[{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://example.com"}},{"type":"emoji","attrs":{"text":":)"}}]}]}]}`
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		t.Fatal(err)
	}
	if got := mapComment(comment{Body: node}).Description; got != "hello\n@Person\nhttps://example.com:)" {
		t.Fatalf("nested ADF %q", got)
	}
	if got := mapComment(comment{Body: commentDocument("a\r\nb")}).Description; got != "a\nb" {
		t.Fatalf("CRLF %q", got)
	}
}

func TestMalformedCreatedComment(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, map[string]any{}) })
	if _, err := c.AddComment(context.Background(), "TST-1", "comment"); err == nil {
		t.Fatal("missing server comment accepted")
	}
	if _, err := c.FetchComments(context.Background(), "TST-1"); err == nil {
		t.Fatal("missing comment array accepted")
	}
}
