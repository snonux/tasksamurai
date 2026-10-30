package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Transition is an available workflow operation and its destination category.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Category struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
	Fields map[string]TransitionField `json:"fields"`
}

// TransitionField describes a screen field including native JSON defaults.
type TransitionField struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	HasDefault bool   `json:"hasDefaultValue"`
	Default    any    `json:"defaultValue"`
	Schema     struct {
		Type string `json:"type"`
	} `json:"schema"`
}

// Transitions lazily loads currently available transitions including screens.
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	path, err := issuePath(key)
	if err != nil {
		return nil, err
	}
	var result struct {
		Transitions []Transition `json:"transitions"`
	}
	if err := c.request(ctx, http.MethodGet, path+"/transitions?expand=transitions.fields", nil, &result, true); err != nil {
		return nil, err
	}
	if result.Transitions == nil {
		return nil, errors.New("Jira: response has no transitions array")
	}
	return result.Transitions, nil
}

// DoTransition executes once; an ambiguous network failure is never retried.
func (c *Client) DoTransition(ctx context.Context, key, id string, fields map[string]any) error {
	path, err := issuePath(key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("Jira: transition ID required")
	}
	body := map[string]any{"transition": map[string]string{"id": id}}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	return c.request(ctx, http.MethodPost, path+"/transitions", body, nil, false)
}

// ChooseTransition selects deterministically within the destination category.
// An override must name an available transition in that category. Defaults
// preserve native JSON shapes, particularly resolution={"name":"Done"}.
func ChooseTransition(available []Transition, category, override string, defaults map[string]any) (Transition, map[string]any, error) {
	var candidates []Transition
	for _, value := range available {
		if value.ID != "" && value.To.Category.Key == category && (override == "" || value.Name == override) {
			candidates = append(candidates, value)
		}
	}
	if len(candidates) == 0 {
		if override != "" {
			return Transition{}, nil, fmt.Errorf("Jira transition %q is unavailable for %s", override, category)
		}
		return Transition{}, nil, fmt.Errorf("Jira: no available transition to %s", category)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].ID < candidates[j].ID
	})
	selected := candidates[0]
	fields, err := transitionFields(selected, defaults)
	return selected, fields, err
}

func transitionFields(selected Transition, defaults map[string]any) (map[string]any, error) {
	fields := make(map[string]any)
	var missing []string
	for key, meta := range selected.Fields {
		value, ok := defaults[key]
		if !ok && meta.Required && meta.Default != nil {
			value, ok = meta.Default, true
		}
		if ok {
			if !validTransitionValue(value, meta.Schema.Type) {
				return nil, fmt.Errorf("Jira transition %q: invalid JSON shape for %s", selected.Name, fieldLabel(key, meta.Name))
			}
			fields[key] = value
		} else if meta.Required && !meta.HasDefault {
			missing = append(missing, fieldLabel(key, meta.Name))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("Jira transition %q requires: %s", selected.Name, strings.Join(missing, ", "))
	}
	return fields, nil
}

func fieldLabel(key, name string) string {
	if name != "" {
		return name
	}
	return key
}

func validTransitionValue(value any, kind string) bool {
	// Normalize TOML-native integer/map/slice types to their JSON equivalents.
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	var decoded any
	if json.Unmarshal(data, &decoded) != nil || decoded == nil {
		return false
	}
	switch kind {
	case "string", "date", "datetime":
		v, ok := decoded.(string)
		return ok && strings.TrimSpace(v) != ""
	case "number", "integer":
		_, ok := decoded.(float64)
		return ok
	case "boolean":
		_, ok := decoded.(bool)
		return ok
	case "array":
		_, ok := decoded.([]any)
		return ok
	case "resolution", "priority", "option", "user", "project", "issuetype", "object":
		_, ok := decoded.(map[string]any)
		return ok
	default:
		return true
	}
}
