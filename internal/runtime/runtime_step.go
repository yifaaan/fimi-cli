package runtime

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func (r Runner) runStep(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
) (StepResult, error) {
	history, err := store.ReadRecentTurns(r.config.ReplyHistoryTurnLimit)
	if err != nil {
		return StepResult{}, fmt.Errorf("read runtime history: %w", err)
	}

	replyInput := ReplyInput{
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
		History:      history,
	}

	var assistantReply AssistantReply
	var textStreamed bool

	if streamingEngine, ok := r.engine.(StreamingEngine); ok {
		handler := StreamHandlerFunc(func(ctx context.Context, event any) error {
			switch ev := event.(type) {
			case interface{ TextDelta() string }:
				return r.emitEvent(ctx, runtimeevents.TextPart{Text: ev.TextDelta()})
			case interface{ ToolCallDelta() (string, string) }:
				toolCallID, delta := ev.ToolCallDelta()
				return r.emitEvent(ctx, runtimeevents.ToolCallPart{ToolCallID: toolCallID, Delta: delta})
			default:
				return nil
			}
		})
		assistantReply, err = streamingEngine.ReplyStream(ctx, replyInput, handler)
		textStreamed = true
	} else {
		assistantReply, err = r.engine.Reply(ctx, replyInput)
	}

	if err != nil {
		return StepResult{}, fmt.Errorf("build assistant reply: %w", err)
	}

	if assistantReply.Usage.TotalTokens > 0 {
		if err := r.shieldContextWrite(ctx, func() error {
			return store.AppendUsage(assistantReply.Usage.TotalTokens)
		}); err != nil {
			return StepResult{}, fmt.Errorf("append usage record: %w", err)
		}
	}

	if len(assistantReply.ToolCalls) > 0 {
		return StepResult{
			Status:        StepStatusIncomplete,
			Kind:          StepKindToolCalls,
			AssistantText: assistantReply.Text,
			ToolCalls:     assistantReply.ToolCalls,
			Usage:         assistantReply.Usage,
			TextStreamed:  textStreamed,
		}, nil
	}

	records := []contextstore.TextRecord{
		contextstore.NewAssistantTextRecord(assistantReply.Text),
	}
	if err := r.shieldContextWrite(ctx, func() error {
		for _, record := range records {
			if err := store.Append(record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return StepResult{}, fmt.Errorf("append runtime record: %w", err)
	}

	return StepResult{
		Status:          StepStatusFinished,
		Kind:            StepKindFinished,
		AssistantText:   assistantReply.Text,
		AppendedRecords: records,
		Usage:           assistantReply.Usage,
		TextStreamed:    textStreamed,
	}, nil
}

func (r Runner) advanceRun(
	ctx context.Context,
	store contextstore.Context,
	result Result,
	stepResult StepResult,
) (Result, bool, error) {
	switch stepResult.Kind {
	case StepKindFinished:
		result.Steps = append(result.Steps, stepResult)
		if err := r.emitStepEvents(ctx, store, stepResult); err != nil {
			return result, false, err
		}
	case StepKindToolCalls:
		if len(stepResult.ToolCalls) == 0 {
			return Result{}, false, fmt.Errorf("step kind %q requires at least one tool call", stepResult.Kind)
		}

		var err error
		result, err = r.advanceToolCallStep(ctx, store, result, stepResult)
		if err != nil {
			return result, false, err
		}
		stepResult = result.Steps[len(result.Steps)-1]
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepKind, stepResult.Kind)
	}

	switch stepResult.Status {
	case StepStatusFinished:
		return result, true, nil
	case StepStatusIncomplete:
		return result, false, nil
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepStatus, stepResult.Status)
	}
}

func (r Runner) advanceToolCallStep(
	ctx context.Context,
	store contextstore.Context,
	result Result,
	stepResult StepResult,
) (Result, error) {
	toolExecutions, err := r.executeToolCalls(ctx, stepResult.ToolCalls)
	if err != nil {
		var toolErr ToolExecutionError
		if !errors.As(err, &toolErr) {
			return Result{}, err
		}

		stepResult.Status = StepStatusFailed
		stepResult.ToolFailure = &toolErr
		if err := r.shieldContextWrite(ctx, func() error {
			for _, record := range stepResult.BuildToolStepRecords() {
				if appendErr := store.Append(record); appendErr != nil {
					return appendErr
				}
			}
			return nil
		}); err != nil {
			return Result{}, fmt.Errorf("append tool failure record: %w", err)
		}
		result.Steps = append(result.Steps, stepResult)
		if emitErr := r.emitStepEvents(ctx, store, stepResult); emitErr != nil {
			return result, emitErr
		}
		return result, err
	}

	stepResult.ToolExecutions = toolExecutions
	if err := r.shieldContextWrite(ctx, func() error {
		for _, record := range stepResult.BuildToolStepRecords() {
			if appendErr := store.Append(record); appendErr != nil {
				return appendErr
			}
		}
		return nil
	}); err != nil {
		return Result{}, fmt.Errorf("append tool step record: %w", err)
	}
	result.Steps = append(result.Steps, stepResult)
	if err := r.emitStepEvents(ctx, store, stepResult); err != nil {
		return result, err
	}

	return result, nil
}

func (r Runner) executeToolCalls(ctx context.Context, calls []ToolCall) ([]ToolExecution, error) {
	toolExecutions := make([]ToolExecution, 0, len(calls))
	for _, call := range calls {
		execution, err := r.toolExecutor.Execute(ctx, call)
		if err != nil {
			return nil, ToolExecutionError{
				Call: call,
				Err:  err,
			}
		}

		toolExecutions = append(toolExecutions, execution)
	}

	return toolExecutions, nil
}
