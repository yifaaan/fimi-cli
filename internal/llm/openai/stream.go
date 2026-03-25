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
	Model      string        `json:"model"`
	Messages   []chatMessage `json:"messages"`
	Tools      []chatToolDef `json:"tools,omitempty"`
	ToolChoice any           `json:"tool_choice,omitempty"` // "auto", "required", 或具体工具
	Stream     bool          `json:"stream"`
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
	callID    string
	name      string
	arguments strings.Builder
}

type responseStreamEvent struct {
	Type string `json:"type"`
}

type responseOutputTextDeltaEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

type responseOutputItemAddedEvent struct {
	Type string             `json:"type"`
	Item responseOutputItem `json:"item"`
}

type responseFunctionCallArgumentsDeltaEvent struct {
	Type   string `json:"type"`
	ItemID string `json:"item_id"`
	Delta  string `json:"delta"`
}

type responseFunctionCallArgumentsDoneEvent struct {
	Type string             `json:"type"`
	Item responseOutputItem `json:"item"`
}

type responseCompletedEvent struct {
	Type     string              `json:"type"`
	Response responseAPIResponse `json:"response"`
}

type responseErrorEvent struct {
	Type    string     `json:"type"`
	Message string     `json:"message,omitempty"`
	Error   *chatError `json:"error,omitempty"`
}

// ReplyStream 实现 llm.StreamingClient 接口。
// 它使用 Server-Sent Events (SSE) 协议接收流式响应。
func (c *Client) ReplyStream(
	ctx context.Context,
	request llm.Request,
	handler llm.StreamHandler,
) (llm.Response, error) {
	switch c.config.WireAPI {
	case WireAPIResp:
		return c.replyResponsesStream(ctx, request, handler)
	default:
		return c.replyChatStream(ctx, request, handler)
	}
}

func (c *Client) replyChatStream(
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

	model := c.config.Model
	if request.Model != "" {
		model = request.Model
	}

	body := chatStreamRequest{
		Model:    model,
		Messages: messages,
		Tools:    buildChatToolDefinitions(request.Tools),
		Stream:   true,
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

	return c.parseChatSSEStream(ctx, resp.Body, handler)
}

func (c *Client) replyResponsesStream(
	ctx context.Context,
	request llm.Request,
	handler llm.StreamHandler,
) (llm.Response, error) {
	model := c.config.Model
	if request.Model != "" {
		model = request.Model
	}

	body := responseRequest{
		Model:        model,
		Instructions: request.SystemPrompt,
		Input:        buildResponseInput(request.Messages),
		Tools:        buildResponseToolDefinitions(request.Tools),
		Stream:       true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal stream request: %w", err)
	}

	url := c.config.BaseURL + responsesPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return llm.Response{}, fmt.Errorf("create stream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
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

	return c.parseResponsesSSEStream(ctx, resp.Body, handler)
}

// parseChatSSEStream 解析 Chat Completions SSE 流并累积响应。
func (c *Client) parseChatSSEStream(
	ctx context.Context,
	body io.Reader,
	handler llm.StreamHandler,
) (llm.Response, error) {
	var (
		textBuilder      strings.Builder
		toolCallBuilders = make(map[string]*toolCallBuilder)
		toolCallOrder    []string
		usage            llm.Usage
	)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var streamResp chatStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			return llm.Response{}, fmt.Errorf("parse stream chunk: %w", err)
		}
		if len(streamResp.Choices) == 0 {
			continue
		}

		delta := streamResp.Choices[0].Delta
		if delta.Content != "" {
			textBuilder.WriteString(delta.Content)
			if err := handler.HandleStreamEvent(ctx, llm.TextDeltaEvent{
				Delta: delta.Content,
			}); err != nil {
				return llm.Response{}, fmt.Errorf("handle text delta: %w", err)
			}
		}

		for _, tc := range delta.ToolCalls {
			builder, exists := toolCallBuilders[tc.ID]
			if !exists {
				builder = &toolCallBuilder{id: tc.ID}
				toolCallBuilders[tc.ID] = builder
				toolCallOrder = append(toolCallOrder, tc.ID)
			}
			if tc.Function.Name != "" {
				builder.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				builder.arguments.WriteString(tc.Function.Arguments)
				if err := handler.HandleStreamEvent(ctx, llm.ToolCallDeltaEvent{
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Delta:      tc.Function.Arguments,
				}); err != nil {
					return llm.Response{}, fmt.Errorf("handle tool call delta: %w", err)
				}
			}
		}

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

// parseResponsesSSEStream 解析 Responses API 的语义化 SSE 事件。
func (c *Client) parseResponsesSSEStream(
	ctx context.Context,
	body io.Reader,
	handler llm.StreamHandler,
) (llm.Response, error) {
	var (
		textBuilder      strings.Builder
		toolCallBuilders = make(map[string]*toolCallBuilder)
		toolCallOrder    []string
		usage            llm.Usage
	)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var event responseStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return llm.Response{}, fmt.Errorf("parse stream event: %w", err)
		}

		switch event.Type {
		case "response.output_text.delta":
			var textEvent responseOutputTextDeltaEvent
			if err := json.Unmarshal([]byte(data), &textEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse text delta event: %w", err)
			}

			textBuilder.WriteString(textEvent.Delta)
			if err := handler.HandleStreamEvent(ctx, llm.TextDeltaEvent{
				Delta: textEvent.Delta,
			}); err != nil {
				return llm.Response{}, fmt.Errorf("handle text delta: %w", err)
			}

		case "response.output_item.added":
			var itemEvent responseOutputItemAddedEvent
			if err := json.Unmarshal([]byte(data), &itemEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse output item event: %w", err)
			}
			if itemEvent.Item.Type != "function_call" {
				continue
			}

			key := firstNonEmpty(itemEvent.Item.ID, itemEvent.Item.CallID)
			if key == "" {
				continue
			}
			builder, exists := toolCallBuilders[key]
			if !exists {
				builder = &toolCallBuilder{id: itemEvent.Item.ID}
				toolCallBuilders[key] = builder
				toolCallOrder = append(toolCallOrder, key)
			}
			builder.id = firstNonEmpty(builder.id, itemEvent.Item.ID, key)
			builder.callID = firstNonEmpty(itemEvent.Item.CallID, builder.callID)
			builder.name = firstNonEmpty(itemEvent.Item.Name, builder.name)

		case "response.function_call_arguments.delta":
			var argEvent responseFunctionCallArgumentsDeltaEvent
			if err := json.Unmarshal([]byte(data), &argEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse tool call delta event: %w", err)
			}

			builder, exists := toolCallBuilders[argEvent.ItemID]
			if !exists {
				builder = &toolCallBuilder{id: argEvent.ItemID}
				toolCallBuilders[argEvent.ItemID] = builder
				toolCallOrder = append(toolCallOrder, argEvent.ItemID)
			}

			name := ""
			if builder.arguments.Len() == 0 {
				name = builder.name
			}
			builder.arguments.WriteString(argEvent.Delta)
			if err := handler.HandleStreamEvent(ctx, llm.ToolCallDeltaEvent{
				ToolCallID: firstNonEmpty(builder.callID, builder.id, argEvent.ItemID),
				Name:       name,
				Delta:      argEvent.Delta,
			}); err != nil {
				return llm.Response{}, fmt.Errorf("handle tool call delta: %w", err)
			}

		case "response.function_call_arguments.done":
			var doneEvent responseFunctionCallArgumentsDoneEvent
			if err := json.Unmarshal([]byte(data), &doneEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse tool call done event: %w", err)
			}

			key := firstNonEmpty(doneEvent.Item.ID, doneEvent.Item.CallID)
			if key == "" {
				continue
			}
			builder, exists := toolCallBuilders[key]
			if !exists {
				builder = &toolCallBuilder{id: doneEvent.Item.ID}
				toolCallBuilders[key] = builder
				toolCallOrder = append(toolCallOrder, key)
			}
			builder.id = firstNonEmpty(builder.id, doneEvent.Item.ID, key)
			builder.callID = firstNonEmpty(doneEvent.Item.CallID, builder.callID)
			builder.name = firstNonEmpty(doneEvent.Item.Name, builder.name)
			if builder.arguments.Len() == 0 && doneEvent.Item.Arguments != "" {
				builder.arguments.WriteString(doneEvent.Item.Arguments)
			}

		case "response.completed":
			var completedEvent responseCompletedEvent
			if err := json.Unmarshal([]byte(data), &completedEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse completed event: %w", err)
			}
			usage = buildLLMUsage(completedEvent.Response.Usage)

			// 某些兼容 provider 只在 completed 事件里给完整输出，这里做兜底。
			if textBuilder.Len() == 0 || len(toolCallBuilders) == 0 {
				text, toolCalls := parseResponseOutput(completedEvent.Response.Output)
				if textBuilder.Len() == 0 {
					textBuilder.WriteString(text)
				}
				if len(toolCallBuilders) == 0 {
					for _, call := range toolCalls {
						key := firstNonEmpty(call.ID, call.Name)
						if key == "" {
							continue
						}
						builder := &toolCallBuilder{
							id:     call.ID,
							callID: call.ID,
							name:   call.Name,
						}
						builder.arguments.WriteString(call.Arguments)
						toolCallBuilders[key] = builder
						toolCallOrder = append(toolCallOrder, key)
					}
				}
			}

		case "error":
			var errEvent responseErrorEvent
			if err := json.Unmarshal([]byte(data), &errEvent); err != nil {
				return llm.Response{}, fmt.Errorf("parse error event: %w", err)
			}
			if errEvent.Error != nil {
				return llm.Response{}, fmt.Errorf("api error: %s: %s", errEvent.Error.Type, errEvent.Error.Message)
			}
			return llm.Response{}, fmt.Errorf("api error: %s", errEvent.Message)
		}
	}

	if err := scanner.Err(); err != nil {
		return llm.Response{}, fmt.Errorf("read stream: %w", markRetryable(err))
	}

	var toolCalls []llm.ToolCall
	for _, key := range toolCallOrder {
		builder := toolCallBuilders[key]
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:        firstNonEmpty(builder.callID, builder.id, key),
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
