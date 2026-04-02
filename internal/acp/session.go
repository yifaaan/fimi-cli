package acp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	runtimeevents "fimi-cli/internal/runtime/events"
	sessionpkg "fimi-cli/internal/session"
	"fimi-cli/internal/wire"
)

// Session wraps the ACP-facing state for a single runtime session.
type Session struct {
	session sessionpkg.Session
	conn    *FramedConn
	mu      sync.Mutex
	modelID string

	// cancelFn cancels the currently-running prompt, if any.
	cancelFn context.CancelFunc

	pendingApprovals map[string]*wire.ApprovalRequest
	startedToolCalls map[string]bool
}

// NewSession creates a new ACP session wrapper.
func NewSession(sess sessionpkg.Session, conn *FramedConn, modelID string) *Session {
	return &Session{
		session:          sess,
		conn:             conn,
		modelID:          modelID,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
		startedToolCalls: make(map[string]bool),
	}
}

// HistoryFile returns the backing session history path.
func (s *Session) HistoryFile() string {
	return s.session.HistoryFile
}

// CurrentModelID returns the session's current model selection.
func (s *Session) CurrentModelID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelID
}

// SetModelID updates the session's current model selection.
func (s *Session) SetModelID(modelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelID = modelID
}

// SetCancel stores the cancel func for the currently-running prompt.
func (s *Session) SetCancel(fn context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelFn = fn
}

// Cancel cancels the currently-running prompt, if any.
func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
}

// VisualizeWire converts wire messages into ACP session/update notifications.
func (s *Session) VisualizeWire() func(ctx context.Context, messages <-chan wire.Message) error {
	return func(ctx context.Context, messages <-chan wire.Message) error {
		for msg := range messages {
			if err := s.translateAndSendMessage(msg); err != nil {
				return err
			}
		}
		return nil
	}
}

func (s *Session) translateAndSendMessage(msg wire.Message) error {
	switch m := msg.(type) {
	case wire.EventMessage:
		return s.translateAndSendEvent(m.Event)
	case *wire.ApprovalRequest:
		return s.sendApprovalRequest(m)
	default:
		return nil
	}
}

func (s *Session) translateAndSendEvent(event runtimeevents.Event) error {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		return s.sendAgentMessageChunk(e.Text)
	case runtimeevents.ToolCallPart:
		return s.sendToolCallPart(e)
	case runtimeevents.ToolCall:
		return s.sendToolCallStart(e)
	case runtimeevents.ToolResult:
		return s.sendToolCallProgress(e)
	default:
		// StepBegin, StepInterrupted, and StatusUpdate are not projected yet.
		return nil
	}
}

func (s *Session) sendAgentMessageChunk(text string) error {
	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: AgentMessageChunk{
			SessionUpdate: "agent_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: text},
		},
	})
}

func (s *Session) sendToolCallPart(part runtimeevents.ToolCallPart) error {
	if strings.TrimSpace(part.Delta) == "" {
		return nil
	}

	s.mu.Lock()
	firstChunk := !s.startedToolCalls[part.ToolCallID]
	s.startedToolCalls[part.ToolCallID] = true
	s.mu.Unlock()

	if firstChunk {
		return s.conn.SendNotification("session/update", SessionUpdateNotification{
			SessionID: s.session.ID,
			Update: ToolCallStart{
				SessionUpdate: "tool_call_start",
				ToolCallID:    part.ToolCallID,
				Title:         "Tool call",
				Status:        "in_progress",
				Content:       buildACPContentItems([]runtimeevents.RichContent{{Type: "text", Text: part.Delta}}, ""),
			},
		})
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    part.ToolCallID,
			Status:        "in_progress",
			Content:       buildACPContentItems([]runtimeevents.RichContent{{Type: "text", Text: part.Delta}}, ""),
		},
	})
}

func (s *Session) sendToolCallStart(tc runtimeevents.ToolCall) error {
	title := tc.Name
	if tc.Subtitle != "" {
		title = fmt.Sprintf("%s(%s)", tc.Name, tc.Subtitle)
	}

	s.mu.Lock()
	started := s.startedToolCalls[tc.ID]
	s.startedToolCalls[tc.ID] = true
	s.mu.Unlock()

	if started {
		return s.conn.SendNotification("session/update", SessionUpdateNotification{
			SessionID: s.session.ID,
			Update: ToolCallProgress{
				SessionUpdate: "tool_call_progress",
				ToolCallID:    tc.ID,
				Title:         title,
				Status:        "in_progress",
			},
		})
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallStart{
			SessionUpdate: "tool_call_start",
			ToolCallID:    tc.ID,
			Title:         title,
			Status:        "in_progress",
			Content:       buildACPContentItems(nil, tc.Arguments),
		},
	})
}

func (s *Session) sendToolCallProgress(tr runtimeevents.ToolResult) error {
	status := "completed"
	if tr.IsError {
		status = "failed"
	}

	s.mu.Lock()
	delete(s.startedToolCalls, tr.ToolCallID)
	s.mu.Unlock()

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    tr.ToolCallID,
			Title:         tr.ToolName,
			Status:        status,
			Content:       buildACPContentItems(tr.Content, firstNonEmptyToolOutput(tr.DisplayOutput, tr.Output)),
		},
	})
}

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

func buildACPContentItems(content []runtimeevents.RichContent, fallbackText string) []ToolCallContentItem {
	if len(content) == 0 {
		return buildTextContentItems(fallbackText)
	}

	items := make([]ToolCallContentItem, 0, len(content)+1)
	if fallbackText != "" && hasNonTextContent(content) {
		items = append(items, ToolCallContentItem{
			Type:    "text",
			Content: ContentBlock{Type: "text", Text: fallbackText},
		})
	}

	for _, item := range content {
		block := ContentBlock{Type: item.Type, Text: item.Text, MIMEType: item.MIMEType, Data: item.Data}
		itemType := item.Type
		if itemType == "" {
			itemType = "text"
			block.Type = "text"
		}
		items = append(items, ToolCallContentItem{
			Type:    itemType,
			Content: block,
		})
	}

	return items
}

func buildTextContentItems(text string) []ToolCallContentItem {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	return []ToolCallContentItem{{
		Type:    "text",
		Content: ContentBlock{Type: "text", Text: text},
	}}
}

func hasNonTextContent(content []runtimeevents.RichContent) bool {
	for _, item := range content {
		if item.Type != "" && item.Type != "text" {
			return true
		}
	}
	return false
}

func firstNonEmptyToolOutput(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			const maxOutputLen = 10000
			if len(value) > maxOutputLen {
				return value[:maxOutputLen] + "\n... (truncated)"
			}
			return value
		}
	}
	return ""
}
