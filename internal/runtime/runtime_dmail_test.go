package runtime

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"fimi-cli/internal/contextstore"
)

type checkpointCountingDMailer struct {
	pending *dmailEntry
	fetched bool
	counts  []int
}

func (m *checkpointCountingDMailer) SetCheckpointCount(n int) {
	m.counts = append(m.counts, n)
}

func (m *checkpointCountingDMailer) Fetch() (string, int, bool) {
	if m.pending != nil && !m.fetched {
		m.fetched = true
		return m.pending.message, m.pending.checkpointID, true
	}
	return "", 0, false
}

func TestRunnerPersistRunStartAppendsCheckpointMarkerAndPrompt(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	dmailer := &checkpointCountingDMailer{}
	runner := New(staticEngine{}, Config{}).WithDMailer(dmailer)
	userRecord := contextstore.NewUserTextRecord("hello world")

	if err := runner.persistRunStart(context.Background(), store, "hello world", userRecord); err != nil {
		t.Fatalf("persistRunStart() error = %v", err)
	}

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("<system>CHECKPOINT 0</system>"),
		contextstore.NewUserTextRecord("hello world"),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}

	checkpoints, err := store.ListCheckpoints()
	if err != nil {
		t.Fatalf("ListCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("len(ListCheckpoints()) = %d, want 1", len(checkpoints))
	}
	if checkpoints[0].ID != 0 {
		t.Fatalf("checkpoints[0].ID = %d, want 0", checkpoints[0].ID)
	}
	if checkpoints[0].PromptPreview != "hello world" {
		t.Fatalf("checkpoints[0].PromptPreview = %q, want %q", checkpoints[0].PromptPreview, "hello world")
	}
	if checkpoints[0].CreatedAt == "" {
		t.Fatal("checkpoints[0].CreatedAt = empty, want non-empty")
	}
	if !reflect.DeepEqual(dmailer.counts, []int{1}) {
		t.Fatalf("SetCheckpointCount() calls = %#v, want %#v", dmailer.counts, []int{1})
	}
}

func TestRunnerApplyPendingDMailRevertsHistoryAndInjectsMessage(t *testing.T) {
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	dmailer := &checkpointCountingDMailer{
		pending: &dmailEntry{message: "prune search results", checkpointID: 1},
	}
	runner := New(staticEngine{}, Config{}).WithDMailer(dmailer)
	userRecord := contextstore.NewUserTextRecord("search for X")

	if err := runner.persistRunStart(context.Background(), store, "search for X", userRecord); err != nil {
		t.Fatalf("persistRunStart() error = %v", err)
	}
	if err := runner.persistStepCheckpoint(context.Background(), store); err != nil {
		t.Fatalf("persistStepCheckpoint() error = %v", err)
	}
	if err := store.Append(contextstore.NewAssistantTextRecord("sending dmail")); err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}

	applied, err := runner.applyPendingDMail(context.Background(), store)
	if err != nil {
		t.Fatalf("applyPendingDMail() error = %v", err)
	}
	if !applied {
		t.Fatal("applyPendingDMail() = false, want true")
	}

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("<system>CHECKPOINT 0</system>"),
		contextstore.NewUserTextRecord("search for X"),
		contextstore.NewUserTextRecord("<system>CHECKPOINT 1</system>"),
		contextstore.NewUserTextRecord("<system>D-Mail received: prune search results</system>\n\nRead the D-Mail above carefully. Act on the information it contains. Do NOT mention the D-Mail mechanism or time travel to the user."),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}

	checkpoints, err := store.ListCheckpoints()
	if err != nil {
		t.Fatalf("ListCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("len(ListCheckpoints()) = %d, want 2", len(checkpoints))
	}
	if checkpoints[0].ID != 0 || checkpoints[1].ID != 1 {
		t.Fatalf("checkpoint IDs = [%d %d], want [0 1]", checkpoints[0].ID, checkpoints[1].ID)
	}
	if !reflect.DeepEqual(dmailer.counts, []int{1, 2, 2}) {
		t.Fatalf("SetCheckpointCount() calls = %#v, want %#v", dmailer.counts, []int{1, 2, 2})
	}
}
