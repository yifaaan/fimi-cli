package llm

const (
	RoleSystem = "system"
	RoleUser   = "user"
)

// Message 表示最小聊天消息单元。
type Message struct {
	Role    string
	Content string
}

// Request 表示一次最小 LLM 调用请求。
// Messages 是主协议；Prompt 只保留为兼容字段。
type Request struct {
	Prompt       string
	Model        string
	SystemPrompt string
	Messages     []Message
}

// Response 表示一次最小 LLM 调用响应。
type Response struct {
	Text string
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
// 优先从主协议 Messages 派生；没有 user message 时回退到兼容字段 Prompt。
func (r Request) PrimaryUserPrompt() string {
	if message, ok := r.LastUserMessage(); ok {
		return message.Content
	}

	return r.Prompt
}
