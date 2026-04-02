package approval

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/wire"
)

// ErrRejected is returned when the user rejects an approval request.
var ErrRejected = errors.New("tool execution rejected by user")

type approvalKey struct{}
type toolCallIDKey struct{}

// Approval manages tool approval decisions.
type Approval struct {
	yolo        bool
	autoApprove map[string]bool
}

// New creates an Approval instance.
// If yolo is true, all approval requests are auto-approved.
func New(yolo bool) *Approval {
	return &Approval{
		yolo:        yolo,
		autoApprove: make(map[string]bool),
	}
}

// Request asks for user approval of an action.
// Returns nil on approval, ErrRejected on rejection.
// Skips the wire round-trip if yolo mode is on or the action is auto-approved.
func (a *Approval) Request(ctx context.Context, action, description string) error {
	if a == nil {
		return nil
	}
	if a.yolo {
		return nil
	}
	if a.autoApprove[action] {
		return nil
	}

	w, ok := wire.Current(ctx)
	if !ok {
		return nil
	}

	req := &wire.ApprovalRequest{
		ID:          fmt.Sprintf("%s:%s", action, description),
		ToolCallID:  ToolCallIDFromContext(ctx),
		Action:      action,
		Description: description,
	}

	resp, err := w.WaitForApproval(ctx, req)
	if err != nil {
		return ErrRejected
	}

	switch resp {
	case wire.ApprovalApprove:
		return nil
	case wire.ApprovalApproveForSession:
		a.autoApprove[action] = true
		return nil
	case wire.ApprovalReject:
		return ErrRejected
	default:
		return ErrRejected
	}
}

// WithContext stores the Approval in ctx for retrieval by tool handlers.
func WithContext(ctx context.Context, a *Approval) context.Context {
	return context.WithValue(ctx, approvalKey{}, a)
}

// FromContext retrieves the Approval from ctx.
// Returns nil if no Approval is set.
func FromContext(ctx context.Context) *Approval {
	a, _ := ctx.Value(approvalKey{}).(*Approval)
	return a
}

// WithToolCallID stores the active tool call ID in ctx so approval requests
// can be correlated with the originating tool invocation.
func WithToolCallID(ctx context.Context, toolCallID string) context.Context {
	return context.WithValue(ctx, toolCallIDKey{}, toolCallID)
}

// ToolCallIDFromContext retrieves the active tool call ID from ctx.
func ToolCallIDFromContext(ctx context.Context) string {
	toolCallID, _ := ctx.Value(toolCallIDKey{}).(string)
	return toolCallID
}
