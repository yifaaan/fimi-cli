# Approval Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tool-level approval gating so dangerous tools (bash, write_file, replace_file) require user confirmation before execution.

**Architecture:** A new `internal/approval` package holds an `Approval` struct threaded through `context.Context`. Tool handlers call `approval.Request(ctx, action, desc)` which sends an `ApprovalRequest` through the existing wire and blocks until the shell UI resolves it. A `--yolo` CLI flag skips all approvals.

**Tech Stack:** Go stdlib (context, sync, errors), existing wire/approval types in `internal/wire`, Bubble Tea shell UI.

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/approval/approval.go` | **New.** `Approval` struct, `Request()`, context helpers, `ErrRejected` |
| `internal/approval/approval_test.go` | **New.** Unit tests for approval logic |
| `internal/tools/builtin.go` | **Modify.** Add approval gates to bash, write_file, replace_file handlers |
| `internal/app/app.go` | **Modify.** Parse `--yolo` flag, create `Approval`, thread into tool handler context |
| `internal/app/help.go` | **Modify.** Add `--yolo` to help text |
| `internal/ui/shell/model.go` | **Modify.** Add approval key handling (y/s/n) and `resolveFirstPending` helper |
| `internal/ui/shell/model_output.go` | **Modify.** Add `LineTypeApproval` for approval prompt rendering in transcript |

---

### Task 1: Create `internal/approval` package

**Files:**
- Create: `internal/approval/approval.go`
- Create: `internal/approval/approval_test.go`

- [ ] **Step 1: Write the approval package**

Create `internal/approval/approval.go`:

```go
package approval

import (
	"context"
	"errors"

	"fimi-cli/internal/wire"
)

// ErrRejected is returned when the user rejects an approval request.
var ErrRejected = errors.New("tool execution rejected by user")

type approvalKey struct{}

// Approval manages tool approval decisions.
// Thread-safe; zero value is not useful — use New().
type Approval struct {
	yolo        bool
	autoApprove map[string]bool
}

// New creates an Approval instance.
// If yolo is true, all approval requests are auto-approved.
func New(yolo bool) *Approval {
	return &Approval{
		yolo:        yolo,
		autoApprove: make(map[string]bool),
	}
}

// Request asks for user approval of an action.
// Returns nil on approval, ErrRejected on rejection.
// Skips the wire round-trip if yolo mode is on or the action is auto-approved for this session.
func (a *Approval) Request(ctx context.Context, action, description string) error {
	if a == nil {
		return nil
	}
	if a.yolo {
		return nil
	}
	if a.autoApprove[action] {
		return nil
	}

	w, ok := wire.Current(ctx)
	if !ok {
		return nil
	}

	req := &wire.ApprovalRequest{
		Action:      action,
		Description: description,
	}

	resp, err := w.WaitForApproval(ctx, req)
	if err != nil {
		return ErrRejected
	}

	switch resp {
	case wire.ApprovalApprove:
		return nil
	case wire.ApprovalApproveForSession:
		a.autoApprove[action] = true
		return nil
	case wire.ApprovalReject:
		return ErrRejected
	default:
		return ErrRejected
	}
}

// WithContext stores the Approval in ctx for retrieval by tool handlers.
func WithContext(ctx context.Context, a *Approval) context.Context {
	return context.WithValue(ctx, approvalKey{}, a)
}

// FromContext retrieves the Approval from ctx.
// Returns nil if no Approval is set.
func FromContext(ctx context.Context) *Approval {
	a, _ := ctx.Value(approvalKey{}).(*Approval)
	return a
}
```

- [ ] **Step 2: Write tests for the approval package**

Create `internal/approval/approval_test.go`:

```go
package approval

import (
	"context"
	"testing"

	"fimi-cli/internal/wire"
)

func TestYoloModeAutoApproves(t *testing.T) {
	a := New(true)
	err := a.Request(context.Background(), "bash", "rm -rf /")
	if err != nil {
		t.Fatalf("expected nil in yolo mode, got %v", err)
	}
}

func TestAutoApproveForSession(t *testing.T) {
	a := New(false)

	// Simulate approve-for-session by calling the internal logic directly
	// (the wire round-trip is tested in integration)
	a.autoApprove["bash"] = true

	err := a.Request(context.Background(), "bash", "ls")
	if err != nil {
		t.Fatalf("expected nil for auto-approved action, got %v", err)
	}
}

func TestRejectReturnsError(t *testing.T) {
	a := New(false)
	w := wire.New(0)
	ctx := wire.WithCurrent(context.Background(), w)

	// Consume the approval request and reject it in background
	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			return
		}
		if req, ok := msg.(*wire.ApprovalRequest); ok {
			req.Resolve(wire.ApprovalReject)
		}
	}()

	err := a.Request(ctx, "bash", "rm -rf /")
	if err != ErrRejected {
		t.Fatalf("expected ErrRejected, got %v", err)
	}
}

func TestApproveReturnsNil(t *testing.T) {
	a := New(false)
	w := wire.New(0)
	ctx := wire.WithCurrent(context.Background(), w)

	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			return
		}
		if req, ok := msg.(*wire.ApprovalRequest); ok {
			req.Resolve(wire.ApprovalApprove)
		}
	}()

	err := a.Request(ctx, "bash", "ls")
	if err != nil {
		t.Fatalf("expected nil on approve, got %v", err)
	}
}

func TestApproveForSessionAddsToAutoApprove(t *testing.T) {
	a := New(false)
	w := wire.New(0)
	ctx := wire.WithCurrent(context.Background(), w)

	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			return
		}
		if req, ok := msg.(*wire.ApprovalRequest); ok {
			req.Resolve(wire.ApprovalApproveForSession)
		}
	}()

	err := a.Request(ctx, "bash", "ls")
	if err != nil {
		t.Fatalf("expected nil on approve-for-session, got %v", err)
	}
	if !a.autoApprove["bash"] {
		t.Fatal("expected bash to be auto-approved after approve-for-session")
	}

	// Second call should skip the wire
	err = a.Request(context.Background(), "bash", "rm -rf /")
	if err != nil {
		t.Fatalf("expected nil for auto-approved second call, got %v", err)
	}
}

func TestFromContextNilWhenNotSet(t *testing.T) {
	a := FromContext(context.Background())
	if a != nil {
		t.Fatal("expected nil when approval not in context")
	}
}

func TestWithContextRoundTrip(t *testing.T) {
	a := New(true)
	ctx := WithContext(context.Background(), a)
	got := FromContext(ctx)
	if got != a {
		t.Fatal("expected to get the same Approval back from context")
	}
}
```

- [ ] **Step 3: Run the tests**

Run: `go test ./internal/approval/ -v`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```
feat(approval): add Approval package with yolo mode and auto-approve
```

Files: `internal/approval/approval.go`, `internal/approval/approval_test.go`

---

### Task 2: Add `--yolo` CLI flag and wire approval into app

**Files:**
- Modify: `internal/app/app.go` (parseRunInput, buildRunnerForAgent, runInput struct)
- Modify: `internal/app/help.go` (help flag text)

- [ ] **Step 1: Add `yolo` field to `runInput` and parse `--yolo` flag**

In `internal/app/app.go`, add `yolo bool` to the `runInput` struct (around line 147):

```go
type runInput struct {
	prompt          string
	forceNewSession bool
	continueSession bool
	modelAlias      string
	outputMode      string
	showHelp        bool
	yolo            bool
}
```

In `parseRunInput` (around line 157), add a case for `--yolo` and `--dangerously-skip-permissions` after the `--continue` case (around line 178):

```go
		if parseFlags && (arg == "--yolo" || arg == "--dangerously-skip-permissions") {
			input.yolo = true
			continue
		}
```

In the return statement at the end of `parseRunInput` (around line 228), add `yolo: input.yolo`.

- [ ] **Step 2: Wire approval into the tool handler context**

The approval instance needs to reach tool handlers through context. The cleanest integration point is `startRuntimeExecution` in the shell's `model.go`, but the `yolo` flag lives in `app.go`.

Add `Yolo` field to `shell.Dependencies` struct in `internal/ui/shell/shell.go` (around line 96):

```go
type Dependencies struct {
	Runner         Runner
	Store          contextstore.Context
	// ... existing fields ...
	Yolo           bool
}
```

In `internal/ui/shell/model.go`, modify `startRuntimeExecution` (around line 1091) to thread approval into context:

```go
func (m Model) startRuntimeExecution(store contextstore.Context, prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx := wire.WithCurrent(context.Background(), m.wire)
		ctx = approval.WithContext(ctx, approval.New(m.deps.Yolo))

		result, err := m.deps.Runner.Run(ctx, store, runtime.Input{
			Prompt:       prompt,
			Model:        m.deps.ModelName,
			SystemPrompt: m.deps.SystemPrompt,
		})

		return RuntimeCompleteMsg{Result: result, Err: err}
	}
}
```

Add the import `"fimi-cli/internal/approval"` to `model.go`.

In `internal/app/startup.go`, add `Yolo: input.yolo` to the `shell.Dependencies` construction in `buildShellDependencies` (around line 148). The function also needs the `yolo` parameter added to its signature. Since `buildShellDependencies` is called from `runShell` in `app.go`, pass `input.yolo` there.

Update `buildShellDependencies` signature to accept `yolo bool`:

```go
func buildShellDependencies(
	runner runtimeRunner,
	store contextstore.Context,
	agent loadedAgent,
	sess session.Session,
	input runInput,
	modelName string,
	historyFile string,
	initialRecords []contextstore.TextRecord,
	startupInfo shell.StartupInfo,
) shell.Dependencies {
	return shell.Dependencies{
		// ... existing fields ...
		Yolo: input.yolo,
	}
}
```

- [ ] **Step 3: Add `--yolo` to help text**

In `internal/app/help.go`, add to `helpFlagLines` (around line 54):

```go
	"  --yolo           Skip all tool approval prompts",
```

- [ ] **Step 4: Run the build**

Run: `go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 5: Commit**

```
feat(app): add --yolo flag and wire approval into shell context
```

Files: `internal/app/app.go`, `internal/app/help.go`, `internal/app/startup.go`, `internal/ui/shell/shell.go`, `internal/ui/shell/model.go`

---

### Task 3: Add approval gates to tool handlers

**Files:**
- Modify: `internal/tools/builtin.go` (bash, write_file, replace_file handlers)

- [ ] **Step 1: Add approval gate to bash handler**

In `internal/tools/builtin.go`, add the import `"fimi-cli/internal/approval"`.

Modify `newBashHandler` (line 212) to add an approval check before execution. The handler receives `call` which has `Arguments` containing the command. Extract the command for the description:

```go
func newBashHandler(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			desc := args.Command
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

		// 模式 1：查询后台任务状态
		if args.TaskID != "" {
			return handleBashTaskQuery(call, args, bgMgr)
		}

		// 模式 2：后台执行
		if args.Background {
			return handleBashBackground(call, args, workDir, bgMgr)
		}

		// 模式 3：前台执行（原有逻辑）
		return handleBashForeground(ctx, call, args, workDir, shaper)
	}
}
```

- [ ] **Step 2: Add approval gate to write_file handler**

Modify `newWriteFileHandler` (line 515):

```go
func newWriteFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeWriteFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "write_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		// ... rest of existing handler unchanged ...
```

- [ ] **Step 3: Add approval gate to replace_file handler**

Modify `newReplaceFileHandler` (line 659):

```go
func newReplaceFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReplaceFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "replace_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		// ... rest of existing handler unchanged ...
```

- [ ] **Step 4: Run the build and existing tests**

Run: `go build ./... && go test ./internal/tools/ -v`
Expected: Clean build, all existing tests pass.

- [ ] **Step 5: Commit**

```
feat(tools): add approval gates to bash, write_file, replace_file
```

Files: `internal/tools/builtin.go`

---

### Task 4: Add shell approval UI (keyboard + rendering)

**Files:**
- Modify: `internal/ui/shell/model.go` (key handling, resolveFirstPending)
- Modify: `internal/ui/shell/model_output.go` (approval line type + rendering)

- [ ] **Step 1: Add `LineTypeApproval` to the output model**

In `internal/ui/shell/model_output.go`, add a new line type constant after `LineTypeError` (around line 33):

```go
	// LineTypeApproval 审批提示
	LineTypeApproval
```

Add a rendering case in `renderLine` (around line 248, before `default`):

```go
	case LineTypeApproval:
		return styles.HelpStyle.Render(line.Content)
```

- [ ] **Step 2: Add `resolveFirstPending` helper to model.go**

Add a new helper method in `internal/ui/shell/model.go` (before `resolveApproval` around line 1802):

```go
// resolveFirstPending resolves the first pending approval request.
func (m Model) resolveFirstPending(resp wire.ApprovalResponse) (tea.Model, tea.Cmd) {
	for id, req := range m.pendingApprovals {
		return m.resolveApproval(id, resp)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}
```

- [ ] **Step 3: Add approval key handling in `handleKeyPress`**

In `internal/ui/shell/model.go`, modify `handleKeyPress` (around line 348). Add the approval mode handling **before** the `if m.mode != ModeIdle` guard at line 377:

```go
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Approval mode key handling
	if m.mode == ModeApprovalPrompt {
		switch msg.String() {
		case "y":
			return m.resolveFirstPending(wire.ApprovalApprove)
		case "s":
			return m.resolveFirstPending(wire.ApprovalApproveForSession)
		case "n":
			return m.resolveFirstPending(wire.ApprovalReject)
		}
		return m, nil
	}

	// 全局快捷键
	switch msg.String() {
	// ... existing code ...
```

- [ ] **Step 4: Add approval prompt to transcript when approval arrives**

Modify the `approvalRequestMsg` handler in `Update()` (around line 262) to also append a transcript line:

```go
	case approvalRequestMsg:
		if msg.Request != nil {
			m.pendingApprovals[msg.Request.ID] = msg.Request
			// Show approval prompt in transcript
			promptText := fmt.Sprintf("⏺ %s (pending approval)\n  %s\n  [y] Approve  [s] For session  [n] Reject",
				msg.Request.Action, msg.Request.Description)
			m.output = m.output.AppendLine(TranscriptLine{
				Type:    LineTypeApproval,
				Content: promptText,
			})
		}
		m.mode = ModeApprovalPrompt
		return m, nil
```

- [ ] **Step 5: Run the build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 6: Commit**

```
feat(shell): add approval prompt UI with y/s/n key handling
```

Files: `internal/ui/shell/model.go`, `internal/ui/shell/model_output.go`

---

### Task 5: Also wire approval into print mode (text / stream-json)

**Files:**
- Modify: `internal/app/app.go` (runPrint method)

In print mode there is no shell UI to resolve approvals, so the approval instance should be yolo (auto-approve all) to avoid blocking forever. This is the same behavior as the Python reference.

- [ ] **Step 1: Ensure print mode auto-approves**

In `internal/app/app.go`, modify `runPrint` (around line 683). After building the runner, the print mode uses `ui.Run` which doesn't have a wire-based UI. Print mode should always use yolo=true. Since `startRuntimeExecution` in the shell already threads `m.deps.Yolo`, we need a different path for print mode.

The print mode uses `ui.Run()` (in `internal/ui/run.go`) which does create a wire. But there's no interactive UI to resolve approvals. The simplest approach: in print mode, force `yolo = true` so no approvals are ever requested.

In `runPrint` (line 682), after building the runner, ensure the input reflects yolo:

```go
func (d dependencies) runPrint(
	ctx context.Context,
	cfg config.Config,
	agent loadedAgent,
	workDir string,
	input runInput,
) error {
	prompt, err := resolvePrintPrompt(input)
	if err != nil {
		return err
	}

	cfg = resolveModelOverride(cfg, agent)

	store, err := d.preparePrintStore(workDir, prompt)
	if err != nil {
		return err
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}

	runtimeInput := buildRuntimePromptInput(cfg, agent, prompt)

	// Print mode has no interactive UI, always auto-approve
	printYoloCtx := approval.WithContext(ctx, approval.New(true))

	_, err = ui.Run(printYoloCtx, runner.Run, store, runtimeInput, d.resolveVisualizer(input.outputMode))

	return err
}
```

Add import `"fimi-cli/internal/approval"` to `app.go`.

- [ ] **Step 2: Run the build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```
feat(app): auto-approve tools in print mode (no interactive UI)
```

Files: `internal/app/app.go`

---

### Task 6: End-to-end smoke test

- [ ] **Step 1: Build the binary**

Run: `go build -o /tmp/fimi ./cmd/fimi`
Expected: Clean build.

- [ ] **Step 2: Test --yolo flag in help**

Run: `/tmp/fimi --help`
Expected: Help output includes `--yolo` line.

- [ ] **Step 3: Test --yolo flag is accepted**

Run: `/tmp/fimi --yolo --help`
Expected: Help output (yolo + help should not conflict).

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: All tests pass.

- [ ] **Step 5: Final commit if any fixups needed**

Only commit if fixups were required.

---

## Self-Review Checklist

- [x] **Spec coverage:** Every spec section maps to a task. Approval struct (Task 1), CLI flag + wiring (Task 2), tool gates (Task 3), UI (Task 4), print mode (Task 5), smoke test (Task 6).
- [x] **Placeholder scan:** No TBD, TODO, "implement later", "fill in details", "add appropriate error handling", "similar to Task N", or steps without code.
- [x] **Type consistency:** `Approval.Request(ctx, action, desc)` returns `error`; `ErrRejected` used consistently; `wire.ApprovalRequest` fields match; `ApprovalResponse` constants used correctly; `LineTypeApproval` defined and used in rendering; `approvalKey{}` context key type used consistently.
