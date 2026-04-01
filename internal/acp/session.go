package acp

import (
	"context"
	"fmt"
	"sync"

	runtimeevents "fimi-cli/internal/runtime/events"
	sessionpkg "fimi-cli/internal/session"
)

// Session 封装一个 ACP 客户端的 session 状态。
type Session struct {
	session sessionpkg.Session
	conn    *FramedConn
	mu      sync.Mutex
	modelID string

	// 运行中的 prompt 上下文
	cancelFn context.CancelFunc
}

// NewSession 创建一个新的 ACP session。
func NewSession(sess sessionpkg.Session, conn *FramedConn, modelID string) *Session {
	return &Session{
		session: sess,
		conn:    conn,
		modelID: modelID,
	}
}

// HistoryFile 返回 session 的历史文件路径。
func (s *Session) HistoryFile() string {
	return s.session.HistoryFile
}

// CurrentModelID 返回 session 当前使用的模型。
func (s *Session) CurrentModelID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelID
}

// SetModelID 设置 session 当前使用的模型。
func (s *Session) SetModelID(modelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelID = modelID
}

// SetCancel 设置用于取消运行中 prompt 的函数。
func (s *Session) SetCancel(fn context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelFn = fn
}

// Cancel 取消运行中的 prompt。
func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
}

// Visualize 返回一个 ui.VisualizeFunc，将 runtime 事件转换为 ACP session_update 通知。
// 这是 ACP 服务端流式架构的核心：runtime 事件 → ACP 通知。
func (s *Session) Visualize() func(ctx context.Context, events <-chan runtimeevents.Event) error {
	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		for event := range events {
			if err := s.translateAndSend(event); err != nil {
				return err
			}
		}
		return nil
	}
}

// translateAndSend 将单个 runtime 事件翻译成 ACP 通知并发送。
func (s *Session) translateAndSend(event runtimeevents.Event) error {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		return s.sendAgentMessageChunk(e.Text)
	case runtimeevents.ToolCall:
		return s.sendToolCallStart(e)
	case runtimeevents.ToolResult:
		return s.sendToolCallProgress(e)
	default:
		// StepBegin, StepInterrupted, StatusUpdate, ToolCallPart 暂不发送给 ACP 客户端
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

func (s *Session) sendToolCallStart(tc runtimeevents.ToolCall) error {
	content := []ToolCallContentItem{
		{
			Type:    "text",
			Content: TextContentBlock{Type: "text", Text: tc.Arguments},
		},
	}

	title := tc.Name
	if tc.Subtitle != "" {
		title = fmt.Sprintf("%s(%s)", tc.Name, tc.Subtitle)
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallStart{
			SessionUpdate: "tool_call_start",
			ToolCallID:    tc.ID,
			Title:         title,
			Status:        "in_progress",
			Content:       content,
		},
	})
}

func (s *Session) sendToolCallProgress(tr runtimeevents.ToolResult) error {
	status := "completed"
	if tr.IsError {
		status = "failed"
	}

	// 截断过长的工具输出
	output := tr.DisplayOutput
	if output == "" {
		output = tr.Output
	}
	const maxOutputLen = 10000
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen] + "\n... (truncated)"
	}

	content := []ToolCallContentItem{
		{
			Type:    "text",
			Content: TextContentBlock{Type: "text", Text: output},
		},
	}

	return s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: s.session.ID,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    tr.ToolCallID,
			Title:         tr.ToolName,
			Status:        status,
			Content:       content,
		},
	})
}
