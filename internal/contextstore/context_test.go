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

func TestContextReadRecentTurnsStartsAtUserBoundary(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	records := []TextRecord{
		NewSystemTextRecord("boot"),
		NewUserTextRecord("u1"),
		NewAssistantTextRecord("a1"),
		NewUserTextRecord("u2"),
		NewAssistantTextRecord("a2"),
		NewUserTextRecord("u3"),
		NewAssistantTextRecord("a3"),
		NewUserTextRecord("u4"),
		NewAssistantTextRecord("a4"),
		NewUserTextRecord("u5"),
	}
	for _, record := range records {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	got, err := ctx.ReadRecentTurns(4)
	if err != nil {
		t.Fatalf("ReadRecentTurns() error = %v", err)
	}

	want := []TextRecord{
		NewUserTextRecord("u2"),
		NewAssistantTextRecord("a2"),
		NewUserTextRecord("u3"),
		NewAssistantTextRecord("a3"),
		NewUserTextRecord("u4"),
		NewAssistantTextRecord("a4"),
		NewUserTextRecord("u5"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadRecentTurns() = %#v, want %#v", got, want)
	}
}

func TestContextReadRecentTurnsReturnsEmptyWhenLimitNonPositive(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	if err := ctx.Append(NewUserTextRecord("hello")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	got, err := ctx.ReadRecentTurns(0)
	if err != nil {
		t.Fatalf("ReadRecentTurns() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(ReadRecentTurns()) = %d, want 0", len(got))
	}
}

func TestContextReadFirstUserRecordSkipsMetadata(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	for _, record := range []TextRecord{
		NewSystemTextRecord("boot"),
		NewAssistantTextRecord("thinking"),
	} {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}
	if err := ctx.AppendUsage(42); err != nil {
		t.Fatalf("AppendUsage() error = %v", err)
	}
	if _, err := ctx.AppendCheckpoint(); err != nil {
		t.Fatalf("AppendCheckpoint() error = %v", err)
	}
	if err := ctx.Append(NewUserTextRecord("first user prompt")); err != nil {
		t.Fatalf("Append(user) error = %v", err)
	}

	got, ok, err := ctx.ReadFirstUserRecord()
	if err != nil {
		t.Fatalf("ReadFirstUserRecord() error = %v", err)
	}
	if !ok {
		t.Fatal("ReadFirstUserRecord() ok = false, want true")
	}
	if want := NewUserTextRecord("first user prompt"); got != want {
		t.Fatalf("ReadFirstUserRecord() = %#v, want %#v", got, want)
	}
}

func TestContextSnapshotAndCountSkipMetadata(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	wantRecords := []TextRecord{
		NewSystemTextRecord("boot"),
		NewUserTextRecord("goal"),
		NewAssistantTextRecord("reply"),
	}
	for _, record := range wantRecords {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}
	if err := ctx.AppendUsage(99); err != nil {
		t.Fatalf("AppendUsage() error = %v", err)
	}
	if _, err := ctx.AppendCheckpoint(); err != nil {
		t.Fatalf("AppendCheckpoint() error = %v", err)
	}

	snapshot, err := ctx.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Count != len(wantRecords) {
		t.Fatalf("Snapshot().Count = %d, want %d", snapshot.Count, len(wantRecords))
	}
	if !snapshot.HasLastRecord {
		t.Fatal("Snapshot().HasLastRecord = false, want true")
	}
	if snapshot.LastRecord != wantRecords[len(wantRecords)-1] {
		t.Fatalf("Snapshot().LastRecord = %#v, want %#v", snapshot.LastRecord, wantRecords[len(wantRecords)-1])
	}

	count, err := ctx.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != len(wantRecords) {
		t.Fatalf("Count() = %d, want %d", count, len(wantRecords))
	}

	last, ok, err := ctx.Last()
	if err != nil {
		t.Fatalf("Last() error = %v", err)
	}
	if !ok {
		t.Fatal("Last() ok = false, want true")
	}
	if last != wantRecords[len(wantRecords)-1] {
		t.Fatalf("Last() = %#v, want %#v", last, wantRecords[len(wantRecords)-1])
	}
}

func TestContextRewriteTextRecordsOverwritesExistingHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	for _, record := range []TextRecord{
		NewSystemTextRecord("boot"),
		NewUserTextRecord("before"),
		NewAssistantTextRecord("before reply"),
	} {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}
	if err := ctx.AppendUsage(123); err != nil {
		t.Fatalf("AppendUsage() error = %v", err)
	}

	want := []TextRecord{
		NewSystemTextRecord("boot compacted"),
		NewUserTextRecord("current goal"),
		NewAssistantTextRecord("working summary"),
	}
	if err := ctx.RewriteTextRecords(want); err != nil {
		t.Fatalf("RewriteTextRecords() error = %v", err)
	}

	got, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadAll() after rewrite = %#v, want %#v", got, want)
	}

	snapshot, err := ctx.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Count != len(want) {
		t.Fatalf("Snapshot().Count = %d, want %d", snapshot.Count, len(want))
	}
	if !snapshot.HasLastRecord {
		t.Fatal("Snapshot().HasLastRecord = false, want true")
	}
	if snapshot.LastRecord != want[len(want)-1] {
		t.Fatalf("Snapshot().LastRecord = %#v, want %#v", snapshot.LastRecord, want[len(want)-1])
	}
}

func TestContextRewriteTextRecordsPreservingBackupRotatesPreviousHistory(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	before := []TextRecord{
		NewSystemTextRecord("boot"),
		NewUserTextRecord("before"),
		NewAssistantTextRecord("before reply"),
	}
	for _, record := range before {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	after := []TextRecord{
		NewSystemTextRecord("boot compacted"),
		NewUserTextRecord("current goal"),
		NewAssistantTextRecord("working summary"),
	}
	if err := ctx.RewriteTextRecordsPreservingBackup(after); err != nil {
		t.Fatalf("RewriteTextRecordsPreservingBackup() error = %v", err)
	}

	got, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(got, after) {
		t.Fatalf("ReadAll() after preserved rewrite = %#v, want %#v", got, after)
	}

	backup := New(historyFile + ".1")
	backupRecords, err := backup.ReadAll()
	if err != nil {
		t.Fatalf("backup ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(backupRecords, before) {
		t.Fatalf("backup records = %#v, want %#v", backupRecords, before)
	}
}

func TestContextRewriteTextRecordsPreservingNamedBackupUsesTaggedRotationPath(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	before := []TextRecord{
		NewUserTextRecord("before"),
		NewAssistantTextRecord("before reply"),
	}
	for _, record := range before {
		if err := ctx.Append(record); err != nil {
			t.Fatalf("Append(%#v) error = %v", record, err)
		}
	}

	after := []TextRecord{NewAssistantTextRecord("working summary")}
	if err := ctx.RewriteTextRecordsPreservingNamedBackup(after, "compact"); err != nil {
		t.Fatalf("RewriteTextRecordsPreservingNamedBackup() error = %v", err)
	}

	backup := New(historyFile + ".compact.1")
	backupRecords, err := backup.ReadAll()
	if err != nil {
		t.Fatalf("compact backup ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(backupRecords, before) {
		t.Fatalf("compact backup records = %#v, want %#v", backupRecords, before)
	}
}

func TestContextLatestTaggedBackupPathReturnsNewestCompactBackup(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	for _, content := range []string{"before one", "before two"} {
		if err := ctx.Append(NewUserTextRecord(content)); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
		if err := ctx.RewriteTextRecordsPreservingNamedBackup([]TextRecord{NewAssistantTextRecord("summary")}, "compact"); err != nil {
			t.Fatalf("RewriteTextRecordsPreservingNamedBackup() error = %v", err)
		}
	}

	got, ok, err := ctx.LatestTaggedBackupPath("compact")
	if err != nil {
		t.Fatalf("LatestTaggedBackupPath() error = %v", err)
	}
	if !ok {
		t.Fatal("LatestTaggedBackupPath() ok = false, want true")
	}
	want := historyFile + ".compact.2"
	if got != want {
		t.Fatalf("LatestTaggedBackupPath() = %q, want %q", got, want)
	}
}

func TestContextLatestTaggedBackupPathReturnsNotFoundWhenMissing(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)

	got, ok, err := ctx.LatestTaggedBackupPath("compact")
	if err != nil {
		t.Fatalf("LatestTaggedBackupPath() error = %v", err)
	}
	if ok {
		t.Fatalf("LatestTaggedBackupPath() ok = true, want false with path %q", got)
	}
	if got != "" {
		t.Fatalf("LatestTaggedBackupPath() path = %q, want empty", got)
	}
}

func TestContextRewriteTextRecordsPreservingBackupCreatesHistoryWhenMissing(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	ctx := New(historyFile)
	want := []TextRecord{
		NewUserTextRecord("current goal"),
		NewAssistantTextRecord("working summary"),
	}

	if err := ctx.RewriteTextRecordsPreservingBackup(want); err != nil {
		t.Fatalf("RewriteTextRecordsPreservingBackup() error = %v", err)
	}

	got, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadAll() after initial preserved rewrite = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(historyFile + ".1"); !os.IsNotExist(err) {
		t.Fatalf("backup file exists unexpectedly, stat err = %v", err)
	}
}
