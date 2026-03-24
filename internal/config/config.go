package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ProviderConfig 存储单个 LLM provider 的配置。
type ProviderConfig struct {
	Type    string `json:"type"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

// ModelConfig 描述一个逻辑模型名如何映射到 provider 和真实模型名。
type ModelConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Config 表示应用当前最小可用的配置集合。
// 现在只保留后续 runtime 一定会依赖的基础字段。
type Config struct {
	DefaultModel  string                    `json:"default_model"`
	LoopControl   LoopControl               `json:"loop_control"`
	HistoryWindow HistoryWindow             `json:"history_window"`
	Models        map[string]ModelConfig    `json:"models"`
	Providers     map[string]ProviderConfig `json:"providers"`
}

// LoopControl 对应 Python 版本里的 agent loop 控制参数。
type LoopControl struct {
	MaxStepsPerRun    int `json:"max_steps_per_run"`
	MaxRetriesPerStep int `json:"max_retries_per_step"`
}

// HistoryWindow 定义 runtime 和 llm 使用的历史窗口策略。
type HistoryWindow struct {
	RuntimeTurns int `json:"runtime_turns"`
	LLMTurns     int `json:"llm_turns"`
}

const (
	AppConfigDirName      = "fimi"
	DefaultConfigFileName = "config.json"
	DefaultModelName      = "kimi-k2-turbo-preview"
	DefaultMaxStepsPerRun = 100
	DefaultMaxRetries     = 3
	DefaultRuntimeTurns   = 4
	DefaultLLMTurns       = 2
)

const (
	ProviderTypePlaceholder = "placeholder"
	ProviderTypeQWEN        = "qwen"
	DefaultProviderName     = ProviderTypePlaceholder
)

// Default 返回内建默认配置。
func Default() Config {
	return Config{
		DefaultModel: DefaultModelName,
		LoopControl: LoopControl{
			MaxStepsPerRun:    DefaultMaxStepsPerRun,
			MaxRetriesPerStep: DefaultMaxRetries,
		},
		HistoryWindow: HistoryWindow{
			RuntimeTurns: DefaultRuntimeTurns,
			LLMTurns:     DefaultLLMTurns,
		},
		Models: map[string]ModelConfig{
			DefaultModelName: {
				Provider: DefaultProviderName,
				Model:    DefaultModelName,
			},
		},
		// 提供示例 provider 配置结构，方便用户参考
		Providers: map[string]ProviderConfig{
			"qwen": {
				Type: ProviderTypeQWEN,
				// APIKey 留空，用户需要自己填写
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			},
		},
	}
}

// Dir 返回默认配置目录，例如 ~/.config/fimi。
func Dir() (string, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(baseDir, AppConfigDirName), nil
}

// File 返回默认配置文件路径。
func File() (string, error) {
	configDir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, DefaultConfigFileName), nil
}

// Load 从默认配置位置读取配置。
// 如果配置文件不存在，就回退到默认配置。
func Load() (Config, error) {
	configFile, err := File()
	if err != nil {
		return Config{}, err
	}

	return LoadFile(configFile)
}

// LoadFile 从指定文件路径读取配置。
// 如果文件不存在，就回退到默认配置。
func LoadFile(configFile string) (Config, error) {
	data, err := os.ReadFile(configFile)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", configFile, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config file %q: %w", configFile, err)
	}
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config file %q: %w", configFile, err)
	}

	return cfg, nil
}

func validate(cfg Config) error {
	if cfg.DefaultModel == "" {
		return errors.New("default_model is required")
	}

	if _, ok := cfg.Models[cfg.DefaultModel]; !ok {
		return fmt.Errorf("default_model %q not found in models", cfg.DefaultModel)
	}

	for modelAlias, modelCfg := range cfg.Models {
		if err := validateModelConfig(modelAlias, modelCfg, cfg.Providers); err != nil {
			return err
		}
	}

	return nil
}

func validateModelConfig(
	modelAlias string,
	modelCfg ModelConfig,
	providers map[string]ProviderConfig,
) error {
	if modelCfg.Provider == "" {
		return fmt.Errorf("models.%s.provider is required", modelAlias)
	}
	if modelCfg.Provider == DefaultProviderName {
		// placeholder 是内建 provider，不要求 users 在 providers 里重复声明。
		return nil
	}
	providerCfg, ok := providers[modelCfg.Provider]
	if !ok {
		return fmt.Errorf("models.%s.provider %q not found in providers", modelAlias, modelCfg.Provider)
	}
	if providerCfg.Type == "" {
		return fmt.Errorf("providers.%s.type is required", modelCfg.Provider)
	}

	// model 允许留空；消费方会把 alias 当作真实模型名兜底。
	return nil
}
