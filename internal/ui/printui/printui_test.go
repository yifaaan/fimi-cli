package printui

import (
	"bytes"
	"context"
	"encoding/json"
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
		"hello", // TextPart 后面跟着 ToolCall，所以会补换行
		"[tool call] read_file {\"path\":\"main.go\"}",
		"[tool result] read_file package main",
		"[status] context used 25%",
		"[interrupted]",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextStreamsMultipleTextPartsOnSameLine(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 5)
	events <- runtimeevents.StepBegin{Number: 1}
	events <- runtimeevents.TextPart{Text: "hel"}
	events <- runtimeevents.TextPart{Text: "lo "}
	events <- runtimeevents.TextPart{Text: "world"}
	events <- runtimeevents.StepBegin{Number: 2} // 触发补换行
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	// 期望：多个 TextPart 拼接在同一行，遇到 StepBegin 才补换行
	want := "[step 1]\nhello world\n[step 2]\n"
	if out.String() != want {
		t.Fatalf("printed output = %q, want %q", out.String(), want)
	}
}

func TestVisualizeTextAddsNewlineAtEndWhenEndsWithTextPart(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 2)
	events <- runtimeevents.StepBegin{Number: 1}
	events <- runtimeevents.TextPart{Text: "final answer"}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	// 期望：流结束时如果最后一个事件是 TextPart，补换行
	want := "[step 1]\nfinal answer\n"
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

	if out.String() != "[tool call] go test ./internal/...\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool call] go test ./internal/...\n")
	}
}

func TestVisualizeTextPrintsHumanizedBashSubtitleWithoutToolName(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolCall{
		Name:      "bash",
		Subtitle:  "Ran git status --short",
		Arguments: `{"command":"git status --short"}`,
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[tool call] Ran git status --short\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool call] Ran git status --short\n")
	}
}

func TestVisualizeTextPrintsThinkSubtitleInsteadOfJSONArguments(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.ToolCall{
		Name:      "think",
		Subtitle:  "compare parser branch behavior",
		Arguments: `{"thought":"compare parser branch behavior"}`,
	}
	close(events)

	err := visualize(context.Background(), events)
	if err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[tool call] compare parser branch behavior\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[tool call] compare parser branch behavior\n")
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

func TestVisualizeTextIncludesRetryStatus(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeText(&out)

	events := make(chan runtimeevents.Event, 1)
	events <- runtimeevents.StatusUpdate{
		Status: runtimeevents.StatusSnapshot{
			ContextUsage: 0.25,
			Retry: &runtimeevents.RetryStatus{
				Attempt:     2,
				MaxAttempts: 4,
				NextDelayMS: 1500,
			},
		},
	}
	close(events)

	if err := visualize(context.Background(), events); err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	if out.String() != "[status] retrying in 1.5s (attempt 2/4); context used 25%\n" {
		t.Fatalf("printed output = %q, want %q", out.String(), "[status] retrying in 1.5s (attempt 2/4); context used 25%\n")
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

	want := "[tool call] " + clampInline("write_file "+strings.Repeat("a", 100)) + "\n"
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

func TestVisualizeStreamJSONWritesEventPerLine(t *testing.T) {
	var out bytes.Buffer
	visualize := VisualizeStreamJSON(&out)

	events := make(chan runtimeevents.Event, 3)
	events <- runtimeevents.StepBegin{Number: 1}
	events <- runtimeevents.TextPart{Text: "hello"}
	events <- runtimeevents.ToolResult{ToolName: "bash", Output: "ok"}
	close(events)

	if err := visualize(context.Background(), events); err != nil {
		t.Fatalf("visualize() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("printed line count = %d, want 3", len(lines))
	}

	var got0 map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &got0); err != nil {
		t.Fatalf("json.Unmarshal(step) error = %v", err)
	}
	if got0["type"] != "step_begin" {
		t.Fatalf("step event type = %#v, want %q", got0["type"], "step_begin")
	}
	if got0["number"] != float64(1) {
		t.Fatalf("step event number = %#v, want %v", got0["number"], 1)
	}

	var got1 map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &got1); err != nil {
		t.Fatalf("json.Unmarshal(text) error = %v", err)
	}
	if got1["type"] != "text_part" {
		t.Fatalf("text event type = %#v, want %q", got1["type"], "text_part")
	}
	if got1["text"] != "hello" {
		t.Fatalf("text event text = %#v, want %q", got1["text"], "hello")
	}

	var got2 map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &got2); err != nil {
		t.Fatalf("json.Unmarshal(tool result) error = %v", err)
	}
	if got2["type"] != "tool_result" {
		t.Fatalf("tool result event type = %#v, want %q", got2["type"], "tool_result")
	}
	if got2["tool_name"] != "bash" {
		t.Fatalf("tool result tool_name = %#v, want %q", got2["tool_name"], "bash")
	}
	if got2["output"] != "ok" {
		t.Fatalf("tool result output = %#v, want %q", got2["output"], "ok")
	}
}

func TestVisualizeStreamJSONIncludesRetryStatusFields(t *testing.T) {
	line, err := marshalEventJSON(runtimeevents.StatusUpdate{Status: runtimeevents.StatusSnapshot{
		ContextUsage: 0.25,
		Retry: &runtimeevents.RetryStatus{
			Attempt:     2,
			MaxAttempts: 4,
			NextDelayMS: 1500,
		},
	}})
	if err != nil {
		t.Fatalf("marshalEventJSON() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("json.Unmarshal(status) error = %v", err)
	}
	if got["type"] != "status_update" {
		t.Fatalf("status event type = %#v, want %q", got["type"], "status_update")
	}
	if got["context_usage"] != 0.25 {
		t.Fatalf("context_usage = %#v, want %v", got["context_usage"], 0.25)
	}
	if got["retry_attempt"] != float64(2) {
		t.Fatalf("retry_attempt = %#v, want %v", got["retry_attempt"], 2)
	}
	if got["retry_max_attempts"] != float64(4) {
		t.Fatalf("retry_max_attempts = %#v, want %v", got["retry_max_attempts"], 4)
	}
	if got["retry_next_delay_ms"] != float64(1500) {
		t.Fatalf("retry_next_delay_ms = %#v, want %v", got["retry_next_delay_ms"], 1500)
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}
