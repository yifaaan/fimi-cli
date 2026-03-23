package llm

import (
	"testing"

	"fimi-cli/internal/runtime"
)

func TestPlaceholderEngineReply(t *testing.T) {
	engine := PlaceholderEngine{}

	reply, err := engine.Reply(runtime.Input{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
}
