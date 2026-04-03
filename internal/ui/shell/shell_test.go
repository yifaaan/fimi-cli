package shell

import (
	"os"
	"strings"
	"testing"
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

func TestShellSourceDoesNotEnableMouseCapture(t *testing.T) {
	data, err := os.ReadFile("shell.go")
	if err != nil {
		t.Fatalf("os.ReadFile(shell.go) error = %v", err)
	}

	source := string(data)
	if strings.Contains(source, "tea.WithMouseCellMotion()") {
		t.Fatalf("shell.go unexpectedly enables mouse capture:\n%s", source)
	}
	if !strings.Contains(source, "tea.NewProgram(") {
		t.Fatalf("shell.go missing tea.NewProgram setup:\n%s", source)
	}
}
