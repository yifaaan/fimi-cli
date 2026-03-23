package llm

import (
	"testing"

	"fimi-cli/internal/runtime"
)

func TestPlaceholderClientReplyText(t *testing.T) {
	client := PlaceholderClient{}

	response, err := client.Reply(Request{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if response.Text != "assistant placeholder reply: hello" {
		t.Fatalf("Reply().Text = %q, want %q", response.Text, "assistant placeholder reply: hello")
	}
}

func TestNewPlaceholderEngine(t *testing.T) {
	engine := NewPlaceholderEngine()

	reply, err := engine.Reply(runtime.Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
}
