package shell

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestUpdateHandlesKeyInput(t *testing.T) {
	m := Model{running: false}

	// Type a character
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := m.Update(keyMsg)

	newModel := updated.(Model)
	if newModel.prompt != "a" {
		t.Errorf("prompt = %q, want %q", newModel.prompt, "a")
	}
}

func TestUpdateHandlesBackspace(t *testing.T) {
	m := Model{prompt: "hello", running: false}

	backspaceMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	updated, _ := m.Update(backspaceMsg)

	newModel := updated.(Model)
	if newModel.prompt != "hell" {
		t.Errorf("prompt = %q, want %q", newModel.prompt, "hell")
	}
}

func TestUpdateHandlesEnter(t *testing.T) {
	m := Model{prompt: "hello world", running: false}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterMsg)

	newModel := updated.(Model)
	// After Enter, prompt should be cleared and running=true
	if newModel.running != true {
		t.Errorf("running = %v, want true", newModel.running)
	}
	if newModel.output.String() != "user: hello world\n" {
		t.Errorf("output buffer = %q, want %q", newModel.output.String(), "user: hello world\n")
	}
	if cmd == nil {
		t.Error("expected non-nil command after Enter")
	}
}

func TestUpdateHandlesRuntimeEvent(t *testing.T) {
	m := Model{running: true}

	eventMsg := runtimeEventMsg{
		event: runtimeevents.TextPart{Text: "hello"},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	if newModel.output.String() != "assistant: hello" {
		t.Errorf("output buffer = %q, want %q", newModel.output.String(), "assistant: hello")
	}
	if newModel.status.Step != 0 {
		t.Errorf("status.Step = %d, want 0", newModel.status.Step)
	}
}

func TestUpdateTracksStatusUpdateContextUsage(t *testing.T) {
	m := Model{running: true}

	eventMsg := runtimeEventMsg{
		event: runtimeevents.StatusUpdate{
			Status: runtimeevents.StatusSnapshot{ContextUsage: 0.25},
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	if newModel.status.ContextUsage != 0.25 {
		t.Errorf("status.ContextUsage = %v, want 0.25", newModel.status.ContextUsage)
	}
}

func TestUpdateHandlesRuntimeDone(t *testing.T) {
	m := Model{running: true}

	doneMsg := runtimeDoneMsg{err: nil}
	updated, _ := m.Update(doneMsg)

	newModel := updated.(Model)
	if newModel.running {
		t.Error("running should be false after runtime done")
	}
}

func TestUpdateHandlesCtrlD(t *testing.T) {
	m := Model{running: false}

	ctrlDMsg := tea.KeyMsg{Type: tea.KeyCtrlD}
	_, cmd := m.Update(ctrlDMsg)

	if cmd == nil {
		t.Error("expected tea.Quit command on Ctrl+D")
	}
}

func TestUpdateHandlesInterruptMsg(t *testing.T) {
	m := Model{running: true}

	updated, _ := m.Update(interruptMsg{})

	newModel := updated.(Model)
	if newModel.running {
		t.Error("running should be false after interrupt")
	}
}

func TestUpdateHandlesWindowSize(t *testing.T) {
	m := Model{running: false, width: 80}

	windowSizeMsg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updated, _ := m.Update(windowSizeMsg)

	newModel := updated.(Model)
	if newModel.width != 120 {
		t.Errorf("width = %d, want 120", newModel.width)
	}
}

func TestUpdateIgnoresKeyInputWhenRunning(t *testing.T) {
	m := Model{prompt: "hello", running: true}

	// Try to type while running
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := m.Update(keyMsg)

	newModel := updated.(Model)
	// Prompt should not change while running
	if newModel.prompt != "hello" {
		t.Errorf("prompt = %q, want %q (should not change while running)", newModel.prompt, "hello")
	}
}

func TestUpdateCtrlCWhenRunningReturnsInterrupt(t *testing.T) {
	m := Model{prompt: "hello", running: true}

	ctrlCMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, cmd := m.Update(ctrlCMsg)

	newModel := updated.(Model)
	if newModel.running {
		t.Error("running should still be true immediately after Ctrl+C")
	}
	// Cmd should send interruptMsg
	if cmd == nil {
		t.Error("expected non-nil command after Ctrl+C to send interruptMsg")
	}
}

func TestUpdateCtrlCWhenNotRunningClearsPrompt(t *testing.T) {
	m := Model{prompt: "hello world", running: false}

	ctrlCMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := m.Update(ctrlCMsg)

	newModel := updated.(Model)
	if newModel.prompt != "" {
		t.Errorf("prompt = %q, want empty string after Ctrl+C", newModel.prompt)
	}
	if newModel.showHelp {
		t.Error("showHelp should be false after Ctrl+C")
	}
}

func TestUpdateEmptyPromptOnEnter(t *testing.T) {
	m := Model{prompt: "   ", running: false}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterMsg)

	newModel := updated.(Model)
	// Empty prompt should not start runtime
	if newModel.running {
		t.Error("running should be false for empty prompt")
	}
	if cmd != nil {
		t.Error("expected nil command for empty prompt")
	}
}

func TestUpdateHandlesMetaCommandHelp(t *testing.T) {
	m := Model{prompt: "/help", running: false}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterMsg)

	newModel := updated.(Model)
	// Meta command should clear prompt
	if newModel.prompt != "" {
		t.Errorf("prompt = %q, want empty after meta command", newModel.prompt)
	}
	// Cmd should trigger help display
	if cmd == nil {
		t.Error("expected non-nil command for /help")
	}
}

func TestUpdateHandlesShowHelpMsg(t *testing.T) {
	m := Model{showHelp: false}

	updated, _ := m.Update(showHelpMsg{})

	newModel := updated.(Model)
	if !newModel.showHelp {
		t.Error("showHelp should be true after showHelpMsg")
	}
}

func TestUpdateRuntimeDoneWithError(t *testing.T) {
	m := Model{running: true}

	doneMsg := runtimeDoneMsg{err: assertError("something went wrong")}
	updated, _ := m.Update(doneMsg)

	newModel := updated.(Model)
	if newModel.running {
		t.Error("running should be false after runtime done")
	}
	if newModel.err == nil {
		t.Error("err should be set after runtime done with error")
	}
}

func TestUpdateRuntimeDoneSkipsAlreadyStreamedAssistantText(t *testing.T) {
	m := Model{running: true}
	m.output.WriteString("assistant: streamed hello")
	m.assistantLineOpen = true

	doneMsg := runtimeDoneMsg{
		result: runtime.Result{
			Steps: []runtime.StepResult{
				{
					TextStreamed: true,
					AppendedRecords: []contextstore.TextRecord{
						contextstore.NewAssistantTextRecord("streamed hello"),
					},
				},
			},
		},
	}
	updated, _ := m.Update(doneMsg)

	newModel := updated.(Model)
	if newModel.output.String() != "assistant: streamed hello\n" {
		t.Errorf("output buffer = %q, want %q", newModel.output.String(), "assistant: streamed hello\n")
	}
}

func TestUpdateRuntimeDoneAppendsAssistantFallbackIntoTranscript(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("hello")

	doneMsg := runtimeDoneMsg{
		result: runtime.Result{
			Steps: []runtime.StepResult{
				{
					AppendedRecords: []contextstore.TextRecord{
						contextstore.NewAssistantTextRecord("done"),
					},
				},
			},
		},
	}
	updated, _ := m.Update(doneMsg)

	newModel := updated.(Model)
	if newModel.output.String() != "user: hello\nassistant: done\n" {
		t.Errorf("output buffer = %q, want %q", newModel.output.String(), "user: hello\nassistant: done\n")
	}
}

func TestUpdateAppendsToolCallToOutput(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("inspect")

	eventMsg := runtimeEventMsg{
		event: runtimeevents.ToolCall{
			ID:        "call_123",
			Name:      "bash",
			Arguments: `echo "hello"`,
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	output := newModel.output.String()
	if output == "" {
		t.Error("output buffer should not be empty after tool call event")
	}
	if !contains(output, "\nassistant:\n  tool: bash") {
		t.Errorf("output buffer = %q, want indented tool line", output)
	}
	if newModel.status.ActiveTool != "bash" {
		t.Errorf("status.ActiveTool = %q, want %q", newModel.status.ActiveTool, "bash")
	}
}

func TestUpdateAppendsToolResultToOutput(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("inspect")

	eventMsg := runtimeEventMsg{
		event: runtimeevents.ToolResult{
			ToolCallID: "call_123",
			ToolName:   "bash",
			Output:     "hello world",
			IsError:    false,
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	output := newModel.output.String()
	if !contains(output, "  result: bash hello world") {
		t.Errorf("output buffer = %q, want indented result line", output)
	}
	if newModel.status.LastToolResult == "" {
		t.Error("status.LastToolResult should be populated after tool result")
	}
	if newModel.status.ActiveTool != "" {
		t.Errorf("status.ActiveTool = %q, want empty after tool result", newModel.status.ActiveTool)
	}
}

func TestUpdateAppendsErrorToolResultToOutput(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("inspect")

	eventMsg := runtimeEventMsg{
		event: runtimeevents.ToolResult{
			ToolCallID: "call_123",
			ToolName:   "bash",
			Output:     "command not found",
			IsError:    true,
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	output := newModel.output.String()
	if !contains(output, "  error: bash command not found") {
		t.Errorf("output buffer = %q, want indented error line", output)
	}
	if !newModel.status.LastToolError {
		t.Error("status.LastToolError should be true for error result")
	}
}

func TestUpdateTracksStepBeginStatus(t *testing.T) {
	m := Model{running: true}

	eventMsg := runtimeEventMsg{
		event: runtimeevents.StepBegin{Number: 2},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	if newModel.status.Step != 2 {
		t.Errorf("status.Step = %d, want 2", newModel.status.Step)
	}
}

func TestUpdateAppendsToolCallPartToOutput(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("inspect")

	eventMsg := runtimeEventMsg{
		event: runtimeevents.ToolCallPart{
			ToolCallID: "call_123",
			Delta:      `{"path":"main.go"}`,
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	output := newModel.output.String()
	if !contains(output, "  tool args: call_123") {
		t.Errorf("output buffer = %q, want indented tool args line", output)
	}
	if !contains(output, `{"path":"main.go"}`) {
		t.Errorf("output buffer = %q, want to contain tool delta", output)
	}
}

func TestUpdateAppendsMultilineToolResultUnderAssistantTurn(t *testing.T) {
	m := Model{running: true}
	m.appendUserTurn("inspect")

	eventMsg := runtimeEventMsg{
		event: runtimeevents.ToolResult{
			ToolCallID: "call_123",
			ToolName:   "bash",
			Output:     "line1\nline2",
			IsError:    false,
		},
	}
	updated, _ := m.Update(eventMsg)

	newModel := updated.(Model)
	output := newModel.output.String()
	if !contains(output, "assistant:\n  result: bash\n    line1\n    line2\n") {
		t.Errorf("output buffer = %q, want multiline tool result block", output)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
