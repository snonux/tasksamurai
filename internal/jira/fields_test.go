package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestSetFieldPayloads(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
		json  string
	}{
		{"summary", "plain summary", `{"fields":{"summary":"plain summary"}}`},
		{"duedate", "2026-12-25", `{"fields":{"duedate":"2026-12-25"}}`},
		{"duedate", nil, `{"fields":{"duedate":null}}`},
		{"priority", map[string]string{"name": "High"}, `{"fields":{"priority":{"name":"High"}}}`},
		{"labels", []string{"existing", "new"}, `{"fields":{"labels":["existing","new"]}}`},
		{"labels", []string{}, `{"fields":{"labels":[]}}`},
	} {
		t.Run(tc.field+fmt.Sprint(tc.value), func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPut || r.URL.Path != "/rest/api/3/issue/TST-42" {
					t.Errorf("request %s %s", r.Method, r.URL)
				}
				var got, want any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(tc.json), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("body %v want %v", got, want)
				}
				w.WriteHeader(204)
			})
			if err := c.SetField(context.Background(), "TST-42", tc.field, tc.value); err != nil || calls != 1 {
				t.Fatalf("calls %d error %v", calls, err)
			}
		})
	}
}

func TestSetFieldRejectsInvalidLocally(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid field reached API") })
	for _, tc := range []struct {
		field string
		value any
	}{
		{"summary", " \n"}, {"summary", map[string]string{"text": "ADF"}},
		{"duedate", "tomorrow"}, {"duedate", "2026-02-30"}, {"duedate", 123},
		{"priority", "H"}, {"priority", map[string]string{"name": ""}},
		{"labels", []string{"has space"}}, {"labels", nil}, {"unknown", "x"},
	} {
		if err := c.SetField(context.Background(), "TST-42", tc.field, tc.value); err == nil {
			t.Errorf("accepted %s %v", tc.field, tc.value)
		}
	}
}

func TestSetFieldNeverRetriesFailures(t *testing.T) {
	for _, field := range []string{"summary", "duedate", "priority", "labels"} {
		value := map[string]any{"summary": "text", "duedate": "2026-12-25", "priority": map[string]string{"name": "Low"}, "labels": []string{"work"}}[field]
		for _, status := range []int{400, 401, 403, 404, 429, 500} {
			t.Run(fmt.Sprintf("%s/%d", field, status), func(t *testing.T) {
				calls := 0
				c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"errorMessages":["no"]}`))
				})
				if err := c.SetField(context.Background(), "TST-42", field, value); err == nil || calls != 1 {
					t.Fatalf("calls %d error %v", calls, err)
				}
			})
		}
	}
}
