package renderers

import (
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
