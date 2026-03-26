package shell

import (
	"fmt"
	"math"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
)

type liveState struct {
	step          int
	contextUsage  float64
	assistantText string
	toolName      string
	toolSummary   string
	toolOutput    string
	toolIsError   bool
	interrupted   bool
}

func (s *liveState) Apply(event runtimeevents.Event) {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		s.step = e.Number
		s.toolName = ""
		s.toolSummary = ""
		s.toolOutput = ""
		s.toolIsError = false
		s.interrupted = false
	case runtimeevents.StatusUpdate:
		s.contextUsage = e.Status.ContextUsage
	case runtimeevents.TextPart:
		s.assistantText += e.Text
	case runtimeevents.ToolCall:
		s.toolName = e.Name
		s.toolSummary = toolCallDisplaySummary(e.Name, e.Subtitle, e.Arguments)
		s.toolOutput = ""
		s.toolIsError = false
	case runtimeevents.ToolCallPart:
		s.toolSummary += e.Delta
	case runtimeevents.ToolResult:
		s.toolName = e.ToolName
		s.toolOutput = e.Output
		s.toolIsError = e.IsError
	case runtimeevents.StepInterrupted:
		s.interrupted = true
	}
}

func (s liveState) Lines() []string {
	lines := make([]string, 0, 8)
	if s.step > 0 {
		lines = append(lines, fmt.Sprintf("[step %d]", s.step))
	}

	if s.contextUsage > 0 {
		bounded := math.Max(0, math.Min(s.contextUsage, 1))
		lines = append(lines, fmt.Sprintf("[status] context used %.0f%%", bounded*100))
	}

	if s.assistantText != "" {
		lines = append(lines, "[assistant]")
		lines = append(lines, splitPreservingEmpty(s.assistantText)...)
	}

	if s.toolName != "" {
		if summary := strings.TrimSpace(s.toolSummary); summary != "" {
			lines = append(lines, fmt.Sprintf("[tool] %s", summary))
		} else {
			lines = append(lines, fmt.Sprintf("[tool] %s", s.toolName))
		}
	}

	if strings.TrimSpace(s.toolOutput) != "" {
		prefix := "[tool result]"
		if s.toolIsError {
			prefix = "[tool error]"
		}
		lines = append(lines, fmt.Sprintf("%s %s", prefix, s.toolName))
		lines = append(lines, splitPreservingEmpty(s.toolOutput)...)
	}

	if s.interrupted {
		lines = append(lines, "[interrupted]")
	}

	return lines
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func splitPreservingEmpty(text string) []string {
	if text == "" {
		return []string{""}
	}

	return strings.Split(text, "\n")
}
