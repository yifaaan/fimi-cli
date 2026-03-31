# Approval Integration Design

Date: 2026-03-30

## Context

The wire transport layer is complete (`internal/wire/`). `ApprovalRequest` and `ApprovalResponse` types exist. The shell has `ModeApprovalPrompt`, `pendingApprovals`, and `resolveApproval()` wired up. What's missing is the approval business logic, tool-level gating, and UI rendering.

## Decisions

| Decision | Choice |
|----------|--------|
| Gate level | Tool-internal explicit call (`approval.Request(ctx, action, desc)`) |
| Yolo mode trigger | CLI flag `--yolo` only |
| Approval UI style | Embedded in transcript area |
| Tools needing approval | `bash`, `write_file`, `replace_file` |

## Architecture

```
CLI --yolo flag
  |
  v
internal/approval.New(yolo)
  |  threaded through context via WithContext/FromContext
  v
Tool handler calls approval.Request(ctx, action, desc)
  |  checks yolo -> auto-approve map -> wire round-trip
  v
ApprovalRequest flows through Wire -> Shell UI
  |
  v
User presses y/s/n -> resolveApproval() -> req.Resolve()
  |
  v
If APPROVE_FOR_SESSION: action added to auto-approve set
```

## Components

### 1. `internal/approval/approval.go`

`Approval` struct:

```go
type Approval struct {
    yolo             bool
    autoApprove      map[string]bool
    mu               sync.Mutex
}
```

- `New(yolo bool) *Approval`
- `Request(ctx context.Context, action, description string) error`
  - If yolo: return nil
  - If action in autoApprove: return nil
  - Get wire via `wire.Current(ctx)`
  - Create `&wire.ApprovalRequest{Action: action, Description: description, ...}`
  - Call `wire.WaitForApproval(ctx, req)`
  - If response is `ApprovalApproveForSession`: add action to autoApprove
  - If rejected: return `ErrRejected`
- `ErrRejected` sentinel error

Context helpers:

- `WithContext(ctx, *Approval) context.Context`
- `FromContext(ctx) *Approval`

### 2. Tool handler changes

Three tools add approval checks at the start of their handler:

**`bash`** (`internal/tools/bash.go`):
```go
func handleBash(ctx context.Context, call tools.ToolCall, def tools.Definition) (tools.ToolExecution, error) {
    if a := approval.FromContext(ctx); a != nil {
        if err := a.Request(ctx, "bash", extractCommand(call)); err != nil {
            return rejectedExecution(call, err), nil
        }
    }
    // existing logic
}
```

**`write_file`** and **`replace_file`** follow the same pattern with action names `"write_file"` and `"replace_file"` respectively.

Description strings:
- bash: the command string (first line or truncated)
- write_file: the file path
- replace_file: the file path

### 3. CLI flag

Add `--yolo` / `--dangerously-skip-permissions` flag to `cmd/fimi`:
- Parsed into the app entry point
- Passed to `approval.New(yolo)` when constructing the approval instance
- The approval instance is threaded into context before tool execution

### 4. Shell approval UI

**View rendering** — When `m.mode == ModeApprovalPrompt`:
- Render the pending approval as a transcript entry:
  ```
  ⏺ bash (pending approval)
  │ rm -rf /tmp/test
  │ [y] Approve  [s] For session  [n] Reject
  ```
- Use existing transcript rendering styles (color, formatting)

**Keyboard handling** — In `handleKeyPress()`, before the `ModeIdle` guard:
```go
if m.mode == ModeApprovalPrompt {
    switch msg.String() {
    case "y":
        return m, resolveFirstPending(m, wire.ApprovalApprove)
    case "s":
        return m, resolveFirstPending(m, wire.ApprovalApproveForSession)
    case "n":
        return m, resolveFirstPending(m, wire.ApprovalReject)
    }
    return m, nil
}
```

Helper `resolveFirstPending` picks the first (or only) pending approval from the map and dispatches `approvalResolveMsg`.

### 5. Auto-approve for session

Handled entirely within `Approval.Request()`:
- When response is `ApprovalApproveForSession`, add `action` to the `autoApprove` map
- Subsequent calls with the same action skip the wire round-trip

### 6. Error handling for rejected tools

When a tool is rejected, it returns a `ToolExecution` with:
- `Error: true`
- `Output: "Tool execution rejected by user"`
- The runtime sees this as a normal tool result (error) and feeds it back to the LLM

## Files to Change

| File | Change |
|------|--------|
| `internal/approval/approval.go` | **New** — Approval struct, Request, context helpers |
| `internal/approval/approval_test.go` | **New** — unit tests |
| `internal/tools/bash.go` | Add approval.Request call |
| `internal/tools/file.go` (or write_file/replace_file handlers) | Add approval.Request calls |
| `internal/ui/shell/model.go` | Add ModeApprovalPrompt key handling + resolveFirstPending helper |
| `internal/ui/shell/model_output.go` | Add approval prompt rendering in transcript |
| `cmd/fimi/main.go` | Add --yolo flag |
| `internal/app/app.go` | Wire approval instance into context |

## Out of Scope

- `patch_file` approval (diff content is self-describing)
- MCP tool approval (can add later per-server)
- `read_file`/`glob`/`grep` approval (read-only)
- `agent` tool approval (subagent delegation is indirect)
- Config-based yolo mode
- Approval batching (multiple approvals at once)
