package openai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
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

func TestClientReplyIncludesToolsInRequest(t *testing.T) {
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
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "List files"},
		},
		Tools: []llm.ToolDefinition{
			{
				Name:        "bash",
				Description: "Run a shell command inside the workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if len(receivedRequest.Tools) != 1 {
		t.Fatalf("len(request.Tools) = %d, want 1", len(receivedRequest.Tools))
	}
	if receivedRequest.Tools[0].Function.Name != "bash" {
		t.Fatalf("request.Tools[0].Function.Name = %q, want %q", receivedRequest.Tools[0].Function.Name, "bash")
	}
	if receivedRequest.Tools[0].Function.Description != "Run a shell command inside the workspace." {
		t.Fatalf("request.Tools[0].Function.Description = %q, want %q", receivedRequest.Tools[0].Function.Description, "Run a shell command inside the workspace.")
	}
	if receivedRequest.ToolChoice != nil {
		t.Fatalf("request.ToolChoice = %#v, want nil", receivedRequest.ToolChoice)
	}
}

func TestClientReplyParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessage{
						Role:    "assistant",
						Content: "I will inspect the file.",
						ToolCalls: []chatToolCall{
							{
								ID:   "call_read",
								Type: "function",
								Function: chatToolFunction{
									Name:      "read_file",
									Arguments: `{"path":"main.go"}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})

	resp, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Inspect main.go"}},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if resp.Text != "I will inspect the file." {
		t.Fatalf("Reply().Text = %q, want %q", resp.Text, "I will inspect the file.")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(Reply().ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0] != (llm.ToolCall{
		ID:        "call_read",
		Name:      "read_file",
		Arguments: `{"path":"main.go"}`,
	}) {
		t.Fatalf("Reply().ToolCalls[0] = %#v, want %#v", resp.ToolCalls[0], llm.ToolCall{
			ID:        "call_read",
			Name:      "read_file",
			Arguments: `{"path":"main.go"}`,
		})
	}
}

func TestClientReplyAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
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
	if runtime.IsRetryable(err) {
		t.Fatalf("runtime.IsRetryable(error) = true, want false")
	}
}

func TestClientReplyMarksRetryableAPIStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		resp := chatResponse{
			Error: &chatError{
				Type:    "rate_limit",
				Message: "too many requests",
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
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !runtime.IsRetryable(err) {
		t.Fatalf("runtime.IsRetryable(error) = false, want true")
	}
}

func TestClientReplyMarksRetryableTransportErrors(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "https://example.invalid",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	client.http = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp: connection refused")
		}),
	}

	_, err := client.Reply(llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !runtime.IsRetryable(err) {
		t.Fatalf("runtime.IsRetryable(error) = false, want true")
	}
}

func TestClientReplyResponsesParsesMessageAndToolCalls(t *testing.T) {
	var receivedBody responseRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != responsesPath {
			t.Fatalf("expected path %s, got %s", responsesPath, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		resp := responseAPIResponse{
			Output: []responseOutputItem{
				{
					Type: "message",
					Content: []responseContentPart{
						{Type: "output_text", Text: "Listing files..."},
					},
				},
				{
					Type:      "function_call",
					ID:        "fc_item_1",
					CallID:    "call_ls",
					Name:      "bash",
					Arguments: `{"command":"ls -la"}`,
				},
			},
			Usage: &responseUsage{
				InputTokens:  11,
				OutputTokens: 7,
				TotalTokens:  18,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.4",
		WireAPI: WireAPIResp,
	})

	resp, err := client.Reply(llm.Request{
		SystemPrompt: "Use tools.",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "list current dir"},
		},
		Tools: []llm.ToolDefinition{
			{
				Name:        "bash",
				Description: "Run shell commands.",
				Parameters: map[string]any{
					"type": "object",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if receivedBody.Instructions != "Use tools." {
		t.Fatalf("instructions = %q, want %q", receivedBody.Instructions, "Use tools.")
	}
	if len(receivedBody.Input) != 1 || receivedBody.Input[0].Role != llm.RoleUser {
		t.Fatalf("input = %#v, want one user message", receivedBody.Input)
	}
	if len(receivedBody.Tools) != 1 || receivedBody.Tools[0].Name != "bash" {
		t.Fatalf("tools = %#v, want bash function tool", receivedBody.Tools)
	}
	if receivedBody.ToolChoice != nil {
		t.Fatalf("tool_choice = %#v, want nil", receivedBody.ToolChoice)
	}

	if resp.Text != "Listing files..." {
		t.Fatalf("Reply().Text = %q, want %q", resp.Text, "Listing files...")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(Reply().ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0] != (llm.ToolCall{
		ID:        "call_ls",
		Name:      "bash",
		Arguments: `{"command":"ls -la"}`,
	}) {
		t.Fatalf("Reply().ToolCalls[0] = %#v", resp.ToolCalls[0])
	}
	if resp.Usage.TotalTokens != 18 {
		t.Fatalf("Reply().Usage.TotalTokens = %d, want %d", resp.Usage.TotalTokens, 18)
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

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
