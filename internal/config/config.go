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
	WireAPI string `json:"wire_api,omitempty"`
}

// ModelConfig 描述一个逻辑模型名如何映射到 provider 和真实模型名。
type ModelConfig struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	ContextWindowTokens int    `json:"context_window_tokens,omitempty"`
}

// ServicesConfig 预留外部服务扩展点；当前不内建具体服务配置。
type ServicesConfig struct{}

type DuckDuckGoConfig struct {
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent"`
}

// MCPServer describes a single MCP server to connect to.
type MCPServer struct {
	Command string            `json:"command"` // e.g. "npx", "python", "/path/to/binary"
	Args    []string          `json:"args"`    // e.g. ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
	Env     map[string]string `json:"env"`     // Optional environment variables
}

// MCPConfig holds MCP server connections.
type MCPConfig struct {
	Enabled bool                   `json:"enabled"`
	Servers map[string]MCPServer  `json:"servers"`
}

type WebConfig struct {
	Enabled       bool             `json:"enabled"`
	SearchBackend string           `json:"search_backend"`
	DuckDuckGo    DuckDuckGoConfig `json:"duckduckgo"`
}

// Config 表示应用当前最小可用的配置集合。
// 现在只保留后续 runtime 一定会依赖的基础字段。
type Config struct {
	DefaultModel  string                    `json:"default_model"`
	LoopControl   LoopControl               `json:"loop_control"`
	HistoryWindow HistoryWindow             `json:"history_window"`
	Models        map[string]ModelConfig    `json:"models"`
	Providers     map[string]ProviderConfig `json:"providers"`
	Services      ServicesConfig            `json:"services"`
	Web           WebConfig                 `json:"web"`
	MCP           MCPConfig                 `json:"mcp"`
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
	ProviderTypeOpenAI      = "openai"
	DefaultProviderName     = ProviderTypePlaceholder
)

const (
	ProviderWireAPIChatCompletions = "chat_completions"
	ProviderWireAPIResponses       = "responses"
)

const (
	DefaultWebSearchBackend    = "duckduckgo"
	DefaultDuckDuckGoBaseURL   = "https://duckduckgo.com/html/"
	DefaultDuckDuckGoUserAgent = "fimi-cli/0.1"
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
		Providers: map[string]ProviderConfig{
			"qwen": {
				Type: ProviderTypeQWEN,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			},
		},
		Web: WebConfig{
			Enabled:       false,
			SearchBackend: DefaultWebSearchBackend,
			DuckDuckGo: DuckDuckGoConfig{
				BaseURL:   DefaultDuckDuckGoBaseURL,
				UserAgent: DefaultDuckDuckGoUserAgent,
			},
		},
		MCP: MCPConfig{
			Enabled: false,
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
	if err := validateWebConfig(cfg.Web); err != nil {
		return err
	}
	if err := validateMCPConfig(cfg.MCP); err != nil {
		return err
	}

	return nil
}

func validateModelConfig(
	modelAlias string,
	modelCfg ModelConfig,
	providers map[string]ProviderConfig,
) error {
	if modelCfg.ContextWindowTokens < 0 {
		return fmt.Errorf("models.%s.context_window_tokens must be >= 0", modelAlias)
	}
	if modelCfg.Provider == "" {
		return fmt.Errorf("models.%s.provider is required", modelAlias)
	}
	if modelCfg.Provider == DefaultProviderName {
		return nil
	}
	providerCfg, ok := providers[modelCfg.Provider]
	if !ok {
		return fmt.Errorf("models.%s.provider %q not found in providers", modelAlias, modelCfg.Provider)
	}
	if providerCfg.Type == "" {
		return fmt.Errorf("providers.%s.type is required", modelCfg.Provider)
	}
	if providerCfg.WireAPI != "" &&
		providerCfg.WireAPI != ProviderWireAPIChatCompletions &&
		providerCfg.WireAPI != ProviderWireAPIResponses {
		return fmt.Errorf(
			"providers.%s.wire_api must be one of %q or %q",
			modelCfg.Provider,
			ProviderWireAPIChatCompletions,
			ProviderWireAPIResponses,
		)
	}

	return nil
}

func validateWebConfig(cfg WebConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.SearchBackend == "" {
		return errors.New("web.search_backend is required when web.enabled is true")
	}
	if cfg.SearchBackend != DefaultWebSearchBackend {
		return fmt.Errorf("web.search_backend %q is not supported", cfg.SearchBackend)
	}
	if cfg.DuckDuckGo.BaseURL == "" {
		return errors.New("web.duckduckgo.base_url is required when web.enabled is true")
	}
	if cfg.DuckDuckGo.UserAgent == "" {
		return errors.New("web.duckduckgo.user_agent is required when web.enabled is true")
	}

	return nil
}

func validateMCPConfig(cfg MCPConfig) error {
	if !cfg.Enabled {
		return nil
	}
	for name, server := range cfg.Servers {
		if server.Command == "" {
			return fmt.Errorf("mcp.servers.%s.command is required", name)
		}
	}

	return nil
}
