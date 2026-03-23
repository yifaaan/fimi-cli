package llm

import (
	"errors"
	"fmt"
)

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderClient 是当前最小可用的 LLM client。
// 它先返回确定性的占位文本，后面再替换成真实 provider client。
type PlaceholderClient struct{}

// ErrUnsupportedClientMode 表示当前 llm client 构造器不支持给定模式。
var ErrUnsupportedClientMode = errors.New("unsupported llm client mode")

// Reply 根据请求返回占位 assistant 回复。
func (PlaceholderClient) Reply(request Request) (Response, error) {
	prompt, _ := request.PrimaryUserPrompt()

	return Response{
		Text: fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt),
	}, nil
}

// NewPlaceholderClient 返回默认的占位 llm client。
func NewPlaceholderClient() Client {
	return PlaceholderClient{}
}

// BuildClient 根据 mode 构造具体 llm client。
func BuildClient(mode string) (Client, error) {
	switch mode {
	case "", "placeholder":
		return NewPlaceholderClient(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedClientMode, mode)
	}
}

// NewPlaceholderEngine 返回默认的占位 LLM engine。
func NewPlaceholderEngine(cfg Config) Engine {
	return NewEngine(NewPlaceholderClient(), cfg)
}
