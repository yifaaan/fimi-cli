package shell

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOutputModelRenderLineUsesCompactActivityStyle(t *testing.T) {
	model := NewOutputModel()

	gotUser := model.renderLine(TranscriptLine{Type: LineTypeUser, Content: "fix the flaky test"}, 0)
	if !strings.Contains(gotUser, "fix the flaky test") {
		t.Fatalf("user line = %q, want rendered user content", gotUser)
	}
	if gotUser == "fix the flaky test" {
		t.Fatalf("user line = %q, want styled output distinct from plain text", gotUser)
	}
	if containsAny(gotUser, []string{"You:", "Assistant:", "[Tool]", "[Result]", "[System]", "[Error]"}) {
		t.Fatalf("user line = %q, want no explicit role label", gotUser)
	}

	gotTool := model.renderLine(TranscriptLine{Type: LineTypeToolCall, Content: "Read internal/app/app.go"}, 0)
	if !containsAll(gotTool, []string{"●", "Read internal/app/app.go"}) {
		t.Fatalf("tool line = %q, want compact bullet activity style", gotTool)
	}
	if containsAny(gotTool, []string{"[Tool]", "[Result]"}) {
		t.Fatalf("tool line = %q, want no old tool labels", gotTool)
	}

	gotResult := model.renderLine(TranscriptLine{Type: LineTypeToolResult, Content: "Removed 1 line"}, 0)
	if !containsAll(gotResult, []string{"⎿", "Removed 1 line", "Ctrl+O to expand"}) {
		t.Fatalf("tool result line = %q, want compact expandable detail", gotResult)
	}
	if containsAny(gotResult, []string{"Output hidden.", "[Result]"}) {
		t.Fatalf("tool result line = %q, want no old result labels", gotResult)
	}

	gotSystem := model.renderLine(TranscriptLine{Type: LineTypeSystem, Content: "Step 2"}, 0)
	if gotSystem == "" {
		t.Fatal("system line = empty, want rendered content")
	}
	if containsAny(gotSystem, []string{"[step", "[System]"}) {
		t.Fatalf("system line = %q, want compact system text", gotSystem)
	}
}

func TestToolResultSummaryUsesCompactClaudeLikeCopy(t *testing.T) {
	if got := toolResultSummary(ToolCallInfo{Output: "Removed 1 line\nextra detail"}); got != "Removed 1 line" {
		t.Fatalf("toolResultSummary(first line) = %q, want %q", got, "Removed 1 line")
	}
	if got := toolResultSummary(ToolCallInfo{}); got != "No output" {
		t.Fatalf("toolResultSummary(empty) = %q, want %q", got, "No output")
	}
	if got := toolResultSummary(ToolCallInfo{IsError: true}); got != "Error" {
		t.Fatalf("toolResultSummary(error empty) = %q, want %q", got, "Error")
	}
}

func TestFormatToolCallLineUsesClaudeLikeTitles(t *testing.T) {
	tests := []struct {
		name string
		tool ToolCallInfo
		want string
	}{
		{
			name: "bash wrapped in title",
			tool: ToolCallInfo{Name: "bash", Args: "go test ./internal/ui/shell"},
			want: "Bash(go test ./internal/ui/shell)",
		},
		{
			name: "read keeps natural summary",
			tool: ToolCallInfo{Name: "read_file", Args: "Read internal/app/app.go"},
			want: "Read internal/app/app.go",
		},
		{
			name: "write becomes write title",
			tool: ToolCallInfo{Name: "write_file", Args: "Wrote internal/app/app.go"},
			want: "Write(internal/app/app.go)",
		},
		{
			name: "replace becomes update title",
			tool: ToolCallInfo{Name: "replace_file", Args: "Updated internal/app/app.go"},
			want: "Update(internal/app/app.go)",
		},
		{
			name: "patch becomes update title",
			tool: ToolCallInfo{Name: "patch_file", Args: "Patched internal/app/app.go"},
			want: "Update(internal/app/app.go)",
		},
		{
			name: "fallback keeps summary",
			tool: ToolCallInfo{Name: "think", Args: "Thought: inspect rewind flow"},
			want: "Thought: inspect rewind flow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatToolCallLine(tt.tool); got != tt.want {
				t.Fatalf("formatToolCallLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutputModelUpdateScrollsWithMouseWheel(t *testing.T) {
	model := NewOutputModel()
	for i := 0; i < 20; i++ {
		model = model.AppendLine(TranscriptLine{Type: LineTypeAssistant, Content: "line"})
	}

	model.height = 12
	model.width = 80
	model.atBottom = true
	model.scrollOffset = 0

	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp}, model.width, model.height)
	if updated.scrollOffset != 3 {
		t.Fatalf("scrollOffset after wheel up = %d, want 3", updated.scrollOffset)
	}
	if updated.atBottom {
		t.Fatal("atBottom = true after wheel up, want false")
	}

	updated, _ = updated.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown}, updated.width, updated.height)
	if updated.scrollOffset != 0 {
		t.Fatalf("scrollOffset after wheel down = %d, want 0", updated.scrollOffset)
	}
	if !updated.atBottom {
		t.Fatal("atBottom = false after wheel down to bottom, want true")
	}
}

func TestRenderToolResultExpandedKeepsTranscriptCompact(t *testing.T) {
	model := NewOutputModel()
	model.expanded[0] = true

	content := strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
		"line 11",
	}, "\n")

	got := model.renderLine(TranscriptLine{Type: LineTypeToolResult, Content: content}, 0)
	if !containsAll(got, []string{"line 1", "line 10", "1 more lines hidden", "Ctrl+O to collapse"}) {
		t.Fatalf("expanded tool result = %q, want bounded expanded content with collapse hint", got)
	}
	if strings.Contains(got, "line 11") {
		t.Fatalf("expanded tool result = %q, want lines beyond threshold hidden", got)
	}
}

func TestRenderUnprintedLinesReturnsOnlyNewCommittedLines(t *testing.T) {
	model := NewOutputModel()
	model = model.AppendLine(TranscriptLine{Type: LineTypeUser, Content: "first"})
	if got := model.RenderUnprintedLines(); len(got) != 1 {
		t.Fatalf("RenderUnprintedLines() len = %d, want 1", len(got))
	}

	model = model.MarkPrinted()
	model = model.AppendLine(TranscriptLine{Type: LineTypeAssistant, Content: "second"})
	got := model.RenderUnprintedLines()
	if len(got) != 1 {
		t.Fatalf("RenderUnprintedLines() len after mark printed = %d, want 1", len(got))
	}
	if !strings.Contains(got[0], "second") {
		t.Fatalf("RenderUnprintedLines()[0] = %q, want latest committed line", got[0])
	}
}

func TestPendingViewOmitsCommittedLines(t *testing.T) {
	model := NewOutputModel()
	model.width = 80
	model.height = 12
	model = model.AppendLine(TranscriptLine{Type: LineTypeUser, Content: "committed"})
	model = model.SetPending([]TranscriptLine{{Type: LineTypeAssistant, Content: "pending"}})

	got := model.PendingView()
	if strings.Contains(got, "committed") {
		t.Fatalf("PendingView() = %q, want committed transcript omitted", got)
	}
	if !strings.Contains(got, "pending") {
		t.Fatalf("PendingView() = %q, want pending transcript included", got)
	}
}

func TestInteractiveViewShowsExpandedToolResultInStepContext(t *testing.T) {
	model := NewOutputModel()
	model.width = 80
	model.height = 12
	model = model.AppendLine(TranscriptLine{Type: LineTypeSystem, Content: "Step 1"})
	model = model.AppendLine(TranscriptLine{Type: LineTypeToolCall, Content: "Bash(Ran pwd && ls -la)"})
	model = model.AppendLine(TranscriptLine{Type: LineTypeToolResult, Content: "line 1\nline 2\nline 3"})
	model = model.AppendLine(TranscriptLine{Type: LineTypeAssistant, Content: "done"})
	model.expanded[2] = true

	got := model.InteractiveView()
	if !containsAll(got, []string{"Step 1", "Bash(Ran pwd && ls -la)", "line 1", "Ctrl+O to collapse"}) {
		t.Fatalf("InteractiveView() = %q, want expanded tool result inline within its step", got)
	}
	if strings.Contains(got, "done") {
		t.Fatalf("InteractiveView() = %q, want assistant final output kept outside folded tool detail", got)
	}
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func containsAll(s string, needles []string) bool {
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
