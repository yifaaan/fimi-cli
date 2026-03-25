package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fimi-cli/internal/llm"
)

// chatStreamRequest 是 OpenAI Chat Completions API 的流式请求格式。
// 与 chatRequest 唯一的区别是 Stream 字段。
type chatStreamRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatToolDef `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

// chatStreamResponse 是 OpenAI 流式响应的单个 chunk 格式。
type chatStreamResponse struct {
	ID      string             `json:"id"`
	Choices []chatStreamChoice `json:"choices"`
	Usage   *chatUsage         `json:"usage,omitempty"` // 可能在最后一个 chunk
}

// chatStreamChoice 是流式响应中的选项。
// 注意：流式使用 Delta 而非 Message。
type chatStreamChoice struct {
	Index        int         `json:"index"`
	Delta        chatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

// chatUsage 是 token 使用量信息。
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// toolCallBuilder 用于累积流式的 tool call delta。
// 因为 tool call 的 arguments 是分多个 delta 发送的，需要累积。
type toolCallBuilder struct {
	id        string
	name      string
	arguments strings.Builder
}

// ReplyStream 实现 llm.StreamingClient 接口。
// 它使用 Server-Sent Events (SSE) 协议接收流式响应。
func (c *Client) ReplyStream(
	ctx context.Context,
	request llm.Request,
	handler llm.StreamHandler,
) (llm.Response, error) {
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

	body := chatStreamRequest{
		Model:    model,
		Messages: messages,
		Tools:    buildChatToolDefinitions(request.Tools),
		Stream:   true, // 启用流式模式
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal stream request: %w", err)
	}

	url := c.config.BaseURL + chatPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return llm.Response{}, fmt.Errorf("create stream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	// OpenAI 需要 Accept 头来接收 SSE
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("send stream request: %w", markRetryable(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBytes, _ := io.ReadAll(resp.Body)
		return llm.Response{}, statusError(resp.StatusCode, respBytes)
	}

	// 解析 SSE 流
	return c.parseSSEStream(ctx, resp.Body, handler)
}

// parseSSEStream 解析 Server-Sent Events 流并累积响应。
// SSE 格式：每行以 "data: " 开头，后面是 JSON，最后是 "data: [DONE]"。
func (c *Client) parseSSEStream(
	ctx context.Context,
	body io.Reader,
	handler llm.StreamHandler,
) (llm.Response, error) {
	var (
		textBuilder    strings.Builder
		toolCallBuilders = make(map[string]*toolCallBuilder)
		toolCallOrder   []string // 保持 tool call 的顺序
		usage           llm.Usage
	)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		// 跳过空行
		if line == "" {
			continue
		}

		// SSE 行以 "data: " 开头
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// 流结束标记
		if data == "[DONE]" {
			break
		}

		// 解析 JSON chunk
		var streamResp chatStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			return llm.Response{}, fmt.Errorf("parse stream chunk: %w", err)
		}

		// 检查是否有 choices
		if len(streamResp.Choices) == 0 {
			continue
		}

		delta := streamResp.Choices[0].Delta

		// 处理文本内容
		if delta.Content != "" {
			textBuilder.WriteString(delta.Content)

			// 发送文本 delta 事件
			if err := handler.HandleStreamEvent(ctx, llm.TextDeltaEvent{
				Delta: delta.Content,
			}); err != nil {
				return llm.Response{}, fmt.Errorf("handle text delta: %w", err)
			}
		}

		// 处理 tool calls
		for _, tc := range delta.ToolCalls {
			// 获取或创建 builder
			builder, exists := toolCallBuilders[tc.ID]
			if !exists {
				builder = &toolCallBuilder{id: tc.ID}
				toolCallBuilders[tc.ID] = builder
				toolCallOrder = append(toolCallOrder, tc.ID)
			}

			// 更新 name（如果非空）
			if tc.Function.Name != "" {
				builder.name = tc.Function.Name
			}

			// 累积 arguments
			if tc.Function.Arguments != "" {
				builder.arguments.WriteString(tc.Function.Arguments)

				// 发送 tool call delta 事件
				if err := handler.HandleStreamEvent(ctx, llm.ToolCallDeltaEvent{
					ToolCallID: tc.ID,
					Name:       tc.Function.Name, // 可能为空
					Delta:      tc.Function.Arguments,
				}); err != nil {
					return llm.Response{}, fmt.Errorf("handle tool call delta: %w", err)
				}
			}
		}

		// 处理 usage（可能在最后一个 chunk）
		if streamResp.Usage != nil {
			usage = llm.Usage{
				InputTokens:  streamResp.Usage.PromptTokens,
				OutputTokens: streamResp.Usage.CompletionTokens,
				TotalTokens:  streamResp.Usage.TotalTokens,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return llm.Response{}, fmt.Errorf("read stream: %w", markRetryable(err))
	}

	// 构建最终的 tool calls
	var toolCalls []llm.ToolCall
	for _, id := range toolCallOrder {
		builder := toolCallBuilders[id]
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:        builder.id,
			Name:      builder.name,
			Arguments: builder.arguments.String(),
		})
	}

	return llm.Response{
		Text:      textBuilder.String(),
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}
