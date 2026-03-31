package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIncludesHistoryWindow(t *testing.T) {
	cfg := Default()

	if cfg.LoopControl.MaxStepsPerRun != DefaultMaxStepsPerRun {
		t.Fatalf("Default().LoopControl.MaxStepsPerRun = %d, want %d", cfg.LoopControl.MaxStepsPerRun, DefaultMaxStepsPerRun)
	}
	if cfg.LoopControl.MaxAdditionalRetriesPerStep != DefaultMaxRetries {
		t.Fatalf("Default().LoopControl.MaxAdditionalRetriesPerStep = %d, want %d", cfg.LoopControl.MaxAdditionalRetriesPerStep, DefaultMaxRetries)
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
	if qwenProvider.Type != ProviderTypeQWEN {
		t.Fatalf("Default().Providers[\"qwen\"].Type = %q, want %q", qwenProvider.Type, ProviderTypeQWEN)
	}
	if qwenProvider.WireAPI != "" {
		t.Fatalf("Default().Providers[\"qwen\"].WireAPI = %q, want empty string", qwenProvider.WireAPI)
	}
}

func TestDefaultIncludesWebConfig(t *testing.T) {
	cfg := Default()

	if cfg.Web.Enabled {
		t.Fatalf("Default().Web.Enabled = true, want false")
	}
	if cfg.Web.SearchBackend != DefaultWebSearchBackend {
		t.Fatalf("Default().Web.SearchBackend = %q, want %q", cfg.Web.SearchBackend, DefaultWebSearchBackend)
	}
	if cfg.Web.DuckDuckGo.BaseURL != DefaultDuckDuckGoBaseURL {
		t.Fatalf("Default().Web.DuckDuckGo.BaseURL = %q, want %q", cfg.Web.DuckDuckGo.BaseURL, DefaultDuckDuckGoBaseURL)
	}
	if cfg.Web.DuckDuckGo.UserAgent != DefaultDuckDuckGoUserAgent {
		t.Fatalf("Default().Web.DuckDuckGo.UserAgent = %q, want %q", cfg.Web.DuckDuckGo.UserAgent, DefaultDuckDuckGoUserAgent)
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

func TestLoadFileMergesLoopControlAndHistoryWindowWithDefaults(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "custom-model",
		"models": {
			"custom-model": {
				"provider": "placeholder",
				"model": "custom-model"
			}
		},
		"loop_control": {
			"max_additional_retries_per_step": 0
		},
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
	if cfg.LoopControl.MaxStepsPerRun != DefaultMaxStepsPerRun {
		t.Fatalf("LoadFile().LoopControl.MaxStepsPerRun = %d, want %d", cfg.LoopControl.MaxStepsPerRun, DefaultMaxStepsPerRun)
	}
	if cfg.LoopControl.MaxAdditionalRetriesPerStep != 0 {
		t.Fatalf("LoadFile().LoopControl.MaxAdditionalRetriesPerStep = %d, want %d", cfg.LoopControl.MaxAdditionalRetriesPerStep, 0)
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
	if modelCfg.Provider != ProviderTypeQWEN {
		t.Fatalf("LoadFile().Models[\"qwen-plus\"].Provider = %q, want %q", modelCfg.Provider, ProviderTypeQWEN)
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
	if qwenCfg.Type != ProviderTypeQWEN {
		t.Fatalf("LoadFile().Providers[\"qwen\"].Type = %q, want %q", qwenCfg.Type, ProviderTypeQWEN)
	}
	if qwenCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("LoadFile().Providers[\"qwen\"].BaseURL = %q, want %q", qwenCfg.BaseURL, "https://dashscope.aliyuncs.com/compatible-mode/v1")
	}
}

func TestLoadFileParsesModelContextWindowTokens(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "custom-model",
		"models": {
			"custom-model": {
				"provider": "placeholder",
				"model": "custom-model",
				"context_window_tokens": 128000
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configFile, err)
	}

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if got := cfg.Models["custom-model"].ContextWindowTokens; got != 128000 {
		t.Fatalf("LoadFile().Models[\"custom-model\"].ContextWindowTokens = %d, want %d", got, 128000)
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
		{
			name: "negative context window tokens rejected",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model",
						"context_window_tokens": -1
					}
				}
			}`,
			wantErrText: `models.custom-model.context_window_tokens must be >= 0`,
		},
		{
			name: "negative max additional retries per step rejected",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model"
					}
				},
				"loop_control": {
					"max_additional_retries_per_step": -1
				}
			}`,
			wantErrText: `loop_control.max_additional_retries_per_step must be >= 0`,
		},
		{
			name: "web backend required when enabled",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model"
					}
				},
				"web": {
					"enabled": true,
					"search_backend": ""
				}
			}`,
			wantErrText: `web.search_backend is required when web.enabled is true`,
		},
		{
			name: "unsupported web backend rejected",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model"
					}
				},
				"web": {
					"enabled": true,
					"search_backend": "bing"
				}
			}`,
			wantErrText: `web.search_backend "bing" is not supported`,
		},
		{
			name: "duckduckgo base url required when enabled",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model"
					}
				},
				"web": {
					"enabled": true,
					"search_backend": "duckduckgo",
					"duckduckgo": {
						"base_url": "",
						"user_agent": "fimi-test/1.0"
					}
				}
			}`,
			wantErrText: `web.duckduckgo.base_url is required when web.enabled is true`,
		},
		{
			name: "duckduckgo user agent required when enabled",
			content: `{
				"default_model": "custom-model",
				"models": {
					"custom-model": {
						"provider": "placeholder",
						"model": "custom-model"
					}
				},
				"web": {
					"enabled": true,
					"search_backend": "duckduckgo",
					"duckduckgo": {
						"base_url": "https://duckduckgo.example/html/",
						"user_agent": ""
					}
				}
			}`,
			wantErrText: `web.duckduckgo.user_agent is required when web.enabled is true`,
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

func TestLoadFileParsesEnabledWebConfig(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{
		"default_model": "custom-model",
		"models": {
			"custom-model": {
				"provider": "placeholder",
				"model": "custom-model"
			}
		},
		"web": {
			"enabled": true,
			"search_backend": "duckduckgo",
			"duckduckgo": {
				"base_url": "https://duckduckgo.example/html/",
				"user_agent": "fimi-test/1.0"
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configFile, err)
	}

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if !cfg.Web.Enabled {
		t.Fatalf("LoadFile().Web.Enabled = false, want true")
	}
	if cfg.Web.SearchBackend != DefaultWebSearchBackend {
		t.Fatalf("LoadFile().Web.SearchBackend = %q, want %q", cfg.Web.SearchBackend, DefaultWebSearchBackend)
	}
	if cfg.Web.DuckDuckGo.BaseURL != "https://duckduckgo.example/html/" {
		t.Fatalf("LoadFile().Web.DuckDuckGo.BaseURL = %q, want %q", cfg.Web.DuckDuckGo.BaseURL, "https://duckduckgo.example/html/")
	}
	if cfg.Web.DuckDuckGo.UserAgent != "fimi-test/1.0" {
		t.Fatalf("LoadFile().Web.DuckDuckGo.UserAgent = %q, want %q", cfg.Web.DuckDuckGo.UserAgent, "fimi-test/1.0")
	}
}

func TestSaveFileWritesValidJSON(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "config.json")

	cfg := Default()
	cfg.DefaultModel = "test-model"
	cfg.Models["test-model"] = ModelConfig{
		Provider: "placeholder",
		Model:    "test-model",
	}

	if err := SaveFile(configFile, cfg); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	// Verify file exists and can be loaded
	loaded, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if loaded.DefaultModel != "test-model" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", loaded.DefaultModel, "test-model")
	}
}

func TestSaveFileCreatesParentDirectory(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "subdir", "nested", "config.json")

	cfg := Default()
	cfg.DefaultModel = "nested-model"
	cfg.Models["nested-model"] = ModelConfig{
		Provider: "placeholder",
		Model:    "nested-model",
	}

	if err := SaveFile(configFile, cfg); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	loaded, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if loaded.DefaultModel != "nested-model" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", loaded.DefaultModel, "nested-model")
	}
}
