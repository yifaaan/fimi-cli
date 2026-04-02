package acp

import (
	"fmt"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
)

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
