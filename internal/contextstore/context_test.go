package contextstore

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestContextReadRecentKeepsTailWindow(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	records := []TextRecord{
		NewSystemTextRecord("boot"),
		NewUserTextRecord("first"),
		NewAssistantTextRecord("first reply"),
		NewUserTextRecord("second"),
		NewAssistantTextRecord("second reply"),
	}
	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	got, err := ctx.ReadRecent(3)
	if err != nil {
		t.Fatalf("ReadRecent() error = %v", err)
	}

	want := []TextRecord{
		NewAssistantTextRecord("first reply"),
		NewUserTextRecord("second"),
		NewAssistantTextRecord("second reply"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadRecent() = %#v, want %#v", got, want)
	}
}

func TestContextReadRecentReturnsEmptyWhenLimitNonPositive(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	if err := ctx.Append(NewUserTextRecord("hello")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	got, err := ctx.ReadRecent(0)
	if err != nil {
		t.Fatalf("ReadRecent() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(ReadRecent()) = %d, want 0", len(got))
	}
}
