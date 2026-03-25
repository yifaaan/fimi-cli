package shell

import (
	"context"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestBindRuntimeEventsForwardsEventsToBubbleTea(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := runtime.New(staticRuntimeEngine{
		reply: runtime.AssistantReply{
			Text: "streamed hello",
		},
	}, runtime.Config{})

	messages := make([]tea.Msg, 0, 4)
	eventfulRunner := bindRuntimeEvents(runner, func(msg tea.Msg) {
		messages = append(messages, msg)
	})

	_, err := eventfulRunner.Run(context.Background(), store, runtime.Input{
		Prompt: "hello",
		Model:  "test-model",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !containsRuntimeEvent(messages, runtimeevents.StepBegin{Number: 1}) {
		t.Fatalf("messages = %#v, want step begin event", messages)
	}
	if !containsRuntimeEvent(messages, runtimeevents.TextPart{Text: "streamed hello"}) {
		t.Fatalf("messages = %#v, want text part event", messages)
	}
}

type staticRuntimeEngine struct {
	reply runtime.AssistantReply
	err   error
}

func (e staticRuntimeEngine) Reply(
	ctx context.Context,
	input runtime.ReplyInput,
) (runtime.AssistantReply, error) {
	return e.reply, e.err
}

func containsRuntimeEvent(messages []tea.Msg, want runtimeevents.Event) bool {
	for _, msg := range messages {
		eventMsg, ok := msg.(runtimeEventMsg)
		if !ok {
			continue
		}
		if eventMsg.event == want {
			return true
		}
	}

	return false
}
