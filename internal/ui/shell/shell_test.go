package shell

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
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

	if !strings.Contains(out.String(), "[step 1]\nassistant reply\n") {
		t.Fatalf("shell output = %q, want assistant transcript", out.String())
	}
	if !strings.HasSuffix(out.String(), promptText) {
		t.Fatalf("shell output = %q, want trailing prompt %q", out.String(), promptText)
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
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
