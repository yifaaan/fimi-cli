package app

import (
	"errors"
	"fmt"

	"fimi-cli/internal/config"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/llm/qwen"
)

// ErrUnsupportedClientMode 表示当前 llm client 构造器不支持给定模式。
var ErrUnsupportedClientMode = errors.New("unsupported llm client mode")

// buildLLMClientFromConfig 根据 config 构造具体 llm client。
// 这是 app 层的构建逻辑，避免 llm 包反向依赖 config 和具体 provider。
func buildLLMClientFromConfig(cfg config.Config) (llm.Client, error) {
	switch cfg.EngineMode {
	case "", "placeholder":
		return llm.NewPlaceholderClient(), nil
	case "qwen":
		return buildQwenClient(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedClientMode, cfg.EngineMode)
	}
}

// buildQwenClient 从配置构建 QWEN client。
func buildQwenClient(cfg config.Config) (llm.Client, error) {
	providerCfg, ok := cfg.Providers["qwen"]
	if !ok {
		return nil, fmt.Errorf("qwen provider config not found in config file; add a \"providers.qwen\" section to your ~/.config/fimi/config.json")
	}
	if providerCfg.APIKey == "" {
		return nil, fmt.Errorf("qwen api_key is required; set providers.qwen.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)")
	}

	return qwen.NewClient(qwen.Config{
		APIKey:  providerCfg.APIKey,
		BaseURL: providerCfg.BaseURL,
	}), nil
}