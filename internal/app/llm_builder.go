package app

import (
	"errors"
	"fmt"

	"fimi-cli/internal/config"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/llm/openai"
)

// ErrUnsupportedProviderType 表示当前 llm client 构造器不支持给定 provider 类型。
var ErrUnsupportedProviderType = errors.New("unsupported provider type")

// buildLLMClientFromConfig 根据 config 构造具体 llm client。
// 这是 app 层的构建逻辑，避免 llm 包反向依赖 config 和具体 provider。
func buildLLMClientFromConfig(cfg config.Config) (llm.Client, error) {
	modelCfg, err := resolveConfiguredModel(cfg)
	if err != nil {
		return nil, err
	}

	providerName, providerCfg, err := resolveConfiguredProvider(cfg, modelCfg)
	if err != nil {
		return nil, err
	}

	return buildLLMClientForProvider(providerName, providerCfg, modelCfg)
}

// buildLLMClientForProvider 根据 provider 类型分发到具体适配器构造器。
func buildLLMClientForProvider(
	providerName string,
	providerCfg config.ProviderConfig,
	modelCfg config.ModelConfig,
) (llm.Client, error) {
		switch providerCfg.Type {
		case config.ProviderTypePlaceholder:
			return buildPlaceholderClient()
		case config.ProviderTypeQWEN:
			return buildQwenClient(providerName, providerCfg, modelCfg)
		case config.ProviderTypeOpenAI:
			return buildOpenAIClient(providerName, providerCfg, modelCfg)
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedProviderType, providerCfg.Type)
		}
}

// resolveConfiguredModel 统一决定当前 run 选择的模型。
// 旧的 engine_mode 路径已经移除，app 只接受 models 配置。
func resolveConfiguredModel(cfg config.Config) (config.ModelConfig, error) {
	modelCfg, ok := cfg.Models[cfg.DefaultModel]
	if !ok {
		return config.ModelConfig{}, fmt.Errorf("default model %q not found in config.models", cfg.DefaultModel)
	}
	if modelCfg.Model == "" {
		modelCfg.Model = cfg.DefaultModel
	}

	return modelCfg, nil
}

func resolveConfiguredProvider(
	cfg config.Config,
	modelCfg config.ModelConfig,
) (string, config.ProviderConfig, error) {
	if modelCfg.Provider == config.DefaultProviderName {
		return config.DefaultProviderName, config.ProviderConfig{
			Type: config.ProviderTypePlaceholder,
		}, nil
	}

	providerCfg, ok := cfg.Providers[modelCfg.Provider]
	if !ok {
		return "", config.ProviderConfig{}, fmt.Errorf("provider %q not found in config.providers", modelCfg.Provider)
	}
	if providerCfg.Type == "" {
		return "", config.ProviderConfig{}, fmt.Errorf("providers.%s.type is required", modelCfg.Provider)
	}

	return modelCfg.Provider, providerCfg, nil
}

// buildPlaceholderClient 构造内建的 placeholder client。
func buildPlaceholderClient() (llm.Client, error) {
	return llm.NewPlaceholderClient(), nil
}

const (
	// DashScope OpenAI 兼容模式的 BaseURL
	qwenDefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	// 默认使用的 QWEN 模型
	qwenDefaultModel = "qwen-plus"
)

// buildQwenClient 从配置构建 QWEN (DashScope) client。
// 内部使用 OpenAI 兼容 API，默认值直接内联避免不必要的包间接层。
func buildQwenClient(
	providerName string,
	providerCfg config.ProviderConfig,
	modelCfg config.ModelConfig,
) (llm.Client, error) {
	if providerCfg.APIKey == "" {
		return nil, fmt.Errorf("qwen api_key is required; set providers.%s.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)", providerName)
	}

	baseURL := providerCfg.BaseURL
	if baseURL == "" {
		baseURL = qwenDefaultBaseURL
	}
	model := modelCfg.Model
	if model == "" {
		model = qwenDefaultModel
	}

	return openai.NewClient(openai.Config{
		APIKey:  providerCfg.APIKey,
		BaseURL: baseURL,
		Model:   model,
	}), nil
}

// buildOpenAIClient 从配置构建 OpenAI 兼容 client。
func buildOpenAIClient(
	providerName string,
	providerCfg config.ProviderConfig,
	modelCfg config.ModelConfig,
) (llm.Client, error) {
	if providerCfg.APIKey == "" {
		return nil, fmt.Errorf("openai api_key is required; set providers.%s.api_key in your config", providerName)
	}

	wireAPI := providerCfg.WireAPI
	if wireAPI == "" {
		// OpenAI 官方 provider 默认走 Responses API。
		wireAPI = config.ProviderWireAPIResponses
	}

	return openai.NewClient(openai.Config{
		APIKey:  providerCfg.APIKey,
		BaseURL: providerCfg.BaseURL,
		Model:   modelCfg.Model,
		WireAPI: wireAPI,
	}), nil
}
