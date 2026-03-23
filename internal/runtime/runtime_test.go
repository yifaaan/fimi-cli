package runtime

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunnerRunAppendsPromptAndEngineReply(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	engine := &spyEngine{
		reply: "assistant placeholder reply: hello",
	}
	runner := New(engine)

	result, err := runner.Run(ctx, Input{
		Prompt:       " hello ",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
		},
	})
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
	if engine.gotInput.Prompt != "hello" {
		t.Fatalf("engine got Prompt = %q, want %q", engine.gotInput.Prompt, "hello")
	}
	if engine.gotInput.Model != "kimi-k2-turbo-preview" {
		t.Fatalf("engine got Model = %q, want %q", engine.gotInput.Model, "kimi-k2-turbo-preview")
	}
	if engine.gotInput.SystemPrompt != "You are fimi, a coding agent." {
		t.Fatalf("engine got SystemPrompt = %q, want %q", engine.gotInput.SystemPrompt, "You are fimi, a coding agent.")
	}
	if !reflect.DeepEqual(engine.gotInput.History, []contextstore.TextRecord{
		contextstore.NewUserTextRecord("previous"),
		contextstore.NewAssistantTextRecord("previous reply"),
	}) {
		t.Fatalf("engine got History = %#v, want %#v", engine.gotInput.History, []contextstore.TextRecord{
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
		})
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

func TestRunnerRunReturnsMissingEngineError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(nil)

	_, err := runner.Run(ctx, Input{Prompt: "hello"})
	if err == nil {
		t.Fatalf("Run() error = nil, want non-nil")
	}
	if err.Error() != "build assistant reply: runtime engine is required" {
		t.Fatalf("Run() error = %q, want %q", err.Error(), "build assistant reply: runtime engine is required")
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

type spyEngine struct {
	gotInput Input
	reply    string
	err      error
}

func (e *spyEngine) Reply(input Input) (string, error) {
	e.gotInput = input
	return e.reply, e.err
}
