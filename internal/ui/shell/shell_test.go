package shell

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestRunProcessesInitialPromptBeforeEnteringLoop(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := runtime.New(fakeRuntimeEngine{
		reply: runtime.AssistantReply{
			Text: "assistant reply",
		},
	}, runtime.Config{})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner:        runner,
		Store:         store,
		Input:         strings.NewReader("/exit\n"),
		Output:        &out,
		ErrOutput:     &errOut,
		ModelName:     "test-model",
		SystemPrompt:  "You are the configured agent.",
		InitialPrompt: "fix tests",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "[step 1]\n") {
		t.Fatalf("shell output = %q, want step header", out.String())
	}
	if !strings.Contains(out.String(), "[assistant]\nassistant reply\n") {
		t.Fatalf("shell output = %q, want assistant live block", out.String())
	}
	if !strings.HasSuffix(out.String(), promptText) {
		t.Fatalf("shell output = %q, want trailing prompt %q", out.String(), promptText)
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

func TestLiveStateBuildsRenderableLines(t *testing.T) {
	state := liveState{}
	state.Apply(runtimeevents.StepBegin{Number: 2})
	state.Apply(runtimeevents.StatusUpdate{
		Status: runtimeevents.StatusSnapshot{ContextUsage: 0.25},
	})
	state.Apply(runtimeevents.TextPart{Text: "hello"})
	state.Apply(runtimeevents.ToolCall{
		Name:      "bash",
		Arguments: `{"command":"ls"}`,
	})
	state.Apply(runtimeevents.ToolCallPart{Delta: ` --color=never`})
	state.Apply(runtimeevents.ToolResult{
		ToolName: "bash",
		Output:   "file.txt",
	})

	rendered := strings.Join(state.Lines(), "\n")
	for _, want := range []string{
		"[step 2]",
		"[status] context used 25%",
		"[assistant]\nhello",
		`[tool] bash {"command":"ls"} --color=never`,
		"[tool result] bash\nfile.txt",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered lines = %q, want substring %q", rendered, want)
		}
	}
}

func TestVisualizeLiveRedrawsShellBlock(t *testing.T) {
	eventsCh := make(chan runtimeevents.Event, 4)
	eventsCh <- runtimeevents.StepBegin{Number: 1}
	eventsCh <- runtimeevents.TextPart{Text: "hel"}
	eventsCh <- runtimeevents.TextPart{Text: "lo"}
	close(eventsCh)

	var out bytes.Buffer
	err := visualizeLive(&out)(context.Background(), eventsCh)
	if err != nil {
		t.Fatalf("visualizeLive() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "hello\n") {
		t.Fatalf("visualizer output = %q, want final assistant text", got)
	}
	if !strings.Contains(got, "\033[") {
		t.Fatalf("visualizer output = %q, want ansi redraw sequence", got)
	}
}

func TestRunHandlesMetaCommandsWithoutCallingRunner(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	runner := &countingRunner{}

	err := Run(context.Background(), Dependencies{
		Runner:    runner,
		Store:     contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:     strings.NewReader("/help\n/clear\n/exit\n"),
		Output:    &out,
		ErrOutput: &errOut,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
	if !strings.Contains(out.String(), "Shell commands:\n") {
		t.Fatalf("shell output = %q, want help text", out.String())
	}
	if !strings.Contains(out.String(), clearScreenANSI) {
		t.Fatalf("shell output = %q, want clear screen sequence", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

type countingRunner struct {
	calls int
}

func (r *countingRunner) Run(
	_ context.Context,
	_ contextstore.Context,
	_ runtime.Input,
) (runtime.Result, error) {
	r.calls++
	return runtime.Result{Status: runtime.RunStatusFinished}, nil
}

type fakeRuntimeEngine struct {
	reply runtime.AssistantReply
	err   error
}

func (e fakeRuntimeEngine) Reply(
	ctx context.Context,
	input runtime.ReplyInput,
) (runtime.AssistantReply, error) {
	return e.reply, e.err
}
