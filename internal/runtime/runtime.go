package runtime

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
)

const DefaultReplyHistoryTurnLimit = 4
const DefaultMaxStepsPerRun = 100

var ErrToolCallsNotSupported = errors.New("runtime tool calls are not supported yet")
var ErrUnknownStepKind = errors.New("unknown runtime step kind")

// Config 定义 runtime 自己关心的最小运行参数。
type Config struct {
	ReplyHistoryTurnLimit int
	MaxStepsPerRun        int
}

// DefaultConfig 返回 runtime 的默认参数。
func DefaultConfig() Config {
	return Config{
		ReplyHistoryTurnLimit: DefaultReplyHistoryTurnLimit,
		MaxStepsPerRun:        DefaultMaxStepsPerRun,
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

// ToolCall 描述 runtime 下一步需要执行的工具调用。
// 当前先保留最小结构，后面再补 schema 和参数类型。
type ToolCall struct {
	Name      string
	Arguments string
}

// StepResult 表示单个 runtime step 的结构化结果。
// 先同时保留追加记录和平铺结果，方便上层渐进迁移。
type StepResult struct {
	Kind            StepKind
	AppendedRecords []contextstore.TextRecord
	ToolCalls       []ToolCall
}

// Engine 负责为 runtime 生成 assistant 回复文本。
// 这里先保持最小接口，后面再扩展为真正的模型调用边界。
type Engine interface {
	Reply(input ReplyInput) (string, error)
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine       Engine
	config       Config
	runStepFn    func(ctx contextstore.Context, input Input, prompt string) (StepResult, error)
	advanceRunFn func(result Result, stepResult StepResult) (Result, bool, error)
}

// New 创建最小 runtime runner。
// 调用方必须显式注入 engine，避免 core runtime 反向依赖具体适配器。
func New(engine Engine, cfg Config) Runner {
	if engine == nil {
		engine = missingEngine{}
	}
	if cfg.ReplyHistoryTurnLimit <= 0 {
		cfg.ReplyHistoryTurnLimit = DefaultReplyHistoryTurnLimit
	}
	if cfg.MaxStepsPerRun <= 0 {
		cfg.MaxStepsPerRun = DefaultMaxStepsPerRun
	}

	return Runner{
		engine: engine,
		config: cfg,
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
		stepResult, err = runStep(ctx, input, prompt)
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
		result.Steps = append(result.Steps, stepResult)

		return result, true, nil
	case StepKindToolCalls:
		if len(stepResult.ToolCalls) == 0 {
			return Result{}, false, fmt.Errorf("step kind %q requires at least one tool call", stepResult.Kind)
		}

		return Result{}, false, fmt.Errorf("%w", ErrToolCallsNotSupported)
	default:
		return Result{}, false, fmt.Errorf("%w: %q", ErrUnknownStepKind, stepResult.Kind)
	}
}

type missingEngine struct{}

func (missingEngine) Reply(input ReplyInput) (string, error) {
	return "", errors.New("runtime engine is required")
}
