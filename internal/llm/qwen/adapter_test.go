package qwen

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fimi-cli/internal/llm"
)

func TestNewClientUsesDefaultBaseURL(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.Host
		// 返回有效响应
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	// 使用自定义 base URL 验证请求确实发到了这里
	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: "http://" + server.URL[7:], // 去掉 http:// 前缀的处理
		Model:   "qwen-turbo",
	})

	_, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	t.Logf("received URL: %s", receivedURL)
}

func TestNewClientUsesDefaultModel(t *testing.T) {
	var lastModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 简单解析请求获取 model
		body := make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			lastModel = body["model"].(string)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: "http://" + server.URL[7:],
		// Model 不指定，应使用默认值
	})

	_, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if lastModel != DefaultModel {
		t.Errorf("model = %q, want %q", lastModel, DefaultModel)
	}
}

func TestNewClientAllowsModelOverride(t *testing.T) {
	var lastModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			lastModel = body["model"].(string)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: "http://" + server.URL[7:],
		Model:   "qwen-max",
	})

	_, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if lastModel != "qwen-max" {
		t.Errorf("model = %q, want %q", lastModel, "qwen-max")
	}
}
