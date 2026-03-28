package app

import "testing"

func TestSummarizeStartupContentCompactsWhitespaceAndTruncates(t *testing.T) {
	got := summarizeStartupContent("line one\n\n   line two   line three", 18)
	if got != "line one line t..." {
		t.Fatalf("summarizeStartupContent() = %q, want %q", got, "line one line t...")
	}
}
