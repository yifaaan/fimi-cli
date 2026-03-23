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
}

func TestLoadFileReturnsDefaultWhenMissing(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "missing.json")

	cfg, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg != Default() {
		t.Fatalf("LoadFile() = %#v, want %#v", cfg, Default())
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
