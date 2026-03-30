package wire

import (
	"context"
	"errors"
	"testing"
	"time"

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
