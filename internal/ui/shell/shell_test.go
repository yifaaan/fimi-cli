package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
		HistoryFile:   filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
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
	display := newDisplay(&out, true)
	err := visualizeLive(display)(context.Background(), eventsCh)
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
	if !strings.HasSuffix(got, "[step 1]\n[assistant]\nhello\n") {
		t.Fatalf("visualizer output = %q, want finalized transcript suffix", got)
	}
	if gotTranscript := display.transcript.Snapshot(); !equalLines(gotTranscript, []string{"[step 1]", "[assistant]", "hello"}) {
		t.Fatalf("transcript snapshot = %#v", gotTranscript)
	}
}

func TestVisualizeLiveFlushesPreviousStepBeforeRenderingNextOne(t *testing.T) {
	eventsCh := make(chan runtimeevents.Event, 6)
	eventsCh <- runtimeevents.StepBegin{Number: 1}
	eventsCh <- runtimeevents.ToolCall{
		Name:      "bash",
		Arguments: `{"command":"pwd && ls -la"}`,
	}
	eventsCh <- runtimeevents.ToolResult{
		ToolName: "bash",
		Output:   "/tmp/project",
	}
	eventsCh <- runtimeevents.StepBegin{Number: 2}
	eventsCh <- runtimeevents.TextPart{Text: "done"}
	close(eventsCh)

	var out bytes.Buffer
	display := newDisplay(&out, true)
	err := visualizeLive(display)(context.Background(), eventsCh)
	if err != nil {
		t.Fatalf("visualizeLive() error = %v", err)
	}

	wantTranscript := []string{
		"[step 1]",
		`[tool] bash {"command":"pwd && ls -la"}`,
		"[tool result] bash",
		"/tmp/project",
		"[step 2]",
		"[assistant]",
		"done",
	}
	if got := display.transcript.Snapshot(); !equalLines(got, wantTranscript) {
		t.Fatalf("transcript snapshot = %#v, want %#v", got, wantTranscript)
	}
	if !strings.Contains(out.String(), "/tmp/project\n") {
		t.Fatalf("visualizer output = %q, want flushed step 1 transcript", out.String())
	}
}

func TestLiveRendererClearRemovesPreviousBlock(t *testing.T) {
	var out bytes.Buffer
	renderer := newLiveRenderer(&out)

	if err := renderer.Render([]string{"[step 1]", "hello"}); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if err := renderer.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if renderer.renderedLines != 0 {
		t.Fatalf("renderedLines = %d, want 0", renderer.renderedLines)
	}
	if !strings.Contains(out.String(), "\033[2A") {
		t.Fatalf("renderer output = %q, want cursor-up clear sequence", out.String())
	}
}

func TestRunHandlesMetaCommandsWithoutCallingRunner(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	runner := &countingRunner{}

	err := Run(context.Background(), Dependencies{
		Runner:        runner,
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("/help\n/clear\n/exit\n"),
		Output:        &out,
		ErrOutput:     &errOut,
		HistoryFile:   filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
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
	if strings.Contains(out.String(), clearScreenANSI) {
		t.Fatalf("shell output = %q, want no clear screen sequence in fallback mode", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

func TestDisplayClearDropsTranscriptState(t *testing.T) {
	var out bytes.Buffer
	display := newDisplay(&out, true)

	if err := display.AppendTranscriptLines([]string{"first line", "second line"}); err != nil {
		t.Fatalf("AppendTranscriptLines() error = %v", err)
	}
	if err := display.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if got := display.transcript.Snapshot(); len(got) != 0 {
		t.Fatalf("transcript snapshot = %#v, want empty", got)
	}
	if !strings.Contains(out.String(), clearScreenANSI) {
		t.Fatalf("display output = %q, want clear sequence", out.String())
	}
}

func TestRunPrintsUnknownCommandToTranscript(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := Run(context.Background(), Dependencies{
		Runner:        &countingRunner{},
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("/nope\n/exit\n"),
		Output:        &out,
		ErrOutput:     &errOut,
		HistoryFile:   filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "unknown command: /nope\n") {
		t.Fatalf("shell output = %q, want unknown command in transcript", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

func TestRunPrintsPromptErrorsToTranscript(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := Run(context.Background(), Dependencies{
		Runner:        &countingRunner{err: runtime.ErrUnknownStepKind},
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("hello\n/exit\n"),
		Output:        &out,
		ErrOutput:     &errOut,
		HistoryFile:   filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "run error: unknown runtime step kind\n") {
		t.Fatalf("shell output = %q, want run error in transcript", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

func TestRunAppendsSubmittedPromptsToShellHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "shell_history.txt")
	var out bytes.Buffer

	err := Run(context.Background(), Dependencies{
		Runner:        &countingRunner{},
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("first prompt\nsecond prompt\n/exit\n"),
		Output:        &out,
		HistoryFile:   historyFile,
		EditorFactory: scriptedEditorFactory(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "first prompt\nsecond prompt\n" {
		t.Fatalf("history file = %q, want %q", string(data), "first prompt\nsecond prompt\n")
	}
}

func TestRunLoadsExistingShellHistoryWithoutWarning(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "shell_history.txt")
	if err := os.WriteFile(historyFile, []byte("first prompt\nsecond prompt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner:        &countingRunner{},
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("/exit\n"),
		Output:        &out,
		ErrOutput:     &errOut,
		HistoryFile:   historyFile,
		EditorFactory: scriptedEditorFactory(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(out.String(), "shell history unavailable:") {
		t.Fatalf("shell output = %q, want no history warning", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("shell stderr = %q, want empty", errOut.String())
	}
}

func TestRunFallsBackToScannerAndTranscriptWhenNotTTY(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := runtime.New(fakeRuntimeEngine{
		reply: runtime.AssistantReply{
			Text: "assistant reply",
		},
	}, runtime.Config{})

	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner:       runner,
		Store:        store,
		Input:        strings.NewReader("hello\n/exit\n"),
		Output:       &out,
		HistoryFile:  filepath.Join(t.TempDir(), "shell_history.txt"),
		ModelName:    "test-model",
		SystemPrompt: "You are the configured agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "[step 1]\n[assistant]\nassistant reply\n") {
		t.Fatalf("shell output = %q, want transcript output", got)
	}
	if strings.Contains(got, "\033[") {
		t.Fatalf("shell output = %q, want no ansi redraw in fallback mode", got)
	}
}

func TestDisplayClearSkipsANSIWhenNotInteractive(t *testing.T) {
	var out bytes.Buffer
	display := newDisplay(&out, false)
	if err := display.AppendTranscriptLines([]string{"line"}); err != nil {
		t.Fatalf("AppendTranscriptLines() error = %v", err)
	}
	if err := display.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if strings.Contains(out.String(), clearScreenANSI) {
		t.Fatalf("display output = %q, want no clear ansi in non-interactive mode", out.String())
	}
}

type countingRunner struct {
	calls int
	err   error
}

func (r *countingRunner) Run(
	_ context.Context,
	_ contextstore.Context,
	_ runtime.Input,
) (runtime.Result, error) {
	r.calls++
	if r.err != nil {
		return runtime.Result{}, r.err
	}
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

func equalLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}

type scriptedEditor struct {
	lines   []string
	index   int
	history []string
	output  io.Writer
}

func scriptedEditorFactory() lineEditorFactory {
	return func(input io.Reader, output io.Writer, history []string) (lineEditor, error) {
		scanner := bufio.NewScanner(input)
		lines := make([]string, 0, 8)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		return &scriptedEditor{
			lines:   lines,
			history: append([]string(nil), history...),
			output:  output,
		}, nil
	}
}

func (e *scriptedEditor) ReadLine(prompt string) (string, error) {
	if e.output != nil {
		if _, err := io.WriteString(e.output, prompt); err != nil {
			return "", err
		}
	}
	if e.index >= len(e.lines) {
		return "", io.EOF
	}
	line := e.lines[e.index]
	e.index++
	if line == "^C" {
		return "", ErrLineReadAborted
	}
	return line, nil
}

func (e *scriptedEditor) AppendHistory(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	e.history = append(e.history, entry)
}

func (e *scriptedEditor) Close() error {
	return nil
}

type failingEditor struct {
	err error
}

func (e failingEditor) ReadLine(prompt string) (string, error) {
	_ = prompt
	return "", e.err
}

func (e failingEditor) AppendHistory(entry string) {
	_ = entry
}

func (e failingEditor) Close() error {
	return nil
}

func TestRunPrintsInterruptedWhenEditorAbortsPrompt(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner:        &countingRunner{},
		Store:         contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:         strings.NewReader("^C\n/exit\n"),
		Output:        &out,
		HistoryFile:   filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "[interrupted]\n") {
		t.Fatalf("shell output = %q, want interrupted transcript", out.String())
	}
}

func TestRunReturnsEditorReadError(t *testing.T) {
	wantErr := errors.New("editor failed")
	err := Run(context.Background(), Dependencies{
		Runner: &countingRunner{},
		Store:  contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Output: io.Discard,
		EditorFactory: func(input io.Reader, output io.Writer, history []string) (lineEditor, error) {
			return failingEditor{err: wantErr}, nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}
