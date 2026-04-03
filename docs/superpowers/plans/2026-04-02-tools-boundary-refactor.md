# internal/tools Boundary Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `internal/tools` easier to read by keeping `executor.go` as the execution boundary, shrinking `builtin.go` to registration-focused code, and moving bash execution/query/background logic into a dedicated `builtin_bash.go` file without changing behavior.

**Architecture:** Keep the `internal/tools` package boundary unchanged. `Executor.Execute(...)` remains the single place that normalizes tool names, checks the allowed-tool set, injects `approval.WithToolCallID(...)`, and wraps handler errors. `builtin.go` keeps builtin registration, lightweight builtin handlers, shared builtin error/argument definitions used across sibling files, while `builtin_bash.go` owns bash argument decoding, approval description generation, foreground execution, background start, background query, and exit-code helpers; `background.go` stays a concrete manager.

**Tech Stack:** Go, Go testing, `context`, `encoding/json`, `os/exec`, `fimi-cli/internal/approval`, `fimi-cli/internal/runtime`, `fimi-cli/internal/wire`

---

## File Structure

- Modify: `internal/tools/executor.go` — keep the execution boundary unchanged unless a tiny comment/import cleanup is required.
- Modify: `internal/tools/executor_test.go` — add a regression test proving `Execute()` injects the active tool-call ID into the approval context.
- Modify: `internal/tools/builtin.go` — keep builtin registration, option plumbing, `newThinkHandler`, `newSetTodoListHandler`, and shared builtin declarations that are still used by sibling files; remove bash-specific constants, argument struct, imports, and handler/helper bodies.
- Create: `internal/tools/builtin_bash.go` — move bash-specific constants, `bashArguments`, approval description logic, foreground/background/query execution, timeout test helper, and exit/exit-code helpers here.
- Modify: `internal/tools/builtin_test.go` — add characterization tests for bash approval behavior, then keep the existing foreground/background tests green after the move.
- Keep unchanged: `internal/tools/background.go` — concrete background manager with no new abstractions.
- Keep unchanged: `internal/tools/background_test.go` — continue using the current condition-based waiting helper.
- No production changes expected: `internal/tools/builtin_readonly.go`, `internal/tools/builtin_write.go`, `internal/tools/builtin_patch.go`, `internal/app/app_acp.go`.

## Constraints

- Do not add a new package.
- Do not change exported names or tool behavior.
- Do not move background lifecycle logic out of `background.go`.
- Do not add new generic coordinators/helpers just to reduce a few repeated branches.
- Do not create commits unless the user explicitly asks for one during execution.

---

### Task 1: Lock down the current tools boundary with characterization tests

**Files:**
- Modify: `internal/tools/builtin_test.go`
- Modify: `internal/tools/executor_test.go`
- Test: `internal/tools/builtin_test.go`
- Test: `internal/tools/executor_test.go`

This is a behavior-preserving refactor. The first tests in this task are characterization tests, so they should pass before any production edit.

- [ ] **Step 1: Add bash approval characterization tests before moving code**

Replace the import block in `internal/tools/builtin_test.go` with the exact block below:

```go
import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/wire"
)
```

Append these exact tests to `internal/tools/builtin_test.go`:

```go
func TestNewBuiltinExecutorBashApprovalRequestUsesToolCallIDAndDescription(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolBash,
		Kind: KindCommand,
	}}, workDir, nil)

	a := approval.New(false)
	w := wire.New(1)
	ctx = approval.WithContext(ctx, a)
	ctx = wire.WithCurrent(ctx, w)

	reqCh := make(chan *wire.ApprovalRequest, 1)
	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			return
		}
		req, ok := msg.(*wire.ApprovalRequest)
		if !ok {
			return
		}
		reqCh <- req
		_ = req.Resolve(wire.ApprovalApprove)
	}()

	_, err := executor.Execute(ctx, runtime.ToolCall{
		ID:        "call-123",
		Name:      ToolBash,
		Arguments: `{"command":"printf 'ok'"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	select {
	case req := <-reqCh:
		if req.Action != "bash" {
			t.Fatalf("ApprovalRequest.Action = %q, want %q", req.Action, "bash")
		}
		if req.Description != "printf 'ok'" {
			t.Fatalf("ApprovalRequest.Description = %q, want %q", req.Description, "printf 'ok'")
		}
		if req.ToolCallID != "call-123" {
			t.Fatalf("ApprovalRequest.ToolCallID = %q, want %q", req.ToolCallID, "call-123")
		}
	case <-time.After(time.Second):
		t.Fatal("expected approval request to be received")
	}
}

func TestNewBuiltinExecutorBashReturnsRejectedOutputWhenApprovalDenied(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolBash,
		Kind: KindCommand,
	}}, t.TempDir(), nil)

	a := approval.New(false)
	w := wire.New(1)
	ctx = approval.WithContext(ctx, a)
	ctx = wire.WithCurrent(ctx, w)

	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			return
		}
		req, ok := msg.(*wire.ApprovalRequest)
		if !ok {
			return
		}
		_ = req.Resolve(wire.ApprovalReject)
	}()

	got, err := executor.Execute(ctx, runtime.ToolCall{
		ID:        "call-456",
		Name:      ToolBash,
		Arguments: `{"command":"printf 'blocked'"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if got.Output != "Tool execution rejected by user" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "Tool execution rejected by user")
	}
	if got.Call.Name != ToolBash {
		t.Fatalf("Execute().Call.Name = %q, want %q", got.Call.Name, ToolBash)
	}
}
```

- [ ] **Step 2: Run the new bash characterization tests before any code move**

Run:

```bash
go test ./internal/tools -run 'TestNewBuiltinExecutorBashApprovalRequestUsesToolCallIDAndDescription|TestNewBuiltinExecutorBashReturnsRejectedOutputWhenApprovalDenied'
```

Expected: PASS.

- [ ] **Step 3: Add the executor approval-context regression test**

Replace the import block in `internal/tools/executor_test.go` with:

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

Append this exact test to `internal/tools/executor_test.go`:

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

- [ ] **Step 4: Run the focused executor/builtin regression set**

Run:

```bash
go test ./internal/tools -run 'TestExecutorExecuteInjectsToolCallIDIntoApprovalContext|TestExecutorExecuteUsesRegisteredHandler|TestExecutorExecuteReturnsErrorForDisallowedTool|TestExecutorExecutePreservesTemporaryClassification|TestNewBuiltinExecutorBashApprovalRequestUsesToolCallIDAndDescription|TestNewBuiltinExecutorBashReturnsRejectedOutputWhenApprovalDenied'
```

Expected: PASS.

---

### Task 2: Move bash execution into `builtin_bash.go` and shrink `builtin.go`

**Files:**
- Create: `internal/tools/builtin_bash.go`
- Modify: `internal/tools/builtin.go`
- Test: `internal/tools/builtin_test.go`
- Test: `internal/tools/background_test.go`

- [ ] **Step 1: Create the dedicated bash implementation file**

Create `internal/tools/builtin_bash.go` with the exact content below:

```go
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

const (
	DefaultBashCommandTimeout = 120 * time.Second
	MaxBashCommandTimeout     = 300 * time.Second
)

var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolCommandTimedOut = errors.New("tool command timed out")

type bashArguments struct {
	Command    string `json:"command"`
	Timeout    int    `json:"timeout"`
	Background bool   `json:"background"`
	TaskID     string `json:"task_id"`
}

func newBashHandler(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if a := approval.FromContext(ctx); a != nil {
			desc := bashApprovalDescription(args)
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			if err := a.Request(ctx, "bash", desc); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		if args.TaskID != "" {
			return handleBashTaskQuery(call, args, bgMgr)
		}
		if args.Background {
			return handleBashBackground(call, args, workDir, bgMgr)
		}
		return handleBashForeground(ctx, call, args, workDir, shaper)
	}
}

func handleBashTaskQuery(call runtime.ToolCall, args bashArguments, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}

	result, err := bgMgr.Status(args.TaskID)
	if err != nil {
		return runtime.ToolExecution{}, markRefused(err)
	}

	var outputParts []string
	outputParts = append(outputParts, fmt.Sprintf("Task %s [%s]", result.ID, result.Status))
	outputParts = append(outputParts, fmt.Sprintf("Command: %s", result.Command))
	outputParts = append(outputParts, fmt.Sprintf("Duration: %s", result.Duration.Round(time.Millisecond)))
	if result.ExitCode != 0 {
		outputParts = append(outputParts, fmt.Sprintf("Exit code: %d", result.ExitCode))
	}
	if result.Stdout != "" {
		outputParts = append(outputParts, "STDOUT:", result.Stdout)
	}
	if result.Stderr != "" {
		outputParts = append(outputParts, "STDERR:", result.Stderr)
	}

	joined := strings.Join(outputParts, "\n")
	return runtime.ToolExecution{
		Call:          call,
		Output:        joined,
		DisplayOutput: buildInlinePreview("", joined),
	}, nil
}

func handleBashBackground(call runtime.ToolCall, args bashArguments, workDir string, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}
	if strings.TrimSpace(args.Command) == "" {
		return runtime.ToolExecution{}, markRefused(ErrToolCommandRequired)
	}

	taskID, err := bgMgr.Start(args.Command, workDir, 0)
	if err != nil {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("start background task: %w", err))
	}

	message := fmt.Sprintf("Background task started: %s (use task_id=\"%s\" to check status)", taskID, taskID)
	return runtime.ToolExecution{
		Call:          call,
		Output:        message,
		DisplayOutput: buildInlinePreview("Ran "+args.Command, message),
	}, nil
}

func handleBashForeground(ctx context.Context, call runtime.ToolCall, args bashArguments, workDir string, shaper OutputShaper) (runtime.ToolExecution, error) {
	timeout := DefaultBashCommandTimeout
	if args.Timeout > 0 {
		timeout = min(time.Duration(args.Timeout)*time.Second, MaxBashCommandTimeout)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
	}
	if err != nil && !isExitError(err) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
	}

	rawStdout := stdout.String()
	shapedStdout := shaper.Shape(rawStdout)
	rawStderr := stderr.String()
	shapedStderr := shaper.Shape(rawStderr)

	var outputParts []string
	outputParts = append(outputParts, shapedStdout.Output)
	if shapedStderr.Output != "" {
		outputParts = append(outputParts, "STDERR:", shapedStderr.Output)
	}

	var truncationMsgs []string
	if shapedStdout.Message != "" {
		truncationMsgs = append(truncationMsgs, "stdout: "+shapedStdout.Message)
	}
	if shapedStderr.Message != "" {
		truncationMsgs = append(truncationMsgs, "stderr: "+shapedStderr.Message)
	}
	if len(truncationMsgs) > 0 {
		outputParts = append(outputParts, "\n["+strings.Join(truncationMsgs, "; ")+"]")
	}

	return runtime.ToolExecution{
		Call:          call,
		Output:        strings.Join(outputParts, "\n"),
		DisplayOutput: buildSectionedPreview("Ran "+args.Command, previewSection{Label: "STDOUT", Content: rawStdout}, previewSection{Label: "STDERR", Content: rawStderr}),
		Stdout:        shapedStdout.Output,
		Stderr:        shapedStderr.Output,
		ExitCode:      exitCodeFromError(err),
	}, nil
}

func newBashHandlerWithTimeout(workDir string, shaper OutputShaper, timeout time.Duration) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
		if workDir != "" {
			cmd.Dir = workDir
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
			}
			if !isExitError(err) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
			}
		}

		shapedStdout := shaper.Shape(stdout.String())
		shapedStderr := shaper.Shape(stderr.String())
		return runtime.ToolExecution{
			Call:          call,
			Output:        shapedStdout.Output,
			DisplayOutput: buildSectionedPreview("Ran "+args.Command, previewSection{Label: "STDOUT", Content: stdout.String()}, previewSection{Label: "STDERR", Content: stderr.String()}),
			Stdout:        shapedStdout.Output,
			Stderr:        shapedStderr.Output,
			ExitCode:      exitCodeFromError(err),
		}, nil
	}
}

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, markRefused(fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.Command = strings.TrimSpace(args.Command)
	args.TaskID = strings.TrimSpace(args.TaskID)
	if args.TaskID != "" {
		return args, nil
	}
	if args.Command == "" {
		return bashArguments{}, markRefused(ErrToolCommandRequired)
	}

	return args, nil
}

func bashApprovalDescription(args bashArguments) string {
	if args.TaskID != "" {
		return fmt.Sprintf("query background task %s", args.TaskID)
	}
	if args.Background {
		return "background: " + args.Command
	}
	return args.Command
}

func isExitError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
```

- [ ] **Step 2: Remove the moved bash code from `builtin.go` so the file becomes registration-focused again**

Replace the import block at the top of `internal/tools/builtin.go` with this exact block:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"fimi-cli/internal/runtime"
)
```

Delete these exact declarations and functions from `internal/tools/builtin.go` because they now live in `internal/tools/builtin_bash.go`:

```go
const (
	DefaultBashCommandTimeout = 120 * time.Second
	MaxBashCommandTimeout     = 300 * time.Second
)

var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolCommandTimedOut = errors.New("tool command timed out")

type bashArguments struct {
	Command    string `json:"command"`
	Timeout    int    `json:"timeout"`
	Background bool   `json:"background"`
	TaskID     string `json:"task_id"`
}

func newBashHandler(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) HandlerFunc
func handleBashTaskQuery(call runtime.ToolCall, args bashArguments, bgMgr *BackgroundManager) (runtime.ToolExecution, error)
func handleBashBackground(call runtime.ToolCall, args bashArguments, workDir string, bgMgr *BackgroundManager) (runtime.ToolExecution, error)
func handleBashForeground(ctx context.Context, call runtime.ToolCall, args bashArguments, workDir string, shaper OutputShaper) (runtime.ToolExecution, error)
func newBashHandlerWithTimeout(workDir string, shaper OutputShaper, timeout time.Duration) HandlerFunc
func decodeBashArguments(raw string) (bashArguments, error)
func bashApprovalDescription(args bashArguments) string
func isExitError(err error) bool
func exitCodeFromError(err error) int
```

Do **not** change `builtinHandlers(...)`; it should still register `ToolBash: newBashHandler(workDir, shaper, bgMgr)` and let the new file provide the implementation.

- [ ] **Step 3: Run the focused bash/background regression suite after the move**

Run:

```bash
go test ./internal/tools -run 'TestNewBuiltinExecutorBashRunsCommandInsideWorkDir|TestNewBuiltinExecutorBashReturnsStructuredNonZeroExit|TestNewBuiltinExecutorBashQueriesBackgroundTaskWithoutCommand|TestNewBuiltinExecutorBashRejectsMissingCommandWithoutTaskID|TestNewBuiltinExecutorBashApprovalRequestUsesToolCallIDAndDescription|TestNewBuiltinExecutorBashReturnsRejectedOutputWhenApprovalDenied|TestNewBashHandlerWithTimeoutCancelsLongRunningCommand|TestBackgroundManagerStatusReturnsCompletedTask|TestBackgroundManagerStatusReturnsFailedTask|TestBackgroundManagerStatusReturnsTimedOutTask|TestBackgroundManagerKillTerminatesRunningTask'
```

Expected: PASS.

- [ ] **Step 4: Run the full `internal/tools` package tests**

Run:

```bash
go test ./internal/tools
```

Expected: PASS.

- [ ] **Step 5: Commit the extraction checkpoint if this implementation run is using intermediate commits**

Run:

```bash
git add internal/tools/builtin.go internal/tools/builtin_bash.go internal/tools/builtin_test.go internal/tools/executor_test.go
git commit -m "refactor(tools): isolate bash execution flow"
```

Expected: a new local commit only if the user asked for commit checkpoints during execution; otherwise skip this step.

---

### Task 3: Finish verification without expanding scope

**Files:**
- Verify unchanged: `internal/tools/executor.go`
- Verify unchanged: `internal/tools/background.go`
- Verify unchanged: `internal/tools/background_test.go`
- Test: `internal/tools`
- Test: `./...`

- [ ] **Step 1: Re-read the final boundary files and confirm the responsibilities match the spec**

Check these files in this order:

```text
internal/tools/executor.go
internal/tools/builtin.go
internal/tools/builtin_bash.go
internal/tools/background.go
```

Confirm all four points are true before moving on:

```text
1. executor.go only normalizes names, checks allowed tools, injects approval.WithToolCallID(...), and wraps handler errors.
2. builtin.go no longer contains the large bash/query/background implementation bodies.
3. builtin_bash.go reads top-to-bottom as decode -> approval -> query/background/foreground -> timeout helper -> small exit helpers.
4. background.go still only manages background task lifecycle and was not turned into a higher-level coordinator.
```

- [ ] **Step 2: Run the package-level regression sweep that matters for this refactor**

Run:

```bash
go test ./internal/tools ./internal/approval
```

Expected: PASS.

- [ ] **Step 3: Run the repository-wide suite once to catch cross-package fallout**

Run:

```bash
go test ./...
```

Expected: PASS.

If `go test ./...` reports only a tiny compile cleanup caused by the file move, apply the smallest possible fix and re-run the same command. Do not add new logic.

- [ ] **Step 4: Create the final commit only if the user asked for one**

Run:

```bash
git add internal/tools/builtin.go internal/tools/builtin_bash.go internal/tools/builtin_test.go internal/tools/executor_test.go
git commit -m "refactor(tools): clarify builtin execution boundaries"
```

Expected: one new local commit only when the user explicitly requested a commit for the implementation run.

---

## Self-Review

### Spec coverage
- `executor.go` remains the boundary layer: covered by Task 1 Step 3 and Task 3 Step 1.
- `builtin.go` shrinks to registration-focused code: covered by Task 2 Step 2.
- bash foreground/background/query logic moves into one themed file: covered by Task 2 Step 1 and Step 3.
- approval behavior stays unchanged: covered by Task 1 Step 1, Step 3, and Step 4.
- background manager stays concrete and behavior-preserving: covered by Task 2 Step 3 and Task 3 Step 1.
- testing stays focused on boundaries instead of expanding feature scope: covered by Task 1 and Task 2 Step 3-4.

### Placeholder scan
- No placeholder markers or deferred-work notes remain.
- Every code-changing step includes exact code blocks or exact declarations to remove.
- Every verification step includes exact commands and expected results.

### Type consistency
- `newBashHandler`, `handleBashTaskQuery`, `handleBashBackground`, `handleBashForeground`, `decodeBashArguments`, `bashApprovalDescription`, `isExitError`, and `exitCodeFromError` keep their current names and signatures.
- `DefaultBashCommandTimeout`, `MaxBashCommandTimeout`, `ErrToolCommandRequired`, and `ErrToolCommandTimedOut` remain package-level names, so existing tests and sibling files keep compiling.
- `background.go` continues calling the same `exitCodeFromError(err)` helper name after the move.
