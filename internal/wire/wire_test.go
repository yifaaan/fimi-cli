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
	var _ Message = msg
}

func TestApprovalRequestIsMessage(t *testing.T) {
	req := &ApprovalRequest{
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

func TestWithCurrentAndCurrent(t *testing.T) {
	w := New(0)
	ctx := WithCurrent(context.Background(), w)

	got, ok := Current(ctx)
	if !ok {
		t.Fatal("Current() ok = false, want true")
	}
	if got != w {
		t.Fatal("Current() returned wrong wire")
	}
}

func TestCurrentWithoutWire(t *testing.T) {
	got, ok := Current(context.Background())
	if ok {
		t.Fatal("Current() ok = true, want false")
	}
	if got != nil {
		t.Fatal("Current() wire != nil, want nil")
	}
}

func TestWireSendAndReceive(t *testing.T) {
	w := New(0)

	msg := EventMessage{Event: runtimeevents.StepBegin{Number: 1}}
	if err := w.Send(msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

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

	_, err := w.Receive(context.Background())
	if !errors.Is(err, ErrWireClosed) {
		t.Fatalf("Receive() error = %v, want ErrWireClosed", err)
	}
}

func TestWireSendReturnsErrorOnClosedWire(t *testing.T) {
	w := New(0)
	w.Shutdown()

	err := w.Send(EventMessage{Event: runtimeevents.StepBegin{Number: 1}})
	if !errors.Is(err, ErrWireClosed) {
		t.Fatalf("Send() error = %v, want ErrWireClosed", err)
	}
}

func TestWireDefaultBufferSize(t *testing.T) {
	w := New(0)

	for i := 0; i < 10; i++ {
		if err := w.Send(EventMessage{Event: runtimeevents.StepBegin{Number: i}}); err != nil {
			t.Fatalf("Send() error = %v", err)
		}
	}

	w.Shutdown()
}

func TestWireReceiveRespectsContext(t *testing.T) {
	w := New(0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := w.Receive(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive() error = %v, want context.Canceled", err)
	}
}

func TestApprovalRequestResolveWithoutWaiting(t *testing.T) {
	var req ApprovalRequest

	err := req.Resolve(ApprovalApprove)
	if !errors.Is(err, ErrApprovalRequestNotWaiting) {
		t.Fatalf("Resolve() error = %v, want ErrApprovalRequestNotWaiting", err)
	}
}

func TestApprovalRequestResolveTwice(t *testing.T) {
	req := &ApprovalRequest{responseCh: make(chan ApprovalResponse, 1), waiting: true}

	if err := req.Resolve(ApprovalApprove); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}

	err := req.Resolve(ApprovalReject)
	if !errors.Is(err, ErrApprovalRequestResolved) {
		t.Fatalf("second Resolve() error = %v, want ErrApprovalRequestResolved", err)
	}
}

func TestApprovalRequestWaitResolved(t *testing.T) {
	w := New(0)
	req := &ApprovalRequest{ID: "approval-1"}

	done := make(chan struct{})
	go func() {
		defer close(done)

		msg, err := w.Receive(context.Background())
		if err != nil {
			t.Errorf("Receive() error = %v", err)
			return
		}

		gotReq, ok := msg.(*ApprovalRequest)
		if !ok {
			t.Errorf("Receive() got %T, want *ApprovalRequest", msg)
			return
		}

		if err := gotReq.Resolve(ApprovalApprove); err != nil {
			t.Errorf("Resolve() error = %v", err)
		}
	}()

	if err := w.Send(req); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	resp, err := req.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if resp != ApprovalApprove {
		t.Fatalf("Wait() response = %q, want %q", resp, ApprovalApprove)
	}

	<-done
}

func TestApprovalRequestWaitReject(t *testing.T) {
	w := New(0)
	req := &ApprovalRequest{ID: "approval-2"}

	go func() {
		msg, err := w.Receive(context.Background())
		if err != nil {
			t.Errorf("Receive() error = %v", err)
			return
		}

		gotReq, ok := msg.(*ApprovalRequest)
		if !ok {
			t.Errorf("Receive() got %T, want *ApprovalRequest", msg)
			return
		}

		if err := gotReq.Resolve(ApprovalReject); err != nil {
			t.Errorf("Resolve() error = %v", err)
		}
	}()

	if err := w.Send(req); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	resp, err := req.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if resp != ApprovalReject {
		t.Fatalf("Wait() response = %q, want %q", resp, ApprovalReject)
	}
}

func TestApprovalRequestWaitContextCancel(t *testing.T) {
	w := New(0)
	req := &ApprovalRequest{ID: "approval-3"}

	if err := w.Send(req); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := req.Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
	if resp != ApprovalReject {
		t.Fatalf("Wait() response = %q, want %q", resp, ApprovalReject)
	}
}

func TestApprovalRequestWaitWireClosed(t *testing.T) {
	w := New(0)
	req := &ApprovalRequest{ID: "approval-4"}

	if err := w.Send(req); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		w.Shutdown()
	}()

	resp, err := req.Wait(context.Background())
	if !errors.Is(err, ErrWireClosed) {
		t.Fatalf("Wait() error = %v, want ErrWireClosed", err)
	}
	if resp != ApprovalReject {
		t.Fatalf("Wait() response = %q, want %q", resp, ApprovalReject)
	}
}
