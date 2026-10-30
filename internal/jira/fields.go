package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SetField updates one supported issue field once, without retrying writes.
func (c *Client) SetField(ctx context.Context, key, field string, value any) error {
	path, err := issuePath(key)
	if err != nil {
		return err
	}
	if err := validateField(field, value); err != nil {
		return err
	}
	return c.request(ctx, http.MethodPut, path, map[string]any{"fields": map[string]any{field: value}}, nil, false)
}

func validateField(field string, value any) error {
	valid := false
	switch field {
	case "summary":
		text, ok := value.(string)
		valid = ok && strings.TrimSpace(text) != ""
	case "duedate":
		if value == nil {
			return nil
		}
		text, ok := value.(string)
		_, err := time.Parse("2006-01-02", text)
		valid = ok && err == nil
	case "priority":
		switch object := value.(type) {
		case map[string]string:
			valid = strings.TrimSpace(object["name"]) != "" || strings.TrimSpace(object["id"]) != ""
		case map[string]any:
			name, _ := object["name"].(string)
			id, _ := object["id"].(string)
			valid = strings.TrimSpace(name) != "" || strings.TrimSpace(id) != ""
		}
	case "labels":
		labels, ok := value.([]string)
		valid = ok && labels != nil
		for _, label := range labels {
			if strings.TrimSpace(label) == "" || strings.ContainsAny(label, " \t\r\n") {
				valid = false
			}
		}
	}
	if !valid {
		return errors.New("Jira: invalid value for field " + field)
	}
	return nil
}
