package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type,omitempty"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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

type retryableError struct {
	err error
}

func (e retryableError) Error() string {
	return e.err.Error()
}

func (e retryableError) Unwrap() error {
	return e.err
}

func (retryableError) Retryable() bool {
	return true
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
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  buildChatToolCalls(msg.ToolCalls),
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
		return llm.Response{}, fmt.Errorf("send request: %w", markRetryable(err))
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return llm.Response{}, statusError(resp.StatusCode, respBytes)
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
		Text:      chatResp.Choices[0].Message.Content,
		ToolCalls: buildLLMToolCalls(chatResp.Choices[0].Message.ToolCalls),
	}, nil
}

func buildChatToolCalls(calls []llm.ToolCall) []chatToolCall {
	if len(calls) == 0 {
		return nil
	}

	chatCalls := make([]chatToolCall, 0, len(calls))
	for _, call := range calls {
		chatCalls = append(chatCalls, chatToolCall{
			ID:   call.ID,
			Type: "function",
			Function: chatToolFunction{
				Name:      call.Name,
				Arguments: call.Arguments,
			},
		})
	}

	return chatCalls
}

func buildLLMToolCalls(calls []chatToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	llmCalls := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		llmCalls = append(llmCalls, llm.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}

	return llmCalls
}

func markRetryable(err error) error {
	if err == nil {
		return nil
	}

	return retryableError{err: err}
}

func statusError(statusCode int, body []byte) error {
	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && chatResp.Error != nil {
		err := fmt.Errorf(
			"api status %d: %s: %s",
			statusCode,
			chatResp.Error.Type,
			chatResp.Error.Message,
		)
		if isRetryableStatus(statusCode) {
			return markRetryable(err)
		}

		return err
	}

	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}

	err := fmt.Errorf("api status %d: %s", statusCode, message)
	if isRetryableStatus(statusCode) {
		return markRetryable(err)
	}

	return err
}

func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
