package llm

import (
	"errors"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestEngineReplyUsesClient(t *testing.T) {
	client := &spyClient{
		response: Response{
			Text: "assistant placeholder reply: hello",
		},
	}
	engine := NewEngine(client)

	reply, err := engine.Reply(runtime.Input{
		Prompt:       " hello ",
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
	if client.gotRequest.Prompt != "hello" {
		t.Fatalf("got Request.Prompt = %q, want %q", client.gotRequest.Prompt, "hello")
	}
	if client.gotRequest.Model != "kimi-k2-turbo-preview" {
		t.Fatalf("got Request.Model = %q, want %q", client.gotRequest.Model, "kimi-k2-turbo-preview")
	}
	if client.gotRequest.SystemPrompt != "You are fimi, a coding agent." {
		t.Fatalf("got Request.SystemPrompt = %q, want %q", client.gotRequest.SystemPrompt, "You are fimi, a coding agent.")
	}
	if len(client.gotRequest.Messages) != 2 {
		t.Fatalf("len(Request.Messages) = %d, want 2", len(client.gotRequest.Messages))
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
		Content: "hello",
	}) {
		t.Fatalf("Request.Messages[1] = %#v, want %#v", client.gotRequest.Messages[1], Message{
			Role:    RoleUser,
			Content: "hello",
		})
	}
}

func TestEngineReplyWrapsClientError(t *testing.T) {
	wantErr := errors.New("client failed")
	engine := NewEngine(staticClient{
		err: wantErr,
	})

	_, err := engine.Reply(runtime.Input{Prompt: "hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reply() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewEngineWithoutClientFails(t *testing.T) {
	engine := NewEngine(nil)

	_, err := engine.Reply(runtime.Input{Prompt: "hello"})
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
	engine := NewEngine(client)

	_, err := engine.Reply(runtime.Input{
		Prompt: " hello ",
		Model:  "kimi-k2-turbo-preview",
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if len(client.gotRequest.Messages) != 1 {
		t.Fatalf("len(Request.Messages) = %d, want 1", len(client.gotRequest.Messages))
	}
	if client.gotRequest.Messages[0] != (Message{
		Role:    RoleUser,
		Content: "hello",
	}) {
		t.Fatalf("Request.Messages[0] = %#v, want %#v", client.gotRequest.Messages[0], Message{
			Role:    RoleUser,
			Content: "hello",
		})
	}
	if client.gotRequest.Prompt != "hello" {
		t.Fatalf("got Request.Prompt = %q, want %q", client.gotRequest.Prompt, "hello")
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
