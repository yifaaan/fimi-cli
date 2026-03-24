package session

import (
	"path/filepath"
	"testing"
)

func TestSetLastSessionIDCreatesMetadataEntry(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")

	if err := setLastSessionID(workDir, "session-123"); err != nil {
		t.Fatalf("setLastSessionID() error = %v", err)
	}

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error = %v", err)
	}

	if len(meta.WorkDirs) != 1 {
		t.Fatalf("len(meta.WorkDirs) = %d, want 1", len(meta.WorkDirs))
	}
	if meta.WorkDirs[0].Path != workDir {
		t.Fatalf("meta.WorkDirs[0].Path = %q, want %q", meta.WorkDirs[0].Path, workDir)
	}
	if meta.WorkDirs[0].LastSessionID != "session-123" {
		t.Fatalf("meta.WorkDirs[0].LastSessionID = %q, want %q", meta.WorkDirs[0].LastSessionID, "session-123")
	}
}

func TestSetLastSessionIDUpdatesExistingEntry(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")

	if err := setLastSessionID(workDir, "session-1"); err != nil {
		t.Fatalf("setLastSessionID() first error = %v", err)
	}
	if err := setLastSessionID(workDir, "session-2"); err != nil {
		t.Fatalf("setLastSessionID() second error = %v", err)
	}

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error = %v", err)
	}

	if len(meta.WorkDirs) != 1 {
		t.Fatalf("len(meta.WorkDirs) = %d, want 1", len(meta.WorkDirs))
	}
	if meta.WorkDirs[0].LastSessionID != "session-2" {
		t.Fatalf("meta.WorkDirs[0].LastSessionID = %q, want %q", meta.WorkDirs[0].LastSessionID, "session-2")
	}
}
