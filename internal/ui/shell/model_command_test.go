package shell

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderLiveStatusTextUsesCurrentToolSummary(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking
	model.runtime.CurrentTool = &ToolCallInfo{
		Name:   "bash",
		Status: ToolStatusRunning,
		Args:   "go test ./internal/ui/shell",
	}

	if got := model.renderLiveStatusText(); got != "Running Bash(go test ./internal/ui/shell)..." {
		t.Fatalf("renderLiveStatusText() = %q, want %q", got, "Running Bash(go test ./internal/ui/shell)...")
	}
}

func TestRenderLiveStatusTextFallsBackWithoutCurrentTool(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeStreaming

	if got := model.renderLiveStatusText(); got != "Running..." {
		t.Fatalf("renderLiveStatusText() = %q, want %q", got, "Running...")
	}
}

func TestRenderLiveStatusTextIgnoresFinishedTool(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking
	model.runtime.CurrentTool = &ToolCallInfo{
		Name:   "bash",
		Status: ToolStatusCompleted,
		Args:   "go test ./internal/ui/shell",
	}

	if got := model.renderLiveStatusText(); got != "Running..." {
		t.Fatalf("renderLiveStatusText() = %q, want %q", got, "Running...")
	}
}

func TestHandleCommandCompactStartsRuntimeExecution(t *testing.T) {
	model := NewModel(Dependencies{ModelName: "test-model", SystemPrompt: "system"}, nil)
	model.output = model.output.SetPending([]TranscriptLine{{Type: LineTypeAssistant, Content: "pending assistant output"}})
	model.runtime.Step = 2
	model.runtime.AssistantText = "stale"
	spec := compactActionSpec()

	updated, cmd := model.handleCommand("/compact")
	if cmd == nil {
		t.Fatal("handleCommand(/compact) cmd = nil, want non-nil")
	}

	gotModel := updated.(Model)
	if gotModel.mode != ModeThinking {
		t.Fatalf("mode = %v, want %v", gotModel.mode, ModeThinking)
	}
	if gotModel.wire == nil {
		t.Fatal("wire = nil, want initialized wire")
	}
	if gotModel.activeShellActionCommand != spec.CommandText {
		t.Fatalf("active shell action = %q, want %q", gotModel.activeShellActionCommand, spec.CommandText)
	}
	if gotModel.runtime.Step != 0 {
		t.Fatalf("runtime step = %d, want 0 after reset", gotModel.runtime.Step)
	}
	if gotModel.runtime.AssistantText != "" {
		t.Fatalf("runtime assistant text = %q, want empty after reset", gotModel.runtime.AssistantText)
	}
	if len(gotModel.output.pending) != 0 {
		t.Fatalf("pending lines = %d, want 0 after flush", len(gotModel.output.pending))
	}
	if len(gotModel.output.lines) != 3 {
		t.Fatalf("handleCommand(/compact) output lines = %d, want 3", len(gotModel.output.lines))
	}
	if gotModel.output.lines[0].Content != "pending assistant output" {
		t.Fatalf("first output content = %q, want flushed pending output", gotModel.output.lines[0].Content)
	}
	if gotModel.output.lines[1].Type != LineTypeSystem {
		t.Fatalf("second output line type = %v, want %v", gotModel.output.lines[1].Type, LineTypeSystem)
	}
	if gotModel.output.lines[1].Content != spec.StatusText {
		t.Fatalf("second output content = %q, want %q", gotModel.output.lines[1].Content, spec.StatusText)
	}
	last := gotModel.output.lines[len(gotModel.output.lines)-1]
	if last.Type != LineTypeUser {
		t.Fatalf("last output line type = %v, want %v", last.Type, LineTypeUser)
	}
	if last.Content != spec.CommandText {
		t.Fatalf("last output content = %q, want %q", last.Content, spec.CommandText)
	}
}

func TestHandleCommandRewindStartsCheckpointListing(t *testing.T) {
	model := NewModel(Dependencies{Store: contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))}, nil)

	updated, cmd := model.handleCommand("/rewind")
	if cmd == nil {
		t.Fatal("handleCommand(/rewind) cmd = nil, want non-nil")
	}

	gotModel := updated.(Model)
	if gotModel.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", gotModel.mode, ModeIdle)
	}
	if len(gotModel.output.lines) != 0 {
		t.Fatalf("output lines = %d, want 0 before async result", len(gotModel.output.lines))
	}
}

func TestHandleCheckpointListResultShowsNoCheckpointNotice(t *testing.T) {
	model := NewModel(Dependencies{}, nil)

	updatedModel, cmd := model.handleCheckpointListResult(CheckpointListMsg{Checkpoints: nil})
	if cmd != nil {
		t.Fatalf("handleCheckpointListResult() cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if len(updated.output.lines) != 1 {
		t.Fatalf("output lines = %d, want 1", len(updated.output.lines))
	}
	line := updated.output.lines[0]
	if line.Type != LineTypeSystem {
		t.Fatalf("line type = %v, want %v", line.Type, LineTypeSystem)
	}
	if line.Content != "No rewind checkpoints found for this session." {
		t.Fatalf("line content = %q, want exact no-checkpoint notice", line.Content)
	}
}

func TestHandleCheckpointListResultEntersCheckpointSelectMode(t *testing.T) {
	model := NewModel(Dependencies{}, nil)

	updatedModel, cmd := model.handleCheckpointListResult(CheckpointListMsg{Checkpoints: []contextstore.CheckpointRecord{
		{Role: contextstore.RoleCheckpoint, ID: 0, CreatedAt: "2026-03-27T10:00:00Z", PromptPreview: "first task"},
		{Role: contextstore.RoleCheckpoint, ID: 1, PromptPreview: "second task"},
	}})
	if cmd != nil {
		t.Fatalf("handleCheckpointListResult() cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.mode != ModeCheckpointSelect {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeCheckpointSelect)
	}
	if len(updated.checkpointList) != 2 {
		t.Fatalf("checkpoint list len = %d, want 2", len(updated.checkpointList))
	}
	if updated.selectedCheckpoint != 0 {
		t.Fatalf("selected checkpoint = %d, want 0", updated.selectedCheckpoint)
	}
	if len(updated.output.lines) != 0 {
		t.Fatalf("output lines = %d, want 0 when entering picker", len(updated.output.lines))
	}
}

func TestHandleCheckpointSelectKeyPressMovesSelection(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeCheckpointSelect
	model.checkpointList = []contextstore.CheckpointRecord{{ID: 0}, {ID: 1}, {ID: 2}}

	updatedModel, cmd := model.handleCheckpointSelectKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd != nil {
		t.Fatalf("handleCheckpointSelectKeyPress(j) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.selectedCheckpoint != 1 {
		t.Fatalf("selected checkpoint = %d, want 1", updated.selectedCheckpoint)
	}
}

func TestHandleCheckpointSelectKeyPressCancelsSelection(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeCheckpointSelect
	model.checkpointList = []contextstore.CheckpointRecord{{ID: 0}, {ID: 1}}
	model.selectedCheckpoint = 1
	model.checkpointScrollOffset = 1

	updatedModel, cmd := model.handleCheckpointSelectKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("handleCheckpointSelectKeyPress(esc) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeIdle)
	}
	if updated.checkpointList != nil {
		t.Fatalf("checkpoint list = %#v, want nil", updated.checkpointList)
	}
	if updated.selectedCheckpoint != 0 {
		t.Fatalf("selected checkpoint = %d, want 0", updated.selectedCheckpoint)
	}
	if updated.checkpointScrollOffset != 0 {
		t.Fatalf("checkpoint scroll offset = %d, want 0", updated.checkpointScrollOffset)
	}
}

func TestHandleCheckpointSelectKeyPressRewindsConversation(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	store := contextstore.New(historyFile)
	for _, record := range []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("first task"),
		contextstore.NewAssistantTextRecord("first answer"),
	} {
		if err := store.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}
	if _, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{CreatedAt: "2026-03-27T10:00:00Z", PromptPreview: "first task"}); err != nil {
		t.Fatalf("AppendCheckpointWithMetadata() error = %v", err)
	}
	for _, record := range []contextstore.TextRecord{
		contextstore.NewUserTextRecord("second task"),
		contextstore.NewAssistantTextRecord("second answer"),
	} {
		if err := store.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	model := NewModel(Dependencies{Store: store}, nil)
	model.mode = ModeCheckpointSelect
	model.checkpointList = []contextstore.CheckpointRecord{{ID: 0, PromptPreview: "first task"}}
	model.selectedCheckpoint = 0
	model.checkpointScrollOffset = 1
	model.output = model.output.AppendLine(TranscriptLine{Type: LineTypeUser, Content: "stale transcript"})
	model.output = model.output.SetPending([]TranscriptLine{{Type: LineTypeAssistant, Content: "pending"}})
	model.runtime.Step = 3
	model.runtime.AssistantText = "stale"

	updatedModel, cmd := model.handleCheckpointSelectKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("handleCheckpointSelectKeyPress(enter) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)

	if updated.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeIdle)
	}
	if updated.checkpointList != nil {
		t.Fatalf("checkpoint list = %#v, want nil", updated.checkpointList)
	}
	if updated.selectedCheckpoint != 0 {
		t.Fatalf("selected checkpoint = %d, want 0", updated.selectedCheckpoint)
	}
	if updated.checkpointScrollOffset != 0 {
		t.Fatalf("checkpoint scroll offset = %d, want 0", updated.checkpointScrollOffset)
	}
	if updated.runtime.Step != 0 {
		t.Fatalf("runtime step = %d, want 0", updated.runtime.Step)
	}
	if updated.runtime.AssistantText != "" {
		t.Fatalf("runtime assistant text = %q, want empty", updated.runtime.AssistantText)
	}
	if len(updated.output.pending) != 0 {
		t.Fatalf("pending lines = %d, want 0", len(updated.output.pending))
	}
	if len(updated.output.lines) != 3 {
		t.Fatalf("output lines = %d, want 3", len(updated.output.lines))
	}
	if updated.output.lines[0].Type != LineTypeUser || updated.output.lines[0].Content != "first task" {
		t.Fatalf("first output line = %#v, want rewound first user turn", updated.output.lines[0])
	}
	if updated.output.lines[1].Type != LineTypeAssistant || updated.output.lines[1].Content != "first answer" {
		t.Fatalf("second output line = %#v, want rewound first assistant turn", updated.output.lines[1])
	}
	last := updated.output.lines[len(updated.output.lines)-1]
	if last.Type != LineTypeSystem {
		t.Fatalf("last output line type = %v, want %v", last.Type, LineTypeSystem)
	}
	if last.Content != "Conversation rewound to checkpoint 0." {
		t.Fatalf("last output line content = %q, want rewind notice", last.Content)
	}
	if updated.deps.StartupInfo.LastRole != contextstore.RoleAssistant {
		t.Fatalf("startup last role = %q, want %q", updated.deps.StartupInfo.LastRole, contextstore.RoleAssistant)
	}
	if updated.deps.StartupInfo.LastSummary != "first answer" {
		t.Fatalf("startup last summary = %q, want %q", updated.deps.StartupInfo.LastSummary, "first answer")
	}

	gotRecords, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("first task"),
		contextstore.NewAssistantTextRecord("first answer"),
	}
	if !reflect.DeepEqual(gotRecords, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", gotRecords, wantRecords)
	}
}

func TestStartShellActionRejectsEmptyInputs(t *testing.T) {
	model := NewModel(Dependencies{}, nil)

	updated, cmd := model.startShellAction(shellActionSpec{CommandText: "   ", StatusText: "status", Prompt: "   "})
	if cmd != nil {
		t.Fatalf("startShellAction() cmd = %#v, want nil", cmd)
	}

	gotModel := updated.(Model)
	if gotModel.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", gotModel.mode, ModeIdle)
	}
	if len(gotModel.output.lines) != 1 {
		t.Fatalf("output lines = %d, want 1", len(gotModel.output.lines))
	}
	line := gotModel.output.lines[0]
	if line.Type != LineTypeError {
		t.Fatalf("line type = %v, want %v", line.Type, LineTypeError)
	}
	if line.Content != "error: shell action requires non-empty command text and prompt" {
		t.Fatalf("line content = %q, want exact error", line.Content)
	}
}

func TestFinishRuntimeCompactsSessionHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	store := contextstore.New(historyFile)
	for _, record := range []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("old task"),
		contextstore.NewAssistantTextRecord("old verbose answer"),
	} {
		if err := store.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	model := NewModel(Dependencies{Store: store}, nil)
	model.mode = ModeThinking
	model.activeShellActionCommand = "/compact"
	model.output = model.output.AppendLine(TranscriptLine{Type: LineTypeSystem, Content: "Compacting..."})
	model.output = model.output.SetPending([]TranscriptLine{{Type: LineTypeAssistant, Content: "draft compact output"}})
	model.runtime.AssistantText = "draft compact output"

	updated := model.finishRuntime(RuntimeCompleteMsg{Result: runtime.Result{
		UserRecord: ptrTextRecord(contextstore.NewUserTextRecord("/compact")),
		Steps: []runtime.StepResult{{
			AssistantText: "Current goal\n- finish compact",
		}},
	}})

	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("old task"),
		contextstore.NewAssistantTextRecord("Current goal\n- finish compact"),
	}
	gotRecords, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(gotRecords, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", gotRecords, wantRecords)
	}
	if updated.activeShellActionCommand != "" {
		t.Fatalf("active shell action = %q, want empty", updated.activeShellActionCommand)
	}
	if updated.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeIdle)
	}
	backupStore := contextstore.New(historyFile + ".compact.1")
	backupRecords, err := backupStore.ReadAll()
	if err != nil {
		t.Fatalf("backup ReadAll() error = %v", err)
	}
	wantBackupRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("old task"),
		contextstore.NewAssistantTextRecord("old verbose answer"),
	}
	if !reflect.DeepEqual(backupRecords, wantBackupRecords) {
		t.Fatalf("backup records = %#v, want %#v", backupRecords, wantBackupRecords)
	}
	if len(updated.output.lines) != 3 {
		t.Fatalf("output lines = %d, want 3 compacted lines", len(updated.output.lines))
	}
	if updated.output.lines[0].Type != LineTypeUser || updated.output.lines[0].Content != "old task" {
		t.Fatalf("first compacted output line = %#v, want original user goal", updated.output.lines[0])
	}
	if updated.output.lines[1].Type != LineTypeAssistant || updated.output.lines[1].Content != "Current goal\n- finish compact" {
		t.Fatalf("second compacted output line = %#v, want compact assistant summary", updated.output.lines[1])
	}
	if updated.output.lines[2].Type != LineTypeSystem || updated.output.lines[2].Content != compactedNoticeText() {
		t.Fatalf("third compacted output line = %#v, want compact completion notice", updated.output.lines[2])
	}
	if updated.deps.StartupInfo.LastRole != contextstore.RoleAssistant {
		t.Fatalf("startup last role = %q, want %q", updated.deps.StartupInfo.LastRole, contextstore.RoleAssistant)
	}
	if updated.deps.StartupInfo.LastSummary != "Current goal\n- finish compact" {
		t.Fatalf("startup last summary = %q, want compacted assistant text", updated.deps.StartupInfo.LastSummary)
	}
}

func TestCompactActionSpecIncludesSummaryIntent(t *testing.T) {
	got := compactActionSpec()
	if got.CommandText != "/compact" {
		t.Fatalf("command text = %q, want %q", got.CommandText, "/compact")
	}
	if got.StatusText == "" {
		t.Fatal("status text = empty, want non-empty")
	}
	for _, want := range []string{
		"Compact the current conversation context",
		"Current goal",
		"Constraints",
		"Decisions",
		"Open tasks",
		"Next step",
		"Keep each section brief and concrete.",
	} {
		if !strings.Contains(got.Prompt, want) {
			t.Fatalf("compactActionSpec().Prompt missing %q in %q", want, got.Prompt)
		}
	}
}

func ptrTextRecord(record contextstore.TextRecord) *contextstore.TextRecord {
	return &record
}
