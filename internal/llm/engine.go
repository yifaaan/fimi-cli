package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/runtime/events"
)

const DefaultHistoryTurnLimit = 2

// Config 定义 llm engine 侧的最小消息构造参数。
type Config struct {
	HistoryTurnLimit int
	Tools            []ToolDefinition
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
	tools            []ToolDefinition
}

// NewEngine 创建一个基于 llm client 的 runtime engine。
func NewEngine(client Client, cfg Config) Engine {
	if client == nil {
		client = missingClient{}
	}
	if cfg.HistoryTurnLimit <= 0 {
		cfg.HistoryTurnLimit = DefaultHistoryTurnLimit
	}

	return Engine{
		client:           client,
		historyTurnLimit: cfg.HistoryTurnLimit,
		tools:            cfg.Tools,
	}
}

// Reply 调用底层 llm client，为 runtime 生成结构化 assistant 回复。
func (e Engine) Reply(ctx context.Context, input runtime.ReplyInput) (runtime.AssistantReply, error) {
	request := Request{
		Model:        input.Model,
		SystemPrompt: composeSystemPrompt(input.SystemPrompt, e.tools),
		Messages:     buildMessages(input.History, e.historyTurnLimit),
		Tools:        e.tools,
	}

	// DEBUG: 打印发送的工具数量
	fmt.Fprintf(os.Stderr, "[DEBUG] Sending %d tools to LLM\n", len(e.tools))
	for _, tool := range e.tools {
		fmt.Fprintf(os.Stderr, "[DEBUG]   - %s\n", tool.Name)
	}

	response, err := e.client.Reply(request)
	if err != nil {
		return runtime.AssistantReply{}, fmt.Errorf("llm client reply: %w", err)
	}

	// DEBUG: 打印返回的工具调用数量
	fmt.Fprintf(os.Stderr, "[DEBUG] LLM returned %d tool calls\n", len(response.ToolCalls))
	for _, tc := range response.ToolCalls {
		fmt.Fprintf(os.Stderr, "[DEBUG]   - %s: %s\n", tc.Name, tc.Arguments)
	}

	return runtime.AssistantReply{
		Text:      response.Text,
		ToolCalls: buildRuntimeToolCalls(response.ToolCalls),
		Usage: runtime.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
	}, nil
}

// ReplyStream 调用底层 llm client 的流式接口，实时发送事件到 sink。
// 如果底层 client 不支持流式，会自动降级到非流式 Reply。
// 这是一个"能力检测"模式：通过类型断言检查 client 是否实现 StreamingClient。
func (e Engine) ReplyStream(
	ctx context.Context,
	input runtime.ReplyInput,
	sink events.Sink,
) (runtime.AssistantReply, error) {
	// 检查 client 是否支持流式
	streamingClient, ok := e.client.(StreamingClient)
	if !ok {
		// 不支持流式，降级到非流式
		return e.Reply(ctx, input)
	}

	request := Request{
		Model:        input.Model,
		SystemPrompt: composeSystemPrompt(input.SystemPrompt, e.tools),
		Messages:     buildMessages(input.History, e.historyTurnLimit),
		Tools:        e.tools,
	}

	// DEBUG: 打印发送的工具数量
	fmt.Fprintf(os.Stderr, "[DEBUG STREAM] Sending %d tools to LLM\n", len(e.tools))
	for _, tool := range e.tools {
		fmt.Fprintf(os.Stderr, "[DEBUG STREAM]   - %s\n", tool.Name)
	}

	// 适配 events.Sink 为 StreamHandler
	// 这是一个适配器模式：将一个接口转换为另一个接口
	handler := StreamHandlerFunc(func(ctx context.Context, event StreamEvent) error {
		switch ev := event.(type) {
		case TextDeltaEvent:
			return sink.Emit(ctx, events.TextPart{Text: ev.Delta})
		case ToolCallDeltaEvent:
			return sink.Emit(ctx, events.ToolCallPart{
				ToolCallID: ev.ToolCallID,
				Delta:      ev.Delta,
			})
		}
		return nil
	})

	response, err := streamingClient.ReplyStream(ctx, request, handler)
	if err != nil {
		return runtime.AssistantReply{}, fmt.Errorf("llm client stream reply: %w", err)
	}

	// DEBUG: 打印返回的工具调用数量
	fmt.Fprintf(os.Stderr, "[DEBUG STREAM] LLM returned %d tool calls\n", len(response.ToolCalls))
	for _, tc := range response.ToolCalls {
		fmt.Fprintf(os.Stderr, "[DEBUG STREAM]   - %s: %s\n", tc.Name, tc.Arguments)
	}

	return runtime.AssistantReply{
		Text:      response.Text,
		ToolCalls: buildRuntimeToolCalls(response.ToolCalls),
		Usage: runtime.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
	}, nil
}

func buildMessages(history []contextstore.TextRecord, historyTurnLimit int) []Message {
	return buildHistoryMessages(history, historyTurnLimit)
}

func composeSystemPrompt(base string, tools []ToolDefinition) string {
	base = strings.TrimSpace(base)
	policy := strings.TrimSpace(buildToolUsePolicy(tools))

	switch {
	case base == "":
		return policy
	case policy == "":
		return base
	default:
		return base + "\n\n" + policy
	}
}

func buildToolUsePolicy(tools []ToolDefinition) string {
	if len(tools) == 0 {
		return ""
	}

	lines := []string{
		"Tool Use Policy:",
		"- Use the available tools directly when they are needed to answer or verify something.",
		"- For repository files, directory listings, command output, searches, or other workspace facts, call a tool instead of guessing.",
		"- Do not say you will run a command, inspect a file, or list files unless you actually emit a tool call.",
		"- Do not ask the user for permission before safe read-only inspection or safe workspace-local commands.",
		"Available tools:",
	}

	for _, tool := range tools {
		line := "- " + tool.Name
		if description := strings.TrimSpace(tool.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func buildRuntimeToolCalls(calls []ToolCall) []runtime.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	runtimeCalls := make([]runtime.ToolCall, 0, len(calls))
	for _, call := range calls {
		runtimeCalls = append(runtimeCalls, runtime.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}

	return runtimeCalls
}

type missingClient struct{}

func (missingClient) Reply(request Request) (Response, error) {
	return Response{}, errors.New("llm client is required")
}
