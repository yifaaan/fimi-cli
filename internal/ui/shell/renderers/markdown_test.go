package renderers

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownRendererPreservesMarkdownStructure(t *testing.T) {
	renderer, err := NewMarkdownRenderer(60)
	if err != nil {
		t.Fatalf("NewMarkdownRenderer() error = %v", err)
	}

	rendered := ansi.Strip(renderer.Render("# Title\n\n- item\n\n```go\nfmt.Println(\"hi\")\n```\n"))
	for _, want := range []string{"Title", "item", "fmt.Println(\"hi\")"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q in:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "```") {
		t.Fatalf("Render() = %q, want fenced code markers removed", rendered)
	}
}

func TestMarkdownRendererTrimsOnlyTrailingNewlines(t *testing.T) {
	renderer, err := NewMarkdownRenderer(60)
	if err != nil {
		t.Fatalf("NewMarkdownRenderer() error = %v", err)
	}

	rendered := renderer.Render("line one\n\nline two\n")
	if strings.HasSuffix(rendered, "\n") || strings.HasSuffix(rendered, "\r") {
		t.Fatalf("Render() = %q, want trailing newlines removed", rendered)
	}
	if !strings.Contains(ansi.Strip(rendered), "line two") {
		t.Fatalf("Render() = %q, want content preserved", rendered)
	}
}

func TestMarkdownRendererDoesNotWriteAutoStyleProbeToStdout(t *testing.T) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer reader.Close()
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	renderer, err := NewMarkdownRenderer(60)
	if err != nil {
		_ = writer.Close()
		t.Fatalf("NewMarkdownRenderer() error = %v", err)
	}
	_ = renderer.Render("# Title\n\nbody")
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	captured, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if len(captured) != 0 {
		t.Fatalf("stdout = %q, want no terminal probe output", string(captured))
	}
}

func TestSanitizeMarkdownRemovesANSIAndControlSequences(t *testing.T) {
	text := "before\x1b]11;?\abg\x1b[48;5;236mcolor\x1b[0m\x00after\n\tindent"
	got := SanitizeMarkdown(text)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "]11;?") || strings.Contains(got, "\x00") {
		t.Fatalf("SanitizeMarkdown() = %q, want control sequences removed", got)
	}
	for _, want := range []string{"before", "bg", "color", "after", "\n\tindent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("SanitizeMarkdown() missing %q in %q", want, got)
		}
	}
}
