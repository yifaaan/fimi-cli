package contextstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContextBootstrapSeedsEmptyHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)
	initialRecord := NewSystemTextRecord("boot")

	result, err := ctx.Bootstrap(initialRecord)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if !result.HistoryExists {
		t.Fatalf("Bootstrap() HistoryExists = false, want true")
	}
	if !result.HistorySeeded {
		t.Fatalf("Bootstrap() HistorySeeded = false, want true")
	}
	if result.Snapshot.Count != 1 {
		t.Fatalf("Bootstrap() Snapshot.Count = %d, want 1", result.Snapshot.Count)
	}
	if !result.Snapshot.HasLastRecord {
		t.Fatalf("Bootstrap() Snapshot.HasLastRecord = false, want true")
	}
	if result.Snapshot.LastRecord != initialRecord {
		t.Fatalf("Bootstrap() Snapshot.LastRecord = %#v, want %#v", result.Snapshot.LastRecord, initialRecord)
	}
}

func TestContextBootstrapKeepsExistingHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(
		historyFile,
		[]byte("{\"role\":\"user\",\"content\":\"hello\"}\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", historyFile, err)
	}

	ctx := New(historyFile)

	result, err := ctx.Bootstrap(NewSystemTextRecord("boot"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if !result.HistoryExists {
		t.Fatalf("Bootstrap() HistoryExists = false, want true")
	}
	if result.HistorySeeded {
		t.Fatalf("Bootstrap() HistorySeeded = true, want false")
	}
	if result.Snapshot.Count != 1 {
		t.Fatalf("Bootstrap() Snapshot.Count = %d, want 1", result.Snapshot.Count)
	}
	if !result.Snapshot.HasLastRecord {
		t.Fatalf("Bootstrap() Snapshot.HasLastRecord = false, want true")
	}
	want := NewUserTextRecord("hello")
	if result.Snapshot.LastRecord != want {
		t.Fatalf("Bootstrap() Snapshot.LastRecord = %#v, want %#v", result.Snapshot.LastRecord, want)
	}
}
