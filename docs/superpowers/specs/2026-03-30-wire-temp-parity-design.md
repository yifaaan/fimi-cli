# Wire Temp-Parity Redesign — Design Spec

## Overview

Refactor the Go runtime/shell transport to match the temp kimi-cli wire model much more closely.

The target shape is:

- one real-time transport: `wire`
- one real-time message model: `EventMessage | ApprovalRequest`
- one shell receive loop: `wire.Receive()`
- approval lifecycle owned by `ApprovalRequest`
- current wire propagated through `context.Context`
- no long-term `eventSink` path
- no long-term shell `eventsCh` path

This is a deliberate structural redesign, not an incremental compatibility layer.

---

## Why Change

Today the Go implementation splits runtime communication across three mechanisms:

1. `runtimeevents.Sink` for streaming engine output
2. `wire` for part of the runtime → shell message flow
3. shell-local `eventsCh` plus `waitForRuntimeEvents()` for batching and completion handling

That makes the runtime lifecycle harder to reason about than the temp reference.

In temp (`temp/src/kimi_cli/soul/wire.py`):

- the wire is the single real-time channel
- the UI keeps receiving from that channel
- approval requests are just another wire message
- the current wire is available throughout the agent loop

The Go redesign should restore the same core model.

---

## Comparison: temp vs current Go

### temp kimi-cli

`temp/src/kimi_cli/soul/wire.py` defines:

- `WireMessage = Event | ApprovalRequest`
- `Wire.send(msg)`
- `await Wire.receive()`
- `ApprovalRequest.wait()`
- `ApprovalRequest.resolve()`
- `current_wire = ContextVar(...)`

This gives the agent loop one unified transport and one unified approval model.

### current Go

The current Go implementation already has pieces of that model:

- `internal/wire/message.go` defines `EventMessage` and `ApprovalRequest`
- `internal/wire/wire.go` defines `Send`, `Receive`, `Shutdown`
- shell can receive wire messages and surface approval prompts

But it still keeps parallel mechanisms alive:

- `internal/runtime/runtime.go` still streams via `runtimeevents.Sink`
- `internal/ui/shell/model.go` still creates and closes `eventsCh`
- `internal/ui/shell/messages.go` still batches events from `eventsCh`

So the Go version is only partially converged on the temp design.

---

## Goals

1. Make `wire` the only runtime/shell real-time transport.
2. Make `ApprovalRequest` own its wait/resolve lifecycle.
3. Make the current wire available through `context.Context`.
4. Remove `runtimeevents.Sink` from the runtime/engine boundary.
5. Remove shell `eventsCh` and `waitForRuntimeEvents()`.
6. Keep the design explicit and idiomatic in Go.

## Non-Goals

1. Preserve long-term backward compatibility for `eventSink`.
2. Keep dual-path transport in place during steady state.
3. Add new user-visible shell features as part of the transport redesign.
4. Redesign the transcript model beyond what is necessary for the new transport.

---

## Target Architecture

### Real-time message model

The real-time transport should carry only two message kinds:

- `wire.EventMessage{Event runtimeevents.Event}`
- `*wire.ApprovalRequest`

That is the Go equivalent of temp's `WireMessage = Event | ApprovalRequest`.

### Current wire propagation

Instead of Python `ContextVar`, Go should use `context.Context` helpers:

- `wire.WithCurrent(ctx, w)`
- `wire.Current(ctx) (*Wire, bool)`

This keeps wire access explicit, scoped to a single run, and safe under concurrency.

### Approval lifecycle

Approval ownership moves onto the request object itself:

- producer creates `ApprovalRequest`
- producer sends it through current wire
- producer calls `req.Wait(ctx)`
- shell receives the request and later calls `req.Resolve(resp)`

This mirrors temp more closely than `Wire.WaitForApproval(...)`.

### Shell event consumption

The shell should have exactly one real-time input path:

- `wire.Receive()` returns `EventMessage` or `ApprovalRequest`
- shell updates runtime UI state from `EventMessage`
- shell enters approval mode from `ApprovalRequest`
- closed wire becomes the normal end-of-stream signal

There should be no parallel `eventsCh` receive loop.

---

## Layer Responsibilities

### `internal/wire`

Responsibilities:

- transport messages with `Send`, `Receive`, `Shutdown`
- carry `ApprovalRequest`
- expose `WithCurrent` / `Current` helpers
- implement request-local `Wait(ctx)` and `Resolve(resp)`

It should not know about shell view state or runtime step semantics.

### `internal/runtime`

Responsibilities:

- act as the sole producer of real-time runtime events
- read the current wire from `ctx`
- translate streaming output into `runtimeevents.Event`
- send approval requests through wire and wait for responses
- define run lifecycle relative to wire usage

It should not maintain both a wire path and a sink path in steady state.

### engine / streaming boundary

The engine should stop depending on `runtimeevents.Sink`.

Instead, the streaming API should expose streamed parts through a smaller boundary that runtime can consume directly. Two acceptable shapes are:

1. callback-based handler
2. streamed part channel / iterator

Recommended shape: callback-based handler.

Why:

- simpler control flow
- fewer goroutines and channels
- closer to temp's "receive part, immediately send message" behavior

The runtime then converts those parts into `runtimeevents.Event` and sends them through the current wire.

### `internal/ui/shell`

Responsibilities:

- create a wire for a run
- put that wire into the runtime execution context
- repeatedly receive wire messages
- update UI state or approval state based on message type
- treat closed wire as normal completion plumbing

It should not own a parallel event batching channel.

---

## API Changes

### `internal/wire`

Add:

```go
type contextKey struct{}

func WithCurrent(ctx context.Context, w *Wire) context.Context
func Current(ctx context.Context) (*Wire, bool)
```

Add to `ApprovalRequest`:

```go
func (req *ApprovalRequest) Wait(ctx context.Context) (ApprovalResponse, error)
func (req *ApprovalRequest) Resolve(resp ApprovalResponse) error
```

Deprecate and then remove:

```go
func (w *Wire) WaitForApproval(ctx context.Context, req *ApprovalRequest) (ApprovalResponse, error)
```

### `internal/runtime`

Remove:

- `EventSinkCapableRunner`
- `WithEventSink(...)`
- `eventSink` field on `Runner`
- sink-based streaming path

Retain or adapt:

- `WithWire(...)` only if it remains useful for tests or explicit runner wiring

Long-term preferred runtime contract:

- runtime finds current wire from `ctx`
- runtime emits events only through wire

### engine streaming API

Replace the sink-shaped API with a streaming part handler.

Conceptually:

```go
type StreamHandler interface {
    OnTextPart(text string) error
    OnToolCallPart(callID string, delta string) error
    OnToolCall(call ToolCall) error
    OnStatusUpdate(status StatusSnapshot) error
}
```

The exact method set can be trimmed to whatever the engine truly emits today.

Important rule: this interface should describe streamed model output, not shell transport.

---

## Runtime Lifecycle

### Start

1. shell creates wire
2. shell creates run context with `wire.WithCurrent(ctx, w)`
3. shell starts runtime with that context
4. shell starts the receive loop for that same wire

### During run

- runtime emits all real-time events through current wire
- approval requests are sent through the same wire
- shell continuously consumes from the same wire

### Completion

The system must avoid dropping tail events.

Ownership rule:

- shell owns wire creation
- shell owns final wire shutdown after runtime work is complete and no further messages should be produced

Reason:

- avoids split ownership
- keeps transport lifetime tied to UI consumption
- makes end-of-stream handling explicit in one place

Completion rule:

- runtime must finish producing messages before shell shuts down the wire
- shell should treat `ErrWireClosed` as a normal completion signal, not a UI error

---

## Approval Flow

### New shape

1. runtime/tool execution detects approval is needed
2. runtime builds `ApprovalRequest`
3. runtime sends request through current wire
4. runtime calls `req.Wait(ctx)`
5. shell receives request and shows approval UI
6. shell resolves request
7. runtime resumes or rejects tool execution based on the response

### Error handling rules

- `Wait(ctx)` must return on context cancellation
- duplicate `Resolve(...)` must return an explicit error
- `Resolve(...)` without an active waiter must return an explicit error
- wire shutdown while waiting must unblock the waiter

This keeps the approval lifecycle explicit and testable.

---

## Migration Plan

### Phase 1: finish `internal/wire`

- add `WithCurrent` / `Current`
- add `ApprovalRequest.Wait(ctx)`
- keep current `Resolve` error semantics
- temporarily keep `Wire.WaitForApproval(...)` only as a compatibility shim if needed during migration

### Phase 2: switch runtime to current wire from context

- update runtime event emission to use `wire.Current(ctx)`
- stop treating sink as a parallel transport path
- move approval waiting to `ApprovalRequest.Wait(ctx)`

### Phase 3: replace engine streaming boundary

- remove `runtimeevents.Sink` from `ReplyStream(...)`
- let runtime consume streamed parts directly
- translate streamed parts to `runtimeevents.Event`
- send those events through wire immediately

### Phase 4: remove shell `eventsCh`

- delete `eventsCh` setup and close logic
- delete `waitForRuntimeEvents()` path
- complete runs based on runtime completion + wire closure instead of local channel drain

### Phase 5: remove obsolete APIs

- delete `WithEventSink(...)`
- delete `EventSinkCapableRunner`
- delete `runtimeevents.Sink` plumbing that only existed for shell streaming
- remove `Wire.WaitForApproval(...)` if still present as a transition helper

The intended end state is single-path, not permanent dual-path compatibility.

---

## Risks and Mitigations

### Risk: dropped tail events

If runtime completes before all final streamed messages are observed, the transcript can lose its last text/tool updates.

Mitigation:

- define a single wire ownership rule
- ensure runtime finishes producing before shell shutdown
- treat wire closure as the only end-of-stream signal for shell real-time consumption

### Risk: approval wait hangs

Mitigation:

- `ApprovalRequest.Wait(ctx)` must respect context cancellation
- shutdown must unblock waiting requests
- duplicate and invalid resolve calls must surface explicit errors

### Risk: interface churn in streaming engine

Mitigation:

- migrate runtime + engine + tests together
- avoid keeping the old sink interface alive for long
- prefer one clean break over extended dual-mode behavior

### Risk: shell receive loop stalls

Mitigation:

- every handled wire message must re-arm the next `wire.Receive()` command
- closed wire should be handled as an expected control-flow event

---

## Testing Strategy

### `internal/wire`

Cover:

- send / receive / shutdown lifecycle
- current wire context helpers
- approval wait / resolve success path
- duplicate resolve
- resolve without waiter
- context cancellation while waiting
- shutdown while waiting

### `internal/runtime`

Cover:

- event emission through current wire
- streaming part to event translation
- approval request send + wait + continue
- cancellation/interruption with waiting approval
- no hidden fallback to removed sink path

### `internal/ui/shell`

Cover:

- wire-only event rendering
- approval prompt from wire request
- approval resolution resumes execution
- run completion after wire closes
- closed wire does not surface as a user-visible error

### end-to-end

Cover:

- plain streamed text response
- streamed tool call + tool result response
- approval-gated tool execution
- interrupted run during streaming
- final streamed output is not lost at shutdown

---

## Decisions

1. Recreate temp's single-message transport model in Go.
2. Use `context.Context` instead of a global/contextvar-style current wire.
3. Make `ApprovalRequest` own waiting and resolving.
4. Remove `eventSink` rather than preserving long-term transport duality.
5. Remove shell `eventsCh` rather than keeping a local completion side channel.
6. Keep engine boundaries transport-agnostic: engine emits streamed parts, runtime writes wire messages.

---

## Open Questions Resolved

### Should engine write to wire directly?
No.

Even though temp looks flatter, the better Go mapping is:

- engine emits stream parts
- runtime converts parts to runtime events
- runtime sends those events to wire

That keeps the engine free of shell transport knowledge while preserving temp's unified runtime/UI message flow.

### Should Go emulate Python `ContextVar` exactly?
No.

Use `context.Context` helpers instead. This preserves per-run scoping without introducing package-global mutable state.
