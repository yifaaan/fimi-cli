package config

import "testing"

func TestDefaultIncludesHistoryWindow(t *testing.T) {
	cfg := Default()

	if cfg.HistoryWindow.RuntimeTurns != DefaultRuntimeTurns {
		t.Fatalf("Default().HistoryWindow.RuntimeTurns = %d, want %d", cfg.HistoryWindow.RuntimeTurns, DefaultRuntimeTurns)
	}
	if cfg.HistoryWindow.LLMTurns != DefaultLLMTurns {
		t.Fatalf("Default().HistoryWindow.LLMTurns = %d, want %d", cfg.HistoryWindow.LLMTurns, DefaultLLMTurns)
	}
}
