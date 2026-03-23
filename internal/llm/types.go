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
// Messages 是主协议。
type Request struct {
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
func (r Request) PrimaryUserPrompt() (string, bool) {
	if message, ok := r.LastUserMessage(); ok {
		return message.Content, true
	}

	return "", false
}
