package runtime

import (
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
)

// Input 表示单次 runtime 执行的最小输入。
type Input struct {
	Prompt string
}

// Result 表示单次 runtime 追加到 history 的记录。
type Result struct {
	AppendedRecords []contextstore.TextRecord
}

// Engine 负责为 runtime 生成 assistant 回复文本。
// 这里先保持最小接口，后面再扩展为真正的模型调用边界。
type Engine interface {
	Reply(input Input) (string, error)
}

// Runner 持有一次 runtime 执行所需的核心依赖。
type Runner struct {
	engine Engine
}

// New 创建最小 runtime runner。
// 如果调用方暂时没有注入 engine，就退回占位实现。
func New(engine Engine) Runner {
	if engine == nil {
		engine = PlaceholderEngine{}
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

	assistantReply, err := r.engine.Reply(Input{
		Prompt: prompt,
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
