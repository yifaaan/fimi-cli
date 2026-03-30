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
