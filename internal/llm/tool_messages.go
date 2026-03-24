package llm

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
)

var ErrToolStepRequired = errors.New("runtime tool step is required")
var ErrToolCallsRequired = errors.New("runtime tool calls are required")
var ErrToolExecutionCountMismatch = errors.New("tool execution count exceeds tool call count")
var ErrToolFailureCallNotFound = errors.New("tool failure call not found in tool calls")

// BuildToolStepMessages 把 runtime 的 tool step 转成按历史追加顺序排列的 llm 消息。
// 返回值固定是 assistant tool-call message 在前，tool result message 在后。
func BuildToolStepMessages(step runtime.StepResult) ([]Message, error) {
	if step.Kind != runtime.StepKindToolCalls {
		return nil, fmt.Errorf("%w: %q", ErrToolStepRequired, step.Kind)
	}
	if len(step.ToolCalls) == 0 {
		return nil, ErrToolCallsRequired
	}
	if len(step.ToolExecutions) > len(step.ToolCalls) {
		return nil, fmt.Errorf(
			"%w: %d executions for %d calls",
			ErrToolExecutionCountMismatch,
			len(step.ToolExecutions),
			len(step.ToolCalls),
		)
	}

	messageCalls := buildMessageToolCalls(step.ToolCalls)
	messages := make([]Message, 0, 1+len(step.ToolExecutions)+1)
	messages = append(messages, Message{
		Role:      RoleAssistant,
		ToolCalls: messageCalls,
	})

	// 当前 runtime 按声明顺序串行执行工具，因此成功执行一定是前缀窗口。
	for index, execution := range step.ToolExecutions {
		messages = append(messages, Message{
			Role:       RoleTool,
			ToolCallID: messageCalls[index].ID,
			Content:    formatToolExecutionContent(execution),
		})
	}

	if step.ToolFailure != nil {
		failureIndex := findToolCallIndex(step.ToolCalls, step.ToolFailure.Call)
		if failureIndex < 0 {
			return nil, fmt.Errorf("%w: %s", ErrToolFailureCallNotFound, step.ToolFailure.Call.Name)
		}

		messages = append(messages, Message{
			Role:       RoleTool,
			ToolCallID: messageCalls[failureIndex].ID,
			Content:    formatToolFailureContent(*step.ToolFailure),
		})
	}

	return messages, nil
}

func buildMessageToolCalls(calls []runtime.ToolCall) []ToolCall {
	messageCalls := make([]ToolCall, 0, len(calls))
	for index, call := range calls {
		messageCalls = append(messageCalls, ToolCall{
			ID:        toolCallID(index),
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}

	return messageCalls
}

func toolCallID(index int) string {
	return fmt.Sprintf("call_%d", index+1)
}

func findToolCallIndex(calls []runtime.ToolCall, target runtime.ToolCall) int {
	for index, call := range calls {
		if call == target {
			return index
		}
	}

	return -1
}

func formatToolExecutionContent(execution runtime.ToolExecution) string {
	if execution.Stdout != "" || execution.Stderr != "" || execution.ExitCode != 0 {
		sections := make([]string, 0, 3)
		if stdout := strings.TrimRight(execution.Stdout, "\n"); stdout != "" {
			sections = append(sections, "stdout:\n"+stdout)
		}
		if stderr := strings.TrimRight(execution.Stderr, "\n"); stderr != "" {
			sections = append(sections, "stderr:\n"+stderr)
		}
		sections = append(sections, fmt.Sprintf("exit_code: %d", execution.ExitCode))

		return strings.Join(sections, "\n\n")
	}

	if execution.Output != "" {
		return execution.Output
	}

	return "tool output is empty."
}

func formatToolFailureContent(toolErr runtime.ToolExecutionError) string {
	lines := []string{
		fmt.Sprintf("error: %v", toolErr.Err),
	}

	switch {
	case runtime.IsTemporary(toolErr):
		lines = append(lines, "failure_kind: temporary")
	case runtime.IsRefused(toolErr):
		lines = append(lines, "failure_kind: refused")
	default:
		lines = append(lines, "failure_kind: error")
	}

	return strings.Join(lines, "\n")
}
