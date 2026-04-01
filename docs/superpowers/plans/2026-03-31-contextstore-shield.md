# Contextstore Shield Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make runtime-owned history/checkpoint persistence finish cleanly before cancellation is surfaced, without shielding tools, LLM calls, retry sleep, or event delivery.

**Architecture:** Add a small closure-based shield helper inside `internal/runtime/runtime.go` and use it around grouped `contextstore` write blocks. Because current `contextstore.Context` methods are synchronous and do not accept a `context.Context`, the Go equivalent of Python's shield is to defer observing cancellation until the write block finishes, then return the original cancellation error before any new work starts.

**Tech Stack:** Go 1.25, `context`, `internal/runtime`, `internal/contextstore`, Go testing

**Spec:** `docs/superpowers/specs/2026-03-31-contextstore-shield-design.md`

---

### Task 1: Add the runtime shield seam and cover prompt/final-reply persistence

**Files:**
- Modify: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write the failing tests for the first shielded blocks**

Add these tests to `internal/runtime/runtime_test.go` near the existing interruption tests (after `TestRunnerRunCheckpointFailureAbortsBeforeHistoryMutation` is a good spot):

```go
func TestRunnerRunReturnsInterruptedAfterShieldedPromptBoundaryPersistence(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	engine := &trackingEngine{}
	runner := New(engine, Config{})

	rootCtx, cancel := context.WithCancel(context.Background())
	shieldCalls := 0
	runner.shieldContextWriteFn = func(ctx context.Context, write func() error) error {
		shieldCalls++
		if err := write(); err != nil {
			return err
		}
		if shieldCalls == 1 {
			cancel()
			return ctx.Err()
		}
		return nil
	}

	result, err := runner.Run(rootCtx, store, Input{Prompt: "hello"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, context.Canceled)
	}
	if result.Status != RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusInterrupted)
	}
	if engine.called {
		t.Fatal("engine called = true, want false")
	}

	records, readErr := store.ReadAll()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}

	checkpoints, listErr := store.ListCheckpoints()
	if listErr != nil {
		t.Fatalf("ListCheckpoints() error = %v", listErr)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("len(ListCheckpoints()) = %d, want 1", len(checkpoints))
	}
}

func TestRunnerRunStepReturnsInterruptedAfterShieldedAssistantPersistence(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := New(staticEngine{reply: AssistantReply{Text: "done"}}, Config{})

	rootCtx, cancel := context.WithCancel(context.Background())
	runner.shieldContextWriteFn = func(ctx context.Context, write func() error) error {
		if err := write(); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}

	_, err := runner.runStep(rootCtx, store, StepConfig{Model: "test-model", SystemPrompt: "test-system"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runStep() error = %v, want wrapped %v", err, context.Canceled)
	}

	records, readErr := store.ReadAll()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewAssistantTextRecord("done"),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}
}
```

- [ ] **Step 2: Run the focused tests and watch them fail**

Run: `go test ./internal/runtime -run 'TestRunnerRunReturnsInterruptedAfterShieldedPromptBoundaryPersistence|TestRunnerRunStepReturnsInterruptedAfterShieldedAssistantPersistence' -count=1`
Expected: FAIL because `Runner` does not yet have `shieldContextWriteFn` / `shieldContextWrite`, and cancellation is still surfaced without the new shield behavior.

- [ ] **Step 3: Add the shield helper and use it for run-start + final-reply persistence**

In `internal/runtime/runtime.go`, make these edits:

1. Add the seam to `Runner`:

```go
type Runner struct {
	engine              Engine
	toolExecutor        ToolExecutor
	dmailer             DMailer
	config              Config
	runStepFn           func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error)
	advanceRunFn        func(ctx context.Context, store contextstore.Context, result Result, stepResult StepResult) (Result, bool, error)
	retryBackoffDelayFn func(attempt int) time.Duration
	retrySleepFn        func(ctx context.Context, delay time.Duration) error
	shieldContextWriteFn func(ctx context.Context, write func() error) error
}
```

2. Add the helper below `retrySleep` and above `emitEvent`:

```go
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

3. In `Run()`, initialize `result` before the first shielded block, then wrap the initial checkpoint / optional marker / user prompt writes in one shielded closure:

```go
	result := Result{
		Status: RunStatusFinished,
		Steps:  make([]StepResult, 0, 1),
	}
	userRecord := contextstore.NewUserTextRecord(prompt)
	result.UserRecord = &userRecord

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
			if err := store.Append(contextstore.NewUserTextRecord(
				fmt.Sprintf("<system>CHECKPOINT %d</system>", checkpointID),
			)); err != nil {
				return err
			}
		}
		return store.Append(userRecord)
	})
	if err != nil {
		if isInterruptedError(err) {
			return r.interruptedResult(ctx, result)
		}
		return Result{Status: RunStatusFailed}, fmt.Errorf("persist prompt boundary: %w", err)
	}
```

4. In `runStep()`, wrap usage persistence and final assistant-record persistence with the helper:

```go
	if assistantReply.Usage.TotalTokens > 0 {
		if err := r.shieldContextWrite(ctx, func() error {
			return store.AppendUsage(assistantReply.Usage.TotalTokens)
		}); err != nil {
			return StepResult{}, fmt.Errorf("append usage record: %w", err)
		}
	}
```

```go
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
```

Keep the rest of the logic unchanged.

- [ ] **Step 4: Run the focused tests again and make sure they pass**

Run: `go test ./internal/runtime -run 'TestRunnerRunReturnsInterruptedAfterShieldedPromptBoundaryPersistence|TestRunnerRunStepReturnsInterruptedAfterShieldedAssistantPersistence' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit the first slice**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go
git commit -m "fix(runtime): shield prompt and reply persistence from cancellation"
```

---

### Task 2: Shield tool-record persistence and the D-Mail rollback write cluster

**Files:**
- Modify: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write the failing tests for tool failure and rollback persistence**

Add these tests to `internal/runtime/runtime_test.go` after the existing D-Mail rollback tests:

```go
func TestRunnerRunReturnsInterruptedAfterShieldedToolFailurePersistence(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := NewWithToolExecutor(staticEngine{}, failingToolExecutor{err: errors.New("tool executor failed")}, Config{MaxStepsPerRun: 1})

	rootCtx, cancel := context.WithCancel(context.Background())
	shieldCalls := 0
	runner.shieldContextWriteFn = func(ctx context.Context, write func() error) error {
		shieldCalls++
		if err := write(); err != nil {
			return err
		}
		if shieldCalls == 2 {
			cancel()
			return ctx.Err()
		}
		return nil
	}
	runner.runStepFn = func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error) {
		return StepResult{
			Status:        StepStatusIncomplete,
			Kind:          StepKindToolCalls,
			AssistantText: "I will run bash",
			ToolCalls: []ToolCall{
				{ID: "call_bash", Name: "bash", Arguments: `{"command":"pwd"}`},
			},
		}, nil
	}

	result, err := runner.Run(rootCtx, store, Input{Prompt: "hello"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, context.Canceled)
	}
	if result.Status != RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusInterrupted)
	}

	records, readErr := store.ReadAll()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	if len(records) != 3 {
		t.Fatalf("len(history records) = %d, want 3", len(records))
	}
	if records[2].Role != contextstore.RoleTool {
		t.Fatalf("records[2].Role = %q, want %q", records[2].Role, contextstore.RoleTool)
	}
	if records[2].ToolCallID != "call_bash" {
		t.Fatalf("records[2].ToolCallID = %q, want %q", records[2].ToolCallID, "call_bash")
	}
}

func TestRunnerDMailRollbackReturnsInterruptedAfterShieldedRollbackPersistence(t *testing.T) {
	dir := t.TempDir()
	store := contextstore.New(filepath.Join(dir, "history.jsonl"))

	callCount := 0
	engine := funcReplyFunc(func(input ReplyInput) (AssistantReply, error) {
		callCount++
		if callCount == 1 {
			return AssistantReply{
				Text: "sending dmail",
				ToolCalls: []ToolCall{{
					ID:        "call_1",
					Name:      "send_dmail",
					Arguments: `{"message":"rewrite the plan","checkpoint_id":0}`,
				}},
			}, nil
		}
		return AssistantReply{Text: "should not run after cancellation"}, nil
	})
	executor := funcToolExecutorFunc(func(ctx context.Context, call ToolCall) (ToolExecution, error) {
		return ToolExecution{Call: call, Output: "D-Mail sent."}, nil
	})
	dmailer := &mockDMailer{pending: &dmailEntry{message: "rewrite the plan", checkpointID: 0}}

	runner := NewWithToolExecutor(engine, executor, Config{})
	runner = runner.WithDMailer(dmailer)
	rootCtx, cancel := context.WithCancel(context.Background())
	shieldCalls := 0
	runner.shieldContextWriteFn = func(ctx context.Context, write func() error) error {
		shieldCalls++
		if err := write(); err != nil {
			return err
		}
		if shieldCalls == 4 {
			cancel()
			return ctx.Err()
		}
		return nil
	}

	result, err := runner.Run(rootCtx, store, Input{Prompt: "search for X"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, context.Canceled)
	}
	if result.Status != RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusInterrupted)
	}

	records, readErr := store.ReadAll()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	if len(records) == 0 {
		t.Fatal("history records = 0, want rollback state persisted")
	}
	last := records[len(records)-1]
	if last.Role != contextstore.RoleUser {
		t.Fatalf("last.Role = %q, want %q", last.Role, contextstore.RoleUser)
	}
	if !strings.Contains(last.Content, "D-Mail received: rewrite the plan") {
		t.Fatalf("last.Content = %q, want persisted D-Mail message", last.Content)
	}
}
```

- [ ] **Step 2: Run the focused tests and confirm the red state**

Run: `go test ./internal/runtime -run 'TestRunnerRunReturnsInterruptedAfterShieldedToolFailurePersistence|TestRunnerDMailRollbackReturnsInterruptedAfterShieldedRollbackPersistence' -count=1`
Expected: FAIL because tool-record writes and the rollback write sequence are still outside the new shield boundary.

- [ ] **Step 3: Wrap the tool-record loop and rollback sequence in grouped shield blocks**

In `internal/runtime/runtime.go`, make these edits:

1. In the tool-failure branch inside `advanceRun()`, wrap the entire `BuildToolStepRecords()` append loop in one shielded closure:

```go
				if err := r.shieldContextWrite(ctx, func() error {
					for _, record := range stepResult.BuildToolStepRecords() {
						if appendErr := store.Append(record); appendErr != nil {
							return appendErr
						}
					}
					return nil
				}); err != nil {
					return Result{}, false, fmt.Errorf("append tool failure record: %w", err)
				}
```

2. In the successful tool-execution branch, wrap the success append loop the same way:

```go
			if err := r.shieldContextWrite(ctx, func() error {
				for _, record := range stepResult.BuildToolStepRecords() {
					if appendErr := store.Append(record); appendErr != nil {
						return appendErr
					}
				}
				return nil
			}); err != nil {
				return Result{}, false, fmt.Errorf("append tool step record: %w", err)
			}
```

3. In the D-Mail rollback branch inside `Run()`, wrap the whole revert + post-rollback persistence sequence in one shielded block:

```go
				var newCheckpointID int
				err := r.shieldContextWrite(ctx, func() error {
					if _, revertErr := store.RevertToCheckpoint(checkpointID); revertErr != nil {
						return revertErr
					}
					var cpErr error
					newCheckpointID, cpErr = store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
						CreatedAt: time.Now().UTC().Format(time.RFC3339),
					})
					if cpErr != nil {
						return cpErr
					}
					r.dmailer.SetCheckpointCount(newCheckpointID + 1)
					if err := store.Append(contextstore.NewUserTextRecord(
						fmt.Sprintf("<system>CHECKPOINT %d</system>", newCheckpointID),
					)); err != nil {
						return err
					}
					dmailContent := fmt.Sprintf("<system>D-Mail received: %s</system>\n\nRead the D-Mail above carefully. Act on the information it contains. Do NOT mention the D-Mail mechanism or time travel to the user.", message)
					return store.Append(contextstore.NewUserTextRecord(dmailContent))
				})
				if err != nil {
					if isInterruptedError(err) {
						return r.interruptedResult(ctx, result)
					}
					return Result{Status: RunStatusFailed}, fmt.Errorf("persist rollback context: %w", err)
				}
```

Do not shield `emitStepEvents`, retry sleep, or tool execution.

- [ ] **Step 4: Re-run the focused tests and make sure they pass**

Run: `go test ./internal/runtime -run 'TestRunnerRunReturnsInterruptedAfterShieldedToolFailurePersistence|TestRunnerDMailRollbackReturnsInterruptedAfterShieldedRollbackPersistence' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit the second slice**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go
git commit -m "fix(runtime): shield tool and rollback persistence"
```

---

### Task 3: Lock in error precedence, verify the package, and update the roadmap

**Files:**
- Modify: `internal/runtime/runtime_test.go`
- Modify: `PLAN.md`

- [ ] **Step 1: Add the write-error precedence regression test**

Add this test to `internal/runtime/runtime_test.go` near the other shield tests:

```go
func TestRunnerRunReturnsContextstoreErrorBeforeCancellation(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "history-parent")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(parentFile) error = %v", err)
	}
	store := contextstore.New(filepath.Join(parentFile, "history.jsonl"))
	runner := New(staticEngine{}, Config{})

	rootCtx, cancel := context.WithCancel(context.Background())
	runner.shieldContextWriteFn = func(ctx context.Context, write func() error) error {
		cancel()
		if err := write(); err != nil {
			return err
		}
		return ctx.Err()
	}

	result, err := runner.Run(rootCtx, store, Input{Prompt: "hello"})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want contextstore failure to win", err)
	}
	if !strings.Contains(err.Error(), "persist prompt boundary") {
		t.Fatalf("Run() error = %v, want wrapped prompt-boundary write error", err)
	}
	if result.Status != RunStatusFailed {
		t.Fatalf("result.Status = %q, want %q", result.Status, RunStatusFailed)
	}
}
```

- [ ] **Step 2: Run the focused regression test and confirm the red state**

Run: `go test ./internal/runtime -run TestRunnerRunReturnsContextstoreErrorBeforeCancellation -count=1`
Expected: FAIL until the shield helper consistently returns the concrete write error before the canceled context.

- [ ] **Step 3: Make the last runtime adjustment if needed, then update `PLAN.md`**

If the focused test still fails, keep `shieldContextWrite()` in this exact order:

```go
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

Then update `PLAN.md` to reflect the finished runtime work:

```md
| Step retry | tenacity + jitter + connection recovery | retryable error classification + backoff/jitter + status updates | `done` |
```

```md
- Main remaining Go parity gaps are shield-like protection around context writes, shell mode toggle, background task management in ShellApp, and the Python auto-update system.
```

Change the Phase 14 checklist to:

```md
- [x] Add exponential backoff with jitter for step retry
- [x] Add shield equivalent for context writes (prevent cancellation corruption)
- [ ] Add background task management in ShellApp
```

And update Immediate Next Steps to:

```md
1. **Runtime parity** (Phase 14) -- background task management in ShellApp
2. **Auto-update** (Phase 15) -- background version check + install flow
3. **Shell remaining polish** (Phase 12) -- mode toggle, external editor, clipboard paste, background task browser
4. **Go-specific cleanup** (Phase 16) -- align tool defaults with Python
```

- [ ] **Step 4: Run package-level and full-suite verification**

Run: `go test ./internal/runtime -count=1 && go test ./...`
Expected: both commands PASS.

- [ ] **Step 5: Commit the final slice**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go PLAN.md
git commit -m "docs(plan): mark contextstore shield runtime parity complete"
```

---

## Self-Review Checklist

- Spec coverage: Task 1 covers prompt/final-reply persistence, Task 2 covers tool-record + rollback persistence, Task 3 covers error precedence and roadmap updates.
- Placeholder scan: no TODO/TBD markers remain; every command and code change is spelled out.
- Type consistency: the plan uses one helper name (`shieldContextWrite`), one test seam name (`shieldContextWriteFn`), and keeps all changes inside `internal/runtime/runtime.go` / `internal/runtime/runtime_test.go` plus the roadmap update in `PLAN.md`.
