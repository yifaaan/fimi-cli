package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunnerAdvanceToolCallStepPersistsToolRecords(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	executor := &spyToolExecutor{}
	runner := Runner{toolExecutor: executor}
	step := StepResult{
		Status:        StepStatusIncomplete,
		Kind:          StepKindToolCalls,
		AssistantText: "I will inspect the file.",
		ToolCalls: []ToolCall{
			{ID: "call_read", Name: "ReadFile", Arguments: `{"path":"main.go"}`},
		},
	}

	got, err := runner.advanceToolCallStep(context.Background(), store, Result{}, step)
	if err != nil {
		t.Fatalf("advanceToolCallStep() error = %v", err)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("len(got.Steps) = %d, want 1", len(got.Steps))
	}
	if len(got.Steps[0].ToolExecutions) != 1 {
		t.Fatalf("len(got.Steps[0].ToolExecutions) = %d, want 1", len(got.Steps[0].ToolExecutions))
	}

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].Role != contextstore.RoleAssistant {
		t.Fatalf("records[0].Role = %q, want %q", records[0].Role, contextstore.RoleAssistant)
	}
	if records[0].Content != "I will inspect the file." {
		t.Fatalf("records[0].Content = %q, want %q", records[0].Content, "I will inspect the file.")
	}
	if records[1].Role != contextstore.RoleTool {
		t.Fatalf("records[1].Role = %q, want %q", records[1].Role, contextstore.RoleTool)
	}
	if records[1].ToolCallID != "call_read" {
		t.Fatalf("records[1].ToolCallID = %q, want %q", records[1].ToolCallID, "call_read")
	}
}
