package runtime

import (
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
)

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// Input 表示单次 runtime 执行的最小输入。
type Input struct {
	Prompt string
}

// Result 表示单次 runtime 追加到 history 的记录。
type Result struct {
	AppendedRecords []contextstore.TextRecord
}

// Run 执行当前最小 runtime 流程。
// 现在它还不调用模型，只把 prompt 和占位回复写入 history。
func Run(ctx contextstore.Context, input Input) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{}, nil
	}

	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord(prompt),
		contextstore.NewAssistantTextRecord(buildAssistantPlaceholderReply(prompt)),
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

func buildAssistantPlaceholderReply(prompt string) string {
	return fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt)
}
