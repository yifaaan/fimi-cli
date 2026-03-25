package llm

import (
	"context"
	"testing"
)

func TestTextDeltaEventImplementsStreamEvent(t *testing.T) {
	// 编译时检查：TextDeltaEvent 必须实现 StreamEvent 接口
	var _ StreamEvent = TextDeltaEvent{Delta: "hello"}
}

func TestToolCallDeltaEventImplementsStreamEvent(t *testing.T) {
	// 编译时检查：ToolCallDeltaEvent 必须实现 StreamEvent 接口
	var _ StreamEvent = ToolCallDeltaEvent{
		ToolCallID: "call_123",
		Name:       "bash",
		Delta:      `{"command": "ls"`,
	}
}

func TestStreamHandlerFuncAdapter(t *testing.T) {
	ctx := context.Background()
	received := make([]StreamEvent, 0)

	// 使用函数适配为 StreamHandler
	handler := StreamHandlerFunc(func(ctx context.Context, event StreamEvent) error {
		received = append(received, event)
		return nil
	})

	// 发送文本事件
	textEvent := TextDeltaEvent{Delta: "Hello"}
	if err := handler.HandleStreamEvent(ctx, textEvent); err != nil {
		t.Fatalf("HandleStreamEvent returned error: %v", err)
	}

	// 发送 tool call 事件
	toolEvent := ToolCallDeltaEvent{
		ToolCallID: "call_1",
		Name:       "bash",
		Delta:      `{"command": "ls"`,
	}
	if err := handler.HandleStreamEvent(ctx, toolEvent); err != nil {
		t.Fatalf("HandleStreamEvent returned error: %v", err)
	}

	// 验证收到的事件
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	if received[0].(TextDeltaEvent).Delta != "Hello" {
		t.Errorf("expected text delta 'Hello', got %v", received[0])
	}

	if received[1].(ToolCallDeltaEvent).ToolCallID != "call_1" {
		t.Errorf("expected tool call id 'call_1', got %v", received[1])
	}
}

func TestStreamHandlerFuncReturnsError(t *testing.T) {
	ctx := context.Background()
	expectedErr := context.Canceled

	// 返回错误的 handler
	handler := StreamHandlerFunc(func(ctx context.Context, event StreamEvent) error {
		return expectedErr
	})

	// 发送事件应该返回错误
	err := handler.HandleStreamEvent(ctx, TextDeltaEvent{Delta: "test"})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// mockStreamingClient 是一个用于测试的 mock streaming client
type mockStreamingClient struct {
	chunks []StreamEvent
	final  Response
	err    error
}

func (m *mockStreamingClient) Reply(request Request) (Response, error) {
	return m.final, m.err
}

func (m *mockStreamingClient) ReplyStream(
	ctx context.Context,
	request Request,
	handler StreamHandler,
) (Response, error) {
	for _, chunk := range m.chunks {
		if err := handler.HandleStreamEvent(ctx, chunk); err != nil {
			return Response{}, err
		}
	}
	return m.final, m.err
}

func TestMockStreamingClientImplementsInterfaces(t *testing.T) {
	// 编译时检查：mockStreamingClient 必须同时实现 Client 和 StreamingClient
	var _ Client = &mockStreamingClient{}
	var _ StreamingClient = &mockStreamingClient{}
}

func TestMockStreamingClientReplay(t *testing.T) {
	client := &mockStreamingClient{
		chunks: []StreamEvent{
			TextDeltaEvent{Delta: "Hello"},
			TextDeltaEvent{Delta: " world"},
			ToolCallDeltaEvent{ToolCallID: "call_1", Name: "bash", Delta: `{"command": "`},
			ToolCallDeltaEvent{ToolCallID: "call_1", Delta: `ls"}`},
		},
		final: Response{
			Text: "Hello world",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{"command": "ls"}`},
			},
		},
	}

	ctx := context.Background()
	received := make([]StreamEvent, 0)

	handler := StreamHandlerFunc(func(ctx context.Context, event StreamEvent) error {
		received = append(received, event)
		return nil
	})

	resp, err := client.ReplyStream(ctx, Request{}, handler)
	if err != nil {
		t.Fatalf("ReplyStream returned error: %v", err)
	}

	// 验证收到的事件数量
	if len(received) != 4 {
		t.Errorf("expected 4 events, got %d", len(received))
	}

	// 验证最终响应
	if resp.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %q", resp.Text)
	}

	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
}