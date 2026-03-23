package llm

// Request 表示一次最小 LLM 调用请求。
// 现在先只保留 prompt，后面再扩展 model、messages 等字段。
type Request struct {
	Prompt       string
	Model        string
	SystemPrompt string
}

// Response 表示一次最小 LLM 调用响应。
type Response struct {
	Text string
}
