package tools

import (
	"errors"
	"reflect"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestExecutorExecuteUsesNoopForAllowedToolWithoutHandler(t *testing.T) {
	executor := NewExecutor([]Definition{
		{
			Name:        ToolReadFile,
			Kind:        KindFile,
			Description: "Read a file from the workspace.",
		},
	}, nil)

	got, err := executor.Execute(runtime.ToolCall{
		Name:      " read_file ",
		Arguments: `{"path":"main.go"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := runtime.ToolExecution{
		Call: runtime.ToolCall{
			Name:      ToolReadFile,
			Arguments: `{"path":"main.go"}`,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Execute() = %#v, want %#v", got, want)
	}
}

func TestExecutorExecuteUsesRegisteredHandler(t *testing.T) {
	var gotDefinition Definition
	var gotCall runtime.ToolCall
	executor := NewExecutor([]Definition{
		{
			Name:        ToolBash,
			Kind:        KindCommand,
			Description: "Run a shell command inside the workspace.",
		},
	}, map[string]HandlerFunc{
		ToolBash: func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			gotCall = call
			gotDefinition = definition

			return runtime.ToolExecution{
				Call: runtime.ToolCall{
					Name:      call.Name,
					Arguments: "handled",
				},
			}, nil
		},
	})

	got, err := executor.Execute(runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotCall.Name != ToolBash {
		t.Fatalf("handler got Call.Name = %q, want %q", gotCall.Name, ToolBash)
	}
	if gotDefinition.Name != ToolBash {
		t.Fatalf("handler got Definition.Name = %q, want %q", gotDefinition.Name, ToolBash)
	}
	if got.Call.Arguments != "handled" {
		t.Fatalf("Execute().Call.Arguments = %q, want %q", got.Call.Arguments, "handled")
	}
}

func TestExecutorExecuteReturnsErrorForDisallowedTool(t *testing.T) {
	executor := NewExecutor([]Definition{
		{
			Name: ToolReadFile,
			Kind: KindFile,
		},
	}, nil)

	_, err := executor.Execute(runtime.ToolCall{Name: ToolBash})
	if !errors.Is(err, ErrToolCallNotAllowed) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolCallNotAllowed)
	}
}

func TestExecutorExecuteReturnsErrorForMissingToolName(t *testing.T) {
	executor := NewExecutor(nil, nil)

	_, err := executor.Execute(runtime.ToolCall{Name: "   "})
	if !errors.Is(err, ErrToolCallNameRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolCallNameRequired)
	}
}

func TestExecutorExecuteWrapsHandlerError(t *testing.T) {
	wantErr := errors.New("handler failed")
	executor := NewExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, map[string]HandlerFunc{
		ToolBash: func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			return runtime.ToolExecution{}, wantErr
		},
	})

	_, err := executor.Execute(runtime.ToolCall{Name: ToolBash})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, wantErr)
	}
}
