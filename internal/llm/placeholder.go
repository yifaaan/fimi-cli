package llm

import (
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
)

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderEngine 是当前最小可用的 LLM 适配器。
// 它先返回确定性的占位文本，后面再替换成真实模型调用。
type PlaceholderEngine struct{}

// Reply 根据用户 prompt 生成占位 assistant 回复。
func (PlaceholderEngine) Reply(input runtime.Input) (string, error) {
	prompt := strings.TrimSpace(input.Prompt)
	return fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt), nil
}
