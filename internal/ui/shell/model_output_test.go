package shell

import (
	"strings"
	"testing"

	runtimeevents "fimi-cli/internal/runtime/events"
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

func TestRuntimeModelTracksLatestRunningToolAfterOtherToolCompletes(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_1",
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_2",
		Name:      "read",
		Arguments: `{"file_path":"/tmp/a"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID: "call_2",
		ToolName:   "read",
		Output:     "file-contents",
		IsError:    false,
	})

	if model.CurrentTool == nil {
		t.Fatal("CurrentTool = nil, want running tool")
	}
	if model.CurrentTool.ID != "call_1" {
		t.Fatalf("CurrentTool.ID = %q, want %q", model.CurrentTool.ID, "call_1")
	}
	if model.CurrentTool.Name != "bash" {
		t.Fatalf("CurrentTool.Name = %q, want %q", model.CurrentTool.Name, "bash")
	}
	if model.CurrentTool.Status != ToolStatusRunning {
		t.Fatalf("CurrentTool.Status = %v, want %v", model.CurrentTool.Status, ToolStatusRunning)
	}
}

func TestRuntimeModelAppliesToolCallPartByToolCallID(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_1",
		Name:      "bash",
		Arguments: `{"command":"pw"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_2",
		Name:      "read",
		Arguments: `{"file_path":"/tmp/a"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolCallPart{
		ToolCallID: "call_1",
		Delta:      ` --json`,
	})

	got := model.ToLines()
	if len(got) != 3 {
		t.Fatalf("len(ToLines()) = %d, want 3; lines=%#v", len(got), got)
	}
	if got[1].Type != LineTypeToolCall || got[1].Content != `Bash(bash {"command":"pw"} --json)` {
		t.Fatalf("ToLines()[1] = %#v, want updated call_1 line", got[1])
	}
	if got[2].Type != LineTypeToolCall || got[2].Content != `read {"file_path":"/tmp/a"}` {
		t.Fatalf("ToLines()[2] = %#v, want unchanged call_2 line", got[2])
	}
}

func TestRuntimeModelDoesNotSwitchActiveToolOnOlderToolCallPart(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_1",
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "call_2",
		Name:      "read",
		Arguments: `{"file_path":"/tmp/a"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolCallPart{
		ToolCallID: "call_1",
		Delta:      ` --json`,
	})

	if model.CurrentTool == nil {
		t.Fatal("CurrentTool = nil, want latest running tool")
	}
	if model.CurrentTool.ID != "call_2" {
		t.Fatalf("CurrentTool.ID = %q, want %q", model.CurrentTool.ID, "call_2")
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
