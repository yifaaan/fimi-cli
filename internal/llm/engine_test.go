package llm

import (
	"errors"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestEngineReplyUsesClient(t *testing.T) {
	engine := NewEngine(staticClient{
		response: Response{
			Text: "assistant placeholder reply: hello",
		},
	})

	reply, err := engine.Reply(runtime.Input{Prompt: " hello "})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
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

type staticClient struct {
	response Response
	err      error
}

func (c staticClient) Reply(request Request) (Response, error) {
	return c.response, c.err
}
