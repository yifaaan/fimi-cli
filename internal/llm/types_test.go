package llm

import "testing"

func TestRequestLastUserMessage(t *testing.T) {
	request := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
			{Role: RoleUser, Content: "first"},
			{Role: RoleUser, Content: "second"},
		},
	}

	message, ok := request.LastUserMessage()
	if !ok {
		t.Fatalf("LastUserMessage() ok = false, want true")
	}
	if message != (Message{
		Role:    RoleUser,
		Content: "second",
	}) {
		t.Fatalf("LastUserMessage() = %#v, want %#v", message, Message{
			Role:    RoleUser,
			Content: "second",
		})
	}
}

func TestRequestPrimaryUserPromptFallsBackToPrompt(t *testing.T) {
	request := Request{
		Prompt: "hello",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
		},
	}

	if got := request.PrimaryUserPrompt(); got != "hello" {
		t.Fatalf("PrimaryUserPrompt() = %q, want %q", got, "hello")
	}
}
