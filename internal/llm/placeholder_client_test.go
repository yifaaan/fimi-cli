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

func TestPlaceholderClientReplyUsesLastUserMessage(t *testing.T) {
	client := PlaceholderClient{}

	response, err := client.Reply(Request{
		Prompt: "ignored fallback",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
			{Role: RoleUser, Content: "first"},
			{Role: RoleUser, Content: "second"},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if response.Text != "assistant placeholder reply: second" {
		t.Fatalf("Reply().Text = %q, want %q", response.Text, "assistant placeholder reply: second")
	}
}

func TestPlaceholderClientReplyFallsBackToPromptWithoutUserMessage(t *testing.T) {
	client := PlaceholderClient{}

	response, err := client.Reply(Request{
		Prompt: "hello",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
		},
	})
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
