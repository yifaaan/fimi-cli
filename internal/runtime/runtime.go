package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
)

const DefaultReplyHistoryTurnLimit = 4
const DefaultMaxStepsPerRun = 100
const DefaultMaxRetriesPerStep = 3

var ErrUnknownStepKind = errors.New("unknown runtime step kind")
var ErrUnknownStepStatus = errors.New("unknown runtime step status")

// Config 定义 runtime 自己关心的最小运行参数。
type Config struct {
	ReplyHistoryTurnLimit int
	MaxStepsPerRun        int
	MaxRetriesPerStep     int
}

// DefaultConfig 返回 runtime 的默认参数。
func DefaultConfig() Config {
	return Config{
		ReplyHistoryTurnLimit: DefaultReplyHistoryTurnLimit,
		MaxStepsPerRun:        DefaultMaxStepsPerRun,
		MaxRetriesPerStep:     DefaultMaxRetriesPerStep,
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
	Call     ToolCall
	Output   string
	Stdout   string
	Stderr   string
	ExitCode int
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
		records = append(records, contextstore.NewToolResultRecord(exec.Call.ID, exec.Output))
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

// StreamingEngine 是支持流式的 engine 扩展接口。
// runtime 在运行时通过类型断言检测 engine 是否支持流式。
// 这是 Go 的接口隔离原则：不强制所有 Engine 实现都支持流式。
type StreamingEngine interface {
	Engine
	// ReplyStream 执行流式 LLM 调用，实时发送事件到 sink。
	// sink 已有 Emit 方法，天然适合接收流式事件。
	ReplyStream(ctx context.Context, input ReplyInput, sink runtimeevents.Sink) (AssistantReply, error)
}

// StepConfig 持有每个 step 需要的不可变 engine 参数。
// 用户 prompt 不在这里，因为它已经在 Run() 开始时追加到 history。
type StepConfig struct {
	Model        string
	SystemPrompt string
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine       Engine
	toolExecutor ToolExecutor
	eventSink    runtimeevents.Sink
	config       Config
	runStepFn    func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error)
	advanceRunFn func(ctx context.Context, store contextstore.Context, result Result, stepResult StepResult) (Result, bool, error)
}

// New 创建最小 runtime runner。
// 调用方必须显式注入 engine，避免 core runtime 反向依赖具体适配器。
func New(engine Engine, cfg Config) Runner {
	return NewWithToolExecutorAndEvents(engine, nil, nil, cfg)
}

// NewWithToolExecutor 创建带显式工具执行边界的 runtime runner。
func NewWithToolExecutor(engine Engine, toolExecutor ToolExecutor, cfg Config) Runner {
	return NewWithToolExecutorAndEvents(engine, toolExecutor, nil, cfg)
}

// NewWithToolExecutorAndEvents 创建带工具执行器和事件输出边界的 runtime runner。
func NewWithToolExecutorAndEvents(
	engine Engine,
	toolExecutor ToolExecutor,
	eventSink runtimeevents.Sink,
	cfg Config,
) Runner {
	if engine == nil {
		engine = missingEngine{}
	}
	if toolExecutor == nil {
		toolExecutor = NoopToolExecutor{}
	}
	if eventSink == nil {
		eventSink = runtimeevents.NoopSink{}
	}
	if cfg.ReplyHistoryTurnLimit <= 0 {
		cfg.ReplyHistoryTurnLimit = DefaultReplyHistoryTurnLimit
	}
	if cfg.MaxStepsPerRun <= 0 {
		cfg.MaxStepsPerRun = DefaultMaxStepsPerRun
	}
	if cfg.MaxRetriesPerStep <= 0 {
		cfg.MaxRetriesPerStep = DefaultMaxRetriesPerStep
	}

	return Runner{
		engine:       engine,
		toolExecutor: toolExecutor,
		eventSink:    eventSink,
		config:       cfg,
	}
}

// WithEventSink 返回一个绑定新事件输出边界的 Runner 副本。
// 这里用值语义返回副本，避免 UI/transport 在共享 Runner 上互相覆盖 sink。
func (r Runner) WithEventSink(eventSink runtimeevents.Sink) Runner {
	if eventSink == nil {
		eventSink = runtimeevents.NoopSink{}
	}

	r.eventSink = eventSink
	return r
}

// Run 执行当前最小 runtime 流程。
// ctx 是 Go 标准的取消/超时传递机制；store 是 fimi 的历史存储接口。
func (r Runner) Run(ctx context.Context, store contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{Status: RunStatusFinished}, nil
	}

	// 关键语义：用户 prompt 只在一次 run 的开始时追加到 history。
	// 后续 step 不再重复注入 prompt，而是完全基于增长的 history 驱动。
	userRecord := contextstore.NewUserTextRecord(prompt)
	if err := store.Append(userRecord); err != nil {
		return Result{Status: RunStatusFailed}, fmt.Errorf("append runtime record: %w", err)
	}

	result := Result{
		Status:     RunStatusFinished,
		UserRecord: &userRecord,
		Steps:      make([]StepResult, 0, 1),
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
	var err error
	for stepNo := 1; stepNo <= r.config.MaxStepsPerRun; stepNo++ {
		// 在每个 step 前检查是否被取消
		if ctx.Err() != nil {
			return r.interruptedResult(ctx, result)
		}
		if err := r.emitEvent(ctx, runtimeevents.StepBegin{Number: stepNo}); err != nil {
			result.Status = RunStatusFailed
			return result, err
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
		if finished {
			result.Status = RunStatusFinished
			return result, nil
		}
	}

	result.Status = RunStatusMaxSteps
	return result, nil
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
	// 这里沿用 Python 参考实现的语义：
	// MaxRetriesPerStep 表示当前 step 的最大尝试次数，而不是"额外重试次数"。
	for attempt := 1; attempt <= r.config.MaxRetriesPerStep; attempt++ {
		// 每次重试前检查是否被取消
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

	// 检查 engine 是否支持流式，以及是否有 eventSink
	// 这是 Go 的"能力检测"模式：通过类型断言检查可选能力
	if streamingEngine, ok := r.engine.(StreamingEngine); ok && r.eventSink != nil {
		assistantReply, err = streamingEngine.ReplyStream(ctx, replyInput, r.eventSink)
	} else {
		assistantReply, err = r.engine.Reply(ctx, replyInput)
	}

	if err != nil {
		return StepResult{}, fmt.Errorf("build assistant reply: %w", err)
	}

	// 持久化 token 使用量
	if assistantReply.Usage.TotalTokens > 0 {
		if err := store.AppendUsage(assistantReply.Usage.TotalTokens); err != nil {
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
		}, nil
	}

	records := []contextstore.TextRecord{
		contextstore.NewAssistantTextRecord(assistantReply.Text),
	}
	for _, record := range records {
		if err := store.Append(record); err != nil {
			return StepResult{}, fmt.Errorf("append runtime record: %w", err)
		}
	}

	return StepResult{
		Status:          StepStatusFinished,
		Kind:            StepKindFinished,
		AssistantText:   assistantReply.Text,
		AppendedRecords: records,
		Usage:           assistantReply.Usage,
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
				for _, record := range stepResult.BuildToolStepRecords() {
					if appendErr := store.Append(record); appendErr != nil {
						return Result{}, false, fmt.Errorf("append tool failure record: %w", appendErr)
					}
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
		for _, record := range stepResult.BuildToolStepRecords() {
			if appendErr := store.Append(record); appendErr != nil {
				return Result{}, false, fmt.Errorf("append tool step record: %w", appendErr)
			}
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
	if strings.TrimSpace(stepResult.AssistantText) != "" {
		if err := r.emitEvent(ctx, runtimeevents.TextPart{Text: stepResult.AssistantText}); err != nil {
			return err
		}
	}

	for _, call := range stepResult.ToolCalls {
		if err := r.emitEvent(ctx, runtimeevents.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		}); err != nil {
			return err
		}
	}

	for _, exec := range stepResult.ToolExecutions {
		if err := r.emitEvent(ctx, runtimeevents.ToolResult{
			ToolCallID: exec.Call.ID,
			ToolName:   exec.Call.Name,
			Output:     exec.Output,
			IsError:    false,
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

	return r.emitEvent(ctx, runtimeevents.StatusUpdate{
		Status: buildStatusSnapshot(store),
	})
}

func (r Runner) emitEvent(ctx context.Context, event runtimeevents.Event) error {
	sink := r.eventSink
	if sink == nil {
		sink = runtimeevents.NoopSink{}
	}

	if err := sink.Emit(ctx, event); err != nil {
		return fmt.Errorf("emit runtime event %q: %w", event.Kind(), err)
	}

	return nil
}

func buildStatusSnapshot(store contextstore.Context) runtimeevents.StatusSnapshot {
	// 当前 runtime 还没有 provider-specific context window，
	// 因此这里只先稳定事件形状，ContextUsage 暂时保留零值。
	return runtimeevents.StatusSnapshot{}
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
