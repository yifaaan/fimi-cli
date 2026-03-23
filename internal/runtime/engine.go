package runtime

import (
	"fmt"
	"strings"
)

const assistantPlaceholderPrefix = "assistant placeholder reply:"

// PlaceholderEngine 是当前最小可用的 assistant 文本生成器。
// 它是临时适配器，后面会被真正的 LLM engine 替换。
type PlaceholderEngine struct{}

// Reply 根据用户 prompt 生成占位 assistant 回复。
func (PlaceholderEngine) Reply(input Input) (string, error) {
	prompt := strings.TrimSpace(input.Prompt)
	return fmt.Sprintf("%s %s", assistantPlaceholderPrefix, prompt), nil
}
