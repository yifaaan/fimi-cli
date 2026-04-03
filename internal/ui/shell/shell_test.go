package shell

import (
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

func TestRunConfiguresProgramForTerminalNativeMouseBehavior(t *testing.T) {
	original := newShellProgram
	t.Cleanup(func() {
		newShellProgram = original
	})

	captured := shellProgramOptions{}
	newShellProgram = func(model tea.Model, opts shellProgramOptions) shellProgram {
		captured = opts
		return stubShellProgram{}
	}

	err := Run(t.Context(), Dependencies{Runner: &contextAwareRunner{}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if captured.mouseCapture {
		t.Fatal("Run() enabled mouse capture, want terminal-native mouse behavior")
	}
	if captured.input == nil {
		t.Fatal("Run() input = nil, want default reader")
	}
	if captured.output == nil {
		t.Fatal("Run() output = nil, want discard writer")
	}
}

type stubShellProgram struct{}

func (stubShellProgram) Run() (tea.Model, error) {
	return nil, nil
}

func (stubShellProgram) Quit() {}
