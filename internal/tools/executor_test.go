package tools

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestExecutorExecuteUsesNoopForAllowedToolWithoutHandler(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor([]Definition{
		{
			Name:        ToolReadFile,
			Kind:        KindFile,
			Description: "Read a file from the workspace.",
		},
	}, nil)

	got, err := executor.Execute(ctx, runtime.ToolCall{
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
	ctx := context.Background()
	var gotDefinition Definition
	var gotCall runtime.ToolCall
	executor := NewExecutor([]Definition{
		{
			Name:        ToolBash,
			Kind:        KindCommand,
			Description: "Run a shell command inside the workspace.",
		},
	}, map[string]HandlerFunc{
		ToolBash: func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			gotCall = call
			gotDefinition = definition

			return runtime.ToolExecution{
				Call: runtime.ToolCall{
					Name:      call.Name,
					Arguments: "handled",
				},
				Output: definition.Description,
			}, nil
		},
	})

	got, err := executor.Execute(ctx, runtime.ToolCall{
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
	if got.Output != "Run a shell command inside the workspace." {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "Run a shell command inside the workspace.")
	}
}

func TestExecutorExecuteReturnsErrorForDisallowedTool(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor([]Definition{
		{
			Name: ToolReadFile,
			Kind: KindFile,
		},
	}, nil)

	_, err := executor.Execute(ctx, runtime.ToolCall{Name: ToolBash})
	if !errors.Is(err, ErrToolCallNotAllowed) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolCallNotAllowed)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestExecutorExecuteReturnsErrorForMissingToolName(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor(nil, nil)

	_, err := executor.Execute(ctx, runtime.ToolCall{Name: "   "})
	if !errors.Is(err, ErrToolCallNameRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolCallNameRequired)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestExecutorExecuteWrapsHandlerError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("handler failed")
	executor := NewExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, map[string]HandlerFunc{
		ToolBash: func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			return runtime.ToolExecution{}, wantErr
		},
	})

	_, err := executor.Execute(ctx, runtime.ToolCall{Name: ToolBash})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestExecutorExecutePreservesTemporaryClassification(t *testing.T) {
	ctx := context.Background()
	wantErr := temporaryHandlerError{err: errors.New("runner unavailable")}
	executor := NewExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, map[string]HandlerFunc{
		ToolBash: func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			return runtime.ToolExecution{}, wantErr
		},
	})

	_, err := executor.Execute(ctx, runtime.ToolCall{Name: ToolBash})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, wantErr)
	}
	if !runtime.IsTemporary(err) {
		t.Fatalf("runtime.IsTemporary(error) = false, want true")
	}
	if runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = true, want false")
	}
}

type temporaryHandlerError struct {
	err error
}

func (e temporaryHandlerError) Error() string {
	return e.err.Error()
}

func (e temporaryHandlerError) Unwrap() error {
	return e.err
}

func (temporaryHandlerError) Temporary() bool {
	return true
}
