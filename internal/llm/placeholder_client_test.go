package llm

import (
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

func TestPlaceholderClientReplyText(t *testing.T) {
	client := PlaceholderClient{}

	response, err := client.Reply(Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	})
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

func TestPlaceholderClientReplyUsesEmptyPromptWithoutUserMessage(t *testing.T) {
	client := PlaceholderClient{}

	response, err := client.Reply(Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if response.Text != "assistant placeholder reply: " {
		t.Fatalf("Reply().Text = %q, want %q", response.Text, "assistant placeholder reply: ")
	}
}

func TestNewPlaceholderEngine(t *testing.T) {
	engine := NewPlaceholderEngine(Config{})

	reply, err := engine.Reply(runtime.ReplyInput{
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
	if len(reply.ToolCalls) != 0 {
		t.Fatalf("len(Reply().ToolCalls) = %d, want 0", len(reply.ToolCalls))
	}
}

func TestNewPlaceholderClient(t *testing.T) {
	client := NewPlaceholderClient()

	response, err := client.Reply(Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if response.Text != "assistant placeholder reply: hello" {
		t.Fatalf("Reply().Text = %q, want %q", response.Text, "assistant placeholder reply: hello")
	}
}
