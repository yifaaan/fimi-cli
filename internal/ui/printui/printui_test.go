package printui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestVisualizeTextPrintsEventsInOrder(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 8)
	events <- runtimeevents.StepBegin{Number: 1}
	events <- runtimeevents.TextPart{Text: "hello"}
	events <- runtimeevents.ToolCall{
		Name:      "read_file",
		Arguments: `{"path":"main.go"}`,
	}
	events <- runtimeevents.ToolResult{
		ToolName: "read_file",
		Output:   "package main",
	}
	events <- runtimeevents.StatusUpdate{
		Status: runtimeevents.StatusSnapshot{ContextUsage: 0.25},
	}
	events <- runtimeevents.StepInterrupted{}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	want := strings.Join([]string{
		"[step 1]",
		"hello",
		`[tool call] read_file {"path":"main.go"}`,
		"[tool result] read_file package main",
		"[status] context used 25%",
		"[interrupted]",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextUsesToolSubtitleWhenPresent(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolCall{
		Name:      "bash",
		Subtitle:  "go test ./internal/...",
		Arguments: `{"command":"go test ./internal/..."}`,
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[tool call] bash go test ./internal/...\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool call] bash go test ./internal/...\n")
	}
}

func TestVisualizeTextSkipsZeroStatusUpdate(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 3)
	events <- runtimeevents.StepBegin{Number: 1}
	events <- runtimeevents.StatusUpdate{
		Status: runtimeevents.StatusSnapshot{ContextUsage: 0},
	}
	events <- runtimeevents.TextPart{Text: "hello"}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	want := "[step 1]\nhello\n"
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextClampsStatusUsageToOneHundredPercent(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.StatusUpdate{
		Status: runtimeevents.StatusSnapshot{ContextUsage: 1.5},
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[status] context used 100%\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[status] context used 100%\n")
	}
}

func TestVisualizeTextClampsLongToolCallSummary(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolCall{
		Name:      "write_file",
		Arguments: strings.Repeat("a", 100),
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	want := "[tool call] write_file " + strings.Repeat("a", 77) + "...\n"
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextPrintsToolError(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolResult{
		ToolName: "bash",
		Output:   "tool execution failed",
		IsError:  true,
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[tool error] bash tool execution failed\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool error] bash tool execution failed\n")
	}
}

func TestVisualizeTextPrintsEmptyToolResultWithoutTrailingPlaceholder(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolResult{
		ToolName: "bash",
		Output:   "   ",
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[tool result] bash\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool result] bash\n")
	}
}

func TestVisualizeTextPrintsMultilineToolResultOnFollowingLines(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolResult{
		ToolName: "read_file",
		Output:   "package main\n\nfunc main() {}",
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	want := "[tool result] read_file\npackage main\n\nfunc main() {}\n"
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextUsesDiscardWhenWriterNil(t *testing.T) {
	visualize := VisualizeText(nil)

	events := make(chan runtimeevents.Event, 1)
	close(events)

	if err := visualize(context.Background(), events); err != nil {
		t.Fatalf("visualize() error = %v", err)
	}
}

func TestVisualizeTextReturnsWriteError(t *testing.T) {
	visualize := VisualizeText(failingWriter{})

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.TextPart{Text: "hello"}
	close(events)

	err := visualize(context.Background(), events)
	if err == nil {
		t.Fatalf("visualize() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "write print ui event") {
		t.Fatalf("visualize() error = %q, want write wrapper", err.Error())
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}
