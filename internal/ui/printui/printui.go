package printui

import (
	"context"
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
	if event.Status.ContextUsage <= 0 {
		return ""
	}

	bounded := math.Max(0, math.Min(event.Status.ContextUsage, 1))
	return fmt.Sprintf("[status] context used %.0f%%", bounded*100)
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

	output := strings.TrimSpace(event.Output)
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
