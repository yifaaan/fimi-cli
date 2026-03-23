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

func lastUserMessage(messages []Message) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role == RoleUser {
			return message.Content, true
		}
	}

	return "", false
}
