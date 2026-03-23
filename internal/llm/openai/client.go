package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"fimi-cli/internal/llm"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	chatPath       = "/chat/completions"
)

// Config 存储 OpenAI 兼容 client 的配置。
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Client 实现 OpenAI Chat Completions API 调用。
type Client struct {
	config Config
	http   *http.Client
}

// NewClient 创建新的 OpenAI 兼容 client。
func NewClient(cfg Config) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		config: Config{
			BaseURL: baseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		},
		http: http.DefaultClient,
	}
}

// chatRequest 是 OpenAI Chat Completions API 的请求格式。
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// chatMessage 是 OpenAI chat API 的消息格式。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse 是 OpenAI Chat Completions API 的响应格式。
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Error   *chatError   `json:"error,omitempty"`
}

// chatChoice 是 OpenAI 响应中的选项。
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// chatError 是 OpenAI API 返回的错误格式。
type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Reply 实现 llm.Client 接口。
func (c *Client) Reply(request llm.Request) (llm.Response, error) {
	messages := make([]chatMessage, 0, len(request.Messages)+1)

	// 添加 system prompt（如果有）
	if request.SystemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    llm.RoleSystem,
			Content: request.SystemPrompt,
		})
	}

	// 添加对话历史
	for _, msg := range request.Messages {
		messages = append(messages, chatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 使用 client 配置的 model（如果请求中没有指定）
	model := c.config.Model
	if request.Model != "" {
		model = request.Model
	}

	body := chatRequest{
		Model:    model,
		Messages: messages,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.BaseURL + chatPath
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return llm.Response{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return llm.Response{}, fmt.Errorf("parse response: %w", err)
	}

	// 检查 API 错误
	if chatResp.Error != nil {
		return llm.Response{}, fmt.Errorf("api error: %s: %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	// 检查响应内容
	if len(chatResp.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("no choices in response")
	}

	return llm.Response{
		Text: chatResp.Choices[0].Message.Content,
	}, nil
}
