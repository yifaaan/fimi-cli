package llm

import "fmt"

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderClient 是当前最小可用的 LLM client。
// 它先返回确定性的占位文本，后面再替换成真实 provider client。
type PlaceholderClient struct{}

// Reply 根据请求返回占位 assistant 回复。
func (PlaceholderClient) Reply(request Request) (Response, error) {
	prompt := resolvePrompt(request)

	return Response{
		Text: fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt),
	}, nil
}

// NewPlaceholderEngine 返回默认的占位 LLM engine。
func NewPlaceholderEngine() Engine {
	return NewEngine(PlaceholderClient{})
}

func resolvePrompt(request Request) string {
	for i := len(request.Messages) - 1; i >= 0; i-- {
		message := request.Messages[i]
		if message.Role == RoleUser {
			return message.Content
		}
	}

	return request.Prompt
}
