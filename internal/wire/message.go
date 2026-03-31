package wire

import (
	"context"
	errorspkg "errors"
	"sync"

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

// ToastMessage carries a toast notification through the wire.
// Unlike EventMessage, toasts travel on their own channel so they
// don't interleave with runtime step events.
type ToastMessage struct {
	Level   string // "info"|"warning"|"error"|"success"
	Message string
	Detail  string
	Action  string
}

func (ToastMessage) isMessage() {}

var ErrApprovalRequestNotWaiting = errorspkg.New("approval request is not waiting for a response")
var ErrApprovalRequestResolved = errorspkg.New("approval request already resolved")

// ApprovalRequest represents a request for user approval.
type ApprovalRequest struct {
	ID          string // unique request ID
	ToolCallID  string // tool call being approved
	Action      string // action type (e.g., "bash_execute")
	Description string // human-readable description

	mu         sync.Mutex
	responseCh chan ApprovalResponse
	wireDone   <-chan struct{}
	waiting    bool
	resolved   bool
}

func (*ApprovalRequest) isMessage() {}

func (req *ApprovalRequest) Wait(ctx context.Context) (ApprovalResponse, error) {
	if req == nil {
		return ApprovalReject, ErrApprovalRequestNotWaiting
	}

	req.mu.Lock()
	if req.responseCh == nil {
		req.responseCh = make(chan ApprovalResponse, 1)
	}
	req.waiting = true
	respCh := req.responseCh
	wireDone := req.wireDone
	req.mu.Unlock()

	select {
	case resp := <-respCh:
		return resp, nil
	case <-wireDone:
		return ApprovalReject, ErrWireClosed
	case <-ctx.Done():
		return ApprovalReject, ctx.Err()
	}
}

// Resolve completes the approval request with the user's response.
// Called by UI after user makes a decision.
func (req *ApprovalRequest) Resolve(resp ApprovalResponse) error {
	if req == nil {
		return ErrApprovalRequestNotWaiting
	}

	req.mu.Lock()
	defer req.mu.Unlock()

	if !req.waiting || req.responseCh == nil {
		return ErrApprovalRequestNotWaiting
	}
	if req.resolved {
		return ErrApprovalRequestResolved
	}

	req.resolved = true
	req.responseCh <- resp
	return nil
}

// ApprovalResponse is the user's response to an approval request.
type ApprovalResponse string

const (
	ApprovalApprove           ApprovalResponse = "approve"
	ApprovalApproveForSession ApprovalResponse = "approve_for_session"
	ApprovalReject            ApprovalResponse = "reject"
)
