package shell

import (
	"errors"
	"strings"
	"testing"
)

func TestViewShowsReadyState(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "test prompt",
	}
	view := m.View()

	if !strings.Contains(view, "● > test prompt") {
		t.Fatalf("expected ready state prompt in view, got: %s", view)
	}
	// Green dot (ASCII bullet) should be present
	if !strings.Contains(view, "●") {
		t.Fatalf("expected status dot in view, got: %s", view)
	}
}

func TestViewShowsRunningState(t *testing.T) {
	m := Model{
		running: true,
		prompt:  "running prompt",
	}
	view := m.View()

	if !strings.Contains(view, "● > running prompt") {
		t.Fatalf("expected running state prompt in view, got: %s", view)
	}
}

func TestViewIncludesOutputBuffer(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "test",
	}
	m.output.WriteString("previous output\n")

	view := m.View()

	if !strings.Contains(view, "previous output") {
		t.Fatalf("expected output buffer content in view, got: %s", view)
	}
}

func TestViewShowsHelp(t *testing.T) {
	m := Model{
		running:  false,
		prompt:   "test",
		showHelp: true,
	}
	view := m.View()

	if !strings.Contains(view, "/exit") {
		t.Fatalf("expected help text in view, got: %s", view)
	}
	if !strings.Contains(view, "/help") {
		t.Fatalf("expected help text in view, got: %s", view)
	}
}

func TestViewShowsError(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "test",
		err:     errors.New("no LLM provider configured"),
	}
	view := m.View()

	if !strings.Contains(view, "no LLM provider configured") {
		t.Fatalf("expected error message in view, got: %s", view)
	}
}

func TestViewEmptyPrompt(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "",
	}
	view := m.View()

	if !strings.Contains(view, "● > ") {
		t.Fatalf("expected prompt line with empty prompt, got: %s", view)
	}
}

func TestViewCombinesOutputAndPrompt(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "test",
	}
	m.output.WriteString("line1\nline2\n")

	view := m.View()

	// Output should come before prompt
	if !strings.Contains(view, "line1\nline2\n● > test") {
		t.Fatalf("expected output before prompt, got: %s", view)
	}
}

func TestViewSeparatesPromptFromUnfinishedAssistantLine(t *testing.T) {
	m := Model{
		running: true,
		prompt:  "test",
	}
	m.output.WriteString("assistant: hello")
	m.assistantTurnOpen = true
	m.assistantLineOpen = true

	view := m.View()

	if !strings.Contains(view, "assistant: hello\n● > test") {
		t.Fatalf("expected prompt to render on a new line, got: %s", view)
	}
}

func TestViewShowsRunStatusBlock(t *testing.T) {
	m := Model{
		running: true,
		prompt:  "test",
		status: runStatus{
			Step:             3,
			ActiveTool:       "bash",
			ActiveToolDetail: `ls -la`,
			LastToolResult:   "bash ok",
			ContextUsage:     0.25,
		},
	}

	view := m.View()

	if !strings.Contains(view, "step: running #3") {
		t.Fatalf("expected step status in view, got: %s", view)
	}
	if !strings.Contains(view, "tool: bash ls -la") {
		t.Fatalf("expected tool status in view, got: %s", view)
	}
	if !strings.Contains(view, "result: bash ok") {
		t.Fatalf("expected result status in view, got: %s", view)
	}
	if !strings.Contains(view, "context: 25%") {
		t.Fatalf("expected context status in view, got: %s", view)
	}
}

func TestViewShowsFinishedStepStatus(t *testing.T) {
	m := Model{
		running: false,
		prompt:  "test",
		status: runStatus{
			Step: 2,
		},
	}

	view := m.View()

	if !strings.Contains(view, "step: finished #2") {
		t.Fatalf("expected finished step status in view, got: %s", view)
	}
}

func TestViewSkipsZeroContextUsage(t *testing.T) {
	m := Model{
		running: true,
		prompt:  "test",
		status: runStatus{
			Step:         1,
			ContextUsage: 0,
		},
	}

	view := m.View()

	if strings.Contains(view, "context:") {
		t.Fatalf("expected zero context usage to be hidden, got: %s", view)
	}
}
