package llm

import "context"

// StreamEvent 表示流式响应中的单个事件。
// 这是一个密封接口（sealed interface），只有本包内的具体类型可以实现。
type StreamEvent interface {
	streamEvent()
}

// TextDeltaEvent 表示文本内容的增量。
// 每个事件包含一小段文本，需要累积成完整输出。
type TextDeltaEvent struct {
	Delta string
}

// streamEvent 实现 StreamEvent 接口。
func (TextDeltaEvent) streamEvent() {}

// ToolCallDeltaEvent 表示 tool call arguments 的增量。
// ToolCallID 用于关联同一个 tool call 的多个 delta。
// Name 仅在某个 tool call 的首个 delta 中非空，用于标识工具名称。
// Delta 是 arguments JSON 字符串的片段，需要累积拼接。
type ToolCallDeltaEvent struct {
	ToolCallID string
	Name       string // 仅在首个 delta 中非空
	Delta      string // arguments JSON 片段
}

// streamEvent 实现 StreamEvent 接口。
func (ToolCallDeltaEvent) streamEvent() {}

// StreamHandler 消费流式事件。
// 这是 Go-idiomatic 的回调模式，比 channel 更适合处理：
// - 错误传播：handler 可以直接返回 error
// - 取消：通过 context 传递
// - 接口简洁：一个方法而不是管理 channel 生命周期
type StreamHandler interface {
	HandleStreamEvent(ctx context.Context, event StreamEvent) error
}

// StreamHandlerFunc 让普通函数适配 StreamHandler。
// 这是一个适配器模式（Adapter Pattern），类似于 http.HandlerFunc。
type StreamHandlerFunc func(ctx context.Context, event StreamEvent) error

// HandleStreamEvent 实现 StreamHandler 接口。
func (f StreamHandlerFunc) HandleStreamEvent(ctx context.Context, event StreamEvent) error {
	return f(ctx, event)
}

// StreamingClient 是支持流式的 LLM client 扩展接口。
// 它扩展了基础的 Client 接口，添加流式调用能力。
// 使用接口隔离原则（Interface Segregation）：
// - 不强制所有 Client 实现都支持流式
// - 调用方通过类型断言检测流式能力
// - 现有的非流式实现（如 PlaceholderClient）无需修改
type StreamingClient interface {
	Client
	// ReplyStream 执行流式 LLM 调用。
	// handler 用于接收每个流式事件。
	// 返回值是最终累积的完整响应（与 Reply 语义一致）。
	ReplyStream(ctx context.Context, request Request, handler StreamHandler) (Response, error)
}