package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	runtimeevents "fimi-cli/internal/runtime/events"
)

// Session 封装一个 ACP 宅户端的 session 状态。
type Session struct {
	id      string
	conn    *FramedConn
	workDir string
	mu      sync.Mutex

	// 运行中的 prompt 上下文
	cancelFn context.CancelFunc
}

// NewSession 创建一个新的 ACP session。
func NewSession(id string, conn *FramedConn, workDir string) *Session {
	return &Session{
		id:      id,
		conn:    conn,
		workDir: workDir,
	}
}

// ID 返回 session 的唯一标识符。
func (s *Session) ID() string {
	return s.id
}

// WorkDir 返回 session 的工作目录。
func (s *Session) WorkDir() string {
	return s.workDir
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
		SessionID: s.id,
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
		SessionID: s.id,
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
	output := tr.Output
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
		SessionID: s.id,
		Update: ToolCallProgress{
			SessionUpdate: "tool_call_progress",
			ToolCallID:    tr.ToolCallID,
			Title:         tr.ToolName,
			Status:        status,
			Content:       content,
		},
	})
}

// ContentBlockFromACP 将 ACP prompt 内容块转换为纯文本。
func ContentBlockToText(raw json.RawMessage) (string, error) {
	var block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &block); err != nil {
		return "", fmt.Errorf("parse content block: %w", err)
	}

	switch block.Type {
	case "text":
		return block.Text, nil
	default:
		return block.Text, nil
	}
}
