package qwen

import (
	"fimi-cli/internal/llm"
	"fimi-cli/internal/llm/openai"
)

const (
	// DashScope OpenAI 兼容模式的 BaseURL
	DefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	// 默认使用的 QWEN 模型
	DefaultModel = "qwen-plus"
)

// Config 存储 QWEN client 的配置。
type Config struct {
	APIKey  string
	BaseURL string // 可选，默认使用 DefaultBaseURL
	Model   string // 可选，默认使用 DefaultModel
}

// NewClient 创建 QWEN (DashScope) client。
// 内部使用 OpenAI 兼容 API。
func NewClient(cfg Config) llm.Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}

	return openai.NewClient(openai.Config{
		BaseURL: baseURL,
		APIKey:  cfg.APIKey,
		Model:   model,
	})
}
