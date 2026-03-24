package llm

import (
	"errors"
	"reflect"
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
		Model:        "kimi-k2-turbo-preview",
		SystemPrompt: "You are fimi, a coding agent.",
		History: []contextstore.TextRecord{
			contextstore.NewSystemTextRecord("boot"),
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
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
	if client.gotRequest.Model != "kimi-k2-turbo-preview" {
		t.Fatalf("got Request.Model = %q, want %q", client.gotRequest.Model, "kimi-k2-turbo-preview")
	}
	if client.gotRequest.SystemPrompt != "You are fimi, a coding agent." {
		t.Fatalf("got Request.SystemPrompt = %q, want %q", client.gotRequest.SystemPrompt, "You are fimi, a coding agent.")
	}
	if len(client.gotRequest.Messages) != 4 {
		t.Fatalf("len(Request.Messages) = %d, want 4", len(client.gotRequest.Messages))
	}
	wantMessages := []Message{
		{
			Role:    RoleSystem,
			Content: "You are fimi, a coding agent.",
		},
		{
			Role:    RoleUser,
			Content: "previous",
		},
		{
			Role:    RoleAssistant,
			Content: "previous reply",
		},
		{
			Role:    RoleUser,
			Content: "hello",
		},
	}
	if !reflect.DeepEqual(client.gotRequest.Messages, wantMessages) {
		t.Fatalf("Request.Messages = %#v, want %#v", client.gotRequest.Messages, wantMessages)
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

	_, err := engine.Reply(runtime.ReplyInput{
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reply() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNewEngineWithoutClientFails(t *testing.T) {
	engine := NewEngine(nil, Config{})

	_, err := engine.Reply(runtime.ReplyInput{
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("hello"),
		},
	})
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
		Model:  "kimi-k2-turbo-preview",
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("previous"),
			contextstore.NewAssistantTextRecord("previous reply"),
			contextstore.NewUserTextRecord("hello"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	wantMessages := []Message{
		{
			Role:    RoleUser,
			Content: "previous",
		},
		{
			Role:    RoleAssistant,
			Content: "previous reply",
		},
		{
			Role:    RoleUser,
			Content: "hello",
		},
	}
	if !reflect.DeepEqual(client.gotRequest.Messages, wantMessages) {
		t.Fatalf("Request.Messages = %#v, want %#v", client.gotRequest.Messages, wantMessages)
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
		History: []contextstore.TextRecord{
			// 历史记录：2 个完整 turn
			contextstore.NewUserTextRecord("first"),
			contextstore.NewAssistantTextRecord("first reply"),
			contextstore.NewUserTextRecord("second"),
			contextstore.NewAssistantTextRecord("second reply"),
			// 当前用户输入（不计入 turn limit）
			contextstore.NewUserTextRecord("hello"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	// HistoryTurnLimit=1 表示保留最近 1 个历史 turn
	// 当前用户输入 "hello" 不计入限制，所以最终是：
	// [second(U), second reply(A), hello(U)]
	// 这实际包含 2 个 user message，但只有 1 个历史 turn + 1 个当前输入
	want := []Message{
		{Role: RoleUser, Content: "second"},
		{Role: RoleAssistant, Content: "second reply"},
		{Role: RoleUser, Content: "hello"},
	}
	if !reflect.DeepEqual(client.gotRequest.Messages, want) {
		t.Fatalf("Request.Messages = %#v, want %#v", client.gotRequest.Messages, want)
	}
}

func TestEngineReplyMapsToolCallsToRuntimeReply(t *testing.T) {
	client := staticClient{
		response: Response{
			Text: "I will inspect the file.",
			ToolCalls: []ToolCall{
				{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
			},
		},
	}
	engine := NewEngine(client, Config{})

	reply, err := engine.Reply(runtime.ReplyInput{
		History: []contextstore.TextRecord{
			contextstore.NewUserTextRecord("inspect main.go"),
		},
	})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	want := runtime.AssistantReply{
		Text: "I will inspect the file.",
		ToolCalls: []runtime.ToolCall{
			{ID: "call_read", Name: "read_file", Arguments: `{"path":"main.go"}`},
		},
	}
	if !reflect.DeepEqual(reply, want) {
		t.Fatalf("Reply() = %#v, want %#v", reply, want)
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
