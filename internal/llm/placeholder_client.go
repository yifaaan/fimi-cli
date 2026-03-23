package llm

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderClient 是当前最小可用的 LLM client。
// 它先返回确定性的占位文本，后面再替换成真实 provider client。
type PlaceholderClient struct{}

// Reply 根据请求返回占位 assistant 回复。
func (PlaceholderClient) Reply(request Request) (Response, error) {
	prompt, _ := request.PrimaryUserPrompt()

	return Response{
		Text: assistantPlaceholderPrefix + " " + prompt,
	}, nil
}

// NewPlaceholderClient 返回默认的占位 llm client。
func NewPlaceholderClient() Client {
	return PlaceholderClient{}
}

// NewPlaceholderEngine 返回默认的占位 LLM engine。
func NewPlaceholderEngine(cfg Config) Engine {
	return NewEngine(NewPlaceholderClient(), cfg)
}
