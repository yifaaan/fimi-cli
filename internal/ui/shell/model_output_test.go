package shell

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestOutputModelRendersTranscriptBlocks(t *testing.T) {
	model := NewOutputModel()
	model.width = 100
	model.height = 40

	model = model.AppendBlock(TranscriptBlock{
		ID:       "user-1",
		Kind:     BlockKindUserPrompt,
		UserText: "Refactor shell transcript for screenshot parity",
	})
	model = model.AppendBlock(TranscriptBlock{
		ID:       "note-1",
		Kind:     BlockKindAssistantNote,
		NoteText: "Inspecting the current transcript renderer.\n\nNeed inline approval parity.",
	})
	model = model.AppendBlock(TranscriptBlock{
		ID:   "explored-1",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind: "explored",
			Title:     "Explored",
			Items: []ActivityItem{
				{Verb: "Read", Text: "internal/ui/shell/model.go"},
				{Verb: "Search", Text: "approval flow"},
			},
			Preview: PreviewBody{
				Text: "internal/ui/shell/model.go:1:package shell\ninternal/ui/shell/model_output.go:1:package shell",
				Kind: PreviewKindText,
			},
		},
	})
	model = model.AppendBlock(TranscriptBlock{ID: "divider-1", Kind: BlockKindDivider})
	model = model.AppendBlock(TranscriptBlock{
		ID:   "elapsed-1",
		Kind: BlockKindElapsed,
		Text: "Worked for 1m 11s",
	})
	model = model.AppendBlock(TranscriptBlock{
		ID:   "command-1",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind: "command",
			Title:     "Ran pwd && rg --files",
			Preview: PreviewBody{
				Text: "STDOUT:\nD:/code/fimi-cli\ninternal/ui/shell/model.go",
				Kind: PreviewKindText,
			},
		},
	})
	model = model.AppendBlock(TranscriptBlock{
		ID:   "edit-1",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind: "edit",
			Title:     "Edited PLAN.md (+1 -1)",
			Preview: PreviewBody{
				Text: "@@ -1,2 +1,2 @@\n-old\n+new",
				Kind: PreviewKindDiff,
			},
		},
	})

	view := model.InteractiveView()
	for _, want := range []string{
		"Refactor shell transcript for screenshot parity",
		"Inspecting the current transcript renderer.",
		"Need inline approval parity.",
		"Explored",
		"Read internal/ui/shell/model.go",
		"Search approval flow",
		"Worked for 1m 11s",
		"Ran pwd && rg --files",
		"Edited PLAN.md (+1 -1)",
		formatDiffNumberedLine("-", 1, "old", 1),
		formatDiffNumberedLine("+", 1, "new", 1),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("InteractiveView() missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "@@ -1,2 +1,2 @@") {
		t.Fatalf("InteractiveView() unexpectedly contains raw hunk header:\n%s", view)
	}
	if strings.Contains(view, "Step 1") {
		t.Fatalf("InteractiveView() unexpectedly contains legacy step marker:\n%s", view)
	}
	if strings.Contains(view, "• Inspecting the current transcript renderer.") {
		t.Fatalf("InteractiveView() assistant response rendered as bullet list, want a single response block:\n%s", view)
	}

	kinds := []TranscriptBlockKind{
		model.blocks[0].Kind,
		model.blocks[1].Kind,
		model.blocks[2].Kind,
		model.blocks[3].Kind,
		model.blocks[4].Kind,
	}
	wantKinds := []TranscriptBlockKind{
		BlockKindUserPrompt,
		BlockKindAssistantNote,
		BlockKindActivityGroup,
		BlockKindDivider,
		BlockKindElapsed,
	}
	for i, want := range wantKinds {
		if kinds[i] != want {
			t.Fatalf("block %d kind = %v, want %v", i, kinds[i], want)
		}
	}
}

func TestRenderBlockUserPromptUsesFallbackWidthWhenWindowSizeUnknown(t *testing.T) {
	model := NewOutputModel()

	rendered := model.renderBlock(TranscriptBlock{
		Kind:     BlockKindUserPrompt,
		UserText: "what is your name?",
	})

	if !strings.Contains(rendered, "what is your name?") {
		t.Fatalf("renderBlock(user prompt) = %q, want prompt text to remain visible", rendered)
	}
}

func TestRenderUserPromptBlockOmitsLegacyYouLabel(t *testing.T) {
	rendered := ansi.Strip(renderUserPromptBlock("what is your name?", 40))

	if strings.Contains(rendered, "You") {
		t.Fatalf("renderUserPromptBlock() = %q, want no legacy You label", rendered)
	}
	if !strings.Contains(rendered, "what is your name?") {
		t.Fatalf("renderUserPromptBlock() = %q, want prompt text", rendered)
	}
}

func TestRenderAssistantNoteBlockOmitsLegacyAssistantLabel(t *testing.T) {
	rendered := ansi.Strip(renderAssistantNoteBlock("I'm fimi.", 40))

	if strings.Contains(rendered, "\nfimi") || strings.HasPrefix(strings.TrimSpace(rendered), "fimi") {
		t.Fatalf("renderAssistantNoteBlock() = %q, want no legacy assistant label", rendered)
	}
	if !strings.Contains(rendered, "I'm fimi.") {
		t.Fatalf("renderAssistantNoteBlock() = %q, want assistant text", rendered)
	}
}

func TestWrapStringPreservesStyledCJKContent(t *testing.T) {
	rendered := styles.UserBubbleStyle.Width(12).Render("发送消息后显示错乱")

	got := wrapString(rendered, 14)
	want := ansi.WrapWc(rendered, 14, "")
	if got != want {
		t.Fatalf("wrapString() = %q, want %q", got, want)
	}

	normalized := strings.Join(strings.Fields(ansi.Strip(got)), "")
	if !strings.Contains(normalized, "发送消息后显示错乱") {
		t.Fatalf("normalized wrapped content = %q, want original CJK text", normalized)
	}
}

func TestRuntimeModelGroupsExplorationToolsAndSkipsThoughtLoggedResult(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "think-1",
		Name:      "think",
		Arguments: `{"thought":"Inspect shell transcript flow"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID: "think-1",
		ToolName:   "think",
		Output:     "Thought logged",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read internal/ui/shell/model.go",
		Arguments: `{"path":"internal/ui/shell/model.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "package shell\nfunc example() {}",
		DisplayOutput: "package shell\nfunc example() {}",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "glob-1",
		Name:      "glob",
		Subtitle:  "Matched internal/ui/shell/*.go",
		Arguments: `{"pattern":"internal/ui/shell/*.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "glob-1",
		ToolName:      "glob",
		Output:        "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
		DisplayOutput: "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "grep-1",
		Name:      "grep",
		Subtitle:  `Searched internal/ui/shell for "approval"`,
		Arguments: `{"path":"internal/ui/shell","pattern":"approval"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "grep-1",
		ToolName:      "grep",
		Output:        "internal/ui/shell/model.go:1:approvalRequestMsg",
		DisplayOutput: "internal/ui/shell/model.go:1:approvalRequestMsg",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "bash-1",
		Name:      "bash",
		Subtitle:  "Ran pwd && rg --files",
		Arguments: `{"command":"pwd && rg --files"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "bash-1",
		ToolName:      "bash",
		Output:        "D:/code/fimi-cli\ninternal/ui/shell/model.go",
		DisplayOutput: "Ran pwd && rg --files\nSTDOUT:\nD:/code/fimi-cli\ninternal/ui/shell/model.go",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 3 {
		t.Fatalf("len(ToBlocks()) = %d, want 3; blocks=%#v", len(blocks), blocks)
	}
	if blocks[0].Kind != BlockKindAssistantNote {
		t.Fatalf("blocks[0].Kind = %v, want assistant note", blocks[0].Kind)
	}
	if !strings.Contains(blocks[0].NoteText, "Inspect shell transcript flow") {
		t.Fatalf("assistant note = %q, want think text", blocks[0].NoteText)
	}
	if strings.Contains(blocks[0].NoteText, "Thought logged") {
		t.Fatalf("assistant note = %q, want tool-result acknowledgement omitted", blocks[0].NoteText)
	}

	explored := blocks[1]
	if explored.Kind != BlockKindActivityGroup || explored.Activity.GroupKind != "explored" {
		t.Fatalf("blocks[1] = %#v, want explored activity group", explored)
	}
	if got := len(explored.Activity.Items); got != 3 {
		t.Fatalf("len(explored.Activity.Items) = %d, want 3", got)
	}
	if explored.Activity.Items[0].Verb != "Read" || explored.Activity.Items[1].Verb != "Search" || explored.Activity.Items[2].Verb != "Search" {
		t.Fatalf("explored items = %#v, want read/search/search", explored.Activity.Items)
	}

	command := blocks[2]
	if command.Kind != BlockKindActivityGroup || command.Activity.GroupKind != "command" {
		t.Fatalf("blocks[2] = %#v, want command activity group", command)
	}
	if command.Activity.Title != "Ran pwd && rg --files" {
		t.Fatalf("command title = %q, want bash title", command.Activity.Title)
	}
	if !strings.Contains(command.Activity.Preview.Text, "STDOUT:") {
		t.Fatalf("command preview = %q, want inline command output", command.Activity.Preview.Text)
	}
}

func TestRuntimeModelPrefersFullReadFileOutputOverClippedDisplayPreview(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read a.cpp",
		Arguments: `{"path":"a.cpp"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "#include <iostream>\n#include <vector>\n#include <utility>\n\nint partition(std::vector<int>& arr, int left, int right) {",
		DisplayOutput: "#include <iostream>\n#include <vector>\n#include <utility>\nint partition(std::vector<int>& arr, int left, int right) {\n... +31 lines",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 1 {
		t.Fatalf("len(ToBlocks()) = %d, want 1", len(blocks))
	}
	if blocks[0].Kind != BlockKindActivityGroup {
		t.Fatalf("blocks[0].Kind = %v, want activity group", blocks[0].Kind)
	}

	preview := blocks[0].Activity.Preview.Text
	if strings.Contains(preview, "... +31 lines") {
		t.Fatalf("preview = %q, want clipped display marker removed", preview)
	}
	if !strings.Contains(preview, "#include <utility>\n\nint partition") {
		t.Fatalf("preview = %q, want blank line preserved between include and function", preview)
	}
}

func TestRuntimeModelStoresPreviewOnEachExploredItem(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read internal/tools/builtin.go",
		Arguments: `{"path":"internal/tools/builtin.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "package tools\nfunc Builtin() {}",
		DisplayOutput: "package tools\nfunc Builtin() {}",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-2",
		Name:      "read_file",
		Subtitle:  "Read internal/tools/builtin_readonly.go",
		Arguments: `{"path":"internal/tools/builtin_readonly.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-2",
		ToolName:      "read_file",
		Output:        "package tools\nfunc Readonly() {}",
		DisplayOutput: "package tools\nfunc Readonly() {}",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 1 {
		t.Fatalf("len(ToBlocks()) = %d, want 1", len(blocks))
	}

	explored := blocks[0]
	if len(explored.Activity.Items) != 2 {
		t.Fatalf("len(explored.Activity.Items) = %d, want 2", len(explored.Activity.Items))
	}
	if explored.Activity.Items[0].Preview.Text == "" {
		t.Fatal("items[0].Preview.Text is empty, want first read preview preserved")
	}
	if explored.Activity.Items[1].Preview.Text == "" {
		t.Fatal("items[1].Preview.Text is empty, want second read preview preserved")
	}
	if strings.Contains(explored.Activity.Items[0].Preview.Text, "Readonly") {
		t.Fatalf("items[0].Preview.Text = %q, want first item to keep first preview", explored.Activity.Items[0].Preview.Text)
	}
	if !strings.Contains(explored.Activity.Items[1].Preview.Text, "Readonly") {
		t.Fatalf("items[1].Preview.Text = %q, want second item to keep second preview", explored.Activity.Items[1].Preview.Text)
	}
}

func TestRuntimeModelMixedExplorationItemsEachKeepTheirOwnPreview(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read internal/ui/shell/model.go",
		Arguments: `{"path":"internal/ui/shell/model.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "package shell",
		DisplayOutput: "package shell",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "glob-1",
		Name:      "glob",
		Subtitle:  "Matched internal/ui/shell/*.go",
		Arguments: `{"pattern":"internal/ui/shell/*.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "glob-1",
		ToolName:      "glob",
		Output:        "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
		DisplayOutput: "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "grep-1",
		Name:      "grep",
		Subtitle:  `Searched internal/ui/shell for "approval"`,
		Arguments: `{"path":"internal/ui/shell","pattern":"approval"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "grep-1",
		ToolName:      "grep",
		Output:        "internal/ui/shell/model.go:1:approvalRequestMsg",
		DisplayOutput: "internal/ui/shell/model.go:1:approvalRequestMsg",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 1 {
		t.Fatalf("len(ToBlocks()) = %d, want 1", len(blocks))
	}
	if len(blocks[0].Activity.Items) != 3 {
		t.Fatalf("len(blocks[0].Activity.Items) = %d, want 3", len(blocks[0].Activity.Items))
	}
	for i, want := range []string{"package shell", "model_output.go", "approvalRequestMsg"} {
		if !strings.Contains(blocks[0].Activity.Items[i].Preview.Text, want) {
			t.Fatalf("items[%d].Preview.Text = %q, want to contain %q", i, blocks[0].Activity.Items[i].Preview.Text, want)
		}
	}
}

func TestOutputModelToggleExpandTargetsLatestCollapsiblePreview(t *testing.T) {
	longText := strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
		"line 11", "line 12", "line 13", "line 14", "line 15",
		"line 16", "line 17", "line 18", "line 19", "line 20",
	}, "\n")

	model := NewOutputModel()
	model.width = 80
	model.height = 20
	model = model.AppendBlock(TranscriptBlock{
		ID:   "older",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			Title:       "Explored",
			Collapsible: true,
			Preview: PreviewBody{
				Text:        longText,
				Kind:        PreviewKindText,
				Collapsible: true,
			},
		},
	})
	model = model.AppendBlock(TranscriptBlock{
		ID:   "latest",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			Title:       "Edited main.go (+1 -1)",
			Collapsible: true,
			Preview: PreviewBody{
				Text:        longText,
				Kind:        PreviewKindDiff,
				Collapsible: true,
			},
		},
	})

	collapsed := model.InteractiveView()
	if strings.Contains(collapsed, "line 20") {
		t.Fatalf("collapsed InteractiveView() unexpectedly includes hidden lines:\n%s", collapsed)
	}

	updated, toggled := model.ToggleExpand()
	if !toggled {
		t.Fatal("ToggleExpand() = false, want latest collapsible block toggled")
	}
	if !updated.expanded["latest"] {
		t.Fatal("latest preview not expanded")
	}
	if updated.expanded["older"] {
		t.Fatal("older preview unexpectedly expanded")
	}

	expanded := updated.InteractiveView()
	if !strings.Contains(expanded, "line 20") {
		t.Fatalf("expanded InteractiveView() missing expanded content:\n%s", expanded)
	}
}

func TestRenderPreviewBodyExpandedShowsAllToolOutputLines(t *testing.T) {
	lines := make([]string, 0, 50)
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}

	model := NewOutputModel()
	model.width = 80
	model.expanded["tool-1"] = true

	rendered := ansi.Strip(model.renderPreviewBody("tool-1", "Ran long command", PreviewBody{
		Text:        strings.Join(lines, "\n"),
		Kind:        PreviewKindText,
		Collapsible: true,
	}))

	if !strings.Contains(rendered, "line 50") {
		t.Fatalf("expanded preview missing tail content:\n%s", rendered)
	}
	if strings.Contains(rendered, "... +") {
		t.Fatalf("expanded preview unexpectedly contains inline truncation marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ctrl+O collapse") {
		t.Fatalf("expanded preview missing collapse hint:\n%s", rendered)
	}
}

func TestRenderEditDiffPreviewBodyKeepsRealHunkContext(t *testing.T) {
	summary := "Edited main.go (+2 -1)"
	content := strings.Join([]string{
		"@@ -1,13 +1,14 @@",
		" line 1",
		" line 2",
		" line 3",
		" line 4",
		"-line 5",
		"+new line 5",
		" line 6",
		" line 7",
		" line 8",
		" line 9",
		" line 10",
		"+new line 11",
		" line 12",
		" line 13",
	}, "\n")

	collapsed, ok := renderEditDiffPreviewBody(summary, content, false)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}
	for _, want := range []string{
		formatDiffNumberedLine("-", 5, "line 5", 2),
		formatDiffNumberedLine("+", 5, "new line 5", 2),
		formatDiffNumberedLine("+", 11, "new line 11", 2),
		formatDiffNumberedLine(" ", 1, "line 1", 2),
		formatDiffNumberedLine(" ", 13, "line 13", 2),
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed diff missing %q in:\n%s", want, collapsed)
		}
	}
	if strings.Contains(collapsed, "@@ -1,13 +1,14 @@") {
		t.Fatalf("collapsed diff unexpectedly contains raw hunk header:\n%s", collapsed)
	}
	if strings.Contains(collapsed, "...") {
		t.Fatalf("collapsed diff unexpectedly contains synthetic omission marker:\n%s", collapsed)
	}

	expanded, ok := renderEditDiffPreviewBody(summary, content, true)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody(expanded) = false, want diff preview rendered")
	}
	for _, want := range []string{"line 1", "line 10", "line 13", "Ctrl+O collapse"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded diff missing %q in:\n%s", want, expanded)
		}
	}
}

func TestRenderEditDiffPreviewBodyExpandedShowsAllDiffLines(t *testing.T) {
	summary := "Edited main.go (+45 -0)"
	body := []string{"@@ -1,0 +1,45 @@"}
	for i := 1; i <= 45; i++ {
		body = append(body, fmt.Sprintf("+line %d", i))
	}

	rendered, ok := renderEditDiffPreviewBody(summary, strings.Join(body, "\n"), true)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}
	if !strings.Contains(rendered, "line 45") {
		t.Fatalf("expanded diff missing tail content:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ctrl+O collapse") {
		t.Fatalf("expanded diff missing collapse hint:\n%s", rendered)
	}
}

func TestRenderEditDiffPreviewBodyCollapsedTruncatesTailLines(t *testing.T) {
	summary := "Edited main.go (+16 -0)"
	body := []string{"@@ -1,0 +1,16 @@"}
	for i := 1; i <= 16; i++ {
		body = append(body, fmt.Sprintf("+line %d", i))
	}
	content := strings.Join(body, "\n")

	collapsed, ok := renderEditDiffPreviewBody(summary, content, false)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}

	for _, want := range []string{
		formatDiffNumberedLine("+", 1, "line 1", 2),
		formatDiffNumberedLine("+", 14, "line 14", 2),
		"+2 diff lines",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed diff missing %q in:\n%s", want, collapsed)
		}
	}
	for _, unwanted := range []string{
		formatDiffNumberedLine("+", 15, "line 15", 2),
		formatDiffNumberedLine("+", 16, "line 16", 2),
		"...",
	} {
		if strings.Contains(collapsed, unwanted) {
			t.Fatalf("collapsed diff unexpectedly contains %q in:\n%s", unwanted, collapsed)
		}
	}
}

func TestRenderEditDiffPreviewBodyUsesCodexStyleLineNumbers(t *testing.T) {
	summary := "Edited main.go (+1 -0)"
	content := strings.Join([]string{
		"@@ -1,3 +1,4 @@",
		" line 1",
		"+inserted line 2",
		" line 2",
		" line 3",
	}, "\n")

	rendered, ok := renderEditDiffPreviewBody(summary, content, true)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}

	for _, want := range []string{
		formatDiffNumberedLine(" ", 1, "line 1", 1),
		formatDiffNumberedLine("+", 2, "inserted line 2", 1),
		formatDiffNumberedLine(" ", 3, "line 2", 1),
		formatDiffNumberedLine(" ", 4, "line 3", 1),
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diff missing %q in:\n%s", want, rendered)
		}
	}
}

func TestRenderEditDiffPreviewBodyUsesSpacerBetweenHunks(t *testing.T) {
	summary := "Edited main.go (+2 -2)"
	content := strings.Join([]string{
		"@@ -1,2 +1,2 @@",
		"-old line 1",
		"+new line 1",
		"@@ -10,2 +10,2 @@",
		"-old line 10",
		"+new line 10",
	}, "\n")

	rendered, ok := renderEditDiffPreviewBody(summary, content, true)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}

	for _, unwanted := range []string{"@@ -1,2 +1,2 @@", "@@ -10,2 +10,2 @@"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered diff unexpectedly contains raw hunk header %q in:\n%s", unwanted, rendered)
		}
	}
	if !strings.Contains(rendered, formatDiffSpacerLine(2, "⋮")) {
		t.Fatalf("rendered diff missing Codex-style hunk spacer in:\n%s", rendered)
	}
}

func TestRenderEditDiffPreviewBodyWrapsLongLinesWithAlignedGutter(t *testing.T) {
	summary := "Edited main.go (+1 -0)"
	content := strings.Join([]string{
		"@@ -8,0 +8,1 @@",
		"+abcdefghijkl",
	}, "\n")

	rendered, ok := renderEditDiffPreviewBodyWithWidth(summary, content, true, 12)
	if !ok {
		t.Fatal("renderEditDiffPreviewBodyWithWidth() = false, want diff preview rendered")
	}

	want := strings.Join([]string{
		formatDiffNumberedLine("+", 8, "abcd", 1),
		"        efgh",
		"        ijkl",
	}, "\n")
	if !strings.Contains(rendered, want) {
		t.Fatalf("rendered diff missing wrapped Codex-style gutter alignment in:\n%s", rendered)
	}
}

func TestRenderEditDiffPreviewBodyWithoutAbsoluteHeaderDoesNotInventLineNumbers(t *testing.T) {
	summary := "Edited a.cpp (+3 -0)"
	content := strings.Join([]string{
		"@@",
		" void printArray(const std::vector<int>& arr) {",
		"+bool isSorted(const std::vector<int>& arr) {",
		"+    return true;",
		"+}",
	}, "\n")

	rendered, ok := renderEditDiffPreviewBody(summary, content, true)
	if !ok {
		t.Fatal("renderEditDiffPreviewBody() = false, want diff preview rendered")
	}
	if strings.Contains(rendered, "1 +bool isSorted") {
		t.Fatalf("rendered diff = %q, want no fabricated line numbers for headerless hunk", rendered)
	}
	if !strings.Contains(rendered, "+bool isSorted") {
		t.Fatalf("rendered diff = %q, want diff body rendered", rendered)
	}
}

func TestBuildTranscriptBlocksFromRecordsPrefersDisplayContentAndFallsBackToContent(t *testing.T) {
	toolCallsJSON, err := json.Marshal([]runtime.ToolCall{
		{
			ID:        "call-1",
			Name:      "bash",
			Arguments: `{"command":"pwd"}`,
		},
		{
			ID:        "call-2",
			Name:      "read_file",
			Arguments: `{"path":"PLAN.md"}`,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("inspect workspace"),
		{
			Role:          contextstore.RoleAssistant,
			Content:       "Looking around.",
			ToolCallsJSON: string(toolCallsJSON),
		},
		contextstore.NewToolResultRecordWithDisplay("call-1", "raw tool output", "Ran pwd\nSTDOUT:\nD:/code/fimi-cli"),
		contextstore.NewToolResultRecord("call-2", "legacy fallback output"),
	}

	blocks := buildTranscriptBlocksFromRecords(records)
	if len(blocks) < 3 {
		t.Fatalf("len(blocks) = %d, want at least 3; blocks=%#v", len(blocks), blocks)
	}
	if blocks[0].Kind != BlockKindUserPrompt || blocks[0].UserText != "inspect workspace" {
		t.Fatalf("blocks[0] = %#v, want user prompt", blocks[0])
	}
	if blocks[1].Kind != BlockKindAssistantNote || !strings.Contains(blocks[1].NoteText, "Looking around.") {
		t.Fatalf("blocks[1] = %#v, want assistant note", blocks[1])
	}

	var previews []string
	for _, block := range blocks {
		if block.Kind != BlockKindActivityGroup {
			continue
		}
		if block.Activity.Preview.Text != "" {
			previews = append(previews, block.Activity.Preview.Text)
		}
		for _, item := range block.Activity.Items {
			if item.Preview.Text != "" {
				previews = append(previews, item.Preview.Text)
			}
		}
	}
	joined := strings.Join(previews, "\n")
	if !strings.Contains(joined, "STDOUT:\nD:/code/fimi-cli") {
		t.Fatalf("joined previews = %q, want DisplayContent preview", joined)
	}
	if !strings.Contains(joined, "legacy fallback output") {
		t.Fatalf("joined previews = %q, want fallback Content preview", joined)
	}
}

func TestRenderUnprintedLinesKeepsLatestTurnInteractive(t *testing.T) {
	model := NewOutputModel()
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "first"})
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: "first reply"})
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "second"})
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: "second reply"})

	rendered := model.RenderUnprintedLines()
	if len(rendered) != 2 {
		t.Fatalf("len(RenderUnprintedLines()) = %d, want 2", len(rendered))
	}
	if !strings.Contains(rendered[0], "first") || !strings.Contains(rendered[1], "first reply") {
		t.Fatalf("RenderUnprintedLines() = %#v, want the older completed turn", rendered)
	}
	if strings.Contains(strings.Join(rendered, "\n"), "second reply") {
		t.Fatalf("RenderUnprintedLines() = %#v, want latest turn kept interactive", rendered)
	}
}

func TestRenderUnprintedCommittedIncludesLatestCompletedTurn(t *testing.T) {
	model := NewOutputModel()
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "list current dir"})
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindApproval, Approval: ApprovalBlock{Action: "bash", Description: "pwd && ls -la", Status: ApprovalStatusApproved}})
	model = model.AppendBlock(TranscriptBlock{
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			Title: "Ran pwd && ls -la",
			Preview: PreviewBody{
				Text: "STDOUT:\n/mnt/d/code/fimi-cli",
				Kind: PreviewKindText,
			},
		},
	})

	rendered := model.RenderUnprintedCommitted()
	if len(rendered) != 3 {
		t.Fatalf("len(RenderUnprintedCommitted()) = %d, want 3", len(rendered))
	}
	if !strings.Contains(rendered[0], "list current dir") {
		t.Fatalf("RenderUnprintedCommitted()[0] = %q, want latest user prompt", rendered[0])
	}
	if !strings.Contains(strings.Join(rendered, "\n"), "Ran pwd && ls -la") {
		t.Fatalf("RenderUnprintedCommitted() = %#v, want completed tool group", rendered)
	}
}

func TestMarkPrintedUntilLeavesLatestUserTurnUnprinted(t *testing.T) {
	model := NewOutputModel()
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: "older reply"})
	model = model.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "latest prompt"})

	model = model.MarkPrintedUntil(model.stablePrintedTarget())

	if model.printedCount != 1 {
		t.Fatalf("printedCount = %d, want 1", model.printedCount)
	}
	rendered := model.RenderUnprintedCommitted()
	if len(rendered) != 1 || !strings.Contains(rendered[0], "latest prompt") {
		t.Fatalf("RenderUnprintedCommitted() = %#v, want latest user prompt still unprinted", rendered)
	}
}

func TestOutputModelMouseWheelScrollsFullHistory(t *testing.T) {
	model := NewOutputModel()
	model.width = 80
	model = model.WithViewportHeight(4)
	for _, text := range []string{
		"first message",
		"second message",
		"third message",
		"fourth message",
		"fifth message",
		"sixth message",
	} {
		model = model.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: text})
	}

	initial := model.View()
	if !strings.Contains(initial, "third message") || !strings.Contains(initial, "sixth message") {
		t.Fatalf("initial View() = %q, want latest viewport slice", initial)
	}
	if strings.Contains(initial, "first message") {
		t.Fatalf("initial View() = %q, want oldest message out of view before scrolling", initial)
	}

	updated := model
	for i := 0; i < 1; i++ {
		updated, _ = updated.Update(tea.MouseMsg{
			Button: tea.MouseButtonWheelUp,
			Action: tea.MouseActionPress,
		}, updated.width, updated.viewportHeight)
	}

	scrolled := updated.View()
	if !strings.Contains(scrolled, "first message") || !strings.Contains(scrolled, "fourth message") {
		t.Fatalf("scrolled View() = %q, want oldest viewport slice after wheel-up", scrolled)
	}
	if strings.Contains(scrolled, "sixth message") {
		t.Fatalf("scrolled View() = %q, want latest message out of view after scrolling to top", scrolled)
	}
}

func TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder(t *testing.T) {
	model := NewOutputModel()
	model.width = 100
	model.height = 40
	model = model.AppendBlock(TranscriptBlock{
		ID:   "explored-seq",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind:   "explored",
			Title:       "Explored",
			Collapsible: true,
			Items: []ActivityItem{
				{
					ToolCallID: "read-1",
					Verb:       "Read",
					Text:       "internal/tools/builtin.go",
					Status:     ActivityItemCompleted,
					Preview: PreviewBody{
						Text:        "package tools\nfunc Builtin() {}",
						Kind:        PreviewKindText,
						Collapsible: false,
					},
				},
				{
					ToolCallID: "read-2",
					Verb:       "Read",
					Text:       "internal/tools/builtin_readonly.go",
					Status:     ActivityItemCompleted,
					Preview: PreviewBody{
						Text:        "package tools\nfunc Readonly() {}",
						Kind:        PreviewKindText,
						Collapsible: false,
					},
				},
			},
		},
	})

	collapsed := model.InteractiveView()
	for _, want := range []string{
		"Explored",
		"Read internal/tools/builtin.go",
		"Read internal/tools/builtin_readonly.go",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed InteractiveView() missing %q in:\n%s", want, collapsed)
		}
	}
	for _, unwanted := range []string{
		"func Builtin() {}",
		"func Readonly() {}",
	} {
		if strings.Contains(collapsed, unwanted) {
			t.Fatalf("collapsed InteractiveView() unexpectedly contains %q in:\n%s", unwanted, collapsed)
		}
	}

	updated, toggled := model.ToggleExpand()
	if !toggled {
		t.Fatal("ToggleExpand() = false, want grouped explored block to expand")
	}

	expanded := updated.InteractiveView()
	for _, want := range []string{
		"Read internal/tools/builtin.go",
		"package tools",
		"func Builtin() {}",
		"Read internal/tools/builtin_readonly.go",
		"func Readonly() {}",
	} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded InteractiveView() missing %q in:\n%s", want, expanded)
		}
	}
	if strings.Index(expanded, "func Builtin() {}") > strings.Index(expanded, "func Readonly() {}") {
		t.Fatalf("expanded InteractiveView() rendered previews out of order:\n%s", expanded)
	}
}

func TestOutputModelExploredGroupFallsBackToLegacyGroupPreview(t *testing.T) {
	model := NewOutputModel()
	model.width = 100
	model.height = 40
	model = model.AppendBlock(TranscriptBlock{
		ID:   "legacy-explored",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind: "explored",
			Title:     "Explored",
			Items: []ActivityItem{
				{Verb: "Read", Text: "internal/tools/builtin.go", Status: ActivityItemCompleted},
			},
			Preview: PreviewBody{
				Text:        "package tools\nfunc Legacy() {}",
				Kind:        PreviewKindText,
				Collapsible: false,
			},
		},
	})

	collapsed := model.InteractiveView()
	if strings.Contains(collapsed, "func Legacy() {}") {
		t.Fatalf("collapsed InteractiveView() unexpectedly shows legacy preview:\n%s", collapsed)
	}
	if !strings.Contains(collapsed, "Ctrl+O expand") {
		t.Fatalf("collapsed InteractiveView() missing expand affordance for hidden legacy preview:\n%s", collapsed)
	}

	updated, toggled := model.ToggleExpand()
	if !toggled {
		t.Fatal("ToggleExpand() = false, want legacy explored block to expand")
	}

	expanded := updated.InteractiveView()
	for _, want := range []string{"Read internal/tools/builtin.go", "func Legacy() {}", "Ctrl+O collapse"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded InteractiveView() missing %q in:\n%s", want, expanded)
		}
	}
}
