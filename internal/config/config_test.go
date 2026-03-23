package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIncludesHistoryWindow(t *testing.T) {
	cfg := Default()

	if cfg.SystemPrompt != DefaultSystemPrompt {
		t.Fatalf("Default().SystemPrompt = %q, want %q", cfg.SystemPrompt, DefaultSystemPrompt)
	}
	if cfg.HistoryWindow.RuntimeTurns != DefaultRuntimeTurns {
		t.Fatalf("Default().HistoryWindow.RuntimeTurns = %d, want %d", cfg.HistoryWindow.RuntimeTurns, DefaultRuntimeTurns)
	}
	if cfg.HistoryWindow.LLMTurns != DefaultLLMTurns {
		t.Fatalf("Default().HistoryWindow.LLMTurns = %d, want %d", cfg.HistoryWindow.LLMTurns, DefaultLLMTurns)
	}
	if cfg.Models == nil {
		t.Fatalf("Default().Models = nil, want non-nil")
	}
	modelCfg, ok := cfg.Models[DefaultModelName]
	if !ok {
		t.Fatalf("Default().Models[%q] not found", DefaultModelName)
	}
	if modelCfg.Provider != DefaultProviderName {
		t.Fatalf("Default().Models[%q].Provider = %q, want %q", DefaultModelName, modelCfg.Provider, DefaultProviderName)
	}
	if modelCfg.Model != DefaultModelName {
		t.Fatalf("Default().Models[%q].Model = %q, want %q", DefaultModelName, modelCfg.Model, DefaultModelName)
	}
	qwenProvider, ok := cfg.Providers["qwen"]
	if !ok {
		t.Fatalf("Default().Providers[\"qwen\"] not found")
	}
	if qwenProvider.Type != "qwen" {
		t.Fatalf("Default().Providers[\"qwen\"].Type = %q, want %q", qwenProvider.Type, "qwen")
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
	if cfg.SystemPrompt != DefaultSystemPrompt {
		t.Fatalf("LoadFile().SystemPrompt = %q, want %q", cfg.SystemPrompt, DefaultSystemPrompt)
	}
	if cfg.Providers == nil {
		t.Fatalf("LoadFile().Providers = nil, want non-nil")
	}
	if _, ok := cfg.Providers["qwen"]; !ok {
		t.Fatalf("LoadFile().Providers[\"qwen\"] not found")
	}
	if cfg.Models == nil {
		t.Fatalf("LoadFile().Models = nil, want non-nil")
	}
	if _, ok := cfg.Models[DefaultModelName]; !ok {
		t.Fatalf("LoadFile().Models[%q] not found", DefaultModelName)
	}
}

func TestLoadFileMergesHistoryWindowWithDefaults(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "custom-model",
		"models": {
			"custom-model": {
				"provider": "placeholder",
				"model": "custom-model"
			}
		},
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
		"default_model": "qwen-plus",
		"models": {
			"qwen-plus": {
				"provider": "qwen",
				"model": "qwen-plus"
			}
		},
		"providers": {
			"qwen": {
				"type": "qwen",
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

	if cfg.DefaultModel != "qwen-plus" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", cfg.DefaultModel, "qwen-plus")
	}
	modelCfg, ok := cfg.Models["qwen-plus"]
	if !ok {
		t.Fatalf("LoadFile().Models[\"qwen-plus\"] not found")
	}
	if modelCfg.Provider != "qwen" {
		t.Fatalf("LoadFile().Models[\"qwen-plus\"].Provider = %q, want %q", modelCfg.Provider, "qwen")
	}
	if modelCfg.Model != "qwen-plus" {
		t.Fatalf("LoadFile().Models[\"qwen-plus\"].Model = %q, want %q", modelCfg.Model, "qwen-plus")
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
	if qwenCfg.Type != "qwen" {
		t.Fatalf("LoadFile().Providers[\"qwen\"].Type = %q, want %q", qwenCfg.Type, "qwen")
	}
	if qwenCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("LoadFile().Providers[\"qwen\"].BaseURL = %q, want %q", qwenCfg.BaseURL, "https://dashscope.aliyuncs.com/compatible-mode/v1")
	}
}

func TestLoadFileReturnsValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErrText string
	}{
		{
			name: "missing default model entry",
			content: `{
				"default_model": "missing",
				"models": {
					"other": {
						"provider": "placeholder",
						"model": "other"
					}
				}
			}`,
			wantErrText: `default_model "missing" not found in models`,
		},
		{
			name: "missing provider mapping",
			content: `{
				"default_model": "qwen-plus",
				"models": {
					"default-placeholder": {
						"provider": "placeholder",
						"model": "default-placeholder"
					},
					"qwen-plus": {
						"provider": "missing-provider",
						"model": "qwen-plus"
					}
				}
			}`,
			wantErrText: `models.qwen-plus.provider "missing-provider" not found in providers`,
		},
		{
			name: "missing model provider field",
			content: `{
				"default_model": "qwen-plus",
				"models": {
					"other": {
						"provider": "placeholder",
						"model": "other"
					},
					"qwen-plus": {
						"model": "qwen-plus"
					}
				}
			}`,
			wantErrText: `models.qwen-plus.provider is required`,
		},
		{
			name: "non default model also validated",
			content: `{
				"default_model": "default-placeholder",
				"models": {
					"default-placeholder": {
						"provider": "placeholder",
						"model": "default-placeholder"
					},
					"broken-secondary": {
						"provider": "missing-provider",
						"model": "broken-secondary"
					}
				}
			}`,
			wantErrText: `models.broken-secondary.provider "missing-provider" not found in providers`,
		},
		{
			name: "referenced provider type required",
			content: `{
				"default_model": "qwen-plus",
				"models": {
					"qwen-plus": {
						"provider": "aliyun-prod",
						"model": "qwen-plus"
					}
				},
				"providers": {
					"aliyun-prod": {
						"api_key": "sk-test-key",
						"base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1"
					}
				}
			}`,
			wantErrText: `providers.aliyun-prod.type is required`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configFile, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", configFile, err)
			}

			_, err := LoadFile(configFile)
			if err == nil {
				t.Fatalf("LoadFile() error = nil, want non-nil")
			}
			if err.Error() != `validate config file "`+configFile+`": `+tt.wantErrText {
				t.Fatalf("LoadFile() error = %q, want %q", err.Error(), `validate config file "`+configFile+`": `+tt.wantErrText)
			}
		})
	}
}

func TestLoadFileAllowsEmptyModelNameToFallBackToAlias(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "qwen-plus",
		"models": {
			"qwen-plus": {
				"provider": "qwen"
			},
			"placeholder-worker": {
				"provider": "placeholder"
			}
		},
		"providers": {
			"qwen": {
				"type": "qwen",
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

	if cfg.Models["qwen-plus"].Model != "" {
		t.Fatalf("LoadFile().Models[\"qwen-plus\"].Model = %q, want empty string", cfg.Models["qwen-plus"].Model)
	}
	if cfg.Models["placeholder-worker"].Model != "" {
		t.Fatalf("LoadFile().Models[\"placeholder-worker\"].Model = %q, want empty string", cfg.Models["placeholder-worker"].Model)
	}
}
