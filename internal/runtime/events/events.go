package events

import "context"

// Kind 标识 runtime 发给 UI/transport 的事件类型。
type Kind string

const (
	KindStepBegin       Kind = "step_begin"
	KindStepInterrupted Kind = "step_interrupted"
	KindStatusUpdate    Kind = "status_update"
	KindTextPart        Kind = "text_part"
	KindToolCall        Kind = "tool_call"
	KindToolCallPart    Kind = "tool_call_part"
	KindToolResult      Kind = "tool_result"
)

// Event 是 runtime 事件流里的最小多态边界。
type Event interface {
	Kind() Kind
}

// StatusSnapshot 表示 UI 可以消费的最小运行状态快照。
type StatusSnapshot struct {
	ContextUsage float64
}

// StepBegin 表示 runtime 开始推进新的 step。
type StepBegin struct {
	Number int
}

// Kind 返回事件种类。
func (StepBegin) Kind() Kind {
	return KindStepBegin
}

// StepInterrupted 表示当前 step 被取消或中断。
type StepInterrupted struct{}

// Kind 返回事件种类。
func (StepInterrupted) Kind() Kind {
	return KindStepInterrupted
}

// StatusUpdate 表示一次最新的运行状态快照。
type StatusUpdate struct {
	Status StatusSnapshot
}

// Kind 返回事件种类。
func (StatusUpdate) Kind() Kind {
	return KindStatusUpdate
}

// TextPart 表示 assistant 输出的一段文本。
type TextPart struct {
	Text string
}

// Kind 返回事件种类。
func (TextPart) Kind() Kind {
	return KindTextPart
}

// ToolCall 表示一个完整的工具调用事件。
// Subtitle 预留给后续 print/shell UI 展示摘要，不要求 runtime 立即提供。
type ToolCall struct {
	ID        string
	Name      string
	Subtitle  string
	Arguments string
}

// Kind 返回事件种类。
func (ToolCall) Kind() Kind {
	return KindToolCall
}

// ToolCallPart 表示工具调用的增量片段。
// 它主要给未来流式模型输出预留，而不是给当前非流式 runtime 使用。
type ToolCallPart struct {
	ToolCallID string
	Delta      string
}

// Kind 返回事件种类。
func (ToolCallPart) Kind() Kind {
	return KindToolCallPart
}

// ToolResult 表示一个工具调用完成后的结果事件。
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Output     string
	IsError    bool
}

// Kind 返回事件种类。
func (ToolResult) Kind() Kind {
	return KindToolResult
}

// Sink 是 runtime 向外发送事件的最小输出边界。
// 后续 print UI、shell UI、ACP 都实现这个接口，而 runtime 本身不依赖具体呈现。
type Sink interface {
	Emit(ctx context.Context, event Event) error
}

// SinkFunc 让普通函数也能直接作为事件接收器使用。
type SinkFunc func(ctx context.Context, event Event) error

// Emit 把函数适配成 Sink。
// nil 函数按 no-op 处理，避免调用方为了“什么都不做”再包一层空实现。
func (f SinkFunc) Emit(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}

	return f(ctx, event)
}

// NoopSink 是 runtime 事件边界的默认实现。
// runtime 后续即使暂时没有 UI，也能安全地无条件调用 Emit。
type NoopSink struct{}

// Emit 丢弃所有事件。
func (NoopSink) Emit(context.Context, Event) error {
	return nil
}
