package printui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui"
)

const maxInlineSummaryLength = 80

// VisualizeText 返回一个最小纯文本事件消费者。
// 这个 visualizer 只负责把 runtime event 渲染成稳定、可测试的文本输出。
// 对于 TextPart 事件，使用流式输出（不自动换行），其他事件按行输出。
func VisualizeText(w io.Writer) ui.VisualizeFunc {
	if w == nil {
		w = io.Discard
	}

	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		// 追踪是否刚打印过文本（用于决定是否需要补换行）
		inTextStream := false

		for event := range events {
			// 如果刚打印过文本，且当前不是 TextPart，先补换行
			if inTextStream {
				if _, isTextPart := event.(runtimeevents.TextPart); !isTextPart {
					if _, err := fmt.Fprintln(w); err != nil {
						return fmt.Errorf("write print ui newline: %w", err)
					}
					inTextStream = false
				}
			}

			rendered, isText := formatEventStreaming(event)
			if rendered == "" {
				continue
			}

			if isText {
				// TextPart：流式输出，不自动换行
				if _, err := fmt.Fprint(w, rendered); err != nil {
					return fmt.Errorf("write print ui event: %w", err)
				}
				inTextStream = true
			} else {
				// 其他事件：按行输出
				if _, err := fmt.Fprintln(w, rendered); err != nil {
					return fmt.Errorf("write print ui event: %w", err)
				}
			}
		}

		// 如果流结束时还在文本流中，补一个换行
		if inTextStream {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write print ui final newline: %w", err)
			}
		}

		return nil
	}
}

// formatEventStreaming 返回渲染后的文本和是否为 TextPart 标记。
// 对于 TextPart，返回 (text, true)；其他事件返回 (text, false)。
func formatEventStreaming(event runtimeevents.Event) (string, bool) {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		return e.Text, true
	default:
		return formatEvent(event), false
	}
}

func formatEvent(event runtimeevents.Event) string {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		return fmt.Sprintf("[step %d]", e.Number)
	case runtimeevents.StepInterrupted:
		return "[interrupted]"
	case runtimeevents.StatusUpdate:
		return formatStatusUpdate(e)
	case runtimeevents.TextPart:
		return e.Text
	case runtimeevents.ToolCall:
		summary := toolCallSummary(e)
		if summary == "" {
			return fmt.Sprintf("[tool call] %s", e.Name)
		}

		return fmt.Sprintf("[tool call] %s", summary)
	case runtimeevents.ToolCallPart:
		return fmt.Sprintf("[tool call part] %s %s", e.ToolCallID, e.Delta)
	case runtimeevents.ToolResult:
		return formatToolResult(e)
	default:
		return fmt.Sprintf("[event %T]", event)
	}
}

func formatStatusUpdate(event runtimeevents.StatusUpdate) string {
	parts := make([]string, 0, 2)
	if event.Status.Retry != nil {
		parts = append(parts, formatRetryStatus(*event.Status.Retry))
	}
	if event.Status.ContextUsage > 0 {
		bounded := math.Max(0, math.Min(event.Status.ContextUsage, 1))
		parts = append(parts, fmt.Sprintf("context used %.0f%%", bounded*100))
	}
	if len(parts) == 0 {
		return ""
	}

	return "[status] " + strings.Join(parts, "; ")
}

func formatRetryStatus(retry runtimeevents.RetryStatus) string {
	seconds := math.Max(0, float64(retry.NextDelayMS)/1000)
	return fmt.Sprintf("retrying in %.1fs (attempt %d/%d)", seconds, retry.Attempt, retry.MaxAttempts)
}

func toolCallSummary(event runtimeevents.ToolCall) string {
	if summary := strings.TrimSpace(event.Subtitle); summary != "" {
		return clampInline(summary)
	}

	arguments := strings.TrimSpace(event.Arguments)
	if arguments == "" {
		return ""
	}

	return clampInline(event.Name + " " + arguments)
}

func formatToolResult(event runtimeevents.ToolResult) string {
	prefix := "[tool result]"
	if event.IsError {
		prefix = "[tool error]"
	}

	output := strings.TrimSpace(event.DisplayOutput)
	if output == "" {
		output = strings.TrimSpace(event.Output)
	}
	if output == "" {
		return fmt.Sprintf("%s %s", prefix, event.ToolName)
	}

	if strings.Contains(output, "\n") {
		return fmt.Sprintf("%s %s\n%s", prefix, event.ToolName, output)
	}

	return fmt.Sprintf("%s %s %s", prefix, event.ToolName, output)
}

func clampInline(text string) string {
	if len(text) <= maxInlineSummaryLength {
		return text
	}

	if maxInlineSummaryLength <= 3 {
		return text[:maxInlineSummaryLength]
	}

	return text[:maxInlineSummaryLength-3] + "..."
}

// VisualizeStreamJSON 返回一个 JSON 行序列化器，每行一个事件。
// 每个事件序列化为 {"type":"step_begin","number":1} 这样的 JSON 对象。
func VisualizeStreamJSON(w io.Writer) ui.VisualizeFunc {
	if w == nil {
		w = io.Discard
	}

	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		for event := range events {
			// 手动构建 JSON 行以精确控制字段名和结构
			line, err := marshalEventJSON(event)
			if err != nil {
				return fmt.Errorf("marshal event to json: %w", err)
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return fmt.Errorf("write json line: %w", err)
			}
		}
		return nil
	}
}

// marshalEventJSON 将单个 runtime 事件序列化为 JSON 对象字符串。
// 不使用 json.Marshal 直接序列化结构体，而是手动构建以确保字段名符合测试期望。
func marshalEventJSON(event runtimeevents.Event) (string, error) {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		return fmt.Sprintf(`{"type":"step_begin","number":%d}`, e.Number), nil
	case runtimeevents.StepInterrupted:
		return `{"type":"step_interrupted"}`, nil
	case runtimeevents.StatusUpdate:
		fields := []string{`"type":"status_update"`}
		if e.Status.ContextUsage > 0 {
			fields = append(fields, fmt.Sprintf(`"context_usage":%.2f`, e.Status.ContextUsage))
		}
		if e.Status.Retry != nil {
			fields = append(fields,
				fmt.Sprintf(`"retry_attempt":%d`, e.Status.Retry.Attempt),
				fmt.Sprintf(`"retry_max_attempts":%d`, e.Status.Retry.MaxAttempts),
				fmt.Sprintf(`"retry_next_delay_ms":%d`, e.Status.Retry.NextDelayMS),
			)
		}
		return `{` + strings.Join(fields, ",") + `}`, nil
	case runtimeevents.TextPart:
		// JSON 字符串需要转义
		escaped, err := json.Marshal(e.Text)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"type":"text_part","text":%s}`, string(escaped)), nil
	case runtimeevents.ToolCall:
		name, _ := json.Marshal(e.Name)
		args, _ := json.Marshal(e.Arguments)
		sub, _ := json.Marshal(e.Subtitle)
		return fmt.Sprintf(`{"type":"tool_call","name":%s,"subtitle":%s,"arguments":%s}`,
			string(name), string(sub), string(args)), nil
	case runtimeevents.ToolCallPart:
		id, _ := json.Marshal(e.ToolCallID)
		delta, _ := json.Marshal(e.Delta)
		return fmt.Sprintf(`{"type":"tool_call_part","tool_call_id":%s,"delta":%s}`,
			string(id), string(delta)), nil
	case runtimeevents.ToolResult:
		name, _ := json.Marshal(e.ToolName)
		output, _ := json.Marshal(e.Output)
		return fmt.Sprintf(`{"type":"tool_result","tool_name":%s,"output":%s,"is_error":%t}`,
			string(name), string(output), e.IsError), nil
	default:
		// 兜底：使用反射获取 type
		escaped, err := json.Marshal(event.Kind())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"type":%s}`, string(escaped)), nil
	}
}
