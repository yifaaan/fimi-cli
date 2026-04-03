package shell

import (
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
