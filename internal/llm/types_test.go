package llm

import (
	"reflect"
	"testing"
)

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
	want := Message{
		Role:    RoleUser,
		Content: "second",
	}
	if !reflect.DeepEqual(message, want) {
		t.Fatalf("LastUserMessage() = %#v, want %#v", message, want)
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

func TestRequestLastUserMessageIgnoresToolResultMessage(t *testing.T) {
	request := Request{
		Messages: []Message{
			{Role: RoleUser, Content: "draft plan"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "read_file"}}},
			{Role: RoleTool, ToolCallID: "call_1", Content: "file content"},
		},
	}

	message, ok := request.LastUserMessage()
	if !ok {
		t.Fatalf("LastUserMessage() ok = false, want true")
	}

	want := Message{
		Role:    RoleUser,
		Content: "draft plan",
	}
	if !reflect.DeepEqual(message, want) {
		t.Fatalf("LastUserMessage() = %#v, want %#v", message, want)
	}
}

func TestMessageHasToolCalls(t *testing.T) {
	message := Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "bash", Arguments: `{"command":"pwd"}`},
		},
	}

	if !message.HasToolCalls() {
		t.Fatalf("HasToolCalls() = false, want true")
	}
}

func TestMessageIsToolResult(t *testing.T) {
	message := Message{
		Role:       RoleTool,
		ToolCallID: "call_1",
		Content:    "command output",
	}

	if !message.IsToolResult() {
		t.Fatalf("IsToolResult() = false, want true")
	}
}

func TestResponseHasToolCalls(t *testing.T) {
	response := Response{
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path":"main.go"}`},
		},
	}

	if !response.HasToolCalls() {
		t.Fatalf("HasToolCalls() = false, want true")
	}
}
