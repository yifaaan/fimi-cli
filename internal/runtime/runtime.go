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
)

const DefaultReplyHistoryTurnLimit = 4
const DefaultMaxStepsPerRun = 100
const DefaultMaxAdditionalRetriesPerStep = 3
const checkpointPromptPreviewMaxLen = 60

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

	toolCallsJSON, _ := json.Marshal(s.ToolCalls)
	records = append(records, contextstore.TextRecord{
		Role:          contextstore.RoleAssistant,
		Content:       s.AssistantText,
		ToolCallsJSON: string(toolCallsJSON),
	})

	for _, exec := range s.ToolExecutions {
		records = append(records, contextstore.NewToolResultRecordWithDisplay(exec.Call.ID, exec.Output, exec.DisplayOutput))
	}

	if s.ToolFailure != nil {
		content := formatToolFailureContent(s.ToolFailure)
		records = append(records, contextstore.NewToolResultRecord(s.ToolFailure.Call.ID, content))
	}

	return records
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

	err := r.persistRunStart(ctx, store, prompt, userRecord)
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
		if ctx.Err() != nil {
			return r.interruptedResult(ctx, result)
		}
		if err := r.emitEvent(ctx, runtimeevents.StepBegin{Number: stepNo}); err != nil {
			result.Status = RunStatusFailed
			return result, err
		}

		if err := r.persistStepCheckpoint(ctx, store); err != nil {
			if isInterruptedError(err) {
				return r.interruptedResult(ctx, result)
			}
			return Result{Status: RunStatusFailed}, fmt.Errorf("persist step checkpoint: %w", err)
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

		if !finished {
			applied, err := r.applyPendingDMail(ctx, store)
			if err != nil {
				if isInterruptedError(err) {
					return r.interruptedResult(ctx, result)
				}
				return Result{Status: RunStatusFailed}, fmt.Errorf("persist rollback context: %w", err)
			}
			if applied {
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
