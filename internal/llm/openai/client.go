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
	responsesPath  = "/responses"
	WireAPIChat    = "chat_completions"
	WireAPIResp    = "responses"
)

// Config 存储 OpenAI 兼容 client 的配置。
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	WireAPI string
}

// Client 实现 OpenAI 兼容 API 调用。
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

	wireAPI := cfg.WireAPI
	if wireAPI == "" {
		wireAPI = WireAPIChat
	}

	return &Client{
		config: Config{
			BaseURL: baseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			WireAPI: wireAPI,
		},
		http: http.DefaultClient,
	}
}

// chatRequest 是 OpenAI Chat Completions API 的请求格式。
type chatRequest struct {
	Model      string        `json:"model"`
	Messages   []chatMessage `json:"messages"`
	Tools      []chatToolDef `json:"tools,omitempty"`
	ToolChoice any           `json:"tool_choice,omitempty"` // "auto", "required", 或具体工具
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

type chatToolDef struct {
	Type     string             `json:"type"`
	Function chatToolDefinition `json:"function"`
}

type chatToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
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

type responseRequest struct {
	Model        string              `json:"model"`
	Instructions string              `json:"instructions,omitempty"`
	Input        []responseInputItem `json:"input,omitempty"`
	Tools        []responseToolDef   `json:"tools,omitempty"`
	ToolChoice   any                 `json:"tool_choice,omitempty"`
	Stream       bool                `json:"stream,omitempty"`
}

type responseInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    any    `json:"output,omitempty"`
}

type responseToolDef struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responseAPIResponse struct {
	ID     string               `json:"id,omitempty"`
	Output []responseOutputItem `json:"output"`
	Usage  *responseUsage       `json:"usage,omitempty"`
	Error  *chatError           `json:"error,omitempty"`
}

type responseOutputItem struct {
	Type      string                `json:"type"`
	ID        string                `json:"id,omitempty"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
	Content   []responseContentPart `json:"content,omitempty"`
}

type responseContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
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
	switch c.config.WireAPI {
	case WireAPIResp:
		return c.replyResponses(request)
	default:
		return c.replyChat(request)
	}
}

func (c *Client) replyChat(request llm.Request) (llm.Response, error) {
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

	model := c.config.Model
	if request.Model != "" {
		model = request.Model
	}

	body := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    buildChatToolDefinitions(request.Tools),
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
		return llm.Response{}, fmt.Errorf("parse response: %w (body: %s)", err, responseBodyPreview(respBytes))
	}
	if chatResp.Error != nil {
		return llm.Response{}, fmt.Errorf("api error: %s: %s", chatResp.Error.Type, chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("no choices in response")
	}

	return llm.Response{
		Text:      chatResp.Choices[0].Message.Content,
		ToolCalls: buildLLMToolCalls(chatResp.Choices[0].Message.ToolCalls),
	}, nil
}

func (c *Client) replyResponses(request llm.Request) (llm.Response, error) {
	model := c.config.Model
	if request.Model != "" {
		model = request.Model
	}

	body := responseRequest{
		Model:        model,
		Instructions: request.SystemPrompt,
		Input:        buildResponseInput(request.Messages),
		Tools:        buildResponseToolDefinitions(request.Tools),
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.BaseURL + responsesPath
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

	var apiResp responseAPIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return llm.Response{}, fmt.Errorf("parse response: %w (body: %s)", err, responseBodyPreview(respBytes))
	}
	if apiResp.Error != nil {
		return llm.Response{}, fmt.Errorf("api error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	text, toolCalls := parseResponseOutput(apiResp.Output)

	return llm.Response{
		Text:      text,
		ToolCalls: toolCalls,
		Usage:     buildLLMUsage(apiResp.Usage),
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

func buildChatToolDefinitions(tools []llm.ToolDefinition) []chatToolDef {
	if len(tools) == 0 {
		return nil
	}

	definitions := make([]chatToolDef, 0, len(tools))
	for _, tool := range tools {
		definitions = append(definitions, chatToolDef{
			Type: "function",
			Function: chatToolDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return definitions
}

func buildResponseInput(messages []llm.Message) []responseInputItem {
	if len(messages) == 0 {
		return nil
	}

	items := make([]responseInputItem, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem, "developer":
			if strings.TrimSpace(msg.Content) != "" {
				items = append(items, responseInputItem{
					Role:    "developer",
					Content: msg.Content,
				})
			}
		case llm.RoleUser, llm.RoleAssistant:
			if strings.TrimSpace(msg.Content) != "" {
				items = append(items, responseInputItem{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
			if msg.Role == llm.RoleAssistant {
				for _, call := range msg.ToolCalls {
					items = append(items, responseInputItem{
						Type:      "function_call",
						CallID:    call.ID,
						Name:      call.Name,
						Arguments: call.Arguments,
					})
				}
			}
		case llm.RoleTool:
			items = append(items, responseInputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		}
	}

	return items
}

func buildResponseToolDefinitions(tools []llm.ToolDefinition) []responseToolDef {
	if len(tools) == 0 {
		return nil
	}

	definitions := make([]responseToolDef, 0, len(tools))
	for _, tool := range tools {
		definitions = append(definitions, responseToolDef{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}

	return definitions
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

func parseResponseOutput(items []responseOutputItem) (string, []llm.ToolCall) {
	var textBuilder strings.Builder
	var toolCalls []llm.ToolCall

	for _, item := range items {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" || part.Type == "text" {
					textBuilder.WriteString(part.Text)
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        firstNonEmpty(item.CallID, item.ID),
				Name:      item.Name,
				Arguments: item.Arguments,
			})
		}
	}

	return textBuilder.String(), toolCalls
}

func buildLLMUsage(usage *responseUsage) llm.Usage {
	if usage == nil {
		return llm.Usage{}
	}

	return llm.Usage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
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

	var responseResp responseAPIResponse
	if err := json.Unmarshal(body, &responseResp); err == nil && responseResp.Error != nil {
		err := fmt.Errorf(
			"api status %d: %s: %s",
			statusCode,
			responseResp.Error.Type,
			responseResp.Error.Message,
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

func responseBodyPreview(body []byte) string {
	preview := strings.Join(strings.Fields(string(body)), " ")
	if preview == "" {
		return "<empty>"
	}
	if len(preview) > 160 {
		return preview[:157] + "..."
	}

	return preview
}
