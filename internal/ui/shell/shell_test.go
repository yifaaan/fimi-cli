package shell

import (
	"bytes"
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

func TestNewShellProgramDoesNotEnableMouseCapture(t *testing.T) {
	var output bytes.Buffer

	p := newShellProgram(shellProgramTestModel{}, strings.NewReader(""), &output)
	if _, err := p.Run(); err != nil {
		t.Fatalf("newShellProgram().Run() error = %v", err)
	}

	got := output.String()
	for _, unwanted := range []string{"\x1b[?1002h", "\x1b[?1003h", "\x1b[?1006h"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("newShellProgram().Run() output contains mouse capture sequence %q in %q", unwanted, got)
		}
	}
}

func TestJoinTranscriptForTeaPrintSkipsEmptyInput(t *testing.T) {
	if got := joinTranscriptForTeaPrint(nil); got != "" {
		t.Fatalf("joinTranscriptForTeaPrint(nil) = %q, want empty", got)
	}
}

type shellProgramTestModel struct{}

func (shellProgramTestModel) Init() tea.Cmd {
	return tea.Quit
}

func (shellProgramTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return shellProgramTestModel{}, nil
}

func (shellProgramTestModel) View() string {
	return ""
}
