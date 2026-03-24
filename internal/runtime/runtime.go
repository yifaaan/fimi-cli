package runtime

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
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
// 这里把 history 放在内部边界上，避免调用方自己拼装上下文。
type ReplyInput struct {
	Prompt       string
	Model        string
	SystemPrompt string
	History      []contextstore.TextRecord
}

// Result 表示单次 runtime 追加到 history 的记录。
type Result struct {
	Status RunStatus
	Steps  []StepResult
}

// RunStatus 表示一次 runtime.Run 的结束状态。
type RunStatus string

const (
	RunStatusFinished RunStatus = "finished"
	RunStatusMaxSteps RunStatus = "max_steps"
	RunStatusFailed   RunStatus = "failed"
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
// 当前先保留最小结构，后面再补 schema 和参数类型。
type ToolCall struct {
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
// runtime 只关心“调用有没有被处理”，不关心具体 bash/file/web 细节。
type ToolExecutor interface {
	Execute(call ToolCall) (ToolExecution, error)
}

// StepResult 表示单个 runtime step 的结构化结果。
// 先同时保留追加记录和平铺结果，方便上层渐进迁移。
type StepResult struct {
	Status          StepStatus
	Kind            StepKind
	AppendedRecords []contextstore.TextRecord
	ToolCalls       []ToolCall
	ToolExecutions  []ToolExecution
	ToolFailure     *ToolExecutionError
}

// Engine 负责为 runtime 生成 assistant 回复文本。
// 这里先保持最小接口，后面再扩展为真正的模型调用边界。
type Engine interface {
	Reply(input ReplyInput) (string, error)
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine       Engine
	toolExecutor ToolExecutor
	config       Config
	runStepFn    func(ctx contextstore.Context, input Input, prompt string) (StepResult, error)
	advanceRunFn func(result Result, stepResult StepResult) (Result, bool, error)
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
	if cfg.MaxRetriesPerStep <= 0 {
		cfg.MaxRetriesPerStep = DefaultMaxRetriesPerStep
	}

	return Runner{
		engine:       engine,
		toolExecutor: toolExecutor,
		config:       cfg,
	}
}

// Run 执行当前最小 runtime 流程。
// 现在它还不调用真实模型，只协调 history 追加顺序和 engine 调用。
func (r Runner) Run(ctx contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{Status: RunStatusFinished}, nil
	}

	result := Result{
		Status: RunStatusFinished,
		Steps:  make([]StepResult, 0, 1),
	}
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
		var stepResult StepResult
		stepResult, err = r.runStepWithRetry(ctx, input, prompt, runStep)
		if err != nil {
			result.Status = RunStatusFailed
			return result, err
		}

		result, finished, err = advanceRun(result, stepResult)
		if err != nil {
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
	ctx contextstore.Context,
	input Input,
	prompt string,
	runStep func(ctx contextstore.Context, input Input, prompt string) (StepResult, error),
) (StepResult, error) {
	var lastErr error
	// 这里沿用 Python 参考实现的语义：
	// MaxRetriesPerStep 表示当前 step 的最大尝试次数，而不是“额外重试次数”。
	for attempt := 1; attempt <= r.config.MaxRetriesPerStep; attempt++ {
		stepResult, err := runStep(ctx, input, prompt)
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
	ctx contextstore.Context,
	input Input,
	prompt string,
) (StepResult, error) {
	history, err := ctx.ReadRecentTurns(r.config.ReplyHistoryTurnLimit)
	if err != nil {
		return StepResult{}, fmt.Errorf("read runtime history: %w", err)
	}

	assistantReply, err := r.engine.Reply(ReplyInput{
		Prompt:       prompt,
		Model:        input.Model,
		SystemPrompt: input.SystemPrompt,
		History:      history,
	})
	if err != nil {
		return StepResult{}, fmt.Errorf("build assistant reply: %w", err)
	}

	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord(prompt),
		contextstore.NewAssistantTextRecord(assistantReply),
	}
	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			return StepResult{}, fmt.Errorf("append runtime record: %w", err)
		}
	}

	return StepResult{
		Status:          StepStatusFinished,
		Kind:            StepKindFinished,
		AppendedRecords: records,
	}, nil
}

func (r Runner) advanceRun(
	result Result,
	stepResult StepResult,
) (Result, bool, error) {
	switch stepResult.Kind {
	case StepKindFinished:
	case StepKindToolCalls:
		if len(stepResult.ToolCalls) == 0 {
			return Result{}, false, fmt.Errorf("step kind %q requires at least one tool call", stepResult.Kind)
		}

		toolExecutions, err := r.executeToolCalls(stepResult.ToolCalls)
		if err != nil {
			var toolErr ToolExecutionError
			if errors.As(err, &toolErr) {
				stepResult.Status = StepStatusFailed
				stepResult.ToolFailure = &toolErr
				result.Steps = append(result.Steps, stepResult)
				return result, false, err
			}

			return Result{}, false, err
		}
		stepResult.ToolExecutions = toolExecutions
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepKind, stepResult.Kind)
	}

	result.Steps = append(result.Steps, stepResult)

	switch stepResult.Status {
	case StepStatusFinished:
		return result, true, nil
	case StepStatusIncomplete:
		return result, false, nil
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepStatus, stepResult.Status)
	}
}

func (r Runner) executeToolCalls(calls []ToolCall) ([]ToolExecution, error) {
	toolExecutions := make([]ToolExecution, 0, len(calls))
	for _, call := range calls {
		execution, err := r.toolExecutor.Execute(call)
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

// NoopToolExecutor 是 runtime 当前默认的占位执行器。
// 它只把 tool call 标记为“已消费”，不做任何真实副作用。
type NoopToolExecutor struct{}

func (NoopToolExecutor) Execute(call ToolCall) (ToolExecution, error) {
	return ToolExecution{
		Call: call,
	}, nil
}

type missingEngine struct{}

func (missingEngine) Reply(input ReplyInput) (string, error) {
	return "", errors.New("runtime engine is required")
}
