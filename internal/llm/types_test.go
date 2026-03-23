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

func TestRequestPrimaryUserPrompt(t *testing.T) {
	request := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
			{Role: RoleUser, Content: "hello"},
		},
	}

	got, ok := request.PrimaryUserPrompt()
	if !ok {
		t.Fatalf("PrimaryUserPrompt() ok = false, want true")
	}
	if got != "hello" {
		t.Fatalf("PrimaryUserPrompt() = %q, want %q", got, "hello")
	}
}

func TestRequestPrimaryUserPromptWithoutUserMessage(t *testing.T) {
	request := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are fimi, a coding agent."},
		},
	}

	got, ok := request.PrimaryUserPrompt()
	if ok {
		t.Fatalf("PrimaryUserPrompt() ok = true, want false")
	}
	if got != "" {
		t.Fatalf("PrimaryUserPrompt() = %q, want empty string", got)
	}
}
