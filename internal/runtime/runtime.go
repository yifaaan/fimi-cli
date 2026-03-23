package runtime

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
)

const replyHistoryTurnLimit = 4

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
	AppendedRecords []contextstore.TextRecord
}

// Engine 负责为 runtime 生成 assistant 回复文本。
// 这里先保持最小接口，后面再扩展为真正的模型调用边界。
type Engine interface {
	Reply(input ReplyInput) (string, error)
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine Engine
}

// New 创建最小 runtime runner。
// 调用方必须显式注入 engine，避免 core runtime 反向依赖具体适配器。
func New(engine Engine) Runner {
	if engine == nil {
		engine = missingEngine{}
	}

	return Runner{
		engine: engine,
	}
}

// Run 执行当前最小 runtime 流程。
// 现在它还不调用真实模型，只协调 history 追加顺序和 engine 调用。
func (r Runner) Run(ctx contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{}, nil
	}

	history, err := ctx.ReadRecentTurns(replyHistoryTurnLimit)
	if err != nil {
		return Result{}, fmt.Errorf("read runtime history: %w", err)
	}

	assistantReply, err := r.engine.Reply(ReplyInput{
		Prompt:       prompt,
		Model:        input.Model,
		SystemPrompt: input.SystemPrompt,
		History:      history,
	})
	if err != nil {
		return Result{}, fmt.Errorf("build assistant reply: %w", err)
	}

	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord(prompt),
		contextstore.NewAssistantTextRecord(assistantReply),
	}

	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			return Result{}, fmt.Errorf("append runtime record: %w", err)
		}
	}

	return Result{
		AppendedRecords: records,
	}, nil
}

type missingEngine struct{}

func (missingEngine) Reply(input ReplyInput) (string, error) {
	return "", errors.New("runtime engine is required")
}
