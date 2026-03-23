package llm

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

// Client 表示最小 LLM client 边界。
// 现在先通过 Request/Response 隔离 provider 侧协议形状。
type Client interface {
	Reply(request Request) (Response, error)
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
func (e Engine) Reply(input runtime.ReplyInput) (string, error) {
	fallbackPrompt := strings.TrimSpace(input.Prompt)
	request := Request{
		Model:        input.Model,
		SystemPrompt: input.SystemPrompt,
		Messages:     buildMessages(input.SystemPrompt, input.History, fallbackPrompt),
	}

	response, err := e.client.Reply(request)
	if err != nil {
		return "", fmt.Errorf("llm client reply: %w", err)
	}

	return response.Text, nil
}

func buildMessages(
	systemPrompt string,
	history []contextstore.TextRecord,
	prompt string,
) []Message {
	messages := make([]Message, 0, 1+historyMessageLimit+1)
	if systemPrompt != "" {
		messages = append(messages, Message{
			Role:    RoleSystem,
			Content: systemPrompt,
		})
	}

	messages = append(messages, buildHistoryMessages(history, historyMessageLimit)...)
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: prompt,
	})

	return messages
}

type missingClient struct{}

func (missingClient) Reply(request Request) (Response, error) {
	return Response{}, errors.New("llm client is required")
}
