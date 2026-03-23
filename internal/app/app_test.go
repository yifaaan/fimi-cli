package app

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

func TestBuildLLMConfig(t *testing.T) {
	cfg := config.Config{
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 5,
		},
	}

	got := buildLLMConfig(cfg)
	if got.HistoryTurnLimit != 5 {
		t.Fatalf("buildLLMConfig().HistoryTurnLimit = %d, want %d", got.HistoryTurnLimit, 5)
	}
}

func TestBuildRuntimeConfig(t *testing.T) {
	cfg := config.Config{
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 7,
		},
	}

	got := buildRuntimeConfig(cfg)
	if got.ReplyHistoryTurnLimit != 7 {
		t.Fatalf("buildRuntimeConfig().ReplyHistoryTurnLimit = %d, want %d", got.ReplyHistoryTurnLimit, 7)
	}
}

func TestBuildRuntimeInput(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
		SystemPrompt: "You are the configured agent.",
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input)
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "custom-model",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestBuildEngineUsesPlaceholderByDefault(t *testing.T) {
	cfg := config.Config{
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 3,
		},
	}

	engine, err := buildEngine(cfg)
	if err != nil {
		t.Fatalf("buildEngine() error = %v", err)
	}

	reply, err := engine.Reply(runtime.ReplyInput{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
}

func TestBuildEngineReturnsErrorForUnsupportedMode(t *testing.T) {
	_, err := buildEngine(config.Config{
		EngineMode: "unsupported",
	})
	if !errors.Is(err, errUnsupportedEngineMode) {
		t.Fatalf("buildEngine() error = %v, want wrapped %v", err, errUnsupportedEngineMode)
	}
}

func TestBuildRunnerRunsWithWiredPlaceholderEngine(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
		SystemPrompt: "You are the configured agent.",
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 2,
			LLMTurns:     1,
		},
	}
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner, err := buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner() error = %v", err)
	}

	result, err := runner.Run(ctx, runtime.Input{
		Prompt:       "hello",
		Model:        cfg.DefaultModel,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantResult := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
		contextstore.NewAssistantTextRecord("assistant placeholder reply: hello"),
	}
	if !reflect.DeepEqual(result.AppendedRecords, wantResult) {
		t.Fatalf("Run().AppendedRecords = %#v, want %#v", result.AppendedRecords, wantResult)
	}

	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(records, wantResult) {
		t.Fatalf("history records = %#v, want %#v", records, wantResult)
	}
}

func TestBuildRunnerReturnsErrorForUnsupportedMode(t *testing.T) {
	_, err := buildRunner(config.Config{
		EngineMode: "unsupported",
	})
	if !errors.Is(err, errUnsupportedEngineMode) {
		t.Fatalf("buildRunner() error = %v, want wrapped %v", err, errUnsupportedEngineMode)
	}
}
