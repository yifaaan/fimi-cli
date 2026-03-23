package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config 表示应用当前最小可用的配置集合。
// 现在只保留后续 runtime 一定会依赖的基础字段。
type Config struct {
	DefaultModel  string        `json:"default_model"`
	SystemPrompt  string        `json:"system_prompt"`
	LoopControl   LoopControl   `json:"loop_control"`
	HistoryWindow HistoryWindow `json:"history_window"`
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
	DefaultSystemPrompt   = "You are fimi, a coding agent."
	DefaultMaxStepsPerRun = 100
	DefaultMaxRetries     = 3
	DefaultRuntimeTurns   = 4
	DefaultLLMTurns       = 2
)

// Default 返回内建默认配置。
func Default() Config {
	return Config{
		DefaultModel: DefaultModelName,
		SystemPrompt: DefaultSystemPrompt,
		LoopControl: LoopControl{
			MaxStepsPerRun:    DefaultMaxStepsPerRun,
			MaxRetriesPerStep: DefaultMaxRetries,
		},
		HistoryWindow: HistoryWindow{
			RuntimeTurns: DefaultRuntimeTurns,
			LLMTurns:     DefaultLLMTurns,
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

	return cfg, nil
}
