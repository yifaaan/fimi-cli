package runtime

import (
	"path/filepath"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunAppendsPromptAndPlaceholder(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	result, err := Run(ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.AppendedRecords) != 2 {
		t.Fatalf("len(AppendedRecords) = %d, want 2", len(result.AppendedRecords))
	}

	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("len(history records) = %d, want 2", len(records))
	}

	if records[0] != contextstore.NewUserTextRecord("hello") {
		t.Fatalf("records[0] = %#v, want %#v", records[0], contextstore.NewUserTextRecord("hello"))
	}

	wantAssistant := contextstore.NewAssistantTextRecord("assistant placeholder reply: hello")
	if records[1] != wantAssistant {
		t.Fatalf("records[1] = %#v, want %#v", records[1], wantAssistant)
	}
}

func TestRunSkipsEmptyPrompt(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	result, err := Run(ctx, Input{Prompt: "   "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.AppendedRecords) != 0 {
		t.Fatalf("len(AppendedRecords) = %d, want 0", len(result.AppendedRecords))
	}

	count, err := ctx.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() = %d, want 0", count)
	}
}
