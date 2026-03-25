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
