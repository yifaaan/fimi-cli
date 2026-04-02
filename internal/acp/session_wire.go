package acp

import (
	"context"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

// VisualizeWire converts wire messages into ACP session/update notifications.
func (s *Session) VisualizeWire() func(ctx context.Context, messages <-chan wire.Message) error {
	return func(ctx context.Context, messages <-chan wire.Message) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msg, ok := <-messages:
				if !ok {
					return nil
				}
				if err := s.translateAndSendMessage(msg); err != nil {
					return err
				}
			}
		}
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
