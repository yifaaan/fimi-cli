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
