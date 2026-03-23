package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fimi-cli/internal/llm"
)

func TestClientReply(t *testing.T) {
	// 创建 mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// 验证路径
		if r.URL.Path != chatPath {
			t.Errorf("expected path %s, got %s", chatPath, r.URL.Path)
		}

		// 验证 Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Authorization 'Bearer test-key', got %q", auth)
		}

		// 返回模拟响应
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 创建 client
	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	// 发送请求
	resp, err := client.Reply(llm.Request{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	expected := "Hello! How can I help you?"
	if resp.Text != expected {
		t.Errorf("Reply() text = %q, want %q", resp.Text, expected)
	}
}

func TestClientReplyWithSystemPrompt(t *testing.T) {
	var receivedRequest chatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedRequest); err != nil {
			t.Errorf("decode request: %v", err)
		}

		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: "OK"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	_, err := client.Reply(llm.Request{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	// 验证 system prompt 被正确添加
	if len(receivedRequest.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(receivedRequest.Messages))
	}
	if receivedRequest.Messages[0].Role != llm.RoleSystem {
		t.Errorf("first message role = %q, want %q", receivedRequest.Messages[0].Role, llm.RoleSystem)
	}
	if receivedRequest.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("system prompt = %q, want %q", receivedRequest.Messages[0].Content, "You are a helpful assistant.")
	}
}

func TestClientReplyAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Error: &chatError{
				Type:    "invalid_api_key",
				Message: "Invalid API key provided",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "bad-key",
		Model:   "test-model",
	})

	_, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 检查错误包含 API 错误信息
	if !containsString(err.Error(), "invalid_api_key") {
		t.Errorf("error should contain 'invalid_api_key', got: %v", err)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}