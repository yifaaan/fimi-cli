package llm

import "fmt"

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderClient 是当前最小可用的 LLM client。
// 它先返回确定性的占位文本，后面再替换成真实 provider client。
type PlaceholderClient struct{}

// Reply 根据请求返回占位 assistant 回复。
func (PlaceholderClient) Reply(request Request) (Response, error) {
	prompt := request.PrimaryUserPrompt()

	return Response{
		Text: fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt),
	}, nil
}

// NewPlaceholderEngine 返回默认的占位 LLM engine。
func NewPlaceholderEngine() Engine {
	return NewEngine(PlaceholderClient{})
}
