package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
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
		reply: AssistantReply{
			Text: "assistant placeholder reply: hello",
		},
	}
	runner := New(engine, Config{})

	result, err := runner.Run(context.Background(), ctx, Input{
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
	if len(step.AppendedRecords) != 1 {
		t.Fatalf("len(Steps[0].AppendedRecords) = %d, want 1 (only assistant; user prompt appended by Run)", len(step.AppendedRecords))
	}
	if len(step.ToolCalls) != 0 {
		t.Fatalf("len(Steps[0].ToolCalls) = %d, want 0", len(step.ToolCalls))
	}
	if len(step.ToolExecutions) != 0 {
		t.Fatalf("len(Steps[0].ToolExecutions) = %d, want 0", len(step.ToolExecutions))
	}
	if step.ToolFailure != nil {
		t.Fatalf("Steps[0].ToolFailure = %#v, want nil", step.ToolFailure)
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
	if step.AssistantText != "assistant placeholder reply: hello" {
		t.Fatalf("Steps[0].AssistantText = %q, want %q", step.AssistantText, "assistant placeholder reply: hello")
	}
	// 多 step 场景下，用户 prompt 已在 history 中，不再单独传递
	if engine.gotInput.Model != "kimi-k2-turbo-preview" {
		t.Fatalf("engine got Model = %q, want %q", engine.gotInput.Model, "kimi-k2-turbo-preview")
	}
	if engine.gotInput.SystemPrompt != "You are fimi, a coding agent." {
		t.Fatalf("engine got SystemPrompt = %q, want %q", engine.gotInput.SystemPrompt, "You are fimi, a coding agent.")
	}
	if !reflect.DeepEqual(engine.gotInput.History, []contextstore.TextRecord{
		contextstore.NewUserTextRecord("previous"),
		contextstore.NewAssistantTextRecord("previous reply"),
		contextstore.NewUserTextRecord("hello"), // 用户 prompt 由 Run() 在开始时追加
	}) {
		t.Fatalf("engine got History = %#v, want %#v", engine.gotInput.History, []contextstore.TextRecord{
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
			contextstore.NewUserTextRecord("hello"),
		})
	}
}

func TestRunnerRunSkipsEmptyPrompt(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	engine := &trackingEngine{}
	runner := New(engine, Config{})

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "   "})
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

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
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

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
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
		reply: AssistantReply{
			Text: "assistant placeholder reply: hello",
		},
	}
	runner := New(engine, Config{})

	_, err := runner.Run(context.Background(), ctx, Input{
		Prompt:       "hello",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Run() 在开始时追加 user prompt "hello"，所以 history 现在包含：
	// boot, u1, a1, u2, a2, u3, a3, u4, a4, u5, hello
	// turn limit = 4 时，最近的 4 个 user turn 是：u3, u4, u5, hello
	// 所以 history 应该是：u3, a3, u4, a4, u5, hello
	want := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("u3"),
		contextstore.NewAssistantTextRecord("a3"),
		contextstore.NewUserTextRecord("u4"),
		contextstore.NewAssistantTextRecord("a4"),
		contextstore.NewUserTextRecord("u5"),
		contextstore.NewUserTextRecord("hello"), // 当前 run 追加的 prompt
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
		reply: AssistantReply{
			Text: "assistant placeholder reply: hello",
		},
	}
	runner := New(engine, Config{
		ReplyHistoryTurnLimit: 1,
	})

	_, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Run() 追加 "hello" 后，history = [u1, a1, u2, a2, hello]
	// turn limit = 1 时，最近 1 个 user turn 是 "hello"
	want := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
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
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		return StepResult{
			Status: StepStatusIncomplete,
			Kind:   StepKindToolCalls,
			ToolCalls: []ToolCall{
				{ID: "call_read", Name: "ReadFile", Arguments: `{"path":"main.go"}`},
			},
		}, nil
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != RunStatusMaxSteps {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusMaxSteps)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 1)
	}
	if len(result.Steps[0].ToolExecutions) != 1 {
		t.Fatalf("len(result.Steps[0].ToolExecutions) = %d, want %d", len(result.Steps[0].ToolExecutions), 1)
	}
	if result.Steps[0].ToolFailure != nil {
		t.Fatalf("result.Steps[0].ToolFailure = %#v, want nil", result.Steps[0].ToolFailure)
	}
}

func TestNewUsesDefaultMaxStepsPerRunWhenInvalid(t *testing.T) {
	runner := New(staticEngine{}, Config{
		ReplyHistoryTurnLimit: 1,
		MaxStepsPerRun:        0,
		MaxRetriesPerStep:     0,
	})

	if runner.config.ReplyHistoryTurnLimit != 1 {
		t.Fatalf("runner.config.ReplyHistoryTurnLimit = %d, want %d", runner.config.ReplyHistoryTurnLimit, 1)
	}
	if runner.config.MaxStepsPerRun != DefaultMaxStepsPerRun {
		t.Fatalf("runner.config.MaxStepsPerRun = %d, want %d", runner.config.MaxStepsPerRun, DefaultMaxStepsPerRun)
	}
	if runner.config.MaxRetriesPerStep != DefaultMaxRetriesPerStep {
		t.Fatalf("runner.config.MaxRetriesPerStep = %d, want %d", runner.config.MaxRetriesPerStep, DefaultMaxRetriesPerStep)
	}
}

func TestNewWithToolExecutorUsesNoopWhenNil(t *testing.T) {
	runner := NewWithToolExecutor(staticEngine{}, nil, Config{})

	execution, err := runner.toolExecutor.Execute(context.Background(), ToolCall{
		ID:        "call_bash",
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatalf("toolExecutor.Execute() error = %v", err)
	}
	if execution.Call.Name != "bash" {
		t.Fatalf("toolExecutor.Execute().Call.Name = %q, want %q", execution.Call.Name, "bash")
	}
}

func TestNewWithToolExecutorAndEventsUsesNoopWhenNil(t *testing.T) {
	runner := NewWithToolExecutorAndEvents(staticEngine{}, nil, nil, Config{})

	if err := runner.emitEvent(context.Background(), runtimeevents.StepBegin{Number: 1}); err != nil {
		t.Fatalf("emitEvent() error = %v", err)
	}
}

func TestRunnerAdvanceRunFinishesOnFinishedStep(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := Runner{}
	initial := Result{}
	step := StepResult{
		Status: StepStatusFinished,
		Kind:   StepKindFinished,
		AppendedRecords: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
			contextstore.NewAssistantTextRecord("world"),
		},
	}

	got, finished, err := runner.advanceRun(context.Background(), ctx, initial, step)
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
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	executor := &spyToolExecutor{}
	runner := Runner{
		toolExecutor: executor,
	}
	step := StepResult{
		Status:        StepStatusIncomplete,
		Kind:          StepKindToolCalls,
		AssistantText: "I will inspect the file.",
		ToolCalls: []ToolCall{
			{ID: "call_read", Name: "ReadFile", Arguments: `{"path":"main.go"}`},
		},
	}

	got, finished, err := runner.advanceRun(context.Background(), ctx, Result{}, step)
	if err != nil {
		t.Fatalf("advanceRun() error = %v", err)
	}
	if finished {
		t.Fatalf("advanceRun() finished = true, want false")
	}
	if !reflect.DeepEqual(executor.gotCalls, []ToolCall{
		{ID: "call_read", Name: "ReadFile", Arguments: `{"path":"main.go"}`},
	}) {
		t.Fatalf("toolExecutor got Calls = %#v, want %#v", executor.gotCalls, []ToolCall{
			{ID: "call_read", Name: "ReadFile", Arguments: `{"path":"main.go"}`},
		})
	}

	// 验证工具记录已写入 history
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ctx.ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	// 第一条是 assistant 消息（带 tool_calls）
	if records[0].Role != "assistant" {
		t.Fatalf("records[0].Role = %q, want %q", records[0].Role, "assistant")
	}
	if records[0].Content != "I will inspect the file." {
		t.Fatalf("records[0].Content = %q, want %q", records[0].Content, "I will inspect the file.")
	}
	// 第二条是 tool result
	if records[1].Role != "tool" {
		t.Fatalf("records[1].Role = %q, want %q", records[1].Role, "tool")
	}
	if records[1].ToolCallID != "call_read" {
		t.Fatalf("records[1].ToolCallID = %q, want %q", records[1].ToolCallID, "call_read")
	}

	// 验证 step 结果
	if len(got.Steps) != 1 {
		t.Fatalf("len(got.Steps) = %d, want 1", len(got.Steps))
	}
	gotStep := got.Steps[0]
	if gotStep.Status != StepStatusIncomplete {
		t.Fatalf("got.Steps[0].Status = %q, want %q", gotStep.Status, StepStatusIncomplete)
	}
	if len(gotStep.ToolExecutions) != 1 {
		t.Fatalf("len(got.Steps[0].ToolExecutions) = %d, want 1", len(gotStep.ToolExecutions))
	}
	if gotStep.ToolExecutions[0].Call.ID != "call_read" {
		t.Fatalf("got.Steps[0].ToolExecutions[0].Call.ID = %q, want %q", gotStep.ToolExecutions[0].Call.ID, "call_read")
	}
}

func TestRunnerAdvanceRunAppendsFailedToolStepBeforeReturningError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	wantErr := temporaryStepError{err: errors.New("bash timed out")}
	runner := Runner{
		toolExecutor: failingToolExecutor{err: wantErr},
	}
	step := StepResult{
		Status: StepStatusIncomplete,
		Kind:   StepKindToolCalls,
		ToolCalls: []ToolCall{
			{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
		},
	}

	got, finished, err := runner.advanceRun(context.Background(), ctx, Result{}, step)
	if err == nil {
		t.Fatalf("advanceRun() error = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("advanceRun() error = %v, want wrapped %v", err, wantErr)
	}
	if finished {
		t.Fatalf("advanceRun() finished = true, want false")
	}
	if len(got.Steps) != 1 {
		t.Fatalf("len(got.Steps) = %d, want %d", len(got.Steps), 1)
	}
	gotStep := got.Steps[0]
	if gotStep.Status != StepStatusFailed {
		t.Fatalf("got.Steps[0].Status = %q, want %q", gotStep.Status, StepStatusFailed)
	}
	if gotStep.Kind != StepKindToolCalls {
		t.Fatalf("got.Steps[0].Kind = %q, want %q", gotStep.Kind, StepKindToolCalls)
	}
	if !reflect.DeepEqual(gotStep.ToolCalls, step.ToolCalls) {
		t.Fatalf("got.Steps[0].ToolCalls = %#v, want %#v", gotStep.ToolCalls, step.ToolCalls)
	}
	if len(gotStep.ToolExecutions) != 0 {
		t.Fatalf("len(got.Steps[0].ToolExecutions) = %d, want %d", len(gotStep.ToolExecutions), 0)
	}
	if gotStep.ToolFailure == nil {
		t.Fatalf("got.Steps[0].ToolFailure = nil, want non-nil")
	}
	if gotStep.ToolFailure.Call != (ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`}) {
		t.Fatalf("got.Steps[0].ToolFailure.Call = %#v, want %#v", gotStep.ToolFailure.Call, ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`})
	}
	if !IsTemporary(gotStep.ToolFailure) {
		t.Fatalf("IsTemporary(got.Steps[0].ToolFailure) = false, want true")
	}

	// 验证失败也写入 tool result 记录
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ctx.ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[1].Role != "tool" {
		t.Fatalf("records[1].Role = %q, want %q", records[1].Role, "tool")
	}
	if records[1].ToolCallID != "call_bash" {
		t.Fatalf("records[1].ToolCallID = %q, want %q", records[1].ToolCallID, "call_bash")
	}
}

func TestRunnerRunStepReturnsToolCallStepWithoutAppendingHistory(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	engine := &spyEngine{
		reply: AssistantReply{
			Text: "I will inspect the repository.",
			ToolCalls: []ToolCall{
				{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
			},
		},
	}
	runner := New(engine, Config{})

	step, err := runner.runStep(context.Background(), ctx, StepConfig{
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
	})
	if err != nil {
		t.Fatalf("runStep() error = %v", err)
	}

	if step.Status != StepStatusIncomplete {
		t.Fatalf("step.Status = %q, want %q", step.Status, StepStatusIncomplete)
	}
	if step.Kind != StepKindToolCalls {
		t.Fatalf("step.Kind = %q, want %q", step.Kind, StepKindToolCalls)
	}
	if step.AssistantText != "I will inspect the repository." {
		t.Fatalf("step.AssistantText = %q, want %q", step.AssistantText, "I will inspect the repository.")
	}
	if !reflect.DeepEqual(step.ToolCalls, []ToolCall{
		{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
	}) {
		t.Fatalf("step.ToolCalls = %#v, want %#v", step.ToolCalls, []ToolCall{
			{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
		})
	}
	if len(step.AppendedRecords) != 0 {
		t.Fatalf("len(step.AppendedRecords) = %d, want 0", len(step.AppendedRecords))
	}

	count, err := ctx.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() = %d, want 0", count)
	}
}

func TestRunnerRunExecutesToolCallsBeforeFinishing(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	executor := &spyToolExecutor{}
	runner := NewWithToolExecutor(staticEngine{}, executor, Config{
		MaxStepsPerRun: 2,
	})

	stepIndex := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		stepIndex++
		if stepIndex == 1 {
			return StepResult{
				Status: StepStatusIncomplete,
				Kind:   StepKindToolCalls,
				ToolCalls: []ToolCall{
					{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
				},
			}, nil
		}

		return StepResult{
			Status: StepStatusFinished,
			Kind:   StepKindFinished,
		}, nil
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFinished)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 2)
	}
	if len(result.Steps[0].ToolExecutions) != 1 {
		t.Fatalf("len(result.Steps[0].ToolExecutions) = %d, want %d", len(result.Steps[0].ToolExecutions), 1)
	}
	if len(executor.gotCalls) != 1 {
		t.Fatalf("len(toolExecutor got Calls) = %d, want %d", len(executor.gotCalls), 1)
	}
}

func TestRunnerRunReturnsToolExecutorError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	wantErr := errors.New("tool executor failed")
	runner := NewWithToolExecutor(staticEngine{}, failingToolExecutor{
		err: wantErr,
	}, Config{
		MaxStepsPerRun: 1,
	})
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		return StepResult{
			Status: StepStatusIncomplete,
			Kind:   StepKindToolCalls,
			ToolCalls: []ToolCall{
				{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		}, nil
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	var toolErr ToolExecutionError
	if !errors.As(err, &toolErr) {
		t.Fatalf("Run() error = %v, want ToolExecutionError", err)
	}
	if toolErr.Call != (ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`}) {
		t.Fatalf("toolErr.Call = %#v, want %#v", toolErr.Call, ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`})
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 1)
	}
	if result.Steps[0].Status != StepStatusFailed {
		t.Fatalf("result.Steps[0].Status = %q, want %q", result.Steps[0].Status, StepStatusFailed)
	}
	if result.Steps[0].ToolFailure == nil {
		t.Fatalf("result.Steps[0].ToolFailure = nil, want non-nil")
	}
}

func TestRunnerRunStopsImmediatelyOnTemporaryToolExecutionError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := NewWithToolExecutor(staticEngine{}, failingToolExecutor{
		err: temporaryStepError{err: errors.New("bash timed out")},
	}, Config{
		MaxStepsPerRun: 3,
	})

	stepCalls := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		stepCalls++
		return StepResult{
			Status: StepStatusIncomplete,
			Kind:   StepKindToolCalls,
			ToolCalls: []ToolCall{
				{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		}, nil
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err == nil {
		t.Fatalf("Run() error = nil, want non-nil")
	}
	if !IsTemporary(err) {
		t.Fatalf("IsTemporary(error) = false, want true")
	}
	var toolErr ToolExecutionError
	if !errors.As(err, &toolErr) {
		t.Fatalf("Run() error = %v, want ToolExecutionError", err)
	}
	if toolErr.Call != (ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`}) {
		t.Fatalf("toolErr.Call = %#v, want %#v", toolErr.Call, ToolCall{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`})
	}
	if stepCalls != 1 {
		t.Fatalf("runStep calls = %d, want %d", stepCalls, 1)
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 1)
	}
	step := result.Steps[0]
	if step.Status != StepStatusFailed {
		t.Fatalf("result.Steps[0].Status = %q, want %q", step.Status, StepStatusFailed)
	}
	if step.ToolFailure == nil {
		t.Fatalf("result.Steps[0].ToolFailure = nil, want non-nil")
	}
	if !IsTemporary(step.ToolFailure) {
		t.Fatalf("IsTemporary(result.Steps[0].ToolFailure) = false, want true")
	}
}

func TestRunnerRunRetriesRetryableStepError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{}, Config{
		MaxRetriesPerStep: 3,
	})

	attempts := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		attempts++
		if attempts < 3 {
			return StepResult{}, retryableStepError{err: errors.New("temporary llm outage")}
		}

		return StepResult{
			Status: StepStatusFinished,
			Kind:   StepKindFinished,
		}, nil
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("runStep attempts = %d, want %d", attempts, 3)
	}
	if result.Status != RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFinished)
	}
}

func TestRunnerRunStopsAtConfiguredRetryAttemptLimit(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	wantErr := retryableStepError{err: errors.New("temporary llm outage")}
	runner := New(staticEngine{}, Config{
		MaxRetriesPerStep: 2,
	})

	attempts := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		attempts++
		return StepResult{}, wantErr
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if attempts != 2 {
		t.Fatalf("runStep attempts = %d, want %d", attempts, 2)
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
}

func TestRunnerRunDoesNotRetryNonRetryableStepError(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	wantErr := errors.New("bad request")
	runner := New(staticEngine{}, Config{
		MaxRetriesPerStep: 3,
	})

	attempts := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		attempts++
		return StepResult{}, wantErr
	}

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("runStep attempts = %d, want %d", attempts, 1)
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
}

func TestRunnerAdvanceRunRejectsUnknownStepStatus(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := Runner{}
	advanceRun := runner.advanceRun
	_, _, err := advanceRun(context.Background(), ctx, Result{}, StepResult{
		Status: StepStatus("bad-status"),
		Kind:   StepKindFinished,
	})
	if !errors.Is(err, ErrUnknownStepStatus) {
		t.Fatalf("advanceRun() error = %v, want wrapped %v", err, ErrUnknownStepStatus)
	}
}

func TestRunnerAdvanceRunRejectsUnknownStepKind(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := Runner{}
	advanceRun := runner.advanceRun
	_, _, err := advanceRun(context.Background(), ctx, Result{}, StepResult{
		Status: StepStatusFinished,
		Kind:   StepKind("bad-kind"),
	})
	if !errors.Is(err, ErrUnknownStepKind) {
		t.Fatalf("advanceRun() error = %v, want wrapped %v", err, ErrUnknownStepKind)
	}
}

func TestIsRefusedReturnsTrueForRefusedError(t *testing.T) {
	err := refusedStepError{err: errors.New("path escapes workspace")}

	if !IsRefused(err) {
		t.Fatalf("IsRefused(error) = false, want true")
	}
}

func TestIsRefusedReturnsTrueForWrappedRefusedError(t *testing.T) {
	err := fmt.Errorf("execute tool: %w", refusedStepError{err: errors.New("path escapes workspace")})

	if !IsRefused(err) {
		t.Fatalf("IsRefused(error) = false, want true")
	}
}

func TestIsRefusedReturnsFalseForOrdinaryError(t *testing.T) {
	if IsRefused(errors.New("ordinary failure")) {
		t.Fatalf("IsRefused(error) = true, want false")
	}
}

func TestIsTemporaryReturnsTrueForTemporaryError(t *testing.T) {
	err := temporaryStepError{err: errors.New("bash timed out")}

	if !IsTemporary(err) {
		t.Fatalf("IsTemporary(error) = false, want true")
	}
}

func TestIsTemporaryReturnsTrueForWrappedTemporaryError(t *testing.T) {
	err := fmt.Errorf("execute tool: %w", temporaryStepError{err: errors.New("bash timed out")})

	if !IsTemporary(err) {
		t.Fatalf("IsTemporary(error) = false, want true")
	}
}

func TestIsTemporaryReturnsFalseForOrdinaryError(t *testing.T) {
	if IsTemporary(errors.New("ordinary failure")) {
		t.Fatalf("IsTemporary(error) = true, want false")
	}
}

func TestRunnerRunReturnsInterruptedStatusWhenContextCancelled(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{}, Config{
		MaxStepsPerRun: 10,
	})

	// 创建一个可取消的 context
	rootCtx, cancel := context.WithCancel(context.Background())

	// 让 runStepFn 在第二次调用时发现 ctx 已被取消
	stepCalls := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		stepCalls++
		if stepCalls == 1 {
			// 第一步正常返回 tool_calls，触发后续执行
			return StepResult{
				Status: StepStatusIncomplete,
				Kind:   StepKindToolCalls,
				ToolCalls: []ToolCall{
					{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
				},
			}, nil
		}
		// 第二步不应该被执行
		return StepResult{
			Status: StepStatusFinished,
			Kind:   StepKindFinished,
		}, nil
	}

	// 使用一个 spy executor 来追踪工具是否被执行
	executor := &spyToolExecutor{}
	runner.toolExecutor = executor

	// 在第一次 step 完成后取消 context
	runner.advanceRunFn = func(ctx context.Context, store contextstore.Context, result Result, stepResult StepResult) (Result, bool, error) {
		// 在第一次 advanceRun 时取消 context
		cancel()
		// 继续正常的 advanceRun 逻辑
		return Runner{toolExecutor: executor}.advanceRun(ctx, store, result, stepResult)
	}

	result, err := runner.Run(rootCtx, ctx, Input{Prompt: "hello"})

	// 验证：run 应该因为 ctx 被取消而返回 interrupted 状态
	if result.Status != RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusInterrupted)
	}
	// 只执行了第一步
	if stepCalls != 1 {
		t.Fatalf("stepCalls = %d, want 1 (should stop after cancellation)", stepCalls)
	}
	// 工具执行应该被中断（取决于 cancel 发生的时机）
	// 这里我们允许工具执行完成，因为 cancel 是在 advanceRun 中触发的
	// 关键是后续的 step 不应该再执行
	if err != context.Canceled {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}
}

func TestRunnerRunEmitsFinishedStepEvents(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	sink := &recordingEventSink{}
	runner := NewWithToolExecutorAndEvents(staticEngine{
		reply: AssistantReply{Text: "assistant reply"},
	}, nil, sink, Config{})

	result, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFinished)
	}

	want := []runtimeevents.Event{
		runtimeevents.StepBegin{Number: 1},
		runtimeevents.TextPart{Text: "assistant reply"},
		runtimeevents.StatusUpdate{Status: runtimeevents.StatusSnapshot{}},
	}
	if !reflect.DeepEqual(sink.events, want) {
		t.Fatalf("captured events = %#v, want %#v", sink.events, want)
	}
}

func TestRunnerRunEmitsContextUsageWhenWindowConfigured(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	sink := &recordingEventSink{}
	runner := NewWithToolExecutorAndEvents(staticEngine{
		reply: AssistantReply{
			Text: "assistant reply",
			Usage: Usage{
				TotalTokens: 50,
			},
		},
	}, nil, sink, Config{
		ContextWindowTokens: 100,
	})

	_, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []runtimeevents.Event{
		runtimeevents.StepBegin{Number: 1},
		runtimeevents.TextPart{Text: "assistant reply"},
		runtimeevents.StatusUpdate{
			Status: runtimeevents.StatusSnapshot{ContextUsage: 0.5},
		},
	}
	if !reflect.DeepEqual(sink.events, want) {
		t.Fatalf("captured events = %#v, want %#v", sink.events, want)
	}
}

func TestRunnerRunEmitsToolStepEvents(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	sink := &recordingEventSink{}
	runner := NewWithToolExecutorAndEvents(staticEngine{}, &spyToolExecutor{}, sink, Config{
		MaxStepsPerRun: 2,
	})

	stepIndex := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		stepIndex++
		if stepIndex == 1 {
			return StepResult{
				Status:        StepStatusIncomplete,
				Kind:          StepKindToolCalls,
				AssistantText: "I will inspect the file.",
				ToolCalls: []ToolCall{
					{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
				},
			}, nil
		}

		return StepResult{
			Status:        StepStatusFinished,
			Kind:          StepKindFinished,
			AssistantText: "done",
		}, nil
	}

	_, err := runner.Run(context.Background(), ctx, Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []runtimeevents.Event{
		runtimeevents.StepBegin{Number: 1},
		runtimeevents.TextPart{Text: "I will inspect the file."},
		runtimeevents.ToolCall{
			ID:        "call_read",
			Name:      "read_file",
			Arguments: `{"path":"main.go"}`,
		},
		runtimeevents.ToolResult{
			ToolCallID: "call_read",
			ToolName:   "read_file",
			Output:     "",
			IsError:    false,
		},
		runtimeevents.StatusUpdate{Status: runtimeevents.StatusSnapshot{}},
		runtimeevents.StepBegin{Number: 2},
		runtimeevents.TextPart{Text: "done"},
		runtimeevents.StatusUpdate{Status: runtimeevents.StatusSnapshot{}},
	}
	if !reflect.DeepEqual(sink.events, want) {
		t.Fatalf("captured events = %#v, want %#v", sink.events, want)
	}
}

func TestRunnerRunEmitsInterruptedEventWhenContextCancelled(t *testing.T) {
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	sink := &recordingEventSink{}
	runner := NewWithToolExecutorAndEvents(staticEngine{}, nil, sink, Config{
		MaxStepsPerRun: 2,
	})

	rootCtx, cancel := context.WithCancel(context.Background())
	stepCalls := 0
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		stepCalls++
		if stepCalls == 1 {
			cancel()
			return StepResult{
				Status: StepStatusIncomplete,
				Kind:   StepKindToolCalls,
				ToolCalls: []ToolCall{
					{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
				},
			}, nil
		}

		return StepResult{
			Status: StepStatusFinished,
			Kind:   StepKindFinished,
		}, nil
	}

	result, err := runner.Run(rootCtx, ctx, Input{Prompt: "hello"})
	if err != context.Canceled {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}
	if result.Status != RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusInterrupted)
	}

	wantLast := runtimeevents.StepInterrupted{}
	if len(sink.events) == 0 {
		t.Fatalf("captured events = 0, want at least 1")
	}
	if sink.events[len(sink.events)-1] != wantLast {
		t.Fatalf("last event = %#v, want %#v", sink.events[len(sink.events)-1], wantLast)
	}
}

type staticEngine struct {
	reply AssistantReply
	err   error
}

func (e staticEngine) Reply(ctx context.Context, input ReplyInput) (AssistantReply, error) {
	return e.reply, e.err
}

type trackingEngine struct {
	called bool
}

func (e *trackingEngine) Reply(ctx context.Context, input ReplyInput) (AssistantReply, error) {
	e.called = true
	return AssistantReply{Text: "unused"}, nil
}

type spyEngine struct {
	gotInput ReplyInput
	reply    AssistantReply
	err      error
}

func (e *spyEngine) Reply(ctx context.Context, input ReplyInput) (AssistantReply, error) {
	e.gotInput = input
	return e.reply, e.err
}

type spyToolExecutor struct {
	gotCalls []ToolCall
}

func (e *spyToolExecutor) Execute(ctx context.Context, call ToolCall) (ToolExecution, error) {
	e.gotCalls = append(e.gotCalls, call)

	return ToolExecution{
		Call: call,
	}, nil
}

type failingToolExecutor struct {
	err error
}

func (e failingToolExecutor) Execute(ctx context.Context, call ToolCall) (ToolExecution, error) {
	return ToolExecution{}, e.err
}

type retryableStepError struct {
	err error
}

func (e retryableStepError) Error() string {
	return e.err.Error()
}

func (e retryableStepError) Unwrap() error {
	return e.err
}

func (retryableStepError) Retryable() bool {
	return true
}

type refusedStepError struct {
	err error
}

func (e refusedStepError) Error() string {
	return e.err.Error()
}

func (e refusedStepError) Unwrap() error {
	return e.err
}

func (refusedStepError) Refused() bool {
	return true
}

type temporaryStepError struct {
	err error
}

func (e temporaryStepError) Error() string {
	return e.err.Error()
}

func (e temporaryStepError) Unwrap() error {
	return e.err
}

func (temporaryStepError) Temporary() bool {
	return true
}

type recordingEventSink struct {
	events []runtimeevents.Event
}

func (s *recordingEventSink) Emit(ctx context.Context, event runtimeevents.Event) error {
	s.events = append(s.events, event)
	return nil
}
