package llm

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall 表示 provider-neutral 的单个工具调用。
// 这里先只保留 runtime 下一阶段最需要的字段。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Message 表示最小聊天消息单元。
type Message struct {
	Role    string
	Content string
	// ToolCallID 只在 role=tool 时有意义，用来把工具结果关联回之前的调用。
	ToolCallID string
	// ToolCalls 只在 role=assistant 时有意义，表示 assistant 请求 runtime 执行工具。
	ToolCalls []ToolCall
}

// Request 表示一次最小 LLM 调用请求。
// Messages 是主协议。
type Request struct {
	Model        string
	SystemPrompt string
	Messages     []Message
}

// Response 表示一次最小 LLM 调用响应。
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage // token 使用量
}

// Usage 表示 LLM 调用的 token 使用量。
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// HasToolCalls 返回消息里是否包含 assistant 发起的工具调用。
func (m Message) HasToolCalls() bool {
	return len(m.ToolCalls) > 0
}

// IsToolResult 返回当前消息是否表示某个工具调用的回填结果。
func (m Message) IsToolResult() bool {
	return m.Role == RoleTool && m.ToolCallID != ""
}

// HasToolCalls 返回模型响应里是否包含待执行的工具调用。
func (r Response) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// LastUserMessage 返回请求里最后一条 user message。
func (r Request) LastUserMessage() (Message, bool) {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		message := r.Messages[i]
		if message.Role == RoleUser {
			return message, true
		}
	}

	return Message{}, false
}

// PrimaryUserPrompt 返回当前请求的主 user prompt。
func (r Request) PrimaryUserPrompt() (string, bool) {
	if message, ok := r.LastUserMessage(); ok {
		return message.Content, true
	}

	return "", false
}
