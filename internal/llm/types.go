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
// 现在同时保留 prompt 和 messages，便于逐步过渡到聊天式请求协议。
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
