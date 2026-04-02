package acp

import (
	"fmt"

	"fimi-cli/internal/wire"
)

func (s *Session) sendApprovalRequest(req *wire.ApprovalRequest) error {
	if req == nil {
		return nil
	}

	s.mu.Lock()
	s.pendingApprovals[req.ID] = req
	s.mu.Unlock()

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ApprovalRequestUpdate{
			SessionUpdate: "approval_request",
			ApprovalID:    req.ID,
			ToolCallID:    req.ToolCallID,
			Action:        req.Action,
			Description:   req.Description,
		},
	})
}

// ResolveApproval applies an ACP client's approval decision to a pending request.
func (s *Session) ResolveApproval(id string, resp wire.ApprovalResponse) error {
	s.mu.Lock()
	req, ok := s.pendingApprovals[id]
	if ok {
		delete(s.pendingApprovals, id)
	}
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("approval not found: %s", id)
	}

	return req.Resolve(resp)
}

// ClearPendingApprovals rejects any approvals that are still tracked by the ACP
// session, typically after a prompt exits or is cancelled.
func (s *Session) ClearPendingApprovals() {
	s.mu.Lock()
	pending := make([]*wire.ApprovalRequest, 0, len(s.pendingApprovals))
	for id, req := range s.pendingApprovals {
		pending = append(pending, req)
		delete(s.pendingApprovals, id)
	}
	s.mu.Unlock()

	for _, req := range pending {
		_ = req.Resolve(wire.ApprovalReject)
	}
}
