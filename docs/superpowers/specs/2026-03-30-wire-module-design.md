# Wire Module — Design Spec

## Overview

Add a bidirectional communication channel (`Wire`) between the runtime (agent loop) and UI (shell), enabling both event streaming and approval request flows. This replaces the current one-way `Sink` pattern with a unified abstraction that matches the Python kimi-cli wire design.

---

## Architecture

### Message Flow

```
┌─────────────┐                      ┌─────────────┐
│   Runtime   │                      │     UI      │
│             │   wire.Send(msg)     │             │
│             ├─────────────────────►│ wire.Receive()
│             │                      │             │
│ wire.Wait   │◄─────────────────────┤ wire.Resolve()
│ ForApproval │   approval result    │             │
│             │                      │             │
└─────────────┘                      └─────────────┘
```

### Package Structure

```
internal/
├── wire/
│   ├── wire.go          # Wire struct + Send/Receive/Wait/Resolve
│   └── message.go       # Message types (Event + ApprovalRequest)
├── runtime/
│   └── runtime.go       # Uses Wire instead of Sink
└── ui/shell/
    └── model.go          # Runs wire.Receive() loop
```

---

## Message Types

### `internal/wire/message.go`

```go
package wire

// Message is any message that flows through the wire.
type Message interface {
    isMessage()
}

// ApprovalRequest represents a request for user approval.
type ApprovalRequest struct {
    ID          string            // unique request ID
    ToolCallID  string            // tool call being approved
    Action      string            // action type (e.g., "bash_execute")
    Description string            // human-readable description

    responseCh  chan ApprovalResponse // internal: response channel
}

func (ApprovalRequest) isMessage() {}

// ApprovalResponse is the user's response.
type ApprovalResponse string

const (
    ApprovalApprove          ApprovalResponse = "approve"
    ApprovalApproveForSession ApprovalResponse = "approve_for_session"
    ApprovalReject           ApprovalResponse = "reject"
)

// EventMessage wraps existing events to satisfy Message interface.
type EventMessage struct {
    Event events.Event
}

func (EventMessage) isMessage() {}
```

---

## Wire Implementation

### `internal/wire/wire.go`

```go
package wire

import (
    "context"
    "errors"
)

var ErrWireClosed = errors.New("wire is closed")

// Wire is a bidirectional channel between runtime and UI.
type Wire struct {
    ch   chan Message  // main message channel
    done chan struct{} // shutdown signal
}

// New creates a Wire with buffered channel (default 64).
func New(bufferSize int) *Wire {
    if bufferSize <= 0 {
        bufferSize = 64
    }
    return &Wire{
        ch:   make(chan Message, bufferSize),
        done: make(chan struct{}),
    }
}

// Send puts a message on the wire (non-blocking).
func (w *Wire) Send(msg Message) {
    select {
    case w.ch <- msg:
    case <-w.done:
        panic("wire: send on closed wire")
    }
}

// Receive waits for next message or shutdown.
func (w *Wire) Receive(ctx context.Context) (Message, error) {
    select {
    case msg := <-w.ch:
        return msg, nil
    case <-w.done:
        return nil, ErrWireClosed
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// Shutdown closes the wire.
func (w *Wire) Shutdown() {
    close(w.done)
}

// WaitForApproval sends request and waits for response.
func (w *Wire) WaitForApproval(ctx context.Context, req *ApprovalRequest) (ApprovalResponse, error) {
    req.responseCh = make(chan ApprovalResponse, 1)
    w.Send(req)

    select {
    case resp := <-req.responseCh:
        return resp, nil
    case <-w.done:
        return ApprovalReject, ErrWireClosed
    case <-ctx.Done():
        return ApprovalReject, ctx.Err()
    }
}

// Resolve completes an approval request.
func (req *ApprovalRequest) Resolve(resp ApprovalResponse) {
    select {
    case req.responseCh <- resp:
    default:
    }
}
```

### Key Design Points

1. **Buffered channel** — `Send()` is non-blocking, runtime never stalls
2. **Separate done channel** — Clean shutdown without closing message channel
3. **Per-request response channel** — Each approval gets its own reply path
4. **Context integration** — All waits respect cancellation/timeout

---

## Runtime Integration

### `internal/runtime/runtime.go`

```go
type Runner struct {
    engine       Engine
    toolExecutor ToolExecutor
    wire         *wire.Wire  // replaces eventSink
    // ...
}

// WithWire binds a wire to the runner.
func (r Runner) WithWire(w *wire.Wire) Runner {
    r.wire = w
    return r
}

// emitEvent sends event through wire.
func (r Runner) emitEvent(ctx context.Context, event events.Event) error {
    if r.wire == nil {
        return nil
    }
    r.wire.Send(wire.EventMessage{Event: event})
    return nil
}

// executeToolCallWithApproval handles approval flow.
func (r Runner) executeToolCallWithApproval(ctx context.Context, call ToolCall) (ToolExecution, error) {
    if needsApproval(call) {
        req := &wire.ApprovalRequest{
            ID:          uuid.NewString(),
            ToolCallID:  call.ID,
            Action:      call.Name,
            Description: buildApprovalDescription(call),
        }

        resp, err := r.wire.WaitForApproval(ctx, req)
        if err != nil || resp == wire.ApprovalReject {
            return ToolExecution{}, ErrApprovalRejected
        }
    }

    return r.toolExecutor.Execute(ctx, call)
}
```

---

## UI Integration (Shell)

### `internal/ui/shell/model.go`

```go
type Model struct {
    wire             *wire.Wire
    pendingApprovals map[string]*wire.ApprovalRequest
    // ...
}

// wireReceiveLoop converts wire messages to tea.Msg.
func (m Model) wireReceiveLoop() tea.Cmd {
    return func() tea.Msg {
        msg, err := m.wire.Receive(context.Background())
        if err != nil {
            return wireErrorMsg{err}
        }

        switch msg := msg.(type) {
        case wire.EventMessage:
            return eventToTeaMsg(msg.Event)
        case *wire.ApprovalRequest:
            return approvalRequestMsg{Request: msg}
        }
        return nil
    }
}

// handleApprovalRequest shows approval UI.
func (m Model) handleApprovalRequest(msg approvalRequestMsg) (Model, tea.Cmd) {
    m.pendingApprovals[msg.Request.ID] = msg.Request
    m.mode = ModeApprovalPrompt
    return m, nil
}

// resolveApproval completes approval flow.
func (m Model) resolveApproval(id string, resp wire.ApprovalResponse) (Model, tea.Cmd) {
    if req, ok := m.pendingApprovals[id]; ok {
        req.Resolve(resp)
        delete(m.pendingApprovals, id)
    }
    m.mode = ModeThinking
    return m, nil
}
```

---

## Migration Path

| Phase | Change |
|-------|--------|
| 1 | Add `internal/wire` package, keep `Sink` working |
| 2 | Add `WithWire()` to runtime, parallel to `WithEventSink()` |
| 3 | Update shell to use `wire`, deprecate `Sink` |
| 4 | Remove `Sink` interface, cleanup |

---

## File Summary

| File | Action |
|------|--------|
| `internal/wire/wire.go` | Create — Wire struct + methods |
| `internal/wire/message.go` | Create — Message types |
| `internal/runtime/runtime.go` | Modify — Add WithWire(), replace Sink |
| `internal/runtime/events/events.go` | Modify — Keep events, add compatibility |
| `internal/ui/shell/model.go` | Modify — Add wire field, receive loop, approval handling |
| `internal/ui/shell/model_runtime.go` | Modify — wireReceiveLoop integration |
| `internal/acp/server.go` | Modify — Wire for ACP transport (optional) |

---

## Estimated Scope

~150 lines new code in `internal/wire`
~80 lines modified in `internal/runtime`
~100 lines modified in `internal/ui/shell`

Total: ~330 lines across 6-7 files