package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
		StartupInfo: StartupInfo{
			SessionID:      "session-123",
			SessionReused:  false,
			ModelName:      "test-model",
			ConversationDB: store.Path(),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "Shell session\n") {
		t.Fatalf("shell output = %q, want startup banner", out.String())
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
	}
}

func TestRunPrintsRestoredTranscriptBeforePromptLoop(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner: &countingRunner{},
		Store:  contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		Input:  strings.NewReader("/exit\n"),
		Output: &out,
		HistoryFile: filepath.Join(t.TempDir(), "shell_history.txt"),
		EditorFactory: scriptedEditorFactory(),
		InitialRecords: []contextstore.TextRecord{
			contextstore.NewSystemTextRecord("session initialized"),
			contextstore.NewUserTextRecord("continue the refactor"),
			{
				Role:          contextstore.RoleAssistant,
				Content:       "picked up\nfrom the latest checkpoint",
				ToolCallsJSON: `[{"ID":"call_read","Name":"read_file","Arguments":"{\"path\":\"main.go\"}"}]`,
			},
			contextstore.NewToolResultRecord("call_read", "package main"),
		},
		StartupInfo: StartupInfo{
			SessionID:      "session-123",
			SessionReused:  true,
			ModelName:      "test-model",
			ConversationDB: "/tmp/history.jsonl",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := out.String()
	wantParts := []string{
		"Shell session\n",
		"[user]\ncontinue the refactor\n",
		"[assistant]\npicked up\nfrom the latest checkpoint\n",
		"[tool] Read main.go\n",
		"[tool result]\npackage main\n",
	}
	lastIndex := -1
	for _, want := range wantParts {
		idx := strings.Index(got, want)
		if idx < 0 {
			t.Fatalf("shell output = %q, want substring %q", got, want)
		}
		if idx <= lastIndex {
			t.Fatalf("shell output order incorrect = %q", got)
		}
		lastIndex = idx
	}
	if !strings.HasSuffix(got, promptText) {
		t.Fatalf("shell output = %q, want trailing prompt %q", got, promptText)
	}
}

func TestTranscriptLineModelsFromRecordsSkipsBootstrapRecord(t *testing.T) {
	got := transcriptLineModelsFromRecords([]contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("continue the refactor"),
		{
			Role:          contextstore.RoleAssistant,
			Content:       "picked up\nfrom the latest checkpoint",
			ToolCallsJSON: `[{"ID":"call_read","Name":"read_file","Arguments":"{\"path\":\"main.go\"}"}]`,
		},
		contextstore.NewToolResultRecord("call_read", "package main"),
	})

	want := []TranscriptLine{
		{Type: LineTypeUser, Content: "continue the refactor"},
		{Type: LineTypeAssistant, Content: "picked up\nfrom the latest checkpoint"},
		{Type: LineTypeToolCall, Content: "Read main.go"},
		{Type: LineTypeToolResult, Content: "package main"},
	}
	if len(got) != len(want) {
		t.Fatalf("len(lines) = %d, want %d; lines=%#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Content != want[i].Content {
			t.Fatalf("lines[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestNewModelStartsWithRestoredTranscript(t *testing.T) {
	model := NewModel(Dependencies{
		InitialRecords: []contextstore.TextRecord{
			contextstore.NewSystemTextRecord("session initialized"),
			contextstore.NewUserTextRecord("continue the refactor"),
			{
				Role:          contextstore.RoleAssistant,
				Content:       "picked up\nfrom the latest checkpoint",
				ToolCallsJSON: `[{"ID":"call_read","Name":"read_file","Arguments":"{\"path\":\"main.go\"}"}]`,
			},
			contextstore.NewToolResultRecord("call_read", "package main"),
		},
		StartupInfo: StartupInfo{SessionID: "session-123"},
	}, nil)

	want := []TranscriptLine{
		{Type: LineTypeUser, Content: "continue the refactor"},
		{Type: LineTypeAssistant, Content: "picked up\nfrom the latest checkpoint"},
		{Type: LineTypeToolCall, Content: "Read main.go"},
		{Type: LineTypeToolResult, Content: "package main"},
	}
	if len(model.output.lines) != len(want) {
		t.Fatalf("len(model.output.lines) = %d, want %d; lines=%#v", len(model.output.lines), len(want), model.output.lines)
	}
	for i := range want {
		if model.output.lines[i].Type != want[i].Type || model.output.lines[i].Content != want[i].Content {
			t.Fatalf("model.output.lines[%d] = %#v, want %#v", i, model.output.lines[i], want[i])
		}
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

func TestLiveStateUsesToolSubtitleForThinkCalls(t *testing.T) {
	state := liveState{}
	state.Apply(runtimeevents.ToolCall{
		Name:      "think",
		Subtitle:  "compare parser branch behavior",
		Arguments: `{"thought":"compare parser branch behavior"}`,
	})

	rendered := strings.Join(state.Lines(), "\n")
	if !strings.Contains(rendered, "[tool] compare parser branch behavior") {
		t.Fatalf("rendered lines = %q, want think subtitle", rendered)
	}
}

func TestRuntimeModelUsesToolSubtitleInTranscriptAndCard(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_think",
		Name:      "think",
		Subtitle:  "compare parser branch behavior",
		Arguments: `{"thought":"compare parser branch behavior"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID: "call_think",
		ToolName:   "think",
		Output:     "Thought logged",
	})

	lines := model.ToLines()
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want at least 2", len(lines))
	}
	if lines[1].Content != "compare parser branch behavior" {
		t.Fatalf("tool call transcript = %q, want %q", lines[1].Content, "compare parser branch behavior")
	}

	card := model.ToolCardView(60)
	if !strings.Contains(card, "compare parser branch behavior") {
		t.Fatalf("tool card = %q, want think subtitle", card)
	}
}

func TestRuntimeModelUsesHumanizedBashSubtitleInTranscript(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_bash",
		Name:      "bash",
		Subtitle:  "Ran git status --short",
		Arguments: `{"command":"git status --short"}`,
	})

	lines := model.ToLines()
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want at least 2", len(lines))
	}
	if lines[1].Content != "Ran git status --short" {
		t.Fatalf("tool call transcript = %q, want %q", lines[1].Content, "Ran git status --short")
	}
}

func TestRuntimeModelPreservesOrderedStepTranscript(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 2})
	model = model.ApplyEvent(runtimeevents.TextPart{Text: "checking"})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_think",
		Name:      "think",
		Subtitle:  "compare parser branch behavior",
		Arguments: `{"thought":"compare parser branch behavior"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID: "call_think",
		ToolName:   "think",
		Output:     "Thought logged",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_read",
		Name:      "read_file",
		Subtitle:  "Read main.go",
		Arguments: `{"path":"main.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID: "call_read",
		ToolName:   "read_file",
		Output:     "package main",
	})

	got := model.ToLines()
	want := []TranscriptLine{
		{Type: LineTypeSystem, Content: "[step 2]"},
		{Type: LineTypeAssistant, Content: "checking"},
		{Type: LineTypeToolCall, Content: "compare parser branch behavior"},
		{Type: LineTypeToolResult, Content: "Thought logged"},
		{Type: LineTypeToolCall, Content: "Read main.go"},
		{Type: LineTypeToolResult, Content: "package main"},
	}
	if len(got) != len(want) {
		t.Fatalf("len(lines) = %d, want %d; lines=%#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Content != want[i].Content {
			t.Fatalf("lines[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestModelFlushesPreviousStepBeforeRenderingNextStep(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.eventsCh = make(chan runtimeevents.Event)
	model.mode = ModeStreaming
	model.runtime = model.runtime.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model.runtime = model.runtime.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_think",
		Name:      "think",
		Subtitle:  "compare parser branch behavior",
		Arguments: `{"thought":"compare parser branch behavior"}`,
	})
	model.output = model.output.SetPending(model.runtime.ToLines())

	updated, cmd := model.Update(RuntimeEventMsg{
		Event: runtimeevents.StepBegin{Number: 2},
	})
	model = updated.(Model)

	if cmd == nil {
		t.Fatal("cmd = nil, want wait command batch")
	}
	if len(model.output.lines) == 0 {
		t.Fatal("flushed transcript lines = 0, want previous step persisted")
	}
	if model.output.lines[0].Content != "[step 1]" {
		t.Fatalf("first flushed line = %q, want %q", model.output.lines[0].Content, "[step 1]")
	}
	if len(model.output.pending) == 0 || model.output.pending[0].Content != "[step 2]" {
		t.Fatalf("pending lines = %#v, want new step header first", model.output.pending)
	}
}

func TestModelAppliesBatchedRuntimeEventsInOrder(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.eventsCh = make(chan runtimeevents.Event)

	updated, _ := model.Update(RuntimeEventsMsg{
		Events: []runtimeevents.Event{
			runtimeevents.StepBegin{Number: 1},
			runtimeevents.TextPart{Text: "hel"},
			runtimeevents.TextPart{Text: "lo"},
			runtimeevents.ToolCall{
				ID:       "call_think",
				Name:     "think",
				Subtitle: "compare parser branch behavior",
			},
		},
	})
	model = updated.(Model)

	if len(model.output.pending) != 3 {
		t.Fatalf("pending lines = %d, want 3", len(model.output.pending))
	}
	if model.output.pending[1].Content != "hello" {
		t.Fatalf("assistant content = %q, want %q", model.output.pending[1].Content, "hello")
	}
	if model.output.pending[2].Content != "compare parser branch behavior" {
		t.Fatalf("tool call content = %q, want %q", model.output.pending[2].Content, "compare parser branch behavior")
	}
}

func TestOutputModelMouseWheelScrollsTranscript(t *testing.T) {
	model := NewOutputModel()
	model.height = 10
	model.width = 80
	for i := 1; i <= 8; i++ {
		model = model.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("line %d", i),
		})
	}

	view := model.View()
	if !strings.Contains(view, "line 8") {
		t.Fatalf("initial view = %q, want latest line", view)
	}
	if strings.Contains(view, "line 1") {
		t.Fatalf("initial view = %q, want oldest line hidden", view)
	}

	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}, model.width, model.height)
	model = updated

	view = model.View()
	if !strings.Contains(view, "line 1") {
		t.Fatalf("scrolled view = %q, want oldest line visible after wheel up", view)
	}
	if strings.Contains(view, "line 8") {
		t.Fatalf("scrolled view = %q, want latest line hidden after wheel up", view)
	}

	updated, _ = model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}, model.width, model.height)
	model = updated

	view = model.View()
	if !strings.Contains(view, "line 8") {
		t.Fatalf("restored view = %q, want latest line visible after wheel down", view)
	}
}

func TestOutputModelPreservesScrollPositionWhenNotAtBottom(t *testing.T) {
	model := NewOutputModel()
	model.height = 10
	model.width = 80
	for i := 1; i <= 8; i++ {
		model = model.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("line %d", i),
		})
	}

	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}, model.width, model.height)
	model = updated
	if model.atBottom {
		t.Fatal("atBottom = true, want false after scrolling up")
	}

	model = model.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: "line 9",
	})

	view := model.View()
	if strings.Contains(view, "line 9") {
		t.Fatalf("view after append = %q, want scroll position preserved while not at bottom", view)
	}
}

func TestOutputModelMouseWheelScrollsWrappedTranscriptRows(t *testing.T) {
	model := NewOutputModel()
	model.height = 10
	model.width = 20
	model = model.AppendLine(TranscriptLine{
		Type: LineTypeAssistant,
		Content: strings.Join([]string{
			"line 1",
			"line 2",
			"line 3",
			"line 4",
			"line 5",
			"line 6",
			"line 7",
			"line 8",
		}, "\n"),
	})

	view := model.View()
	if !strings.Contains(view, "line 8") {
		t.Fatalf("initial view = %q, want latest wrapped row", view)
	}
	if strings.Contains(view, "line 1") {
		t.Fatalf("initial view = %q, want top wrapped row hidden before scroll", view)
	}

	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}, model.width, model.height)
	model = updated

	view = model.View()
	if !strings.Contains(view, "line 1") {
		t.Fatalf("scrolled view = %q, want earliest wrapped row visible after wheel up", view)
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
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
	var errOut bytes.Buffer
	err := Run(context.Background(), Dependencies{
		Runner:       runner,
		Store:        store,
		Input:        strings.NewReader("hello\n/exit\n"),
		Output:       &out,
		ErrOutput:    &errOut,
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
	if !strings.Contains(errOut.String(), "shell ui disabled: stdin is not a TTY; falling back to text mode\n") {
		t.Fatalf("shell stderr = %q, want fallback reason", errOut.String())
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

func TestModelReplacesPendingSnapshotForRuntimeEvents(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.eventsCh = make(chan runtimeevents.Event)

	updated, _ := model.Update(RuntimeEventMsg{Event: runtimeevents.TextPart{Text: "hel"}})
	model = updated.(Model)

	updated, _ = model.Update(RuntimeEventMsg{Event: runtimeevents.TextPart{Text: "lo"}})
	model = updated.(Model)

	if len(model.output.pending) != 1 {
		t.Fatalf("pending lines = %d, want 1", len(model.output.pending))
	}
	if model.output.pending[0].Content != "hello" {
		t.Fatalf("pending content = %q, want %q", model.output.pending[0].Content, "hello")
	}
}

func TestInputModelBackspaceDeletesSingleASCIICharacter(t *testing.T) {
	input := NewInputModel()

	input, _ = input.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("abc"),
	}, 80)
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyBackspace}, 80)

	if input.Value() != "ab" {
		t.Fatalf("input value = %q, want %q", input.Value(), "ab")
	}
}

func TestInputModelBackspaceDeletesSingleChineseCharacter(t *testing.T) {
	input := NewInputModel()

	input, _ = input.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("你好"),
	}, 80)
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyBackspace}, 80)

	if input.Value() != "你" {
		t.Fatalf("input value = %q, want %q", input.Value(), "你")
	}
}


func TestModelAdvancesSpinnerWhileRunning(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking

	initialView := model.runtime.SpinnerView()
	tickCmd := model.runtime.SpinnerCmd()
	msg := tickCmd()

	if _, ok := msg.(spinner.TickMsg); !ok {
		t.Fatalf("tick msg type = %T, want spinner.TickMsg", msg)
	}

	updated, nextCmd := model.Update(msg)
	model = updated.(Model)

	if nextCmd == nil {
		t.Fatalf("nextCmd = nil, want next spinner tick command")
	}
	if model.runtime.SpinnerView() == initialView {
		t.Fatalf("spinner view = %q, want animation frame to advance from %q", model.runtime.SpinnerView(), initialView)
	}
}

func TestModelIgnoresLateRuntimeEventsAfterCompletion(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeIdle
	model.output = model.output.SetPending([]TranscriptLine{
		{Type: LineTypeAssistant, Content: "partial"},
	})

	model = model.finishRuntime(RuntimeCompleteMsg{
		Result: runtime.Result{Status: runtime.RunStatusFinished},
	})

	if model.mode != ModeIdle {
		t.Fatalf("mode after completion = %v, want %v", model.mode, ModeIdle)
	}

	updated, cmd := model.Update(RuntimeEventMsg{
		Event: runtimeevents.StatusUpdate{
			Status: runtimeevents.StatusSnapshot{ContextUsage: 0.5},
		},
	})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd after late event = %#v, want nil", cmd)
	}
	if model.mode != ModeIdle {
		t.Fatalf("mode after late event = %v, want %v", model.mode, ModeIdle)
	}
	if model.eventsCh != nil {
		t.Fatal("eventsCh should remain nil after late event")
	}
}

func TestModelDefersCompletionUntilRuntimeEventsClosed(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeStreaming
	model.eventsCh = make(chan runtimeevents.Event)
	model.runtime = model.runtime.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model.runtime = model.runtime.ApplyEvent(runtimeevents.TextPart{Text: "partial"})
	model.output = model.output.SetPending(model.runtime.ToLines())

	updated, cmd := model.Update(RuntimeCompleteMsg{
		Result: runtime.Result{Status: runtime.RunStatusFinished},
	})
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("cmd after completion = %#v, want nil while waiting for drain", cmd)
	}
	if model.mode != ModeStreaming {
		t.Fatalf("mode after early completion = %v, want %v", model.mode, ModeStreaming)
	}
	if model.pendingCompletion == nil {
		t.Fatal("pendingCompletion = nil, want deferred completion")
	}

	updated, _ = model.Update(RuntimeEventsMsg{
		Events: []runtimeevents.Event{
			runtimeevents.TextPart{Text: " answer"},
		},
		Closed: true,
	})
	model = updated.(Model)

	if model.mode != ModeIdle {
		t.Fatalf("mode after drain = %v, want %v", model.mode, ModeIdle)
	}
	if model.pendingCompletion != nil {
		t.Fatal("pendingCompletion != nil after drain")
	}
	if len(model.output.lines) < 2 || model.output.lines[1].Content != "partial answer" {
		t.Fatalf("flushed transcript = %#v, want final assistant text preserved", model.output.lines)
	}
}

func TestStartRuntimeExecutionStreamsEventsBeforeCompletion(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner := runtime.New(fakeRuntimeEngine{
		reply: runtime.AssistantReply{
			Text: "assistant reply",
		},
	}, runtime.Config{})

	model := NewModel(Dependencies{
		Runner:       runner,
		Store:        store,
		ModelName:    "test-model",
		SystemPrompt: "You are the configured agent.",
	}, nil)

	eventsCh := make(chan runtimeevents.Event, 32)
	cmd := model.startRuntimeExecution("hello", eventsCh)

	done := make(chan RuntimeCompleteMsg, 1)
	go func() {
		msg := cmd()
		complete, ok := msg.(RuntimeCompleteMsg)
		if !ok {
			t.Errorf("completion msg type = %T, want RuntimeCompleteMsg", msg)
			return
		}
		done <- complete
	}()

	var allEvents []runtimeevents.Event
	for {
		msg := waitForRuntimeEvents(eventsCh)()
		batch, ok := msg.(RuntimeEventsMsg)
		if !ok {
			t.Fatalf("event msg type = %T, want RuntimeEventsMsg", msg)
		}
		allEvents = append(allEvents, batch.Events...)
		if batch.Closed {
			break
		}
	}

	complete := <-done
	if complete.Err != nil {
		t.Fatalf("completion err = %v, want nil", complete.Err)
	}
	if complete.Result.Status != runtime.RunStatusFinished {
		t.Fatalf("completion status = %q, want %q", complete.Result.Status, runtime.RunStatusFinished)
	}

	if len(allEvents) < 3 {
		t.Fatalf("len(allEvents) = %d, want at least 3", len(allEvents))
	}
	if stepBegin, ok := allEvents[0].(runtimeevents.StepBegin); !ok || stepBegin.Number != 1 {
		t.Fatalf("first event = %#v, want StepBegin{Number:1}", allEvents[0])
	}
	textSeen := false
	statusSeen := false
	for _, event := range allEvents[1:] {
		switch e := event.(type) {
		case runtimeevents.TextPart:
			if e.Text == "assistant reply" {
				textSeen = true
			}
		case runtimeevents.StatusUpdate:
			statusSeen = true
		}
	}
	if !textSeen {
		t.Fatalf("events = %#v, want assistant text event", allEvents)
	}
	if !statusSeen {
		t.Fatalf("events = %#v, want status update event", allEvents)
	}
}

func TestWaitForRuntimeEventsBatchesBufferedEventsAndSignalsClosure(t *testing.T) {
	eventsCh := make(chan runtimeevents.Event, runtimeEventBatchSize+4)
	for i := 0; i < runtimeEventBatchSize+4; i++ {
		eventsCh <- runtimeevents.TextPart{Text: fmt.Sprintf("chunk-%d", i)}
	}
	close(eventsCh)

	firstMsg := waitForRuntimeEvents(eventsCh)()
	firstBatch, ok := firstMsg.(RuntimeEventsMsg)
	if !ok {
		t.Fatalf("first msg type = %T, want RuntimeEventsMsg", firstMsg)
	}
	if len(firstBatch.Events) != runtimeEventBatchSize {
		t.Fatalf("first batch len = %d, want %d", len(firstBatch.Events), runtimeEventBatchSize)
	}
	if firstBatch.Closed {
		t.Fatal("first batch closed = true, want false while buffered events remain")
	}

	secondMsg := waitForRuntimeEvents(eventsCh)()
	secondBatch, ok := secondMsg.(RuntimeEventsMsg)
	if !ok {
		t.Fatalf("second msg type = %T, want RuntimeEventsMsg", secondMsg)
	}
	if len(secondBatch.Events) != 4 {
		t.Fatalf("second batch len = %d, want 4", len(secondBatch.Events))
	}
	if !secondBatch.Closed {
		t.Fatal("second batch closed = false, want true after channel drain")
	}
	if got := secondBatch.Events[3].(runtimeevents.TextPart).Text; got != "chunk-67" {
		t.Fatalf("last event text = %q, want %q", got, "chunk-67")
	}
}

func TestInteractiveTTYStatusReturnsTERMReasonFirst(t *testing.T) {
	t.Setenv("TERM", "dumb")

	interactive, reason := interactiveTTYStatus(nil, nil)
	if interactive {
		t.Fatalf("interactive = true, want false")
	}
	if reason != "TERM=dumb" {
		t.Fatalf("reason = %q, want %q", reason, "TERM=dumb")
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
