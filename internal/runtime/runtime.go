package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

const DefaultReplyHistoryTurnLimit = 4
const DefaultMaxStepsPerRun = 100
const DefaultMaxAdditionalRetriesPerStep = 3
const checkpointPromptPreviewMaxLen = 60
const retryBackoffBaseDelay = 200 * time.Millisecond
const retryBackoffMaxDelay = 2 * time.Second
const retryBackoffJitterWindow = 100 * time.Millisecond

var ErrUnknownStepKind = errors.New("unknown runtime step kind")
var ErrUnknownStepStatus = errors.New("unknown runtime step status")

// Config 定义 runtime 自己关心的最小运行参数。
type Config struct {
	ReplyHistoryTurnLimit       int
	MaxStepsPerRun              int
	MaxAdditionalRetriesPerStep int
	ContextWindowTokens         int
}

// DefaultConfig 返回 runtime 的默认参数。
func DefaultConfig() Config {
	return Config{
		ReplyHistoryTurnLimit:       DefaultReplyHistoryTurnLimit,
		MaxStepsPerRun:              DefaultMaxStepsPerRun,
		MaxAdditionalRetriesPerStep: DefaultMaxAdditionalRetriesPerStep,
	}
}

// Input 表示单次 runtime 执行的最小输入。
type Input struct {
	Prompt       string
	Model        string
	SystemPrompt string
}

// ReplyInput 表示 runtime 传给 engine 的富化输入。
// runtime 把 history 放在内部边界上，避免调用方自己拼装上下文。
// 注意：用户 prompt 已经由 Run() 追加到 history，因此这里不再单独传 Prompt。
type ReplyInput struct {
	Model        string
	SystemPrompt string
	History      []contextstore.TextRecord
}

// AssistantReply 表示 engine 返回给 runtime 的结构化 assistant 回复。
// 当 ToolCalls 不为空时，runtime 需要执行工具调用而不是直接结束。
type AssistantReply struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage // token 使用量
}

// Usage 表示单次 LLM 调用的 token 使用量。
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Result 表示单次 runtime 追加到 history 的记录。
type Result struct {
	Status     RunStatus
	UserRecord *contextstore.TextRecord
	Steps      []StepResult
}

// RunStatus 表示一次 runtime.Run 的结束状态。
type RunStatus string

const (
	RunStatusFinished    RunStatus = "finished"
	RunStatusMaxSteps    RunStatus = "max_steps"
	RunStatusFailed      RunStatus = "failed"
	RunStatusInterrupted RunStatus = "interrupted"
)

// StepKind 表示单个 runtime step 当前产出的类型。
type StepKind string

const (
	StepKindFinished  StepKind = "finished"
	StepKindToolCalls StepKind = "tool_calls"
)

// StepStatus 表示当前 step 的推进结果：已完成、失败，或需要 runtime 继续推进。
type StepStatus string

const (
	StepStatusFinished   StepStatus = "finished"
	StepStatusFailed     StepStatus = "failed"
	StepStatusIncomplete StepStatus = "incomplete"
)

// ToolCall 描述 runtime 下一步需要执行的工具调用。
// ID 是模型返回的唯一标识，用于关联 tool result。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolExecution 表示 runtime 已经把某个工具调用交给执行器消费。
// Output 先保留给通用文本工具使用；命令类工具再补充 stdout/stderr/exit_code。
type ToolExecution struct {
	Call          ToolCall
	Output        string
	DisplayOutput string
	Content       []runtimeevents.RichContent
	Stdout        string
	Stderr        string
	ExitCode      int
}

// ToolExecutionError 表示某个 tool call 在 runtime 推进阶段执行失败。
// 它保留失败的 call，方便上层判断是哪个工具中断了本次 run。
type ToolExecutionError struct {
	Call ToolCall
	Err  error
}

func (e ToolExecutionError) Error() string {
	return fmt.Sprintf("execute tool call %q: %v", e.Call.Name, e.Err)
}

func (e ToolExecutionError) Unwrap() error {
	return e.Err
}

// ToolExecutor 定义 runtime 消费工具调用的最小边界。
// runtime 只关心”调用有没有被处理”，不关心具体 bash/file/web 细节。
type ToolExecutor interface {
	Execute(ctx context.Context, call ToolCall) (ToolExecution, error)
}

// DMailer is the interface for the D-Mail state machine.
// Defined here to avoid runtime importing the dmail package directly.
type DMailer interface {
	// Fetch retrieves and clears the pending D-Mail, if any.
	// Returns (message, checkpointID, true) if a D-Mail was pending,
	// or ("", 0, false) if none was pending.
	Fetch() (message string, checkpointID int, ok bool)
	// SetCheckpointCount updates the known number of checkpoints.
	SetCheckpointCount(n int)
}

// StepResult 表示单个 runtime step 的结构化结果。
// 先同时保留追加记录和平铺结果，方便上层渐进迁移。
type StepResult struct {
	Status          StepStatus
	Kind            StepKind
	AssistantText   string
	AppendedRecords []contextstore.TextRecord
	ToolCalls       []ToolCall
	ToolExecutions  []ToolExecution
	ToolFailure     *ToolExecutionError
	Usage           Usage // 本次 step 的 token 使用量
	TextStreamed    bool  // 文本是否已通过流式发送（用于避免重复打印）
}

// BuildToolStepRecords 构造工具调用步骤需要追加到 history 的记录。
// 返回 assistant 消息（带 tool_calls）和每个 tool 的 result 记录。
func (s StepResult) BuildToolStepRecords() []contextstore.TextRecord {
	if s.Kind != StepKindToolCalls || len(s.ToolCalls) == 0 {
		return nil
	}

	records := make([]contextstore.TextRecord, 0, 1+len(s.ToolCalls))

	// 构造 assistant 消息，包含 tool_calls
	toolCallsJSON, _ := json.Marshal(s.ToolCalls)
	records = append(records, contextstore.TextRecord{
		Role:          contextstore.RoleAssistant,
		Content:       s.AssistantText,
		ToolCallsJSON: string(toolCallsJSON),
	})

	// 构造每个 tool result 记录
	for _, exec := range s.ToolExecutions {
		records = append(records, contextstore.NewToolResultRecordWithDisplay(exec.Call.ID, exec.Output, exec.DisplayOutput))
	}

	// 如果有工具执行失败，也要写入失败的 tool result
	if s.ToolFailure != nil {
		content := formatToolFailureContent(s.ToolFailure)
		records = append(records, contextstore.NewToolResultRecord(s.ToolFailure.Call.ID, content))
	}

	return records
}

// formatToolFailureContent 格式化工具执行失败的内容，使其对模型可见。
func formatToolFailureContent(err *ToolExecutionError) string {
	failureKind := "error"
	if IsTemporary(err) {
		failureKind = "temporary"
	} else if IsRefused(err) {
		failureKind = "refused"
	}

	return fmt.Sprintf("tool execution failed (failure_kind: %s): %s", failureKind, err.Err.Error())
}

// Engine 负责为 runtime 生成 assistant 回复文本。
// 这里先保持最小接口，后面再扩展为真正的模型调用边界。
type Engine interface {
	Reply(ctx context.Context, input ReplyInput) (AssistantReply, error)
}

// StreamHandler handles low-level LLM stream events for runtime.
type StreamHandler interface {
	HandleStreamEvent(ctx context.Context, event any) error
}

// StreamHandlerFunc adapts a function into a StreamHandler.
type StreamHandlerFunc func(ctx context.Context, event any) error

func (f StreamHandlerFunc) HandleStreamEvent(ctx context.Context, event any) error {
	return f(ctx, event)
}

// StreamingEngine 是支持流式的 engine 扩展接口。
// runtime 在运行时通过类型断言检测 engine 是否支持流式。
// 这是 Go 的接口隔离原则：不强制所有 Engine 实现都支持流式。
type StreamingEngine interface {
	Engine
	ReplyStream(ctx context.Context, input ReplyInput, handler StreamHandler) (AssistantReply, error)
}

// StepConfig 持有每个 step 需要的不可变 engine 参数。
// 用户 prompt 不在这里，因为它已经在 Run() 开始时追加到 history。
type StepConfig struct {
	Model        string
	SystemPrompt string
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine               Engine
	toolExecutor         ToolExecutor
	dmailer              DMailer
	config               Config
	runStepFn            func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error)
	advanceRunFn         func(ctx context.Context, store contextstore.Context, result Result, stepResult StepResult) (Result, bool, error)
	retryBackoffDelayFn  func(attempt int) time.Duration
	retrySleepFn         func(ctx context.Context, delay time.Duration) error
	shieldContextWriteFn func(ctx context.Context, write func() error) error
}

// New 创建最小 runtime runner。
// 调用方必须显式注入 engine，避免 core runtime 反向依赖具体适配器。
func New(engine Engine, cfg Config) Runner {
	return NewWithToolExecutor(engine, nil, cfg)
}

// NewWithToolExecutor 创建带显式工具执行边界的 runtime runner。
func NewWithToolExecutor(engine Engine, toolExecutor ToolExecutor, cfg Config) Runner {
	if engine == nil {
		engine = missingEngine{}
	}
	if toolExecutor == nil {
		toolExecutor = NoopToolExecutor{}
	}
	if cfg.ReplyHistoryTurnLimit <= 0 {
		cfg.ReplyHistoryTurnLimit = DefaultReplyHistoryTurnLimit
	}
	if cfg.MaxStepsPerRun <= 0 {
		cfg.MaxStepsPerRun = DefaultMaxStepsPerRun
	}
	if cfg.MaxAdditionalRetriesPerStep < 0 {
		cfg.MaxAdditionalRetriesPerStep = 0
	}

	return Runner{
		engine:       engine,
		toolExecutor: toolExecutor,
		config:       cfg,
	}
}

// WithDMailer returns a Runner copy with the D-Mail state machine attached.
func (r Runner) WithDMailer(dmailer DMailer) Runner {
	r.dmailer = dmailer
	return r
}

// Run 执行当前最小 runtime 流程。
// ctx 是 Go 标准的取消/超时传递机制；store 是 fimi 的历史存储接口。
func (r Runner) Run(ctx context.Context, store contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{Status: RunStatusFinished}, nil
	}

	userRecord := contextstore.NewUserTextRecord(prompt)
	result := Result{
		Status:     RunStatusFinished,
		UserRecord: &userRecord,
		Steps:      make([]StepResult, 0, 1),
	}

	var checkpointID int
	err := r.shieldContextWrite(ctx, func() error {
		var err error
		checkpointID, err = store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			PromptPreview: checkpointPromptPreview(prompt),
		})
		if err != nil {
			return err
		}

		// When D-Mail is enabled, inject visible checkpoint markers so the LLM
		// can target them with send_dmail. Without this, checkpoint IDs are invisible.
		if r.dmailer != nil {
			r.dmailer.SetCheckpointCount(checkpointID + 1)
			if err := store.Append(contextstore.NewUserTextRecord(
				fmt.Sprintf("<system>CHECKPOINT %d</system>", checkpointID),
			)); err != nil {
				return err
			}
		}

		// 关键语义：用户 prompt 只在一次 run 的开始时追加到 history。
		// 后续 step 不再重复注入 prompt，而是完全基于增长的 history 驱动。
		return store.Append(userRecord)
	})
	if err != nil {
		if isInterruptedError(err) {
			return r.interruptedResult(ctx, result)
		}
		return Result{Status: RunStatusFailed}, fmt.Errorf("persist prompt boundary: %w", err)
	}
	cfg := StepConfig{Model: input.Model, SystemPrompt: input.SystemPrompt}

	runStep := r.runStepFn
	if runStep == nil {
		runStep = r.runStep
	}
	advanceRun := r.advanceRunFn
	if advanceRun == nil {
		advanceRun = r.advanceRun
	}
	var finished bool
	for stepNo := 1; stepNo <= r.config.MaxStepsPerRun; stepNo++ {
		// 在每个 step 前检查是否被取消
		if ctx.Err() != nil {
			return r.interruptedResult(ctx, result)
		}
		if err := r.emitEvent(ctx, runtimeevents.StepBegin{Number: stepNo}); err != nil {
			result.Status = RunStatusFailed
			return result, err
		}

		// Per-step checkpoint: only needed when D-Mail is enabled,
		// because D-Mail needs mid-turn rollback targets.
		if r.dmailer != nil {
			err := r.shieldContextWrite(ctx, func() error {
				stepCheckpointID, cpErr := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
				})
				if cpErr != nil {
					return cpErr
				}
				r.dmailer.SetCheckpointCount(stepCheckpointID + 1)
				return store.Append(contextstore.NewUserTextRecord(
					fmt.Sprintf("<system>CHECKPOINT %d</system>", stepCheckpointID),
				))
			})
			if err != nil {
				if isInterruptedError(err) {
					return r.interruptedResult(ctx, result)
				}
				return Result{Status: RunStatusFailed}, fmt.Errorf("persist step checkpoint: %w", err)
			}
		}

		var stepResult StepResult
		stepResult, err = r.runStepWithRetry(ctx, store, cfg, runStep)
		if err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			result.Status = RunStatusFailed
			return result, err
		}

		result, finished, err = advanceRun(ctx, store, result, stepResult)
		if err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			result.Status = RunStatusFailed
			return result, err
		}

		// Check for pending D-Mail after tool execution
		if !finished && r.dmailer != nil {
			if message, checkpointID, ok := r.dmailer.Fetch(); ok {
				err := r.shieldContextWrite(ctx, func() error {
					if _, revertErr := store.RevertToCheckpoint(checkpointID); revertErr != nil {
						return revertErr
					}

					newCheckpointID, cpErr := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
						CreatedAt: time.Now().UTC().Format(time.RFC3339),
					})
					if cpErr != nil {
						return cpErr
					}
					r.dmailer.SetCheckpointCount(newCheckpointID + 1)
					if err := store.Append(contextstore.NewUserTextRecord(
						fmt.Sprintf("<system>CHECKPOINT %d</system>", newCheckpointID),
					)); err != nil {
						return err
					}

					dmailContent := fmt.Sprintf("<system>D-Mail received: %s</system>\n\nRead the D-Mail above carefully. Act on the information it contains. Do NOT mention the D-Mail mechanism or time travel to the user.", message)
					return store.Append(contextstore.NewUserTextRecord(dmailContent))
				})
				if err != nil {
					if isInterruptedError(err) {
						return r.interruptedResult(ctx, result)
					}
					return Result{Status: RunStatusFailed}, fmt.Errorf("persist rollback context: %w", err)
				}

				// Reset step count and continue from the reverted state
				stepNo = 0
				continue
			}
		}

		if finished {
			result.Status = RunStatusFinished
			return result, nil
		}
	}

	result.Status = RunStatusMaxSteps
	return result, nil
}

func checkpointPromptPreview(prompt string) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if len(preview) <= checkpointPromptPreviewMaxLen {
		return preview
	}

	return preview[:checkpointPromptPreviewMaxLen-3] + "..."
}

func isInterruptedError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// IsRetryable 判断某个错误是否允许 runtime 重新尝试当前 step。
// 这里约定：只要错误链里存在实现 Retryable() bool 的值，并返回 true，就视为可重试。
func IsRetryable(err error) bool {
	type retryable interface {
		Retryable() bool
	}

	var target retryable
	if !errors.As(err, &target) {
		return false
	}

	return target.Retryable()
}

// IsRefused 判断某个错误是否表示“请求被系统拒绝执行”。
// 这类错误通常发生在真正副作用发生之前，例如越界路径或无效参数。
func IsRefused(err error) bool {
	type refused interface {
		Refused() bool
	}

	var target refused
	if !errors.As(err, &target) {
		return false
	}

	return target.Refused()
}

// IsTemporary 判断某个错误是否表示“执行环境暂时失败”。
// temporary 只描述错误性质，不等价于 runtime 现在就应该自动重试。
func IsTemporary(err error) bool {
	type temporary interface {
		Temporary() bool
	}

	var target temporary
	if !errors.As(err, &target) {
		return false
	}

	return target.Temporary()
}

func (r Runner) runStepWithRetry(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
	runStep func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error),
) (StepResult, error) {
	var lastErr error
	retryStatusEmitted := false
	maxAttempts := 1 + r.config.MaxAdditionalRetriesPerStep
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return StepResult{}, ctx.Err()
		}

		stepResult, err := runStep(ctx, store, cfg)
		if err == nil {
			return stepResult, nil
		}
		if !IsRetryable(err) {
			return StepResult{}, err
		}

		lastErr = err
		if attempt == maxAttempts {
			break
		}

		delay := r.retryBackoffDelay(attempt)
		if emitErr := r.emitRetryStatusUpdate(ctx, store, &runtimeevents.RetryStatus{
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			NextDelayMS: delay.Milliseconds(),
		}); emitErr != nil {
			return StepResult{}, emitErr
		}
		retryStatusEmitted = true
		if sleepErr := r.retrySleep(ctx, delay); sleepErr != nil {
			return StepResult{}, sleepErr
		}
	}

	if retryStatusEmitted {
		if clearErr := r.emitRetryStatusUpdate(ctx, store, nil); clearErr != nil {
			return StepResult{}, clearErr
		}
	}

	return StepResult{}, lastErr
}

func (r Runner) runStep(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
) (StepResult, error) {
	history, err := store.ReadRecentTurns(r.config.ReplyHistoryTurnLimit)
	if err != nil {
		return StepResult{}, fmt.Errorf("read runtime history: %w", err)
	}

	// 构建 ReplyInput
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

	// 持久化 token 使用量
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
	case StepKindToolCalls:
		if len(stepResult.ToolCalls) == 0 {
			return Result{}, false, fmt.Errorf("step kind %q requires at least one tool call", stepResult.Kind)
		}

		toolExecutions, err := r.executeToolCalls(ctx, stepResult.ToolCalls)
		if err != nil {
			var toolErr ToolExecutionError
			if errors.As(err, &toolErr) {
				stepResult.Status = StepStatusFailed
				stepResult.ToolFailure = &toolErr
				// 在返回错误前，先把 tool records 写入 history
				if err := r.shieldContextWrite(ctx, func() error {
					for _, record := range stepResult.BuildToolStepRecords() {
						if appendErr := store.Append(record); appendErr != nil {
							return appendErr
						}
					}
					return nil
				}); err != nil {
					return Result{}, false, fmt.Errorf("append tool failure record: %w", err)
				}
				result.Steps = append(result.Steps, stepResult)
				if emitErr := r.emitStepEvents(ctx, store, stepResult); emitErr != nil {
					return result, false, emitErr
				}
				return result, false, err
			}

			return Result{}, false, err
		}
		stepResult.ToolExecutions = toolExecutions
		// 成功执行后，把 tool records 写入 history
		if err := r.shieldContextWrite(ctx, func() error {
			for _, record := range stepResult.BuildToolStepRecords() {
				if appendErr := store.Append(record); appendErr != nil {
					return appendErr
				}
			}
			return nil
		}); err != nil {
			return Result{}, false, fmt.Errorf("append tool step record: %w", err)
		}
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepKind, stepResult.Kind)
	}

	result.Steps = append(result.Steps, stepResult)
	if err := r.emitStepEvents(ctx, store, stepResult); err != nil {
		return result, false, err
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

func (r Runner) executeToolCalls(ctx context.Context, calls []ToolCall) ([]ToolExecution, error) {
	toolExecutions := make([]ToolExecution, 0, len(calls))
	for _, call := range calls {
		execution, err := r.toolExecutor.Execute(ctx, call)
		if err != nil {
			// 当前阶段先把工具执行失败视为本次 run 的终止条件。
			// 后续如果要把失败反馈给模型，再在这里调整为结构化 step 产物。
			return nil, ToolExecutionError{
				Call: call,
				Err:  err,
			}
		}

		toolExecutions = append(toolExecutions, execution)
	}

	return toolExecutions, nil
}

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

func (r Runner) retryBackoffDelay(attempt int) time.Duration {
	if r.retryBackoffDelayFn != nil {
		return r.retryBackoffDelayFn(attempt)
	}

	return calculateRetryBackoffDelay(attempt, retryBackoffJitter())
}

func calculateRetryBackoffDelay(attempt int, jitter time.Duration) time.Duration {
	baseDelay := retryBackoffBaseDelayForAttempt(attempt)
	clampedJitter := clampRetryBackoffJitter(jitter)
	return clampRetryBackoffDelay(baseDelay + clampedJitter)
}

func retryBackoffBaseDelayForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return retryBackoffBaseDelay
	}

	baseDelay := retryBackoffBaseDelay
	for i := 1; i < attempt; i++ {
		if baseDelay >= retryBackoffMaxDelay/2 {
			return retryBackoffMaxDelay
		}
		baseDelay *= 2
	}

	return clampRetryBackoffDelay(baseDelay)
}

func retryBackoffJitter() time.Duration {
	window := retryBackoffJitterWindow
	if window <= 0 {
		return 0
	}

	return time.Duration(time.Now().UnixNano() % int64(window+1))
}

func clampRetryBackoffJitter(jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return 0
	}
	if jitter > retryBackoffJitterWindow {
		return retryBackoffJitterWindow
	}
	return jitter
}

func clampRetryBackoffDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return retryBackoffBaseDelay
	}
	if delay > retryBackoffMaxDelay {
		return retryBackoffMaxDelay
	}
	return delay
}

func (r Runner) retrySleep(ctx context.Context, delay time.Duration) error {
	if r.retrySleepFn != nil {
		return r.retrySleepFn(ctx, delay)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r Runner) shieldContextWrite(ctx context.Context, write func() error) error {
	if r.shieldContextWriteFn != nil {
		return r.shieldContextWriteFn(ctx, write)
	}
	if err := write(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
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

func buildStatusSnapshot(store contextstore.Context) runtimeevents.StatusSnapshot {
	return buildStatusSnapshotWithWindow(store, 0, nil)
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

// NoopToolExecutor 是 runtime 当前默认的占位执行器。
// 它只把 tool call 标记为”已消费”，不做任何真实副作用。
type NoopToolExecutor struct{}

func (NoopToolExecutor) Execute(ctx context.Context, call ToolCall) (ToolExecution, error) {
	return ToolExecution{
		Call: call,
	}, nil
}

type missingEngine struct{}

func (missingEngine) Reply(ctx context.Context, input ReplyInput) (AssistantReply, error) {
	return AssistantReply{}, errors.New("runtime engine is required")
}
