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

func TestNilApprovalAlwaysApproves(t *testing.T) {
	var a *Approval
	err := a.Request(context.Background(), "bash", "rm -rf /")
	if err != nil {
		t.Fatalf("expected nil for nil approval, got %v", err)
	}
}
