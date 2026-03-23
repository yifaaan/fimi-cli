package llm

import (
	"errors"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

func TestEngineReplyUsesClient(t *testing.T) {
	client := &spyClient{
		response: Response{
			Text: "assistant placeholder reply: hello",
		},
	}
	engine := NewEngine(client, Config{})

	reply, err := engine.Reply(runtime.ReplyInput{
		Prompt:       " hello ",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
		History: []contextstore.TextRecord{
			contextstore.NewSystemTextRecord("boot"),
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
	if client.gotRequest.Model != "kimi-k2-turbo-preview" {
		t.Fatalf("got Request.Model = %q, want %q", client.gotRequest.Model, "kimi-k2-turbo-preview")
	}
	if client.gotRequest.SystemPrompt != "You are fimi, a coding agent." {
		t.Fatalf("got Request.SystemPrompt = %q, want %q", client.gotRequest.SystemPrompt, "You are fimi, a coding agent.")
	}
	if len(client.gotRequest.Messages) != 4 {
		t.Fatalf("len(Request.Messages) = %d, want 4", len(client.gotRequest.Messages))
	}
	if client.gotRequest.Messages[0] != (Message{
		Role:    RoleSystem,
		Content: "You are fimi, a coding agent.",
	}) {
		t.Fatalf("Request.Messages[0] = %#v, want %#v", client.gotRequest.Messages[0], Message{
			Role:    RoleSystem,
			Content: "You are fimi, a coding agent.",
		})
	}
	if client.gotRequest.Messages[1] != (Message{
		Role:    RoleUser,
		Content: "previous",
	}) {
		t.Fatalf("Request.Messages[1] = %#v, want %#v", client.gotRequest.Messages[1], Message{
			Role:    RoleUser,
			Content: "previous",
		})
	}
	if client.gotRequest.Messages[2] != (Message{
		Role:    RoleAssistant,
		Content: "previous reply",
	}) {
		t.Fatalf("Request.Messages[2] = %#v, want %#v", client.gotRequest.Messages[2], Message{
			Role:    RoleAssistant,
			Content: "previous reply",
		})
	}
	if client.gotRequest.Messages[3] != (Message{
		Role:    RoleUser,
		Content: "hello",
	}) {
		t.Fatalf("Request.Messages[3] = %#v, want %#v", client.gotRequest.Messages[3], Message{
			Role:    RoleUser,
			Content: "hello",
		})
	}
	prompt, ok := client.gotRequest.PrimaryUserPrompt()
	if !ok {
		t.Fatalf("PrimaryUserPrompt() ok = false, want true")
	}
	if prompt != "hello" {
		t.Fatalf("PrimaryUserPrompt() = %q, want %q", prompt, "hello")
	}
}

func TestEngineReplyWrapsClientError(t *testing.T) {
	wantErr := errors.New("client failed")
	engine := NewEngine(staticClient{
		err: wantErr,
	}, Config{})

	_, err := engine.Reply(runtime.ReplyInput{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reply() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewEngineWithoutClientFails(t *testing.T) {
	engine := NewEngine(nil, Config{})

	_, err := engine.Reply(runtime.ReplyInput{Prompt: "hello"})
	if err == nil {
		t.Fatalf("Reply() error = nil, want non-nil")
	}
	if err.Error() != "llm client reply: llm client is required" {
		t.Fatalf("Reply() error = %q, want %q", err.Error(), "llm client reply: llm client is required")
	}
}

func TestEngineReplyBuildsUserOnlyMessageWhenSystemPromptEmpty(t *testing.T) {
	client := &spyClient{
		response: Response{
			Text: "assistant placeholder reply: hello",
		},
	}
	engine := NewEngine(client, Config{})

	_, err := engine.Reply(runtime.ReplyInput{
		Prompt: " hello ",
		Model:  "kimi-k2-turbo-preview",
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if len(client.gotRequest.Messages) != 3 {
		t.Fatalf("len(Request.Messages) = %d, want 3", len(client.gotRequest.Messages))
	}
	if client.gotRequest.Messages[0] != (Message{
		Role:    RoleUser,
		Content: "previous",
	}) {
		t.Fatalf("Request.Messages[0] = %#v, want %#v", client.gotRequest.Messages[0], Message{
			Role:    RoleUser,
			Content: "previous",
		})
	}
	if client.gotRequest.Messages[1] != (Message{
		Role:    RoleAssistant,
		Content: "previous reply",
	}) {
		t.Fatalf("Request.Messages[1] = %#v, want %#v", client.gotRequest.Messages[1], Message{
			Role:    RoleAssistant,
			Content: "previous reply",
		})
	}
	if client.gotRequest.Messages[2] != (Message{
		Role:    RoleUser,
		Content: "hello",
	}) {
		t.Fatalf("Request.Messages[2] = %#v, want %#v", client.gotRequest.Messages[2], Message{
			Role:    RoleUser,
			Content: "hello",
		})
	}
	prompt, ok := client.gotRequest.PrimaryUserPrompt()
	if !ok {
		t.Fatalf("PrimaryUserPrompt() ok = false, want true")
	}
	if prompt != "hello" {
		t.Fatalf("PrimaryUserPrompt() = %q, want %q", prompt, "hello")
	}
}

func TestEngineReplyUsesConfiguredTurnLimit(t *testing.T) {
	client := &spyClient{
		response: Response{
			Text: "assistant placeholder reply: hello",
		},
	}
	engine := NewEngine(client, Config{
		HistoryTurnLimit: 1,
	})

	_, err := engine.Reply(runtime.ReplyInput{
		Prompt: "hello",
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("first"),
			contextstore.NewAssistantTextRecord("first reply"),
			contextstore.NewUserTextRecord("second"),
			contextstore.NewAssistantTextRecord("second reply"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	want := []Message{
		{Role: RoleUser, Content: "second"},
		{Role: RoleAssistant, Content: "second reply"},
		{Role: RoleUser, Content: "hello"},
	}
	if len(client.gotRequest.Messages) != len(want) {
		t.Fatalf("len(Request.Messages) = %d, want %d", len(client.gotRequest.Messages), len(want))
	}
	for i, message := range want {
		if client.gotRequest.Messages[i] != message {
			t.Fatalf("Request.Messages[%d] = %#v, want %#v", i, client.gotRequest.Messages[i], message)
		}
	}
}

type staticClient struct {
	response Response
	err      error
}

func (c staticClient) Reply(request Request) (Response, error) {
	return c.response, c.err
}

type spyClient struct {
	gotRequest Request
	response   Response
	err        error
}

func (c *spyClient) Reply(request Request) (Response, error) {
	c.gotRequest = request
	return c.response, c.err
}
