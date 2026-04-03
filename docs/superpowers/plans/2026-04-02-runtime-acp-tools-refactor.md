# Runtime / ACP / tools Core-Flow Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the runtime / ACP core flow into smaller, topic-focused files while preserving current behavior, event projection, approval handling, retry semantics, and D-Mail rollback behavior.

**Architecture:** Keep all package boundaries and public APIs stable. Split `internal/runtime/runtime.go` into topic-focused sibling files for D-Mail, retry, events, errors, and step advancement; split `internal/acp/session.go` into state, wire projection, tool-call projection, approval handling, and content helpers. Keep `internal/tools/executor.go` concrete and small, and only add regression coverage around approval context injection.

**Tech Stack:** Go, Go testing, `context`, `fimi-cli/internal/contextstore`, `fimi-cli/internal/runtime/events`, `fimi-cli/internal/wire`, ACP JSON-RPC types

---

## File Structure

### Runtime
- Create: `internal/runtime/runtime_dmail.go` — checkpoint marker helpers, run-start persistence, per-step checkpoint persistence, rollback + D-Mail injection.
- Create: `internal/runtime/runtime_errors.go` — interrupted / retryable / refused / temporary classification and tool-failure formatting.
- Create: `internal/runtime/runtime_retry.go` — retry loop, backoff, jitter, and sleep helpers.
- Create: `internal/runtime/runtime_events.go` — wire event emission, retry status emission, context-usage snapshots, display-output fallback, interrupted result.
- Create: `internal/runtime/runtime_step.go` — `runStep`, `advanceRun`, `advanceToolCallStep`, `executeToolCalls`.
- Modify: `internal/runtime/runtime.go` — keep public types, constructors, `Run()`, `NoopToolExecutor`, and `missingEngine`; remove moved method bodies.
- Create: `internal/runtime/runtime_dmail_test.go` — focused tests for extracted D-Mail helpers.
- Create: `internal/runtime/runtime_step_test.go` — focused tests for the extracted tool-call advancement helper.

### ACP
- Create: `internal/acp/session_wire.go` — wire channel consumption and message/event dispatch.
- Create: `internal/acp/session_toolcalls.go` — tool-call start/progress/result projection.
- Create: `internal/acp/session_approval.go` — approval request tracking, resolution, and cleanup.
- Create: `internal/acp/content.go` — ACP content block construction and output truncation helpers.
- Modify: `internal/acp/session.go` — keep only session state and state accessors.
- Create: `internal/acp/content_test.go` — focused tests for content truncation / fallback behavior.
- Create: `internal/acp/session_approval_test.go` — focused tests for approval cleanup.

### Tools
- Modify: `internal/tools/executor_test.go` — add a regression test proving `Execute()` injects the tool-call ID into the approval context.
- Keep unchanged unless cleanup is truly required: `internal/tools/executor.go`.

### App
- No production change expected: `internal/app/app_acp.go` should stay as the thin assembly layer unless the compiler proves otherwise during the final verification pass.

### Commit policy
- Do **not** create commits unless the user explicitly asks for one.

---

### Task 1: Extract Runtime D-Mail helpers first

**Files:**
- Create: `internal/runtime/runtime_dmail_test.go`
- Create: `internal/runtime/runtime_dmail.go`
- Modify: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write failing tests for the new D-Mail helper boundary**

Create `internal/runtime/runtime_dmail_test.go` with the exact tests below:

```go
package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunnerPersistRunStartAppendsCheckpointMarkerAndPrompt(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{reply: AssistantReply{Text: "done"}}, Config{})
	runner = runner.WithDMailer(&mockDMailer{})

	checkpointID, err := runner.persistRunStart(context.Background(), store, " hello ")
	if err != nil {
		t.Fatalf("persistRunStart() error = %v", err)
	}
	if checkpointID != 0 {
		t.Fatalf("checkpointID = %d, want 0", checkpointID)
	}

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].Content != "<system>CHECKPOINT 0</system>" {
		t.Fatalf("records[0].Content = %q, want checkpoint marker", records[0].Content)
	}
	if records[1] != contextstore.NewUserTextRecord("hello") {
		t.Fatalf("records[1] = %#v, want %#v", records[1], contextstore.NewUserTextRecord("hello"))
	}
}

func TestRunnerApplyPendingDMailRevertsHistoryAndInjectsMessage(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if _, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{}); err != nil {
		t.Fatalf("AppendCheckpointWithMetadata() error = %v", err)
	}
	if err := store.Append(contextstore.NewUserTextRecord("keep this")); err != nil {
		t.Fatalf("Append(keep this) error = %v", err)
	}
	if err := store.Append(contextstore.NewAssistantTextRecord("drop this")); err != nil {
		t.Fatalf("Append(drop this) error = %v", err)
	}

	dmailer := &mockDMailer{
		pending: &dmailEntry{message: "rewrite the search", checkpointID: 0},
	}
	runner := New(staticEngine{reply: AssistantReply{Text: "done"}}, Config{})
	runner = runner.WithDMailer(dmailer)

	applied, err := runner.applyPendingDMail(context.Background(), store)
	if err != nil {
		t.Fatalf("applyPendingDMail() error = %v", err)
	}
	if !applied {
		t.Fatal("applyPendingDMail() = false, want true")
	}

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("len(records) = 0, want rollback state")
	}

	last := records[len(records)-1]
	if last.Role != contextstore.RoleUser {
		t.Fatalf("last.Role = %q, want %q", last.Role, contextstore.RoleUser)
	}
	if !strings.Contains(last.Content, "D-Mail received: rewrite the search") {
		t.Fatalf("last.Content = %q, want persisted D-Mail instruction", last.Content)
	}
}
```

- [ ] **Step 2: Run the new runtime helper tests and confirm they fail before the helpers exist**

Run:

```bash
go test ./internal/runtime -run 'TestRunnerPersistRunStartAppendsCheckpointMarkerAndPrompt|TestRunnerApplyPendingDMailRevertsHistoryAndInjectsMessage'
```

Expected: FAIL with compile errors mentioning missing methods such as `runner.persistRunStart undefined` and `runner.applyPendingDMail undefined`.

- [ ] **Step 3: Implement the extracted D-Mail helper file**

Create `internal/runtime/runtime_dmail.go` with the exact code below:

```go
package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
)

func (r Runner) persistRunStart(ctx context.Context, store contextstore.Context, prompt string) (int, error) {
	var checkpointID int
	err := r.shieldContextWrite(ctx, func() error {
		var err error
		checkpointID, err = store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			PromptPreview: checkpointPromptPreview(prompt),
		})
		if err != nil {
			return err
		}
		if r.dmailer != nil {
			r.dmailer.SetCheckpointCount(checkpointID + 1)
			if err := r.appendCheckpointMarker(store, checkpointID); err != nil {
				return err
			}
		}
		return store.Append(contextstore.NewUserTextRecord(strings.TrimSpace(prompt)))
	})
	if err != nil {
		return 0, err
	}
	return checkpointID, nil
}

func (r Runner) persistStepCheckpoint(ctx context.Context, store contextstore.Context) error {
	if r.dmailer == nil {
		return nil
	}
	return r.shieldContextWrite(ctx, func() error {
		checkpointID, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
		r.dmailer.SetCheckpointCount(checkpointID + 1)
		return r.appendCheckpointMarker(store, checkpointID)
	})
}

func (r Runner) applyPendingDMail(ctx context.Context, store contextstore.Context) (bool, error) {
	if r.dmailer == nil {
		return false, nil
	}
	message, checkpointID, ok := r.dmailer.Fetch()
	if !ok {
		return false, nil
	}

	err := r.shieldContextWrite(ctx, func() error {
		if _, err := store.RevertToCheckpoint(checkpointID); err != nil {
			return err
		}
		newCheckpointID, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
		r.dmailer.SetCheckpointCount(newCheckpointID + 1)
		if err := r.appendCheckpointMarker(store, newCheckpointID); err != nil {
			return err
		}
		dmailContent := fmt.Sprintf("<system>D-Mail received: %s</system>\n\nRead the D-Mail above carefully. Act on the information it contains. Do NOT mention the D-Mail mechanism or time travel to the user.", message)
		return store.Append(contextstore.NewUserTextRecord(dmailContent))
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r Runner) appendCheckpointMarker(store contextstore.Context, checkpointID int) error {
	return store.Append(contextstore.NewUserTextRecord(
		fmt.Sprintf("<system>CHECKPOINT %d</system>", checkpointID),
	))
}

func checkpointPromptPreview(prompt string) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if len(preview) <= checkpointPromptPreviewMaxLen {
		return preview
	}
	return preview[:checkpointPromptPreviewMaxLen-3] + "..."
}
```

- [ ] **Step 4: Simplify `Runner.Run()` so it calls the D-Mail helpers instead of inlining the logic**

Update the `Run()` method in `internal/runtime/runtime.go` to the exact version below:

```go
func (r Runner) Run(ctx context.Context, store contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{Status: RunStatusFinished}, nil
	}

	userRecord := contextstore.NewUserTextRecord(prompt)
	result := Result{
		Status:     RunStatusFinished,
		UserRecord: &userRecord,
		Steps:      make([]StepResult, 0, 1),
	}

	if _, err := r.persistRunStart(ctx, store, prompt); err != nil {
		if isInterruptedError(err) {
			return r.interruptedResult(ctx, result)
		}
		return Result{Status: RunStatusFailed}, fmt.Errorf("persist prompt boundary: %w", err)
	}

	cfg := StepConfig{Model: input.Model, SystemPrompt: input.SystemPrompt}
	runStep := r.runStepFn
	if runStep == nil {
		runStep = r.runStep
	}
	advanceRun := r.advanceRunFn
	if advanceRun == nil {
		advanceRun = r.advanceRun
	}

	for stepNo := 1; stepNo <= r.config.MaxStepsPerRun; stepNo++ {
		if ctx.Err() != nil {
			return r.interruptedResult(ctx, result)
		}
		if err := r.emitEvent(ctx, runtimeevents.StepBegin{Number: stepNo}); err != nil {
			result.Status = RunStatusFailed
			return result, err
		}
		if err := r.persistStepCheckpoint(ctx, store); err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			return Result{Status: RunStatusFailed}, fmt.Errorf("persist step checkpoint: %w", err)
		}

		stepResult, err := r.runStepWithRetry(ctx, store, cfg, runStep)
		if err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			result.Status = RunStatusFailed
			return result, err
		}

		finishedResult, finished, err := advanceRun(ctx, store, result, stepResult)
		if err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, finishedResult)
			}
			finishedResult.Status = RunStatusFailed
			return finishedResult, err
		}
		result = finishedResult

		applied, err := r.applyPendingDMail(ctx, store)
		if err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			return Result{Status: RunStatusFailed}, fmt.Errorf("persist rollback context: %w", err)
		}
		if applied {
			stepNo = 0
			continue
		}
		if finished {
			result.Status = RunStatusFinished
			return result, nil
		}
	}

	result.Status = RunStatusMaxSteps
	return result, nil
}
```

- [ ] **Step 5: Re-run the D-Mail helper tests and the existing D-Mail regression tests**

Run:

```bash
go test ./internal/runtime -run 'TestRunnerPersistRunStartAppendsCheckpointMarkerAndPrompt|TestRunnerApplyPendingDMailRevertsHistoryAndInjectsMessage|TestRunnerDMailRollbackRevertsContextAndContinues|TestRunnerWithDMailerInjectsCheckpointMarkers|TestRunnerWithoutDMailerDoesNotInjectCheckpointMarkers'
```

Expected: PASS.

---

### Task 2: Split the rest of Runtime by topic

**Files:**
- Create: `internal/runtime/runtime_step_test.go`
- Create: `internal/runtime/runtime_errors.go`
- Create: `internal/runtime/runtime_retry.go`
- Create: `internal/runtime/runtime_events.go`
- Create: `internal/runtime/runtime_step.go`
- Modify: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write a failing test for the extracted tool-call advancement helper**

Create `internal/runtime/runtime_step_test.go` with this exact test:

```go
package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestRunnerAdvanceToolCallStepPersistsToolRecords(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := NewWithToolExecutor(
		staticEngine{reply: AssistantReply{Text: "unused"}},
		funcToolExecutorFunc(func(ctx context.Context, call ToolCall) (ToolExecution, error) {
			return ToolExecution{Call: call, Output: "ok"}, nil
		}),
		Config{},
	)

	step := StepResult{
		Status:        StepStatusIncomplete,
		Kind:          StepKindToolCalls,
		AssistantText: "run tool",
		ToolCalls: []ToolCall{{
			ID:        "call-1",
			Name:      "bash",
			Arguments: `{"command":"pwd"}`,
		}},
	}

	advanced, err := runner.advanceToolCallStep(context.Background(), store, step)
	if err != nil {
		t.Fatalf("advanceToolCallStep() error = %v", err)
	}
	if len(advanced.ToolExecutions) != 1 {
		t.Fatalf("len(ToolExecutions) = %d, want 1", len(advanced.ToolExecutions))
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
	if records[1].Role != contextstore.RoleTool {
		t.Fatalf("records[1].Role = %q, want %q", records[1].Role, contextstore.RoleTool)
	}
}
```

- [ ] **Step 2: Run the new runtime-step test and confirm it fails before the helper exists**

Run:

```bash
go test ./internal/runtime -run TestRunnerAdvanceToolCallStepPersistsToolRecords
```

Expected: FAIL with a compile error mentioning `runner.advanceToolCallStep undefined`.

- [ ] **Step 3: Extract runtime error and retry helpers into focused files**

Create `internal/runtime/runtime_errors.go`:

```go
package runtime

import (
	"context"
	"errors"
	"fmt"
)

func formatToolFailureContent(err *ToolExecutionError) string {
	failureKind := "error"
	if IsTemporary(err) {
		failureKind = "temporary"
	} else if IsRefused(err) {
		failureKind = "refused"
	}
	return fmt.Sprintf("tool execution failed (failure_kind: %s): %s", failureKind, err.Err.Error())
}

func isInterruptedError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func IsRetryable(err error) bool {
	type retryable interface{ Retryable() bool }
	var target retryable
	if !errors.As(err, &target) {
		return false
	}
	return target.Retryable()
}

func IsRefused(err error) bool {
	type refused interface{ Refused() bool }
	var target refused
	if !errors.As(err, &target) {
		return false
	}
	return target.Refused()
}

func IsTemporary(err error) bool {
	type temporary interface{ Temporary() bool }
	var target temporary
	if !errors.As(err, &target) {
		return false
	}
	return target.Temporary()
}
```

Create `internal/runtime/runtime_retry.go`:

```go
package runtime

import (
	"context"
	"time"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
)

const retryBackoffBaseDelay = 200 * time.Millisecond
const retryBackoffMaxDelay = 2 * time.Second
const retryBackoffJitterWindow = 100 * time.Millisecond

func (r Runner) runStepWithRetry(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
	runStep func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error),
) (StepResult, error) {
	var lastErr error
	retryStatusEmitted := false
	maxAttempts := 1 + r.config.MaxAdditionalRetriesPerStep
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return StepResult{}, ctx.Err()
		}

		stepResult, err := runStep(ctx, store, cfg)
		if err == nil {
			return stepResult, nil
		}
		if !IsRetryable(err) {
			return StepResult{}, err
		}

		lastErr = err
		if attempt == maxAttempts {
			break
		}

		delay := r.retryBackoffDelay(attempt)
		if emitErr := r.emitRetryStatusUpdate(ctx, store, &runtimeevents.RetryStatus{
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			NextDelayMS: delay.Milliseconds(),
		}); emitErr != nil {
			return StepResult{}, emitErr
		}
		retryStatusEmitted = true
		if sleepErr := r.retrySleep(ctx, delay); sleepErr != nil {
			return StepResult{}, sleepErr
		}
	}

	if retryStatusEmitted {
		if clearErr := r.emitRetryStatusUpdate(ctx, store, nil); clearErr != nil {
			return StepResult{}, clearErr
		}
	}

	return StepResult{}, lastErr
}

func (r Runner) retryBackoffDelay(attempt int) time.Duration {
	if r.retryBackoffDelayFn != nil {
		return r.retryBackoffDelayFn(attempt)
	}
	return calculateRetryBackoffDelay(attempt, retryBackoffJitter())
}

func calculateRetryBackoffDelay(attempt int, jitter time.Duration) time.Duration {
	baseDelay := retryBackoffBaseDelayForAttempt(attempt)
	clampedJitter := clampRetryBackoffJitter(jitter)
	return clampRetryBackoffDelay(baseDelay + clampedJitter)
}

func retryBackoffBaseDelayForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return retryBackoffBaseDelay
	}
	baseDelay := retryBackoffBaseDelay
	for i := 1; i < attempt; i++ {
		if baseDelay >= retryBackoffMaxDelay/2 {
			return retryBackoffMaxDelay
		}
		baseDelay *= 2
	}
	return clampRetryBackoffDelay(baseDelay)
}

func retryBackoffJitter() time.Duration {
	window := retryBackoffJitterWindow
	if window <= 0 {
		return 0
	}
	return time.Duration(time.Now().UnixNano() % int64(window+1))
}

func clampRetryBackoffJitter(jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return 0
	}
	if jitter > retryBackoffJitterWindow {
		return retryBackoffJitterWindow
	}
	return jitter
}

func clampRetryBackoffDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return retryBackoffBaseDelay
	}
	if delay > retryBackoffMaxDelay {
		return retryBackoffMaxDelay
	}
	return delay
}

func (r Runner) retrySleep(ctx context.Context, delay time.Duration) error {
	if r.retrySleepFn != nil {
		return r.retrySleepFn(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r Runner) shieldContextWrite(ctx context.Context, write func() error) error {
	if r.shieldContextWriteFn != nil {
		return r.shieldContextWriteFn(ctx, write)
	}
	if err := write(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Extract runtime events and step advancement into focused files, then delete the moved copies from `runtime.go`**

Create `internal/runtime/runtime_events.go`:

```go
package runtime

import (
	"context"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

func (r Runner) emitStepEvents(
	ctx context.Context,
	store contextstore.Context,
	stepResult StepResult,
) error {
	if !stepResult.TextStreamed && strings.TrimSpace(stepResult.AssistantText) != "" {
		if err := r.emitEvent(ctx, runtimeevents.TextPart{Text: stepResult.AssistantText}); err != nil {
			return err
		}
	}
	for _, call := range stepResult.ToolCalls {
		if err := r.emitEvent(ctx, runtimeevents.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Subtitle:  ToolCallSubtitle(call),
			Arguments: call.Arguments,
		}); err != nil {
			return err
		}
	}
	for _, exec := range stepResult.ToolExecutions {
		if err := r.emitEvent(ctx, runtimeevents.ToolResult{
			ToolCallID:    exec.Call.ID,
			ToolName:      exec.Call.Name,
			Output:        exec.Output,
			DisplayOutput: firstNonEmptyDisplay(exec.DisplayOutput, exec.Output),
			Content:       exec.Content,
			IsError:       false,
		}); err != nil {
			return err
		}
	}
	if stepResult.ToolFailure != nil {
		if err := r.emitEvent(ctx, runtimeevents.ToolResult{
			ToolCallID: stepResult.ToolFailure.Call.ID,
			ToolName:   stepResult.ToolFailure.Call.Name,
			Output:     formatToolFailureContent(stepResult.ToolFailure),
			IsError:    true,
		}); err != nil {
			return err
		}
	}
	return r.emitEvent(ctx, runtimeevents.StatusUpdate{
		Status: buildStatusSnapshotWithWindow(store, r.config.ContextWindowTokens, nil),
	})
}

func firstNonEmptyDisplay(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r Runner) emitRetryStatusUpdate(
	ctx context.Context,
	store contextstore.Context,
	retry *runtimeevents.RetryStatus,
) error {
	return r.emitEvent(ctx, runtimeevents.StatusUpdate{
		Status: buildStatusSnapshotWithWindow(store, r.config.ContextWindowTokens, retry),
	})
}

func (r Runner) emitEvent(ctx context.Context, event runtimeevents.Event) error {
	w, ok := wire.Current(ctx)
	if !ok {
		return nil
	}
	if err := w.Send(wire.EventMessage{Event: event}); err != nil {
		return fmt.Errorf("send runtime event %q: %w", event.Kind(), err)
	}
	return nil
}

func buildStatusSnapshot(store contextstore.Context) runtimeevents.StatusSnapshot {
	return buildStatusSnapshotWithWindow(store, 0, nil)
}

func buildStatusSnapshotWithWindow(
	store contextstore.Context,
	contextWindowTokens int,
	retry *runtimeevents.RetryStatus,
) runtimeevents.StatusSnapshot {
	snapshot := runtimeevents.StatusSnapshot{Retry: retry}
	if contextWindowTokens <= 0 {
		return snapshot
	}
	lastUsage, err := store.ReadUsage()
	if err != nil || lastUsage <= 0 {
		return snapshot
	}
	snapshot.ContextUsage = float64(lastUsage) / float64(contextWindowTokens)
	return snapshot
}

func (r Runner) interruptedResult(ctx context.Context, result Result) (Result, error) {
	result.Status = RunStatusInterrupted
	if err := r.emitEvent(ctx, runtimeevents.StepInterrupted{}); err != nil {
		return result, err
	}
	return result, ctx.Err()
}
```

Create `internal/runtime/runtime_step.go`:

```go
package runtime

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func (r Runner) runStep(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
) (StepResult, error) {
	history, err := store.ReadRecentTurns(r.config.ReplyHistoryTurnLimit)
	if err != nil {
		return StepResult{}, fmt.Errorf("read runtime history: %w", err)
	}

	replyInput := ReplyInput{Model: cfg.Model, SystemPrompt: cfg.SystemPrompt, History: history}
	var assistantReply AssistantReply
	var textStreamed bool

	if streamingEngine, ok := r.engine.(StreamingEngine); ok {
		handler := StreamHandlerFunc(func(ctx context.Context, event any) error {
			switch ev := event.(type) {
			case interface{ TextDelta() string }:
				return r.emitEvent(ctx, runtimeevents.TextPart{Text: ev.TextDelta()})
			case interface{ ToolCallDelta() (string, string) }:
				toolCallID, delta := ev.ToolCallDelta()
				return r.emitEvent(ctx, runtimeevents.ToolCallPart{ToolCallID: toolCallID, Delta: delta})
			default:
				return nil
			}
		})
		assistantReply, err = streamingEngine.ReplyStream(ctx, replyInput, handler)
		textStreamed = true
	} else {
		assistantReply, err = r.engine.Reply(ctx, replyInput)
	}
	if err != nil {
		return StepResult{}, fmt.Errorf("build assistant reply: %w", err)
	}

	if assistantReply.Usage.TotalTokens > 0 {
		if err := r.shieldContextWrite(ctx, func() error {
			return store.AppendUsage(assistantReply.Usage.TotalTokens)
		}); err != nil {
			return StepResult{}, fmt.Errorf("append usage record: %w", err)
		}
	}

	if len(assistantReply.ToolCalls) > 0 {
		return StepResult{
			Status:        StepStatusIncomplete,
			Kind:          StepKindToolCalls,
			AssistantText: assistantReply.Text,
			ToolCalls:     assistantReply.ToolCalls,
			Usage:         assistantReply.Usage,
			TextStreamed:  textStreamed,
		}, nil
	}

	records := []contextstore.TextRecord{contextstore.NewAssistantTextRecord(assistantReply.Text)}
	if err := r.shieldContextWrite(ctx, func() error {
		for _, record := range records {
			if err := store.Append(record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return StepResult{}, fmt.Errorf("append runtime record: %w", err)
	}

	return StepResult{
		Status:          StepStatusFinished,
		Kind:            StepKindFinished,
		AssistantText:   assistantReply.Text,
		AppendedRecords: records,
		Usage:           assistantReply.Usage,
		TextStreamed:    textStreamed,
	}, nil
}

func (r Runner) advanceRun(
	ctx context.Context,
	store contextstore.Context,
	result Result,
	stepResult StepResult,
) (Result, bool, error) {
	switch stepResult.Kind {
	case StepKindFinished:
	case StepKindToolCalls:
		advanced, err := r.advanceToolCallStep(ctx, store, stepResult)
		if err != nil {
			var toolErr ToolExecutionError
			if errors.As(err, &toolErr) {
				result.Steps = append(result.Steps, advanced)
				if emitErr := r.emitStepEvents(ctx, store, advanced); emitErr != nil {
					return result, false, emitErr
				}
				return result, false, err
			}
			return Result{}, false, err
		}
		stepResult = advanced
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepKind, stepResult.Kind)
	}

	result.Steps = append(result.Steps, stepResult)
	if err := r.emitStepEvents(ctx, store, stepResult); err != nil {
		return result, false, err
	}

	switch stepResult.Status {
	case StepStatusFinished:
		return result, true, nil
	case StepStatusIncomplete:
		return result, false, nil
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepStatus, stepResult.Status)
	}
}

func (r Runner) advanceToolCallStep(
	ctx context.Context,
	store contextstore.Context,
	stepResult StepResult,
) (StepResult, error) {
	if len(stepResult.ToolCalls) == 0 {
		return StepResult{}, fmt.Errorf("step kind %q requires at least one tool call", stepResult.Kind)
	}

	toolExecutions, err := r.executeToolCalls(ctx, stepResult.ToolCalls)
	if err != nil {
		var toolErr ToolExecutionError
		if errors.As(err, &toolErr) {
			stepResult.Status = StepStatusFailed
			stepResult.ToolFailure = &toolErr
			if writeErr := r.shieldContextWrite(ctx, func() error {
				for _, record := range stepResult.BuildToolStepRecords() {
					if appendErr := store.Append(record); appendErr != nil {
						return appendErr
					}
				}
				return nil
			}); writeErr != nil {
				return StepResult{}, fmt.Errorf("append tool failure record: %w", writeErr)
			}
			return stepResult, err
		}
		return StepResult{}, err
	}

	stepResult.ToolExecutions = toolExecutions
	if err := r.shieldContextWrite(ctx, func() error {
		for _, record := range stepResult.BuildToolStepRecords() {
			if appendErr := store.Append(record); appendErr != nil {
				return appendErr
			}
		}
		return nil
	}); err != nil {
		return StepResult{}, fmt.Errorf("append tool step record: %w", err)
	}
	return stepResult, nil
}

func (r Runner) executeToolCalls(ctx context.Context, calls []ToolCall) ([]ToolExecution, error) {
	toolExecutions := make([]ToolExecution, 0, len(calls))
	for _, call := range calls {
		execution, err := r.toolExecutor.Execute(ctx, call)
		if err != nil {
			return nil, ToolExecutionError{Call: call, Err: err}
		}
		toolExecutions = append(toolExecutions, execution)
	}
	return toolExecutions, nil
}
```

After creating these files, delete the moved function bodies from `internal/runtime/runtime.go` so only the public types, constructors, `Run()`, `BuildToolStepRecords()`, `NoopToolExecutor`, and `missingEngine` remain.

- [ ] **Step 5: Run the focused runtime suite for helper extraction, retry behavior, and event projection**

Run:

```bash
go test ./internal/runtime -run 'TestRunnerAdvanceToolCallStepPersistsToolRecords|TestRunnerAdvanceRunContinuesOnToolCallStep|TestRunnerRunWaitsBetweenRetryableStepFailures|TestRunnerRunEmitsToolStepEvents|TestRunnerEmitEventThroughWire|TestIsRefusedReturnsTrueForRefusedError|TestIsTemporaryReturnsTrueForTemporaryError'
```

Expected: PASS.

---

### Task 3: Split ACP session responsibilities and lock down ACP regressions

**Files:**
- Create: `internal/acp/content_test.go`
- Create: `internal/acp/session_approval_test.go`
- Create: `internal/acp/content.go`
- Create: `internal/acp/session_wire.go`
- Create: `internal/acp/session_toolcalls.go`
- Create: `internal/acp/session_approval.go`
- Modify: `internal/acp/session.go`
- Test: `internal/acp/session_test.go`

- [ ] **Step 1: Add ACP regression tests for output truncation and approval cleanup**

Create `internal/acp/content_test.go`:

```go
package acp

import (
	"strings"
	"testing"
)

func TestFirstNonEmptyToolOutputTruncatesLongOutput(t *testing.T) {
	long := strings.Repeat("a", 10001)
	got := firstNonEmptyToolOutput("", long)
	if !strings.HasSuffix(got, "\n... (truncated)") {
		t.Fatalf("firstNonEmptyToolOutput() = %q, want truncated suffix", got)
	}
	if len(got) <= 10000 {
		t.Fatalf("len(firstNonEmptyToolOutput()) = %d, want > 10000 after suffix", len(got))
	}
}
```

Create `internal/acp/session_approval_test.go`:

```go
package acp

import (
	"context"
	"testing"
	"time"

	"fimi-cli/internal/wire"
)

func TestSessionClearPendingApprovalsRejectsOutstandingRequests(t *testing.T) {
	acpSession, _ := newTestACPSession(t)
	req := &wire.ApprovalRequest{ID: "approval-1", Action: "bash", Description: "pwd"}

	acpSession.pendingApprovals[req.ID] = req
	waitDone := make(chan wire.ApprovalResponse, 1)
	go func() {
		resp, err := req.Wait(context.Background())
		if err != nil {
			t.Errorf("ApprovalRequest.Wait() error = %v", err)
			return
		}
		waitDone <- resp
	}()

	time.Sleep(10 * time.Millisecond)
	acpSession.ClearPendingApprovals()

	resp := <-waitDone
	if resp != wire.ApprovalReject {
		t.Fatalf("resp = %q, want %q", resp, wire.ApprovalReject)
	}
}
```

- [ ] **Step 2: Run the ACP regression tests before moving code**

Run:

```bash
go test ./internal/acp -run 'TestFirstNonEmptyToolOutputTruncatesLongOutput|TestSessionClearPendingApprovalsRejectsOutstandingRequests|TestSessionVisualizeWireProjectsToolCallPartAndRichResult|TestSessionVisualizeWireSendsApprovalRequestAndResolvesIt'
```

Expected: PASS.

- [ ] **Step 3: Move ACP content and projection helpers into focused files**

Create `internal/acp/content.go`:

```go
package acp

import (
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func buildACPContentItems(content []runtimeevents.RichContent, fallbackText string) []ToolCallContentItem {
	if len(content) == 0 {
		return buildTextContentItems(fallbackText)
	}

	items := make([]ToolCallContentItem, 0, len(content)+1)
	if fallbackText != "" && hasNonTextContent(content) {
		items = append(items, ToolCallContentItem{
			Type:    "text",
			Content: ContentBlock{Type: "text", Text: fallbackText},
		})
	}
	for _, item := range content {
		block := ContentBlock{Type: item.Type, Text: item.Text, MIMEType: item.MIMEType, Data: item.Data}
		itemType := item.Type
		if itemType == "" {
			itemType = "text"
			block.Type = "text"
		}
		items = append(items, ToolCallContentItem{Type: itemType, Content: block})
	}
	return items
}

func buildTextContentItems(text string) []ToolCallContentItem {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []ToolCallContentItem{{
		Type:    "text",
		Content: ContentBlock{Type: "text", Text: text},
	}}
}

func hasNonTextContent(content []runtimeevents.RichContent) bool {
	for _, item := range content {
		if item.Type != "" && item.Type != "text" {
			return true
		}
	}
	return false
}

func firstNonEmptyToolOutput(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			const maxOutputLen = 10000
			if len(value) > maxOutputLen {
				return value[:maxOutputLen] + "\n... (truncated)"
			}
			return value
		}
	}
	return ""
}
```

Create `internal/acp/session_wire.go`:

```go
package acp

import (
	"context"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

func (s *Session) VisualizeWire() func(ctx context.Context, messages <-chan wire.Message) error {
	return func(ctx context.Context, messages <-chan wire.Message) error {
		for msg := range messages {
			if err := s.translateAndSendMessage(msg); err != nil {
				return err
			}
		}
		return nil
	}
}

func (s *Session) translateAndSendMessage(msg wire.Message) error {
	switch m := msg.(type) {
	case wire.EventMessage:
		return s.translateAndSendEvent(m.Event)
	case *wire.ApprovalRequest:
		return s.sendApprovalRequest(m)
	default:
		return nil
	}
}

func (s *Session) translateAndSendEvent(event runtimeevents.Event) error {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		return s.sendAgentMessageChunk(e.Text)
	case runtimeevents.ToolCallPart:
		return s.sendToolCallPart(e)
	case runtimeevents.ToolCall:
		return s.sendToolCallStart(e)
	case runtimeevents.ToolResult:
		return s.sendToolCallProgress(e)
	default:
		return nil
	}
}

func (s *Session) sendAgentMessageChunk(text string) error {
	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: AgentMessageChunk{
			SessionUpdate: "agent_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: text},
		},
	})
}
```

Create `internal/acp/session_toolcalls.go`:

```go
package acp

import (
	"fmt"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func (s *Session) sendToolCallPart(part runtimeevents.ToolCallPart) error {
	if strings.TrimSpace(part.Delta) == "" {
		return nil
	}

	s.mu.Lock()
	firstChunk := !s.startedToolCalls[part.ToolCallID]
	s.startedToolCalls[part.ToolCallID] = true
	s.mu.Unlock()

	if firstChunk {
		return s.conn.SendNotification("session/update", SessionUpdateNotification{
			SessionID: s.session.ID,
			Update: ToolCallStart{
				SessionUpdate: "tool_call_start",
				ToolCallID:    part.ToolCallID,
				Title:         "Tool call",
				Status:        "in_progress",
				Content:       buildACPContentItems([]runtimeevents.RichContent{{Type: "text", Text: part.Delta}}, ""),
			},
		})
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    part.ToolCallID,
			Status:        "in_progress",
			Content:       buildACPContentItems([]runtimeevents.RichContent{{Type: "text", Text: part.Delta}}, ""),
		},
	})
}

func (s *Session) sendToolCallStart(tc runtimeevents.ToolCall) error {
	title := tc.Name
	if tc.Subtitle != "" {
		title = fmt.Sprintf("%s(%s)", tc.Name, tc.Subtitle)
	}

	s.mu.Lock()
	started := s.startedToolCalls[tc.ID]
	s.startedToolCalls[tc.ID] = true
	s.mu.Unlock()

	if started {
		return s.conn.SendNotification("session/update", SessionUpdateNotification{
			SessionID: s.session.ID,
			Update: ToolCallProgress{
				SessionUpdate: "tool_call_progress",
				ToolCallID:    tc.ID,
				Title:         title,
				Status:        "in_progress",
			},
		})
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallStart{
			SessionUpdate: "tool_call_start",
			ToolCallID:    tc.ID,
			Title:         title,
			Status:        "in_progress",
			Content:       buildACPContentItems(nil, tc.Arguments),
		},
	})
}

func (s *Session) sendToolCallProgress(tr runtimeevents.ToolResult) error {
	status := "completed"
	if tr.IsError {
		status = "failed"
	}

	s.mu.Lock()
	delete(s.startedToolCalls, tr.ToolCallID)
	s.mu.Unlock()

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    tr.ToolCallID,
			Title:         tr.ToolName,
			Status:        status,
			Content:       buildACPContentItems(tr.Content, firstNonEmptyToolOutput(tr.DisplayOutput, tr.Output)),
		},
	})
}
```

Create `internal/acp/session_approval.go`:

```go
package acp

import (
	"fmt"

	"fimi-cli/internal/wire"
)

func (s *Session) sendApprovalRequest(req *wire.ApprovalRequest) error {
	if req == nil {
		return nil
	}

	s.mu.Lock()
	s.pendingApprovals[req.ID] = req
	s.mu.Unlock()

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ApprovalRequestUpdate{
			SessionUpdate: "approval_request",
			ApprovalID:    req.ID,
			ToolCallID:    req.ToolCallID,
			Action:        req.Action,
			Description:   req.Description,
		},
	})
}

func (s *Session) ResolveApproval(id string, resp wire.ApprovalResponse) error {
	s.mu.Lock()
	req, ok := s.pendingApprovals[id]
	if ok {
		delete(s.pendingApprovals, id)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("approval not found: %s", id)
	}
	return req.Resolve(resp)
}

func (s *Session) ClearPendingApprovals() {
	s.mu.Lock()
	pending := make([]*wire.ApprovalRequest, 0, len(s.pendingApprovals))
	for id, req := range s.pendingApprovals {
		pending = append(pending, req)
		delete(s.pendingApprovals, id)
	}
	s.mu.Unlock()

	for _, req := range pending {
		_ = req.Resolve(wire.ApprovalReject)
	}
}
```

- [ ] **Step 4: Trim `internal/acp/session.go` so it only keeps ACP session state and state accessors**

Replace `internal/acp/session.go` with the exact file content below:

```go
package acp

import (
	"context"
	"sync"

	sessionpkg "fimi-cli/internal/session"
	"fimi-cli/internal/wire"
)

// Session wraps the ACP-facing state for a single runtime session.
type Session struct {
	session sessionpkg.Session
	conn    *FramedConn
	mu      sync.Mutex
	modelID string

	cancelFn context.CancelFunc

	pendingApprovals map[string]*wire.ApprovalRequest
	startedToolCalls map[string]bool
}

func NewSession(sess sessionpkg.Session, conn *FramedConn, modelID string) *Session {
	return &Session{
		session:          sess,
		conn:             conn,
		modelID:          modelID,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
		startedToolCalls: make(map[string]bool),
	}
}

func (s *Session) HistoryFile() string {
	return s.session.HistoryFile
}

func (s *Session) CurrentModelID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelID
}

func (s *Session) SetModelID(modelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelID = modelID
}

func (s *Session) SetCancel(fn context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelFn = fn
}

func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
}
```

- [ ] **Step 5: Run the ACP package tests after the split**

Run:

```bash
go test ./internal/acp -run 'TestFirstNonEmptyToolOutputTruncatesLongOutput|TestSessionClearPendingApprovalsRejectsOutstandingRequests|TestSessionVisualizeWireProjectsToolCallPartAndRichResult|TestSessionVisualizeWireSendsApprovalRequestAndResolvesIt'
```

Expected: PASS.

---

### Task 4: Lock down the tools boundary and run the final verification sweep

**Files:**
- Modify: `internal/tools/executor_test.go`
- Verify unchanged unless compiler requires import cleanup: `internal/app/app_acp.go`
- Test: `internal/runtime/runtime_test.go`
- Test: `internal/acp/session_test.go`
- Test: `internal/tools/executor_test.go`

- [ ] **Step 1: Add a regression test proving `Execute()` injects the tool-call ID into the approval context**

Append this test to `internal/tools/executor_test.go` and add the `approval` import:

```go
func TestExecutorExecuteInjectsToolCallIDIntoApprovalContext(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor([]Definition{{
		Name: ToolBash,
		Kind: KindCommand,
	}}, map[string]HandlerFunc{
		ToolBash: func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
			if got := approval.ToolCallIDFromContext(ctx); got != "call-1" {
				t.Fatalf("approval.ToolCallIDFromContext(ctx) = %q, want %q", got, "call-1")
			}
			return runtime.ToolExecution{Call: call, Output: "ok"}, nil
		},
	})

	_, err := executor.Execute(ctx, runtime.ToolCall{ID: "call-1", Name: ToolBash})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
```

Also update the import block in `internal/tools/executor_test.go` to:

```go
import (
	"context"
	"errors"
	"reflect"
	"testing"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)
```

- [ ] **Step 2: Run the focused tools test and confirm the tools boundary still behaves the same**

Run:

```bash
go test ./internal/tools -run 'TestExecutorExecuteInjectsToolCallIDIntoApprovalContext|TestExecutorExecuteUsesRegisteredHandler|TestExecutorExecuteReturnsErrorForDisallowedTool|TestExecutorExecutePreservesTemporaryClassification'
```

Expected: PASS.

- [ ] **Step 3: Run the package-level regression sweep for runtime, ACP, and tools**

Run:

```bash
go test ./internal/runtime ./internal/acp ./internal/tools
```

Expected: PASS for all three packages.

- [ ] **Step 4: Run the full repository test suite and keep `internal/app/app_acp.go` unchanged unless the compiler forces a tiny cleanup**

Run:

```bash
go test ./...
```

Expected: PASS.

If the compiler reports a now-unused import or a stale local name in `internal/app/app_acp.go`, apply only the minimal compile fix needed and re-run `go test ./...`. Do not add new logic there.

---

## Self-Review

### Spec coverage
- Runtime file split: covered by Task 1 and Task 2.
- Runtime main-flow simplification: covered by Task 1 Step 4 and Task 2 Step 4.
- D-Mail / checkpoint consolidation: covered by Task 1.
- Retry / events / errors extraction: covered by Task 2.
- ACP session split: covered by Task 3.
- ACP content / approval regressions: covered by Task 3.
- Tools executor stays concrete and gains regression coverage: covered by Task 4 Step 1.
- `app_acp.go` stays thin: covered by Task 4 Step 4.

### Placeholder scan
- No `TBD`, `TODO`, or “similar to above” references remain.
- Every code step includes exact code blocks.
- Every test step includes exact commands and expected outcomes.

### Type consistency
- New private methods consistently use `Runner` value receivers.
- ACP helper files consistently use `*Session` receivers.
- Tool-call regression test uses `approval.ToolCallIDFromContext(ctx)` to match the existing approval package API.
- No new public types or renamed methods are introduced.
