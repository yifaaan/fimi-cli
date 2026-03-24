package app

import (
	"errors"
	"testing"

	"fimi-cli/internal/config"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
)

func TestBuildEngineUsesPlaceholderByDefault(t *testing.T) {
	cfg := config.Config{
		DefaultModel: config.DefaultModelName,
		Models: map[string]config.ModelConfig{
			config.DefaultModelName: {
				Provider: config.ProviderTypePlaceholder,
				Model:    config.DefaultModelName,
			},
		},
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

func TestDependenciesBuildEngineUsesInjectedClientBuilder(t *testing.T) {
	var gotCfg config.Config
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotCfg = cfg
			return llm.NewPlaceholderClient(), nil
		},
	}

	engine, err := deps.buildEngine(config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 3,
		},
	})
	if err != nil {
		t.Fatalf("buildEngine() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if gotCfg.Models["custom-model"].Provider != config.ProviderTypePlaceholder {
		t.Fatalf("builder got provider = %q, want %q", gotCfg.Models["custom-model"].Provider, config.ProviderTypePlaceholder)
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
		DefaultModel: "broken",
		Models: map[string]config.ModelConfig{
			"broken": {
				Provider: "custom-provider",
				Model:    "broken-model",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"custom-provider": {
				Type: "unsupported",
			},
		},
	})
	if !errors.Is(err, ErrUnsupportedProviderType) {
		t.Fatalf("buildEngine() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

func TestBuildLLMClientForProviderReturnsErrorForUnsupportedType(t *testing.T) {
	_, err := buildLLMClientForProvider(
		"custom-provider",
		config.ProviderConfig{Type: "unsupported"},
		config.ModelConfig{Model: "broken-model"},
	)
	if !errors.Is(err, ErrUnsupportedProviderType) {
		t.Fatalf("buildLLMClientForProvider() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

func TestBuildLLMClientForProviderUsesPlaceholderBuilder(t *testing.T) {
	client, err := buildLLMClientForProvider(
		config.DefaultProviderName,
		config.ProviderConfig{Type: config.ProviderTypePlaceholder},
		config.ModelConfig{Model: "unused"},
	)
	if err != nil {
		t.Fatalf("buildLLMClientForProvider() error = %v", err)
	}

	reply, err := client.Reply(llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("client.Reply() error = %v", err)
	}
	if reply.Text != "assistant placeholder reply: hello" {
		t.Fatalf("client.Reply().Text = %q, want %q", reply.Text, "assistant placeholder reply: hello")
	}
}

func TestBuildPlaceholderClient(t *testing.T) {
	client, err := buildPlaceholderClient(
		config.DefaultProviderName,
		config.ProviderConfig{Type: config.ProviderTypePlaceholder},
		config.ModelConfig{Model: "unused"},
	)
	if err != nil {
		t.Fatalf("buildPlaceholderClient() error = %v", err)
	}

	reply, err := client.Reply(llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("client.Reply() error = %v", err)
	}
	if reply.Text != "assistant placeholder reply: hello" {
		t.Fatalf("client.Reply().Text = %q, want %q", reply.Text, "assistant placeholder reply: hello")
	}
}

func TestBuildLLMClientForProviderUsesQWENBuilder(t *testing.T) {
	_, err := buildLLMClientForProvider(
		"aliyun-prod",
		config.ProviderConfig{
			Type:    config.ProviderTypeQWEN,
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		config.ModelConfig{
			Model: "qwen-plus",
		},
	)
	if err == nil {
		t.Fatalf("buildLLMClientForProvider() error = nil, want non-nil")
	}
	want := "qwen api_key is required; set providers.aliyun-prod.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)"
	if err.Error() != want {
		t.Fatalf("buildLLMClientForProvider() error = %q, want %q", err.Error(), want)
	}
}

func TestBuildEngineResolvesProviderByTypeNotProviderName(t *testing.T) {
	_, err := buildEngine(config.Config{
		DefaultModel: "qwen-workhorse",
		Models: map[string]config.ModelConfig{
			"qwen-workhorse": {
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"aliyun-prod": {
				Type:    config.ProviderTypeQWEN,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			},
		},
	})
	if err == nil {
		t.Fatalf("buildEngine() error = nil, want non-nil")
	}
	want := "qwen api_key is required; set providers.aliyun-prod.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)"
	if err.Error() != "build llm client: "+want {
		t.Fatalf("buildEngine() error = %q, want %q", err.Error(), "build llm client: "+want)
	}
}
