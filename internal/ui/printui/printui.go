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
func VisualizeText(w io.Writer) ui.VisualizeFunc {
	if w == nil {
		w = io.Discard
	}

	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		for event := range events {
			rendered := formatEvent(event)
			if rendered == "" {
				continue
			}

			if _, err := fmt.Fprintln(w, rendered); err != nil {
				return fmt.Errorf("write print ui event: %w", err)
			}
		}

		return nil
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

		return fmt.Sprintf("[tool call] %s %s", e.Name, summary)
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

	return clampInline(strings.TrimSpace(event.Arguments))
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
