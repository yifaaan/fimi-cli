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
	if err := ctx.Append(contextstore.NewUserTextRecord("previous")); err != nil {
		t.Fatalf("Append(previous user) error = %v", err)
	}
	if err := ctx.Append(contextstore.NewAssistantTextRecord("previous reply")); err != nil {
		t.Fatalf("Append(previous assistant) error = %v", err)
	}

	engine := &spyEngine{
		reply: "assistant placeholder reply: hello",
	}
	runner := New(engine, Config{})

	result, err := runner.Run(ctx, Input{
		Prompt:       " hello ",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(result.Steps))
	}
	if result.Status != RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFinished)
	}
	step := result.Steps[0]
	if step.Status != StepStatusFinished {
		t.Fatalf("Steps[0].Status = %q, want %q", result.Steps[0].Status, StepStatusFinished)
	}
	if step.Kind != StepKindFinished {
		t.Fatalf("Steps[0].Kind = %q, want %q", result.Steps[0].Kind, StepKindFinished)
	}
	if len(step.AppendedRecords) != 2 {
		t.Fatalf("len(Steps[0].AppendedRecords) = %d, want 2", len(step.AppendedRecords))
	}
	if len(step.ToolCalls) != 0 {
		t.Fatalf("len(Steps[0].ToolCalls) = %d, want 0", len(step.ToolCalls))
	}

	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(records) != 4 {
		t.Fatalf("len(history records) = %d, want 4", len(records))
	}

	if records[0] != contextstore.NewUserTextRecord("previous") {
		t.Fatalf("records[0] = %#v, want %#v", records[0], contextstore.NewUserTextRecord("previous"))
	}
	if records[1] != contextstore.NewAssistantTextRecord("previous reply") {
		t.Fatalf("records[1] = %#v, want %#v", records[1], contextstore.NewAssistantTextRecord("previous reply"))
	}
	if records[2] != contextstore.NewUserTextRecord("hello") {
		t.Fatalf("records[2] = %#v, want %#v", records[2], contextstore.NewUserTextRecord("hello"))
	}

	wantAssistant := contextstore.NewAssistantTextRecord("assistant placeholder reply: hello")
	if records[3] != wantAssistant {
		t.Fatalf("records[3] = %#v, want %#v", records[3], wantAssistant)
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
	runner := New(engine, Config{})

	result, err := runner.Run(ctx, Input{Prompt: "   "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Steps) != 0 {
		t.Fatalf("len(Steps) = %d, want 0", len(result.Steps))
	}
	if result.Status != RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFinished)
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
	}, Config{})

	result, err := runner.Run(ctx, Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
}

func TestRunnerRunReturnsMissingEngineError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(nil, Config{})

	result, err := runner.Run(ctx, Input{Prompt: "hello"})
	if err == nil {
		t.Fatalf("Run() error = nil, want non-nil")
	}
	if err.Error() != "build assistant reply: runtime engine is required" {
		t.Fatalf("Run() error = %q, want %q", err.Error(), "build assistant reply: runtime engine is required")
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
}

func TestRunnerRunReadsRecentTurnWindow(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	records := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("boot"),
		contextstore.NewUserTextRecord("u1"),
		contextstore.NewAssistantTextRecord("a1"),
		contextstore.NewUserTextRecord("u2"),
		contextstore.NewAssistantTextRecord("a2"),
		contextstore.NewUserTextRecord("u3"),
		contextstore.NewAssistantTextRecord("a3"),
		contextstore.NewUserTextRecord("u4"),
		contextstore.NewAssistantTextRecord("a4"),
		contextstore.NewUserTextRecord("u5"),
	}
	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	engine := &spyEngine{
		reply: "assistant placeholder reply: hello",
	}
	runner := New(engine, Config{})

	_, err := runner.Run(ctx, Input{
		Prompt:       "hello",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("u2"),
		contextstore.NewAssistantTextRecord("a2"),
		contextstore.NewUserTextRecord("u3"),
		contextstore.NewAssistantTextRecord("a3"),
		contextstore.NewUserTextRecord("u4"),
		contextstore.NewAssistantTextRecord("a4"),
		contextstore.NewUserTextRecord("u5"),
	}
	if !reflect.DeepEqual(engine.gotInput.History, want) {
		t.Fatalf("engine got History = %#v, want %#v", engine.gotInput.History, want)
	}
}

func TestRunnerRunUsesConfiguredTurnLimit(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("u1"),
		contextstore.NewAssistantTextRecord("a1"),
		contextstore.NewUserTextRecord("u2"),
		contextstore.NewAssistantTextRecord("a2"),
	}
	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	engine := &spyEngine{
		reply: "assistant placeholder reply: hello",
	}
	runner := New(engine, Config{
		ReplyHistoryTurnLimit: 1,
	})

	_, err := runner.Run(ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("u2"),
		contextstore.NewAssistantTextRecord("a2"),
	}
	if !reflect.DeepEqual(engine.gotInput.History, want) {
		t.Fatalf("engine got History = %#v, want %#v", engine.gotInput.History, want)
	}
}

func TestRunnerRunReturnsMaxStepsStatusWhenLoopExhausted(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{}, Config{
		MaxStepsPerRun: 1,
	})
	runner.runStepFn = func(ctx contextstore.Context, input Input, prompt string) (StepResult, error) {
		return StepResult{
			Status: StepStatusIncomplete,
			Kind:   StepKindToolCalls,
			ToolCalls: []ToolCall{
				{Name: "ReadFile", Arguments: `{"path":"main.go"}`},
			},
		}, nil
	}

	result, err := runner.Run(ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != RunStatusMaxSteps {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusMaxSteps)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 1)
	}
}

func TestNewUsesDefaultMaxStepsPerRunWhenInvalid(t *testing.T) {
	runner := New(staticEngine{}, Config{
		ReplyHistoryTurnLimit: 1,
		MaxStepsPerRun:        0,
	})

	if runner.config.ReplyHistoryTurnLimit != 1 {
		t.Fatalf("runner.config.ReplyHistoryTurnLimit = %d, want %d", runner.config.ReplyHistoryTurnLimit, 1)
	}
	if runner.config.MaxStepsPerRun != DefaultMaxStepsPerRun {
		t.Fatalf("runner.config.MaxStepsPerRun = %d, want %d", runner.config.MaxStepsPerRun, DefaultMaxStepsPerRun)
	}
}

func TestRunnerAdvanceRunFinishesOnFinishedStep(t *testing.T) {
	runner := Runner{}
	initial := Result{}
	step := StepResult{
		Status: StepStatusFinished,
		Kind: StepKindFinished,
		AppendedRecords: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
			contextstore.NewAssistantTextRecord("world"),
		},
	}

	got, finished, err := runner.advanceRun(initial, step)
	if err != nil {
		t.Fatalf("advanceRun() error = %v", err)
	}
	if !finished {
		t.Fatalf("advanceRun() finished = false, want true")
	}
	if !reflect.DeepEqual(got.Steps, []StepResult{step}) {
		t.Fatalf("advanceRun().Steps = %#v, want %#v", got.Steps, []StepResult{step})
	}
}

func TestRunnerAdvanceRunContinuesOnToolCallStep(t *testing.T) {
	runner := Runner{}
	step := StepResult{
		Status: StepStatusIncomplete,
		Kind: StepKindToolCalls,
		ToolCalls: []ToolCall{
			{Name: "ReadFile", Arguments: `{"path":"main.go"}`},
		},
	}

	got, finished, err := runner.advanceRun(Result{}, step)
	if err != nil {
		t.Fatalf("advanceRun() error = %v", err)
	}
	if finished {
		t.Fatalf("advanceRun() finished = true, want false")
	}
	if !reflect.DeepEqual(got.Steps, []StepResult{step}) {
		t.Fatalf("advanceRun().Steps = %#v, want %#v", got.Steps, []StepResult{step})
	}
}

func TestRunnerAdvanceRunRejectsUnknownStepStatus(t *testing.T) {
	runner := Runner{}
	_, _, err := runner.advanceRun(Result{}, StepResult{
		Status: StepStatus("bad-status"),
		Kind:   StepKindFinished,
	})
	if !errors.Is(err, ErrUnknownStepStatus) {
		t.Fatalf("advanceRun() error = %v, want wrapped %v", err, ErrUnknownStepStatus)
	}
}

func TestRunnerAdvanceRunRejectsUnknownStepKind(t *testing.T) {
	runner := Runner{}
	_, _, err := runner.advanceRun(Result{}, StepResult{
		Status: StepStatusFinished,
		Kind: StepKind("bad-kind"),
	})
	if !errors.Is(err, ErrUnknownStepKind) {
		t.Fatalf("advanceRun() error = %v, want wrapped %v", err, ErrUnknownStepKind)
	}
}

type staticEngine struct {
	reply string
	err   error
}

func (e staticEngine) Reply(input ReplyInput) (string, error) {
	return e.reply, e.err
}

type trackingEngine struct {
	called bool
}

func (e *trackingEngine) Reply(input ReplyInput) (string, error) {
	e.called = true
	return "unused", nil
}

type spyEngine struct {
	gotInput ReplyInput
	reply    string
	err      error
}

func (e *spyEngine) Reply(input ReplyInput) (string, error) {
	e.gotInput = input
	return e.reply, e.err
}
