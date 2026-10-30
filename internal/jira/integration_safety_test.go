package jira

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/snonux/tasksamurai/internal/task"
)

func validIntegrationProject(project string) bool {
	return regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`).MatchString(project)
}

// ownedIntegrationIssue is checked against a fresh GET immediately before
// every live mutation or deletion, including cleanup and fallback transitions.
func ownedIntegrationIssue(value task.Task, project, marker string) error {
	runID := strings.TrimPrefix(marker, "ts-integration-")
	id, err := strconv.ParseUint(runID, 10, 64)
	if !validIntegrationProject(project) || !strings.HasPrefix(marker, "ts-integration-") || err != nil || id == 0 {
		return errors.New("invalid integration project or exact run marker")
	}
	sequence := strings.TrimPrefix(value.IDText, project+"-")
	issueID, err := strconv.ParseUint(sequence, 10, 64)
	if value.Project != project || !strings.HasPrefix(value.IDText, project+"-") || err != nil || issueID == 0 || value.UUID != "jira:"+value.IDText || !strings.HasPrefix(value.Description, marker+" ") {
		return errors.New("refusing integration mutation: project, issue key or exact run marker does not match")
	}
	return nil
}

func TestIntegrationOwnershipFence(t *testing.T) {
	good := task.Task{IDText: "TST-1", UUID: "jira:TST-1", Project: "TST", Description: "ts-integration-123456789 fixture"}
	if err := ownedIntegrationIssue(good, "TST", "ts-integration-123456789"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name            string
		edit            func(*task.Task)
		project, marker string
	}{
		{"other project", func(v *task.Task) { v.Project = "OTHER" }, "TST", "ts-integration-123456789"},
		{"other key", func(v *task.Task) { v.IDText = "TST2-1"; v.UUID = "jira:TST2-1" }, "TST", "ts-integration-123456789"},
		{"unmarked", func(v *task.Task) { v.Description = "real user issue" }, "TST", "ts-integration-123456789"},
		{"other run", func(v *task.Task) { v.Description = "ts-integration-987654321 fixture" }, "TST", "ts-integration-123456789"},
		{"prefix collision", func(v *task.Task) { v.Description = "ts-integration-1234567890 fixture" }, "TST", "ts-integration-123456789"},
		{"wrong UUID", func(v *task.Task) { v.UUID = "tw-uuid" }, "TST", "ts-integration-123456789"},
		{"broad marker", func(*task.Task) {}, "TST", "ts-integration-"},
		{"blank project", func(*task.Task) {}, "", "ts-integration-123456789"},
		{"JQL injection", func(*task.Task) {}, `TST OR project=OTHER`, "ts-integration-123456789"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			value := good
			tt.edit(&value)
			if err := ownedIntegrationIssue(value, tt.project, tt.marker); err == nil {
				t.Fatal("unsafe fixture passed ownership fence")
			}
		})
	}
}
