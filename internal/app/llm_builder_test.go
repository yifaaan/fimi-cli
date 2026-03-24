package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
)

func TestResolveConfiguredModel(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		want    config.ModelConfig
		wantErr string
	}{
		{
			name: "configured model found",
			cfg: config.Config{
				DefaultModel: "qwen-workhorse",
				Models: map[string]config.ModelConfig{
					"qwen-workhorse": {
						Provider: "aliyun-prod",
						Model:    "qwen-plus",
					},
				},
			},
			want: config.ModelConfig{
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
		},
		{
			name: "empty model falls back to alias",
			cfg: config.Config{
				DefaultModel: "qwen-workhorse",
				Models: map[string]config.ModelConfig{
					"qwen-workhorse": {
						Provider: "aliyun-prod",
					},
				},
			},
			want: config.ModelConfig{
				Provider: "aliyun-prod",
				Model:    "qwen-workhorse",
			},
		},
		{
			name: "missing default model",
			cfg: config.Config{
				DefaultModel: "missing",
				Models: map[string]config.ModelConfig{
					"other": {
						Provider: config.ProviderTypePlaceholder,
					},
				},
			},
			wantErr: `default model "missing" not found in config.models`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveConfiguredModel(tt.cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveConfiguredModel() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("resolveConfiguredModel() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveConfiguredModel() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveConfiguredModel() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveConfiguredProvider(t *testing.T) {
	tests := []struct {
		name             string
		cfg              config.Config
		modelCfg         config.ModelConfig
		wantProviderName string
		wantProviderCfg  config.ProviderConfig
		wantErr          string
	}{
		{
			name: "placeholder provider is synthesized",
			cfg:  config.Config{},
			modelCfg: config.ModelConfig{
				Provider: config.DefaultProviderName,
				Model:    "unused",
			},
			wantProviderName: config.DefaultProviderName,
			wantProviderCfg: config.ProviderConfig{
				Type: config.ProviderTypePlaceholder,
			},
		},
		{
			name: "external provider resolved",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"aliyun-prod": {
						Type:    config.ProviderTypeQWEN,
						BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
						APIKey:  "sk-test-key",
					},
				},
			},
			modelCfg: config.ModelConfig{
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
			wantProviderName: "aliyun-prod",
			wantProviderCfg: config.ProviderConfig{
				Type:    config.ProviderTypeQWEN,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
				APIKey:  "sk-test-key",
			},
		},
		{
			name: "missing provider entry",
			cfg:  config.Config{},
			modelCfg: config.ModelConfig{
				Provider: "missing-provider",
				Model:    "qwen-plus",
			},
			wantErr: `provider "missing-provider" not found in config.providers`,
		},
		{
			name: "provider type required",
			cfg: config.Config{
				Providers: map[string]config.ProviderConfig{
					"aliyun-prod": {
						BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
					},
				},
			},
			modelCfg: config.ModelConfig{
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
			wantErr: "providers.aliyun-prod.type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotCfg, err := resolveConfiguredProvider(tt.cfg, tt.modelCfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveConfiguredProvider() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("resolveConfiguredProvider() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveConfiguredProvider() error = %v", err)
			}
			if gotName != tt.wantProviderName {
				t.Fatalf("resolveConfiguredProvider() providerName = %q, want %q", gotName, tt.wantProviderName)
			}
			if gotCfg != tt.wantProviderCfg {
				t.Fatalf("resolveConfiguredProvider() providerCfg = %#v, want %#v", gotCfg, tt.wantProviderCfg)
			}
		})
	}
}

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

	reply, err := engine.Reply(context.Background(), runtime.ReplyInput{
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply.Text != "assistant placeholder reply: hello" {
		t.Fatalf("Reply().Text = %q, want %q", reply.Text, "assistant placeholder reply: hello")
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

	reply, err := engine.Reply(context.Background(), runtime.ReplyInput{
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply.Text != "assistant placeholder reply: hello" {
		t.Fatalf("Reply().Text = %q, want %q", reply.Text, "assistant placeholder reply: hello")
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

func TestBuildQwenClientIncludesProviderNameInAPIKeyError(t *testing.T) {
	_, err := buildQwenClient(
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
		t.Fatalf("buildQwenClient() error = nil, want non-nil")
	}
	want := "qwen api_key is required; set providers.aliyun-prod.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)"
	if err.Error() != want {
		t.Fatalf("buildQwenClient() error = %q, want %q", err.Error(), want)
	}
}

func TestBuildQwenClientPassesConfiguredModel(t *testing.T) {
	var receivedModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		model, ok := body["model"].(string)
		if !ok {
			t.Fatalf("request model type = %T, want string", body["model"])
		}
		receivedModel = model

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client, err := buildQwenClient(
		"aliyun-prod",
		config.ProviderConfig{
			Type:    config.ProviderTypeQWEN,
			APIKey:  "sk-test-key",
			BaseURL: server.URL,
		},
		config.ModelConfig{
			Model: "qwen-plus",
		},
	)
	if err != nil {
		t.Fatalf("buildQwenClient() error = %v", err)
	}

	_, err = client.Reply(llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("client.Reply() error = %v", err)
	}

	if receivedModel != "qwen-plus" {
		t.Fatalf("request model = %q, want %q", receivedModel, "qwen-plus")
	}
}

func TestBuildLLMClientFromConfigBuildsQWENClientFromModelAndProviderChain(t *testing.T) {
	var receivedModel string
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		body := make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		model, ok := body["model"].(string)
		if !ok {
			t.Fatalf("request model type = %T, want string", body["model"])
		}
		receivedModel = model

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client, err := buildLLMClientFromConfig(config.Config{
		DefaultModel: "workhorse",
		Models: map[string]config.ModelConfig{
			"workhorse": {
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"aliyun-prod": {
				Type:    config.ProviderTypeQWEN,
				APIKey:  "sk-test-key",
				BaseURL: server.URL,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildLLMClientFromConfig() error = %v", err)
	}

	resp, err := client.Reply(llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("client.Reply() error = %v", err)
	}

	if resp.Text != "ok" {
		t.Fatalf("client.Reply().Text = %q, want %q", resp.Text, "ok")
	}
	if receivedModel != "qwen-plus" {
		t.Fatalf("request model = %q, want %q", receivedModel, "qwen-plus")
	}
	if receivedAuth != "Bearer sk-test-key" {
		t.Fatalf("Authorization = %q, want %q", receivedAuth, "Bearer sk-test-key")
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
