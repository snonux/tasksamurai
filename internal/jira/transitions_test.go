package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func testTransition(id, name, category string) Transition {
	v := Transition{ID: id, Name: name}
	v.To.Category.Key = category
	return v
}

func TestChooseTransition(t *testing.T) {
	values := []Transition{testTransition("9", "Zulu close", "done"), testTransition("2", "Alpha close", "done"), testTransition("3", "Custom working", "indeterminate"), testTransition("4", "Custom queue", "new")}
	for _, tt := range []struct {
		name, category, override, want string
		fail                           bool
	}{
		{"ambiguous deterministic", "done", "", "2", false},
		{"override", "done", "Zulu close", "9", false},
		{"single", "indeterminate", "", "3", false},
		{"stop", "new", "", "4", false},
		{"unavailable override", "done", "missing", "", true},
		{"wrong-category override", "done", "Custom working", "", true},
		{"zero candidates", "unknown", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := ChooseTransition(values, tt.category, tt.override, nil)
			if (err != nil) != tt.fail || !tt.fail && got.ID != tt.want {
				t.Fatalf("transition %+v error %v", got, err)
			}
		})
	}
	duplicate := []Transition{testTransition("20", "Same name", "done"), testTransition("10", "Same name", "done")}
	if got, _, err := ChooseTransition(duplicate, "done", "", nil); err != nil || got.ID != "10" {
		t.Fatalf("ID tie breaker %+v %v", got, err)
	}
	if values[0].ID != "9" {
		t.Fatal("selection mutated caller's transition order")
	}
}

func TestTransitionRequiredFields(t *testing.T) {
	var required Transition
	if err := json.Unmarshal([]byte(`{"id":"1","name":"Close Issue","to":{"statusCategory":{"key":"done"}},"fields":{"resolution":{"name":"Resolution","required":true,"schema":{"type":"resolution"}}}}`), &required); err != nil {
		t.Fatal(err)
	}
	_, _, err := ChooseTransition([]Transition{required}, "done", "", nil)
	if err == nil || !strings.Contains(err.Error(), "Close Issue") || !strings.Contains(err.Error(), "Resolution") {
		t.Fatalf("missing required diagnostic %v", err)
	}
	for _, value := range []any{"Done", []string{"Done"}, nil} {
		if _, _, err := ChooseTransition([]Transition{required}, "done", "", map[string]any{"resolution": value}); err == nil {
			t.Fatalf("invalid resolution shape accepted: %T", value)
		}
	}
	_, fields, err := ChooseTransition([]Transition{required}, "done", "", map[string]any{"resolution": map[string]string{"name": "Done"}, "notOnScreen": "ignored"})
	if err != nil || len(fields) != 1 {
		t.Fatalf("native resolution %+v %v", fields, err)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"resolution":{"name":"Done"}}` {
		t.Fatalf("resolution JSON %s", encoded)
	}
	meta := required.Fields["resolution"]
	meta.Default = map[string]any{"id": "10000"}
	required.Fields["resolution"] = meta
	if _, fields, err := ChooseTransition([]Transition{required}, "done", "", nil); err != nil || fields["resolution"] == nil {
		t.Fatalf("metadata default %+v %v", fields, err)
	}
	meta.Default = nil
	meta.HasDefault = true
	required.Fields["resolution"] = meta
	if _, fields, err := ChooseTransition([]Transition{required}, "done", "", nil); err != nil || len(fields) != 0 {
		t.Fatalf("server default %+v %v", fields, err)
	}
}

func TestTransitionHTTPMetadataAndWrite(t *testing.T) {
	var gets, posts atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/TST-1/transitions" {
			t.Errorf("wrong path %s", r.URL)
		}
		if r.Method == "GET" {
			gets.Add(1)
			if r.URL.Query().Get("expand") != "transitions.fields" {
				t.Error("screen metadata not requested")
			}
			reply(t, w, map[string]any{"transitions": []Transition{testTransition("12", "Custom close", "done")}})
			return
		}
		posts.Add(1)
		var body struct {
			Transition struct {
				ID string `json:"id"`
			} `json:"transition"`
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Transition.ID != "12" || body.Fields["resolution"].(map[string]any)["name"] != "Done" {
			t.Errorf("incorrect transition body %+v", body)
		}
		w.WriteHeader(204)
	})
	values, err := c.Transitions(context.Background(), "TST-1")
	if err != nil || len(values) != 1 || values[0].To.Category.Key != "done" {
		t.Fatalf("available %+v %v", values, err)
	}
	if err := c.DoTransition(context.Background(), "TST-1", "12", map[string]any{"resolution": map[string]string{"name": "Done"}}); err != nil {
		t.Fatal(err)
	}
	if gets.Load() != 1 || posts.Load() != 1 {
		t.Fatal("transition unexpectedly refetched or retried")
	}
	if err := c.DoTransition(context.Background(), "TST-1", "", nil); err == nil {
		t.Fatal("empty transition ID accepted")
	}
	if _, err := c.Transitions(context.Background(), ""); err == nil {
		t.Fatal("empty issue key accepted")
	}
	if posts.Load() != 1 {
		t.Fatal("invalid input invoked write")
	}
}

func TestTransitionHTTPFailuresDoNotRetryWrite(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"denied", 403, `{}`}, {"server", 503, `{"errorMessages":["unavailable"]}`}, {"HTML", 200, `<html>proxy</html>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			if err := c.DoTransition(context.Background(), "TST-1", "1", nil); err == nil || calls.Load() != 1 {
				t.Fatalf("write error %v calls %d", err, calls.Load())
			}
		})
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, map[string]any{}) })
	if _, err := c.Transitions(context.Background(), "TST-1"); err == nil {
		t.Fatal("missing transitions array accepted")
	}
}
