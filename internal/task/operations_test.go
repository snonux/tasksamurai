package task

import (
	"strings"
	"testing"
)

func TestModifyTask(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ID",
			id:      1,
			args:    []string{"status:pending"},
			wantErr: false,
		},
		{
			name:    "zero ID",
			id:      0,
			args:    []string{"status:pending"},
			wantErr: true,
			errMsg:  "invalid task ID: 0",
		},
		{
			name:    "negative ID",
			id:      -1,
			args:    []string{"status:pending"},
			wantErr: true,
			errMsg:  "invalid task ID: -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := modifyTask(tt.id, tt.args...)
			
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
		id      int
		command string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ID",
			id:      1,
			command: "done",
			wantErr: false,
		},
		{
			name:    "zero ID",
			id:      0,
			command: "done",
			wantErr: true,
			errMsg:  "invalid task ID: 0",
		},
		{
			name:    "negative ID",
			id:      -5,
			command: "done",
			wantErr: true,
			errMsg:  "invalid task ID: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := simpleTaskCommand(tt.id, tt.command)
			
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
	// Test that all task operations validate IDs
	invalidID := -1
	
	operations := []struct {
		name string
		fn   func() error
	}{
		{"SetStatus", func() error { return SetStatus(invalidID, "pending") }},
		{"Start", func() error { return Start(invalidID) }},
		{"Stop", func() error { return Stop(invalidID) }},
		{"Done", func() error { return Done(invalidID) }},
		{"Delete", func() error { return Delete(invalidID) }},
		{"SetPriority", func() error { return SetPriority(invalidID, "H") }},
		{"SetRecurrence", func() error { return SetRecurrence(invalidID, "daily") }},
		{"SetDueDate", func() error { return SetDueDate(invalidID, "tomorrow") }},
		{"SetDescription", func() error { return SetDescription(invalidID, "test") }},
		{"Annotate", func() error { return Annotate(invalidID, "note") }},
		{"Denotate", func() error { return Denotate(invalidID, "note") }},
	}
	
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			err := op.fn()
			if err == nil {
				t.Errorf("%s() with invalid ID = nil, want error", op.name)
			} else if !strings.Contains(err.Error(), "invalid task ID") {
				t.Errorf("%s() error = %v, want error containing 'invalid task ID'", op.name, err)
			}
		})
	}
}