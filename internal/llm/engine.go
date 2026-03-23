package llm

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

const DefaultHistoryTurnLimit = 2

// Config 定义 llm engine 侧的最小消息构造参数。
type Config struct {
	HistoryTurnLimit int
}

// DefaultConfig 返回 llm engine 的默认参数。
func DefaultConfig() Config {
	return Config{
		HistoryTurnLimit: DefaultHistoryTurnLimit,
	}
}

// Client 表示最小 LLM client 边界。
// 现在先通过 Request/Response 隔离 provider 侧协议形状。
type Client interface {
	Reply(request Request) (Response, error)
}

// Engine 把 llm client 适配为 runtime.Engine。
type Engine struct {
	client           Client
	historyTurnLimit int
}

// NewEngine 创建一个基于 llm client 的 runtime engine。
func NewEngine(client Client, cfg Config) Engine {
	if client == nil {
		client = missingClient{}
	}
	if cfg.HistoryTurnLimit <= 0 {
		cfg = DefaultConfig()
	}

	return Engine{
		client:           client,
		historyTurnLimit: cfg.HistoryTurnLimit,
	}
}

// Reply 调用底层 llm client，为 runtime 生成 assistant 文本。
func (e Engine) Reply(input runtime.ReplyInput) (string, error) {
	fallbackPrompt := strings.TrimSpace(input.Prompt)
	request := Request{
		Model:        input.Model,
		SystemPrompt: input.SystemPrompt,
		Messages:     buildMessages(input.SystemPrompt, input.History, fallbackPrompt, e.historyTurnLimit),
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
	historyTurnLimit int,
) []Message {
	messages := make([]Message, 0, 1+historyTurnLimit*2+1)
	if systemPrompt != "" {
		messages = append(messages, Message{
			Role:    RoleSystem,
			Content: systemPrompt,
		})
	}

	messages = append(messages, buildHistoryMessages(history, historyTurnLimit)...)
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
