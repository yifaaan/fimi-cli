package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fimi-cli/internal/llm"
)

func TestClientReplyStreamTextDeltasAccumulate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// 两个 text delta + DONE
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test"})

	ctx := context.Background()
	var deltas []string
	handler := llm.StreamHandlerFunc(func(ctx context.Context, event llm.StreamEvent) error {
		switch ev := event.(type) {
		case llm.TextDeltaEvent:
			deltas = append(deltas, ev.Delta)
		}
		return nil
	})

	resp, err := client.ReplyStream(ctx, llm.Request{Model: "test", Messages: []llm.Message{}}, handler)
	if err != nil {
		t.Fatalf("ReplyStream returned error: %v", err)
	}

	if strings.Join(deltas, "") != "Hello world" {
		t.Fatalf("expected deltas to accumulate to %q, got %q", "Hello world", strings.Join(deltas, ""))
	}
	if resp.Text != "Hello world" {
		t.Fatalf("expected resp.Text %q, got %q", "Hello world", resp.Text)
	}
}

func TestClientReplyStreamToolCallDeltasAccumulate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// tool call 分两段 arguments
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"command\\\": \\\"\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"function\":{\"arguments\":\"ls\\\"}\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test"})

	ctx := context.Background()
	var toolDeltas []llm.ToolCallDeltaEvent
	handler := llm.StreamHandlerFunc(func(ctx context.Context, event llm.StreamEvent) error {
		switch ev := event.(type) {
		case llm.ToolCallDeltaEvent:
			toolDeltas = append(toolDeltas, ev)
		}
		return nil
	})

	resp, err := client.ReplyStream(ctx, llm.Request{Model: "test", Messages: []llm.Message{}}, handler)
	if err != nil {
		t.Fatalf("ReplyStream returned error: %v", err)
	}

	if len(toolDeltas) != 2 {
		t.Fatalf("expected 2 tool deltas, got %d", len(toolDeltas))
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Name != "bash" {
		t.Fatalf("expected tool call name bash, got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments != `{"command": "ls"}` {
		t.Fatalf("expected accumulated arguments %q, got %q", `{"command": "ls"}`, resp.ToolCalls[0].Arguments)
	}
}
