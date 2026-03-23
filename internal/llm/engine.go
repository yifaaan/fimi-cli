package llm

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
)

// Client 表示最小 LLM client 边界。
// 现在它只接收纯文本 prompt，后面再扩展到消息数组或更完整的请求对象。
type Client interface {
	ReplyText(prompt string) (string, error)
}

// Engine 把 llm client 适配为 runtime.Engine。
type Engine struct {
	client Client
}

// NewEngine 创建一个基于 llm client 的 runtime engine。
func NewEngine(client Client) Engine {
	if client == nil {
		client = missingClient{}
	}

	return Engine{
		client: client,
	}
}

// Reply 调用底层 llm client，为 runtime 生成 assistant 文本。
func (e Engine) Reply(input runtime.Input) (string, error) {
	prompt := strings.TrimSpace(input.Prompt)
	reply, err := e.client.ReplyText(prompt)
	if err != nil {
		return "", fmt.Errorf("llm client reply: %w", err)
	}

	return reply, nil
}

type missingClient struct{}

func (missingClient) ReplyText(prompt string) (string, error) {
	return "", errors.New("llm client is required")
}
