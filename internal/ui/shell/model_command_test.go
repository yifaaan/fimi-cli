package shell

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestRuntimeModelApplyStatusUpdateDoesNotChangeTranscriptBlocks(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.TextPart{Text: "Inspecting transcript."})
	baseline := model.ToBlocks()

	model = model.ApplyEvent(runtimeevents.StatusUpdate{Status: runtimeevents.StatusSnapshot{
		ContextUsage: 0.25,
		Retry: &runtimeevents.RetryStatus{
			Attempt:     1,
			MaxAttempts: 3,
			NextDelayMS: 750,
		},
	}})

	if model.Retry == nil {
		t.Fatal("runtime retry = nil, want retry status stored")
	}
	if model.Retry.Attempt != 1 || model.Retry.MaxAttempts != 3 || model.Retry.NextDelayMS != 750 {
		t.Fatalf("runtime retry = %#v, want attempt=1 max=3 delay=750", model.Retry)
	}
	got := model.ToBlocks()
	if len(got) != len(baseline) || got[0].NoteText != baseline[0].NoteText {
		t.Fatalf("ToBlocks() after status update = %#v, want unchanged %#v", got, baseline)
	}
}

func TestHandleSubmitKeepsBannerVisibleForRegularPrompts(t *testing.T) {
	model := NewModel(Dependencies{
		StartupInfo: StartupInfo{
			SessionID: "session-1",
		},
	}, nil)

	updatedModel, cmd := model.handleSubmit("what is your name?")
	if cmd == nil {
		t.Fatal("handleSubmit() cmd = nil, want runtime command batch")
	}

	updated := updatedModel.(Model)
	if !updated.showBanner {
		t.Fatal("showBanner = false, want startup banner to remain visible")
	}
	if updated.mode != ModeThinking {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeThinking)
	}
	if len(updated.output.pending) != 1 {
		t.Fatalf("pending blocks = %d, want 1", len(updated.output.pending))
	}
	if updated.output.pending[0].Kind != BlockKindUserPrompt || updated.output.pending[0].UserText != "what is your name?" {
		t.Fatalf("pending block = %#v, want submitted user prompt", updated.output.pending[0])
	}
}

func TestSpinnerTickKeepsSubmittedUserPromptPending(t *testing.T) {
	model := NewModel(Dependencies{}, nil)

	updatedModel, _ := model.handleSubmit("what is your name?")
	updated := updatedModel.(Model)

	tickedModel, cmd := updated.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("Update(spinner.TickMsg) cmd = nil, want spinner command")
	}
	ticked := tickedModel.(Model)

	if len(ticked.output.pending) != 1 {
		t.Fatalf("pending blocks after spinner tick = %d, want 1", len(ticked.output.pending))
	}
	if ticked.output.pending[0].Kind != BlockKindUserPrompt || ticked.output.pending[0].UserText != "what is your name?" {
		t.Fatalf("pending block after spinner tick = %#v, want submitted user prompt", ticked.output.pending[0])
	}
}

func TestRenderOutputForLayoutShowsFullTranscript(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.width = 80
	model.height = 20
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "first question"})
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: "first answer"})
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "second question"})
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: "second answer"})

	before, after := model.mainViewLayoutSections()
	got := model.renderOutputForLayout(before, after)

	for _, want := range []string{"first question", "first answer", "second question", "second answer"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderOutputForLayout() missing %q in:\n%s", want, got)
		}
	}
}

func TestHandleCommandCompactStartsRuntimeExecution(t *testing.T) {
	model := NewModel(Dependencies{ModelName: "test-model", SystemPrompt: "system"}, nil)
	model.output = model.output.SetPending([]TranscriptBlock{{
		ID:       "pending-note",
		Kind:     BlockKindAssistantNote,
		NoteText: "pending assistant output",
	}})
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
	if len(gotModel.output.pending) != 2 {
		t.Fatalf("pending blocks = %d, want 2 interactive shell-action blocks", len(gotModel.output.pending))
	}
	if len(gotModel.output.blocks) != 1 {
		t.Fatalf("committed blocks = %d, want 1 flushed pending note", len(gotModel.output.blocks))
	}
	if gotModel.output.blocks[0].Kind != BlockKindAssistantNote || gotModel.output.blocks[0].NoteText != "pending assistant output" {
		t.Fatalf("first block = %#v, want flushed pending note", gotModel.output.blocks[0])
	}
	if gotModel.output.pending[0].Kind != BlockKindSystemNotice || gotModel.output.pending[0].Text != spec.StatusText {
		t.Fatalf("first pending block = %#v, want shell-action status", gotModel.output.pending[0])
	}
	last := gotModel.output.pending[len(gotModel.output.pending)-1]
	if last.Kind != BlockKindUserPrompt || last.UserText != spec.CommandText {
		t.Fatalf("last pending block = %#v, want user shell command prompt", last)
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
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "stale transcript"})
	model.output = model.output.SetPending([]TranscriptBlock{{Kind: BlockKindAssistantNote, NoteText: "pending"}})
	model.runtime.Step = 3
	model.runtime.AssistantText = "stale"

	updatedModel, cmd := model.handleCheckpointSelectKeyPress(keyEnter())
	if cmd != nil {
		t.Fatalf("handleCheckpointSelectKeyPress(enter) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)

	if updated.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeIdle)
	}
	if updated.runtime.Step != 0 || updated.runtime.AssistantText != "" {
		t.Fatalf("runtime = %#v, want reset state", updated.runtime)
	}
	if len(updated.output.pending) != 0 {
		t.Fatalf("pending blocks = %d, want 0", len(updated.output.pending))
	}
	if len(updated.output.blocks) != 3 {
		t.Fatalf("committed blocks = %d, want 3", len(updated.output.blocks))
	}
	if updated.output.blocks[0].Kind != BlockKindUserPrompt || updated.output.blocks[0].UserText != "first task" {
		t.Fatalf("first output block = %#v, want rewound first user turn", updated.output.blocks[0])
	}
	if updated.output.blocks[1].Kind != BlockKindAssistantNote || updated.output.blocks[1].NoteText != "first answer" {
		t.Fatalf("second output block = %#v, want rewound first assistant turn", updated.output.blocks[1])
	}
	last := updated.output.blocks[len(updated.output.blocks)-1]
	if last.Kind != BlockKindSystemNotice || last.Text != "Conversation rewound to checkpoint 0." {
		t.Fatalf("last output block = %#v, want rewind notice", last)
	}
}

func TestFinishRuntimeCompactsSessionHistoryIntoBlocks(t *testing.T) {
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
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindSystemNotice, Text: "Compacting..."})
	model.output = model.output.SetPending([]TranscriptBlock{{Kind: BlockKindAssistantNote, NoteText: "draft compact output"}})
	model.runtime.AssistantText = "draft compact output"

	updated := model.finishRuntime(RuntimeCompleteMsg{Result: runtime.Result{
		Steps: []runtime.StepResult{{
			AssistantText: "Current goal\n- finish compact",
		}},
	}})

	gotRecords, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("session initialized"),
		contextstore.NewUserTextRecord("old task"),
		contextstore.NewAssistantTextRecord("Current goal\n- finish compact"),
	}
	if len(gotRecords) != len(wantRecords) {
		t.Fatalf("len(history records) = %d, want %d", len(gotRecords), len(wantRecords))
	}
	for i, want := range wantRecords {
		if gotRecords[i].Role != want.Role || gotRecords[i].Content != want.Content {
			t.Fatalf("record %d = %#v, want %#v", i, gotRecords[i], want)
		}
	}

	if len(updated.output.blocks) != 3 {
		t.Fatalf("output blocks = %d, want 3", len(updated.output.blocks))
	}
	if updated.output.blocks[0].Kind != BlockKindUserPrompt || updated.output.blocks[0].UserText != "old task" {
		t.Fatalf("first block = %#v, want original user goal", updated.output.blocks[0])
	}
	if updated.output.blocks[1].Kind != BlockKindAssistantNote || updated.output.blocks[1].NoteText != "Current goal\n- finish compact" {
		t.Fatalf("second block = %#v, want compacted assistant summary", updated.output.blocks[1])
	}
	if updated.output.blocks[2].Kind != BlockKindSystemNotice || updated.output.blocks[2].Text != compactedNoticeText() {
		t.Fatalf("third block = %#v, want compact completion notice", updated.output.blocks[2])
	}
}

func TestHandleRuntimeEventsCommitsLateAssistantBlockAfterCompletion(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeStreaming
	model.output = model.output.SetPending([]TranscriptBlock{{Kind: BlockKindAssistantNote, NoteText: "pending assistant"}})

	finished := model.finishRuntime(RuntimeCompleteMsg{})
	if len(finished.output.blocks) != 1 {
		t.Fatalf("committed blocks after finish = %d, want 1", len(finished.output.blocks))
	}
	if len(finished.output.pending) != 0 {
		t.Fatalf("pending blocks after finish = %d, want 0", len(finished.output.pending))
	}

	updatedModel, cmd := finished.handleRuntimeEvents(RuntimeEventsMsg{Events: []runtimeevents.Event{
		runtimeevents.TextPart{Text: "late assistant reply"},
	}})
	if cmd != nil {
		t.Fatalf("handleRuntimeEvents(late text) cmd = %#v, want nil while idle", cmd)
	}

	updated := updatedModel.(Model)
	if len(updated.output.pending) != 0 {
		t.Fatalf("pending blocks after late text = %d, want 0", len(updated.output.pending))
	}
	if len(updated.output.blocks) != 1 {
		t.Fatalf("committed blocks after late text = %d, want 1 merged assistant block", len(updated.output.blocks))
	}
	if updated.output.blocks[0].Kind != BlockKindAssistantNote {
		t.Fatalf("merged block = %#v, want assistant note", updated.output.blocks[0])
	}
	if !strings.Contains(updated.output.blocks[0].NoteText, "pending assistant") || !strings.Contains(updated.output.blocks[0].NoteText, "late assistant reply") {
		t.Fatalf("merged assistant note = %q, want both pending and late text", updated.output.blocks[0].NoteText)
	}
}

func TestStartRuntimeExecutionUsesDependenciesRunContext(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &contextAwareRunner{}
	model := NewModel(Dependencies{
		Runner:       runner,
		ModelName:    "test-model",
		SystemPrompt: "system",
		RunContext:   runCtx,
	}, nil)

	msg := model.startRuntimeExecution(contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")), "hello")()
	complete, ok := msg.(RuntimeCompleteMsg)
	if !ok {
		t.Fatalf("cmd() message type = %T, want RuntimeCompleteMsg", msg)
	}
	if !errors.Is(complete.Err, context.Canceled) {
		t.Fatalf("RuntimeCompleteMsg.Err = %v, want %v", complete.Err, context.Canceled)
	}
	if !errors.Is(runner.seenErr, context.Canceled) {
		t.Fatalf("runner.seenErr = %v, want %v", runner.seenErr, context.Canceled)
	}
}

func TestRenderLiveStatusTextUsesWorkingElapsed(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking
	model.runtimeStartedAt = time.Now().Add(-11 * time.Second)

	got := model.renderLiveStatusText()
	if !strings.HasPrefix(got, "Working (") {
		t.Fatalf("renderLiveStatusText() = %q, want Working prefix", got)
	}
	if !strings.Contains(got, "11s") && !strings.Contains(got, "10s") && !strings.Contains(got, "12s") {
		t.Fatalf("renderLiveStatusText() = %q, want elapsed seconds", got)
	}
}

func TestRenderLiveStatusKeepsLeftAlignedAcrossLines(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking
	model.runtimeStartedAt = time.Now().Add(-11 * time.Second)

	lines := nonEmptyLines(ansi.Strip(model.renderLiveStatus()))
	if len(lines) < 3 {
		t.Fatalf("len(lines) = %d, want multi-line live-status box", len(lines))
	}

	for i, line := range lines {
		if got := leadingSpaces(line); got != 0 {
			t.Fatalf("line %d leading spaces = %d, want 0; line=%q", i, got, line)
		}
	}
}

func TestRenderDropdownKeepsLeftAlignedAcrossLines(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.width = 80

	lines := nonEmptyLines(ansi.Strip(model.renderDropdown("Commands", []string{"/help", "/clear"}, 0, 1)))
	if len(lines) < 3 {
		t.Fatalf("len(lines) = %d, want multi-line dropdown box", len(lines))
	}

	for i, line := range lines {
		if got := leadingSpaces(line); got != 0 {
			t.Fatalf("line %d leading spaces = %d, want 0; line=%q", i, got, line)
		}
	}
}

func TestJoinTranscriptForTeaPrintKeepsCommittedBlockOrder(t *testing.T) {
	got := joinTranscriptForTeaPrint([]string{
		"hello",
		"Hello!",
		"what is your name",
		"I'm fimi.",
	})

	want := strings.Join([]string{
		"hello",
		"Hello!",
		"what is your name",
		"I'm fimi.",
	}, "\n")
	if got != want {
		t.Fatalf("joinTranscriptForTeaPrint() = %q, want %q", got, want)
	}
}

type contextAwareRunner struct {
	seenErr error
}

func (r *contextAwareRunner) Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error) {
	r.seenErr = ctx.Err()
	if r.seenErr == nil {
		<-ctx.Done()
		r.seenErr = ctx.Err()
	}
	return runtime.Result{Status: runtime.RunStatusInterrupted}, ctx.Err()
}

func keyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}
