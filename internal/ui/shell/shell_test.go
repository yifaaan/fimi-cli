package shell

import (
	"reflect"
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

	capturedMouse := false
	newShellProgram = func(model tea.Model, options ...tea.ProgramOption) shellProgram {
		startupOptions := func(program *tea.Program) int64 {
			return reflect.ValueOf(program).Elem().FieldByName("startupOptions").Int()
		}
		mouseMask := startupOptions(tea.NewProgram(model, tea.WithMouseCellMotion())) |
			startupOptions(tea.NewProgram(model, tea.WithMouseAllMotion()))
		capturedMouse = startupOptions(tea.NewProgram(model, options...))&mouseMask != 0
		return stubShellProgram{}
	}

	err := Run(t.Context(), Dependencies{Runner: &contextAwareRunner{}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if capturedMouse {
		t.Fatal("Run() enabled mouse capture, want terminal-native mouse behavior")
	}
}

type stubShellProgram struct{}

func (stubShellProgram) Run() (tea.Model, error) {
	return nil, nil
}

func (stubShellProgram) Quit() {}
