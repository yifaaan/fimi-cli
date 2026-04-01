package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestBuildInlinePreviewClipsLongBodies(t *testing.T) {
	preview := buildInlinePreview("Ran pwd", strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4",
		"line 5", "line 6", "line 7", "line 8", "line 9",
	}, "\n"))

	if !strings.Contains(preview, "Ran pwd") {
		t.Fatalf("preview = %q, want title included", preview)
	}
	if !strings.Contains(preview, "... +1 lines") {
		t.Fatalf("preview = %q, want truncation marker", preview)
	}
}

func TestNewWriteFileHandlerSetsDisplayOutput(t *testing.T) {
	workDir := t.TempDir()
	handler := newWriteFileHandler(workDir)

	result, err := handler(context.Background(), runtime.ToolCall{
		ID:        "call-1",
		Name:      ToolWriteFile,
		Arguments: `{"path":"notes.txt","content":"first line\nsecond line"}`,
	}, Definition{Name: ToolWriteFile})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !strings.Contains(result.DisplayOutput, "Edited notes.txt") {
		t.Fatalf("DisplayOutput = %q, want edited title", result.DisplayOutput)
	}
	if !strings.Contains(result.DisplayOutput, "first line") {
		t.Fatalf("DisplayOutput = %q, want content preview", result.DisplayOutput)
	}
}

func TestNewReplaceFileHandlerSetsDisplayOutput(t *testing.T) {
	workDir := t.TempDir()
	target := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(target, []byte("before line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler := newReplaceFileHandler(workDir)
	result, err := handler(context.Background(), runtime.ToolCall{
		ID:        "call-1",
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"before line","new":"after line"}`,
	}, Definition{Name: ToolReplaceFile})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	for _, want := range []string{"Updated notes.txt", "-before line", "+after line"} {
		if !strings.Contains(result.DisplayOutput, want) {
			t.Fatalf("DisplayOutput missing %q in %q", want, result.DisplayOutput)
		}
	}
}

func TestNewSearchWebHandlerSetsDisplayOutput(t *testing.T) {
	searcher := &previewWebSearcher{
		results: []WebSearchResult{{
			Title:   "Transcript parity",
			URL:     "https://example.com/parity",
			Snippet: "Shell transcript redesign notes.",
		}},
	}
	handler := NewSearchWebHandler(searcher, NewOutputShaperWithLimits(1000, 1000))

	result, err := handler(context.Background(), runtime.ToolCall{
		ID:        "call-1",
		Name:      ToolSearchWeb,
		Arguments: `{"query":"transcript parity","limit":1}`,
	}, Definition{Name: ToolSearchWeb})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	for _, want := range []string{"Transcript parity", "Shell transcript redesign notes."} {
		if !strings.Contains(result.DisplayOutput, want) {
			t.Fatalf("DisplayOutput missing %q in %q", want, result.DisplayOutput)
		}
	}
}

type previewWebSearcher struct {
	results []WebSearchResult
}

func (s *previewWebSearcher) Search(ctx context.Context, query string, limit int, includeContent bool) ([]WebSearchResult, error) {
	return append([]WebSearchResult(nil), s.results...), nil
}
