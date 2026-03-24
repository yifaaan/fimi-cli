package llm

import (
	"errors"
	"reflect"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestBuildToolStepMessagesBuildsAssistantAndToolResultMessages(t *testing.T) {
	step := runtime.StepResult{
		Kind: runtime.StepKindToolCalls,
		ToolCalls: []runtime.ToolCall{
			{Name: "read_file", Arguments: `{"path":"main.go"}`},
			{Name: "bash", Arguments: `{"command":"pwd"}`},
		},
		ToolExecutions: []runtime.ToolExecution{
			{
				Call:   runtime.ToolCall{Name: "read_file", Arguments: `{"path":"main.go"}`},
				Output: "package main\n",
			},
			{
				Call:     runtime.ToolCall{Name: "bash", Arguments: `{"command":"pwd"}`},
				Stdout:   "/repo\n",
				Stderr:   "warn\n",
				ExitCode: 0,
			},
		},
	}

	got, err := BuildToolStepMessages(step)
	if err != nil {
		t.Fatalf("BuildToolStepMessages() error = %v", err)
	}

	want := []Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "read_file", Arguments: `{"path":"main.go"}`},
				{ID: "call_2", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_1",
			Content:    "package main\n",
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_2",
			Content:    "stdout:\n/repo\n\nstderr:\nwarn\n\nexit_code: 0",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildToolStepMessages() = %#v, want %#v", got, want)
	}
}

func TestBuildToolStepMessagesBuildsTemporaryFailureMessage(t *testing.T) {
	step := runtime.StepResult{
		Kind: runtime.StepKindToolCalls,
		ToolCalls: []runtime.ToolCall{
			{Name: "bash", Arguments: `{"command":"pwd"}`},
		},
		ToolFailure: &runtime.ToolExecutionError{
			Call: runtime.ToolCall{Name: "bash", Arguments: `{"command":"pwd"}`},
			Err:  temporaryToolMessageError{err: errors.New("bash timed out")},
		},
	}

	got, err := BuildToolStepMessages(step)
	if err != nil {
		t.Fatalf("BuildToolStepMessages() error = %v", err)
	}

	want := []Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_1",
			Content:    "error: bash timed out\nfailure_kind: temporary",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildToolStepMessages() = %#v, want %#v", got, want)
	}
}

func TestBuildToolStepMessagesReturnsErrorWhenFailureCallMissing(t *testing.T) {
	step := runtime.StepResult{
		Kind: runtime.StepKindToolCalls,
		ToolCalls: []runtime.ToolCall{
			{Name: "read_file", Arguments: `{"path":"main.go"}`},
		},
		ToolFailure: &runtime.ToolExecutionError{
			Call: runtime.ToolCall{Name: "bash", Arguments: `{"command":"pwd"}`},
			Err:  errors.New("bash failed"),
		},
	}

	_, err := BuildToolStepMessages(step)
	if !errors.Is(err, ErrToolFailureCallNotFound) {
		t.Fatalf("BuildToolStepMessages() error = %v, want wrapped %v", err, ErrToolFailureCallNotFound)
	}
}

func TestBuildToolStepMessagesReturnsErrorForNonToolStep(t *testing.T) {
	_, err := BuildToolStepMessages(runtime.StepResult{
		Kind: runtime.StepKindFinished,
	})
	if !errors.Is(err, ErrToolStepRequired) {
		t.Fatalf("BuildToolStepMessages() error = %v, want wrapped %v", err, ErrToolStepRequired)
	}
}

type temporaryToolMessageError struct {
	err error
}

func (e temporaryToolMessageError) Error() string {
	return e.err.Error()
}

func (e temporaryToolMessageError) Unwrap() error {
	return e.err
}

func (temporaryToolMessageError) Temporary() bool {
	return true
}
