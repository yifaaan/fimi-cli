package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIncludesHistoryWindow(t *testing.T) {
	cfg := Default()

	if cfg.EngineMode != DefaultEngineMode {
		t.Fatalf("Default().EngineMode = %q, want %q", cfg.EngineMode, DefaultEngineMode)
	}
	if cfg.SystemPrompt != DefaultSystemPrompt {
		t.Fatalf("Default().SystemPrompt = %q, want %q", cfg.SystemPrompt, DefaultSystemPrompt)
	}
	if cfg.HistoryWindow.RuntimeTurns != DefaultRuntimeTurns {
		t.Fatalf("Default().HistoryWindow.RuntimeTurns = %d, want %d", cfg.HistoryWindow.RuntimeTurns, DefaultRuntimeTurns)
	}
	if cfg.HistoryWindow.LLMTurns != DefaultLLMTurns {
		t.Fatalf("Default().HistoryWindow.LLMTurns = %d, want %d", cfg.HistoryWindow.LLMTurns, DefaultLLMTurns)
	}
}

func TestLoadFileReturnsDefaultWhenMissing(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "missing.json")

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	// 因为包含 map，不能直接比较整个结构体，检查关键字段
	if cfg.DefaultModel != DefaultModelName {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", cfg.DefaultModel, DefaultModelName)
	}
	if cfg.EngineMode != DefaultEngineMode {
		t.Fatalf("LoadFile().EngineMode = %q, want %q", cfg.EngineMode, DefaultEngineMode)
	}
	if cfg.SystemPrompt != DefaultSystemPrompt {
		t.Fatalf("LoadFile().SystemPrompt = %q, want %q", cfg.SystemPrompt, DefaultSystemPrompt)
	}
}

func TestLoadFileMergesHistoryWindowWithDefaults(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "custom-model",
		"system_prompt": "You are the configured agent.",
		"history_window": {
			"llm_turns": 5
		}
	}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configFile, err)
	}

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.DefaultModel != "custom-model" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", cfg.DefaultModel, "custom-model")
	}
	if cfg.EngineMode != DefaultEngineMode {
		t.Fatalf("LoadFile().EngineMode = %q, want %q", cfg.EngineMode, DefaultEngineMode)
	}
	if cfg.SystemPrompt != "You are the configured agent." {
		t.Fatalf("LoadFile().SystemPrompt = %q, want %q", cfg.SystemPrompt, "You are the configured agent.")
	}
	if cfg.HistoryWindow.RuntimeTurns != DefaultRuntimeTurns {
		t.Fatalf("LoadFile().HistoryWindow.RuntimeTurns = %d, want %d", cfg.HistoryWindow.RuntimeTurns, DefaultRuntimeTurns)
	}
	if cfg.HistoryWindow.LLMTurns != 5 {
		t.Fatalf("LoadFile().HistoryWindow.LLMTurns = %d, want %d", cfg.HistoryWindow.LLMTurns, 5)
	}
}

func TestLoadFileParsesProviders(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"engine_mode": "qwen",
		"providers": {
			"qwen": {
				"api_key": "sk-test-key",
				"base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1"
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configFile, err)
	}

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.EngineMode != "qwen" {
		t.Fatalf("LoadFile().EngineMode = %q, want %q", cfg.EngineMode, "qwen")
	}
	if cfg.Providers == nil {
		t.Fatalf("LoadFile().Providers = nil, want non-nil")
	}
	qwenCfg, ok := cfg.Providers["qwen"]
	if !ok {
		t.Fatalf("LoadFile().Providers[\"qwen\"] not found")
	}
	if qwenCfg.APIKey != "sk-test-key" {
		t.Fatalf("LoadFile().Providers[\"qwen\"].APIKey = %q, want %q", qwenCfg.APIKey, "sk-test-key")
	}
	if qwenCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("LoadFile().Providers[\"qwen\"].BaseURL = %q, want %q", qwenCfg.BaseURL, "https://dashscope.aliyuncs.com/compatible-mode/v1")
	}
}
