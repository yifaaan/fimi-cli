package runtime

import (
	"errors"
	"path/filepath"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunnerRunAppendsPromptAndEngineReply(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{
		reply: "assistant placeholder reply: hello",
	})

	result, err := runner.Run(ctx, Input{Prompt: "hello"})
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

func TestRunnerRunSkipsEmptyPrompt(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	engine := &trackingEngine{}
	runner := New(engine)

	result, err := runner.Run(ctx, Input{Prompt: "   "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.AppendedRecords) != 0 {
		t.Fatalf("len(AppendedRecords) = %d, want 0", len(result.AppendedRecords))
	}

	if engine.called {
		t.Fatalf("engine called = true, want false")
	}

	count, err := ctx.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() = %d, want 0", count)
	}
}

func TestRunnerRunReturnsEngineError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	wantErr := errors.New("engine failed")
	runner := New(staticEngine{
		err: wantErr,
	})

	_, err := runner.Run(ctx, Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewUsesPlaceholderEngineByDefault(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(nil)

	result, err := runner.Run(ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantAssistant := contextstore.NewAssistantTextRecord("assistant placeholder reply: hello")
	if result.AppendedRecords[1] != wantAssistant {
		t.Fatalf("result.AppendedRecords[1] = %#v, want %#v", result.AppendedRecords[1], wantAssistant)
	}
}

type staticEngine struct {
	reply string
	err   error
}

func (e staticEngine) Reply(input Input) (string, error) {
	return e.reply, e.err
}

type trackingEngine struct {
	called bool
}

func (e *trackingEngine) Reply(input Input) (string, error) {
	e.called = true
	return "unused", nil
}
