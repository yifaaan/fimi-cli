package shell

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResumeCommandText(t *testing.T) {
	if got := resumeCommandText(); got != "fimi --continue" {
		t.Fatalf("resumeCommandText() = %q, want %q", got, "fimi --continue")
	}
}

func TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine(t *testing.T) {
	got := helpText()

	for _, want := range []string{
		"Available commands:",
		"Keyboard shortcuts:",
		"Ctrl+O          Toggle tool result expansion",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("helpText() missing %q in:\n%s", want, got)
		}
	}

	for _, unwanted := range []string{
		"Mouse wheel     Scroll transcript history",
		"mouse wheel",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("helpText() unexpectedly contains %q in:\n%s", unwanted, got)
		}
	}
}

func TestShellStartupDoesNotEnableMouseCapture(t *testing.T) {
	original := newShellProgram
	t.Cleanup(func() {
		newShellProgram = original
	})

	called := false
	newShellProgram = func(model tea.Model, input io.Reader, output io.Writer, enableMouseCapture bool) shellProgram {
		called = true
		if input == nil {
			t.Fatal("newShellProgram() input = nil, want non-nil reader")
		}
		if output == nil {
			t.Fatal("newShellProgram() output = nil, want non-nil writer")
		}
		if enableMouseCapture {
			t.Fatal("Run() enabled mouse capture, want terminal-native mouse behavior")
		}
		return stubShellProgram{}
	}

	err := Run(t.Context(), Dependencies{Runner: &contextAwareRunner{}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("Run() did not create a shell program")
	}
}

type stubShellProgram struct{}

func (stubShellProgram) Run() (tea.Model, error) {
	return nil, nil
}

func (stubShellProgram) Quit() {}
