package runtime

import (
	"context"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

func (r Runner) emitStepEvents(
	ctx context.Context,
	store contextstore.Context,
	stepResult StepResult,
) error {
	if !stepResult.TextStreamed && strings.TrimSpace(stepResult.AssistantText) != "" {
		if err := r.emitEvent(ctx, runtimeevents.TextPart{Text: stepResult.AssistantText}); err != nil {
			return err
		}
	}

	for _, call := range stepResult.ToolCalls {
		if err := r.emitEvent(ctx, runtimeevents.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Subtitle:  ToolCallSubtitle(call),
			Arguments: call.Arguments,
		}); err != nil {
			return err
		}
	}

	for _, exec := range stepResult.ToolExecutions {
		if err := r.emitEvent(ctx, runtimeevents.ToolResult{
			ToolCallID:    exec.Call.ID,
			ToolName:      exec.Call.Name,
			Output:        exec.Output,
			DisplayOutput: firstNonEmptyDisplay(exec.DisplayOutput, exec.Output),
			Content:       exec.Content,
			IsError:       false,
		}); err != nil {
			return err
		}
	}

	if stepResult.ToolFailure != nil {
		if err := r.emitEvent(ctx, runtimeevents.ToolResult{
			ToolCallID: stepResult.ToolFailure.Call.ID,
			ToolName:   stepResult.ToolFailure.Call.Name,
			Output:     formatToolFailureContent(stepResult.ToolFailure),
			IsError:    true,
		}); err != nil {
			return err
		}
	}

	if err := r.emitEvent(ctx, runtimeevents.StatusUpdate{
		Status: buildStatusSnapshotWithWindow(store, r.config.ContextWindowTokens, nil),
	}); err != nil {
		return err
	}

	return nil
}

func firstNonEmptyDisplay(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

func (r Runner) emitRetryStatusUpdate(
	ctx context.Context,
	store contextstore.Context,
	retry *runtimeevents.RetryStatus,
) error {
	return r.emitEvent(ctx, runtimeevents.StatusUpdate{
		Status: buildStatusSnapshotWithWindow(store, r.config.ContextWindowTokens, retry),
	})
}

func (r Runner) emitEvent(ctx context.Context, event runtimeevents.Event) error {
	w, ok := wire.Current(ctx)
	if !ok {
		return nil
	}

	if err := w.Send(wire.EventMessage{Event: event}); err != nil {
		return fmt.Errorf("send runtime event %q: %w", event.Kind(), err)
	}

	return nil
}

// buildStatusSnapshotWithWindow 计算当前 step 的上下文占用率近似值。
// 这里故意使用“最后一次 LLM 调用的 total_tokens / context window”：
// - 它不等于累计 usage
// - 但足够表达“当前这轮请求离窗口上限还有多远”
func buildStatusSnapshotWithWindow(
	store contextstore.Context,
	contextWindowTokens int,
	retry *runtimeevents.RetryStatus,
) runtimeevents.StatusSnapshot {
	snapshot := runtimeevents.StatusSnapshot{
		Retry: retry,
	}
	if contextWindowTokens <= 0 {
		return snapshot
	}

	lastUsage, err := store.ReadUsage()
	if err != nil || lastUsage <= 0 {
		return snapshot
	}

	snapshot.ContextUsage = float64(lastUsage) / float64(contextWindowTokens)
	return snapshot
}

func (r Runner) interruptedResult(ctx context.Context, result Result) (Result, error) {
	result.Status = RunStatusInterrupted
	if err := r.emitEvent(ctx, runtimeevents.StepInterrupted{}); err != nil {
		return result, err
	}

	return result, ctx.Err()
}
