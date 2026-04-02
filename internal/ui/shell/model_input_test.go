package shell

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestInputViewKeepsComposerLeftAlignedAcrossLines(t *testing.T) {
	model := NewInputModel()
	model.width = 80

	lines := nonEmptyLines(ansi.Strip(model.View()))
	if len(lines) < 3 {
		t.Fatalf("len(lines) = %d, want multi-line composer box", len(lines))
	}

	for i, line := range lines {
		if got := leadingSpaces(line); got != 0 {
			t.Fatalf("line %d leading spaces = %d, want 0; line=%q", i, got, line)
		}
	}
}

func nonEmptyLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func leadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}
