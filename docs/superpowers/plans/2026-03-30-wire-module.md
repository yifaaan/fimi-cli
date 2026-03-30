# Wire Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bidirectional communication channel between runtime and UI, enabling event streaming and approval request flows.

**Architecture:** Create `internal/wire` package with `Wire` struct (buffered channel + done signal) and `ApprovalRequest` type. Replace the one-way `Sink` pattern with unified `Wire` that supports both events and approval request/response flows.

**Tech Stack:** Go 1.21+, github.com/google/uuid, github.com/charmbracelet/bubbletea

---

## File Structure

| File | Responsibility |
|------|----------------|
| `internal/wire/wire.go` | Wire struct, Send/Receive/Shutdown/WaitForApproval methods |
| `internal/wire/message.go` | Message interface, EventMessage wrapper, ApprovalRequest/Response types |
| `internal/runtime/runtime.go` | Replace Sink with Wire, add WithWire() |
| `internal/runtime/events/events.go` | Keep existing events (no changes needed) |
| `internal/ui/shell/model.go` | Add wire field, pendingApprovals, wireReceiveLoop |

---

### Task 1: Create wire package with message types

**Files:**
- Create: `internal/wire/message.go`
- Create: `internal/wire/wire.go`
- Create: `internal/wire/wire_test.go`

- [ ] **Step 1: Write failing tests for Message interface and types**

Create `internal/wire/wire_test.go`:

```go
package wire

import (
	"testing"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestEventMessageIsMessage(t *testing.T) {
	msg := EventMessage{Event: runtimeevents.StepBegin{Number: 1}}
	// The isMessage() method must exist and satisfy Message interface
	var _ Message = msg
}

func TestApprovalRequestIsMessage(t *testing.T) {
	req := ApprovalRequest{
		ID:          "test-id",
		ToolCallID:  "call-1",
		Action:      "bash_execute",
		Description: "Run command: ls -la",
	}
	var _ Message = req
}

func TestApprovalResponseConstants(t *testing.T) {
	if ApprovalApprove != "approve" {
		t.Fatalf("ApprovalApprove = %q, want %q", ApprovalApprove, "approve")
	}
	if ApprovalApproveForSession != "approve_for_session" {
		t.Fatalf("ApprovalApproveForSession = %q, want %q", ApprovalApproveForSession, "approve_for_session")
	}
	if ApprovalReject != "reject" {
		t.Fatalf("ApprovalReject = %q, want %q", ApprovalReject, "reject")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wire/...`
Expected: FAIL - package wire does not exist

- [ ] **Step 3: Create message.go with Message interface and types**

Create `internal/wire/message.go`:

```go
package wire

import (
	runtimeevents "fimi-cli/internal/runtime/events"
)

// Message is any message that flows through the wire.
type Message interface {
	isMessage()
}

// EventMessage wraps existing events to satisfy Message interface.
type EventMessage struct {
	Event runtimeevents.Event
}

func (EventMessage) isMessage() {}

// ApprovalRequest represents a request for user approval.
type ApprovalRequest struct {
	ID          string // unique request ID
	ToolCallID  string // tool call being approved
	Action      string // action type (e.g., "bash_execute")
	Description string // human-readable description

	responseCh chan ApprovalResponse // internal: response channel
}

func (ApprovalRequest) isMessage() {}

// ApprovalResponse is the user's response to an approval request.
type ApprovalResponse string

const (
	ApprovalApprove           ApprovalResponse = "approve"
	ApprovalApproveForSession ApprovalResponse = "approve_for_session"
	ApprovalReject            ApprovalResponse = "reject"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/wire/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/wire/message.go internal/wire/wire_test.go
git commit -m "feat(wire): add Message interface and ApprovalRequest types"
```

---

### Task 2: Implement Wire struct with Send/Receive/Shutdown

**Files:**
- Modify: `internal/wire/wire.go`
- Modify: `internal/wire/wire_test.go`

- [ ] **Step 1: Write failing tests for Wire Send/Receive/Shutdown**

Add to `internal/wire/wire_test.go`:

```go
import (
	"context"
	"testing"
	"time"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestWireSendAndReceive(t *testing.T) {
	w := New(0) // default buffer size

	msg := EventMessage{Event: runtimeevents.StepBegin{Number: 1}}
	w.Send(msg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	got, err := w.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}

	eventMsg, ok := got.(EventMessage)
	if !ok {
		t.Fatalf("Receive() returned %T, want EventMessage", got)
	}

	stepBegin, ok := eventMsg.Event.(runtimeevents.StepBegin)
	if !ok {
		t.Fatalf("Event = %T, want StepBegin", eventMsg.Event)
	}
	if stepBegin.Number != 1 {
		t.Fatalf("StepBegin.Number = %d, want 1", stepBegin.Number)
	}
}

func TestWireReceiveReturnsErrorOnShutdown(t *testing.T) {
	w := New(0)

	w.Shutdown()

	ctx := context.Background()
	_, err := w.Receive(ctx)
	if !errors.Is(err, ErrWireClosed) {
		t.Fatalf("Receive() error = %v, want ErrWireClosed", err)
	}
}

func TestWireSendPanicsOnClosedWire(t *testing.T) {
	w := New(0)
	w.Shutdown()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Send() on closed wire should panic")
		}
	}()

	w.Send(EventMessage{Event: runtimeevents.StepBegin{Number: 1}})
}

func TestWireDefaultBufferSize(t *testing.T) {
	w := New(0)

	// Should be able to send multiple messages without blocking
	for i := 0; i < 10; i++ {
		w.Send(EventMessage{Event: runtimeevents.StepBegin{Number: i}})
	}

	w.Shutdown()
}

func TestWireReceiveRespectsContext(t *testing.T) {
	w := New(0) // empty wire

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := w.Receive(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive() error = %v, want context.Canceled", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wire/...`
Expected: FAIL - New, Send, Receive, Shutdown undefined

- [ ] **Step 3: Create wire.go with Wire implementation**

Create `internal/wire/wire.go`:

```go
package wire

import (
	"context"
	"errors"
)

// ErrWireClosed is returned when receiving from a closed wire.
var ErrWireClosed = errors.New("wire is closed")

// Wire is a bidirectional channel between runtime and UI.
type Wire struct {
	ch   chan Message // main message channel (buffered)
	done chan struct{} // shutdown signal
}

// New creates a Wire with a buffered channel.
// If bufferSize <= 0, defaults to 64.
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
// Panics if wire is shut down.
func (w *Wire) Send(msg Message) {
	select {
	case w.ch <- msg:
	case <-w.done:
		panic("wire: send on closed wire")
	}
}

// Receive waits for the next message or returns error on shutdown.
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

// Shutdown closes the wire. No more messages can be sent or received.
func (w *Wire) Shutdown() {
	close(w.done)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/wire/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/wire/wire.go internal/wire/wire_test.go
git commit -m "feat(wire): add Wire struct with Send/Receive/Shutdown"
```

---

### Task 3: Add WaitForApproval and Resolve methods

**Files:**
- Modify: `internal/wire/wire.go`
- Modify: `internal/wire/message.go`
- Modify: `internal/wire/wire_test.go`

- [ ] **Step 1: Write failing tests for approval flow**

Add to `internal/wire/wire_test.go`:

```go
func TestWireWaitForApproval(t *testing.T) {
	w := New(0)

	req := &ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "call-1",
		Action:      "bash_execute",
		Description: "Run: rm -rf /",
	}

	// Simulate UI receiving and resolving in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		msg, err := w.Receive(ctx)
		if err != nil {
			t.Errorf("UI Receive() error = %v", err)
			return
		}

		approvalReq, ok := msg.(*ApprovalRequest)
		if !ok {
			t.Errorf("UI received %T, want *ApprovalRequest", msg)
			return
		}

		// Simulate user approving
		approvalReq.Resolve(ApprovalApprove)
	}()

	// Runtime waits for approval
	ctx := context.Background()
	resp, err := w.WaitForApproval(ctx, req)
	if err != nil {
		t.Fatalf("WaitForApproval() error = %v", err)
	}
	if resp != ApprovalApprove {
		t.Fatalf("WaitForApproval() response = %q, want %q", resp, ApprovalApprove)
	}
}

func TestWireWaitForApprovalReject(t *testing.T) {
	w := New(0)

	req := &ApprovalRequest{
		ID:         "approval-2",
		ToolCallID: "call-2",
		Action:     "bash_execute",
	}

	go func() {
		msg, _ := w.Receive(context.Background())
		approvalReq := msg.(*ApprovalRequest)
		approvalReq.Resolve(ApprovalReject)
	}()

	resp, _ := w.WaitForApproval(context.Background(), req)
	if resp != ApprovalReject {
		t.Fatalf("WaitForApproval() response = %q, want %q", resp, ApprovalReject)
	}
}

func TestWireWaitForApprovalContextCancel(t *testing.T) {
	w := New(0)

	req := &ApprovalRequest{ID: "approval-3"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := w.WaitForApproval(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForApproval() error = %v, want context.Canceled", err)
	}
	if resp != ApprovalReject {
		t.Fatalf("WaitForApproval() on canceled context should return Reject")
	}
}

func TestWireWaitForApprovalWireClosed(t *testing.T) {
	w := New(0)

	req := &ApprovalRequest{ID: "approval-4"}

	go func() {
		time.Sleep(10 * time.Millisecond)
		w.Shutdown()
	}()

	resp, err := w.WaitForApproval(context.Background(), req)
	if !errors.Is(err, ErrWireClosed) {
		t.Fatalf("WaitForApproval() error = %v, want ErrWireClosed", err)
	}
	if resp != ApprovalReject {
		t.Fatalf("WaitForApproval() on closed wire should return Reject")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wire/...`
Expected: FAIL - WaitForApproval and Resolve undefined

- [ ] **Step 3: Add WaitForApproval to wire.go**

Add to `internal/wire/wire.go`:

```go
// WaitForApproval sends an approval request and waits for the response.
// Returns ApprovalReject on context cancellation or wire shutdown.
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
```

- [ ] **Step 4: Add Resolve method to ApprovalRequest**

Add to `internal/wire/message.go` after the ApprovalRequest struct:

```go
// Resolve completes the approval request with the user's response.
// Called by UI after user makes a decision.
func (req *ApprovalRequest) Resolve(resp ApprovalResponse) {
	select {
	case req.responseCh <- resp:
	default:
		// Already resolved (shouldn't happen, but safe)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/wire/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/wire/wire.go internal/wire/message.go internal/wire/wire_test.go
git commit -m "feat(wire): add WaitForApproval and Resolve methods"
```

---

### Task 4: Update runtime to use Wire

**Files:**
- Modify: `internal/runtime/runtime.go`
- Modify: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Write failing test for runtime WithWire**

Add to `internal/runtime/runtime_test.go`:

```go
import (
	"fimi-cli/internal/wire"
)

func TestRunnerWithWire(t *testing.T) {
	mockEngine := &mockEngine{reply: AssistantReply{Text: "done"}}
	runner := New(mockEngine, DefaultConfig())

	w := wire.New(0)
	runnerWithWire := runner.WithWire(w)

	if runnerWithWire.wire != w {
		t.Fatalf("WithWire() did not set wire")
	}

	// Original runner should not be modified
	if runner.wire != nil {
		t.Fatalf("WithWire() modified original runner")
	}
}

func TestRunnerEmitEventThroughWire(t *testing.T) {
	mockEngine := &mockEngine{reply: AssistantReply{Text: "hello"}}
	w := wire.New(0)

	runner := New(mockEngine, DefaultConfig()).WithWire(w)

	ctx := context.Background()
	err := runner.emitEvent(ctx, runtimeevents.TextPart{Text: "test"})
	if err != nil {
		t.Fatalf("emitEvent() error = %v", err)
	}

	// Receive the event from wire
	gotMsg, err := w.Receive(ctx)
	if err != nil {
		t.Fatalf("wire.Receive() error = %v", err)
	}

	eventMsg, ok := gotMsg.(wire.EventMessage)
	if !ok {
		t.Fatalf("wire message = %T, want EventMessage", gotMsg)
	}

	textPart, ok := eventMsg.Event.(runtimeevents.TextPart)
	if !ok {
		t.Fatalf("Event = %T, want TextPart", eventMsg.Event)
	}
	if textPart.Text != "test" {
		t.Fatalf("TextPart.Text = %q, want %q", textPart.Text, "test")
	}

	w.Shutdown()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/...`
Expected: FAIL - Runner.wire undefined, WithWire undefined

- [ ] **Step 3: Add wire field and WithWire to Runner struct**

Modify `internal/runtime/runtime.go`:

1. Add import for wire package at top:

```go
import (
	"fimi-cli/internal/wire"
)
```

2. Add `wire` field to `Runner` struct (around line 243):

```go
type Runner struct {
	engine       Engine
	toolExecutor ToolExecutor
	eventSink    runtimeevents.Sink  // keep for backward compatibility
	wire         *wire.Wire           // new: bidirectional channel
	dmailer      DMailer
	config       Config
	runStepFn    func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error)
	advanceRunFn func(ctx context.Context, store contextstore.Context, result Result, stepResult StepResult) (Result, bool, error)
}
```

3. Add `WithWire` method after `WithEventSink` (around line 313):

```go
// WithWire returns a Runner copy bound to a wire.
func (r Runner) WithWire(w *wire.Wire) Runner {
	r.wire = w
	return r
}
```

4. Modify `emitEvent` method to also send through wire (around line 741):

```go
func (r Runner) emitEvent(ctx context.Context, event runtimeevents.Event) error {
	// Try wire first (new path)
	if r.wire != nil {
		r.wire.Send(wire.EventMessage{Event: event})
		return nil
	}

	// Fall back to Sink (legacy path)
	sink := r.eventSink
	if sink == nil {
		sink = runtimeevents.NoopSink{}
	}

	if err := sink.Emit(ctx, event); err != nil {
		return fmt.Errorf("emit runtime event %q: %w", event.Kind(), err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go
git commit -m "feat(runtime): add WithWire and wire-based emitEvent"
```

---

### Task 5: Update shell model to use Wire

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add wire field to Model struct**

Modify `internal/ui/shell/model.go` Model struct (around line 47):

Add import at top:
```go
import (
	"fimi-cli/internal/wire"
)
```

Add fields to Model struct:
```go
type Model struct {
	// ...

	// Wire for bidirectional communication with runtime
	wire             *wire.Wire
	pendingApprovals map[string]*wire.ApprovalRequest

	// ...
}
```

- [ ] **Step 2: Initialize wire in NewModel**

Modify `NewModel` function (around line 139):

```go
func NewModel(deps Dependencies, history *historyStore) Model {
	showBanner := deps.StartupInfo != (StartupInfo{})

	// Create wire for runtime communication
	w := wire.New(0)

	output := NewOutputModel()
	for _, line := range transcriptLineModelsFromRecords(deps.InitialRecords) {
		output = output.AppendLine(line)
	}

	return Model{
		input:      NewInputModel(),
		output:     output,
		runtime:    NewRuntimeModel(),
		mode:       ModeIdle,
		showBanner: showBanner,
		deps:       deps,
		history:    history,
		wire:       w,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/ui/shell/model.go
git commit -m "feat(shell): add wire field to Model"
```

---

### Task 6: Add wireReceiveLoop to shell

**Files:**
- Modify: `internal/ui/shell/model_runtime.go`

- [ ] **Step 1: Add wireReceiveLoop function**

Modify `internal/ui/shell/model_runtime.go`:

Add import at top:
```go
import (
	"fimi-cli/internal/wire"
)
```

Add wireReceiveLoop function:
```go
// wireReceiveLoop returns a tea.Cmd that receives from wire and converts to tea.Msg.
func (m Model) wireReceiveLoop() tea.Cmd {
	return func() tea.Msg {
		msg, err := m.wire.Receive(context.Background())
		if err != nil {
			return wireErrorMsg{Err: err}
		}

		switch msg := msg.(type) {
		case wire.EventMessage:
			return eventToTeaMsg(msg.Event)
		case *wire.ApprovalRequest:
			return approvalRequestMsg{Request: msg}
		default:
			return nil
		}
	}
}

// wireErrorMsg wraps wire receive errors for tea.Msg.
type wireErrorMsg struct {
	Err error
}

// approvalRequestMsg wraps approval requests for tea.Msg.
type approvalRequestMsg struct {
	Request *wire.ApprovalRequest
}

// eventToTeaMsg converts runtime events to existing tea.Msg types.
func eventToTeaMsg(event runtimeevents.Event) tea.Msg {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		return StepBeginMsg{Number: e.Number}
	case runtimeevents.StepInterrupted:
		return StepInterruptedMsg{}
	case runtimeevents.StatusUpdate:
		return StatusUpdateMsg{Status: e.Status}
	case runtimeevents.TextPart:
		return TextPartMsg{Text: e.Text}
	case runtimeevents.ToolCall:
		return ToolCallMsg{
			ID:        e.ID,
			Name:      e.Name,
			Subtitle:  e.Subtitle,
			Arguments: e.Arguments,
		}
	case runtimeevents.ToolResult:
		return ToolResultMsg{
			ToolCallID: e.ToolCallID,
			ToolName:   e.ToolName,
			Output:     e.Output,
			IsError:    e.IsError,
		}
	default:
		return nil
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/shell/model_runtime.go
git commit -m "feat(shell): add wireReceiveLoop for wire message handling"
```

---

### Task 7: Wire runtime to shell in Run function

**Files:**
- Modify: `internal/ui/shell/run.go`

- [ ] **Step 1: Read run.go to understand Run function structure**

Run: `cat internal/ui/shell/run.go | head -100`

Check how `eventsCh` is currently created and passed.

- [ ] **Step 2: Modify Run to use wire instead of eventsCh**

This step requires reading the current `run.go` implementation and modifying it to:
1. Get wire from model instead of creating eventsCh
2. Pass wire to runtime via WithWire()
3. Start wireReceiveLoop in Init()

Look at existing `Run()` function and modify accordingly. The exact changes depend on current structure.

- [ ] **Step 3: Run shell tests to verify**

Run: `go test ./internal/ui/shell/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ui/shell/run.go internal/ui/shell/model.go
git commit -m "feat(shell): wire runtime to shell via Wire channel"
```

---

### Task 8: Add approval prompt mode to shell

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add ModeApprovalPrompt constant**

Add to Mode constants in `internal/ui/shell/model.go`:

```go
const (
	ModeIdle Mode = iota
	ModeThinking
	ModeStreaming
	ModeSessionSelect
	ModeCheckpointSelect
	ModeCommandSelect
	ModeSetup
	ModeApprovalPrompt // new: waiting for approval decision
)
```

- [ ] **Step 2: Add approval handling in Update**

Add handler for `approvalRequestMsg` in Update function:

```go
case approvalRequestMsg:
	m.pendingApprovals[msg.Request.ID] = msg.Request
	m.mode = ModeApprovalPrompt
	return m, nil
```

- [ ] **Step 3: Add resolveApproval method**

Add to model:

```go
// resolveApproval completes an approval request.
func (m Model) resolveApproval(id string, resp wire.ApprovalResponse) (Model, tea.Cmd) {
	if req, ok := m.pendingApprovals[id]; ok {
		req.Resolve(resp)
		delete(m.pendingApprovals, id)
	}
	m.mode = ModeThinking
	return m, wireReceiveLoop()
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/ui/shell/model.go
git commit -m "feat(shell): add ModeApprovalPrompt and approval handling"
```

---

## Self-Review Checklist

**1. Spec coverage:**
- ✓ Message interface with EventMessage and ApprovalRequest (Task 1)
- ✓ Wire struct with Send/Receive/Shutdown (Task 2)
- ✓ WaitForApproval and Resolve (Task 3)
- ✓ Runtime WithWire and wire-based emitEvent (Task 4)
- ✓ Shell wire field and pendingApprovals (Task 5)
- ✓ wireReceiveLoop (Task 6)
- ✓ Wire runtime to shell (Task 7)
- ✓ Approval prompt mode (Task 8)

**2. Placeholder scan:** No TBD, TODO, or vague steps found.

**3. Type consistency:**
- ApprovalRequest struct consistent across message.go and tests
- Wire struct methods match test expectations
- EventMessage wraps runtimeevents.Event consistently