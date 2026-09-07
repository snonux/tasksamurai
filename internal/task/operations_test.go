package task

import (
	"context"
	"strings"
	"testing"
)

func TestModifyTask(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid UUID address",
			addr:    "7977b12b-a6cd-4cd5-875d-c91aa610ac96",
			args:    []string{"status:pending"},
			wantErr: false,
		},
		{
			name:    "valid numeric address",
			addr:    "1",
			args:    []string{"status:pending"},
			wantErr: false,
		},
		{
			name:    "empty address",
			addr:    "",
			args:    []string{"status:pending"},
			wantErr: true,
			errMsg:  "invalid task address",
		},
		{
			name:    "whitespace address",
			addr:    "   ",
			args:    []string{"status:pending"},
			wantErr: true,
			errMsg:  "invalid task address",
		},
		{
			name:    "zero numeric address",
			addr:    "0",
			args:    []string{"status:pending"},
			wantErr: true,
			errMsg:  "invalid task address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := modifyTask(tt.addr, tt.args...)

			// We can't test actual taskwarrior commands without it installed
			// So we just test the validation
			if tt.wantErr {
				if err == nil {
					t.Errorf("modifyTask() error = nil, wantErr %v", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("modifyTask() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestSimpleTaskCommand(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		command string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid UUID address",
			addr:    "7977b12b-a6cd-4cd5-875d-c91aa610ac96",
			command: "done",
			wantErr: false,
		},
		{
			name:    "valid numeric address",
			addr:    "1",
			command: "done",
			wantErr: false,
		},
		{
			name:    "empty address",
			addr:    "",
			command: "done",
			wantErr: true,
			errMsg:  "invalid task address",
		},
		{
			name:    "whitespace address",
			addr:    "   ",
			command: "done",
			wantErr: true,
			errMsg:  "invalid task address",
		},
		{
			name:    "zero numeric address",
			addr:    "0",
			command: "done",
			wantErr: true,
			errMsg:  "invalid task address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := simpleTaskCommand(tt.addr, tt.command)

			// We can't test actual taskwarrior commands without it installed
			// So we just test the validation
			if tt.wantErr {
				if err == nil {
					t.Errorf("simpleTaskCommand() error = nil, wantErr %v", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("simpleTaskCommand() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestTaskOperationsValidation(t *testing.T) {
	// Test that all task operations validate the address
	invalidAddr := ""

	operations := []struct {
		name string
		fn   func() error
	}{
		{"Start", func() error { return StartContext(context.Background(), invalidAddr) }},
		{"Stop", func() error { return StopContext(context.Background(), invalidAddr) }},
		{"Done", func() error { return DoneContext(context.Background(), invalidAddr) }},
		{"Delete", func() error { return DeleteContext(context.Background(), invalidAddr) }},
		{"SetPriority", func() error { return SetPriorityContext(context.Background(), invalidAddr, "H") }},
		{"SetRecurrence", func() error { return SetRecurrenceContext(context.Background(), invalidAddr, "daily") }},
		{"SetDueDate", func() error { return SetDueDateContext(context.Background(), invalidAddr, "tomorrow") }},
		{"SetDescription", func() error { return SetDescriptionContext(context.Background(), invalidAddr, "test") }},
		{"Annotate", func() error { return AnnotateContext(context.Background(), invalidAddr, "note") }},
		{"Denotate", func() error { return DenotateContext(context.Background(), invalidAddr, "note") }},
		{"AddTags", func() error { return AddTagsContext(context.Background(), invalidAddr, []string{"x"}) }},
		{"RemoveTags", func() error { return RemoveTagsContext(context.Background(), invalidAddr, []string{"x"}) }},
		{"SetTags", func() error { return SetTags(context.Background(), invalidAddr, []string{"x"}) }},
		{"ReplaceAnnotations", func() error { return ReplaceAnnotations(context.Background(), invalidAddr, "note") }},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			err := op.fn()
			if err == nil {
				t.Errorf("%s() with invalid address = nil, want error", op.name)
			} else if !strings.Contains(err.Error(), "invalid task address") {
				t.Errorf("%s() error = %v, want error containing 'invalid task address'", op.name, err)
			}
		})
	}
}
