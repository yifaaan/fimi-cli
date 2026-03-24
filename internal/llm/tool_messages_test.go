package llm

import (
	"errors"
	"reflect"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestBuildToolStepMessagesBuildsAssistantAndToolResultMessages(t *testing.T) {
	step := runtime.StepResult{
		Kind:          runtime.StepKindToolCalls,
		AssistantText: "I will inspect the file.",
		ToolCalls: []runtime.ToolCall{
			{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
			{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
		},
		ToolExecutions: []runtime.ToolExecution{
			{
				Call:   runtime.ToolCall{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
				Output: "package main\n",
			},
			{
				Call:     runtime.ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
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
			Role:    RoleAssistant,
			Content: "I will inspect the file.",
			ToolCalls: []ToolCall{
				{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
				{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_read",
			Content:    "package main\n",
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_bash",
			Content:    "stdout:\n/repo\n\nstderr:\nwarn\n\nexit_code: 0",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildToolStepMessages() = %#v, want %#v", got, want)
	}
}

func TestBuildToolStepMessagesBuildsTemporaryFailureMessage(t *testing.T) {
	step := runtime.StepResult{
		Kind:          runtime.StepKindToolCalls,
		AssistantText: "I will run the command.",
		ToolCalls: []runtime.ToolCall{
			{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
		},
		ToolFailure: &runtime.ToolExecutionError{
			Call: runtime.ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			Err:  temporaryToolMessageError{err: errors.New("bash timed out")},
		},
	}

	got, err := BuildToolStepMessages(step)
	if err != nil {
		t.Fatalf("BuildToolStepMessages() error = %v", err)
	}

	want := []Message{
		{
			Role:    RoleAssistant,
			Content: "I will run the command.",
			ToolCalls: []ToolCall{
				{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		},
		{
			Role:       RoleTool,
			ToolCallID: "call_bash",
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
			{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
		},
		ToolFailure: &runtime.ToolExecutionError{
			Call: runtime.ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
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

func TestBuildToolStepMessagesReturnsErrorWhenToolCallIDMissing(t *testing.T) {
	_, err := BuildToolStepMessages(runtime.StepResult{
		Kind: runtime.StepKindToolCalls,
		ToolCalls: []runtime.ToolCall{
			{Name: "bash", Arguments: `{"command":"pwd"}`},
		},
	})
	if !errors.Is(err, ErrToolCallIDRequired) {
		t.Fatalf("BuildToolStepMessages() error = %v, want wrapped %v", err, ErrToolCallIDRequired)
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
