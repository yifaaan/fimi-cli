package session

import (
	"errors"
	"os"
	"testing"
)

func TestOpenLatestOrCreateReusesLatestSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := t.TempDir()

	first, reused, err := OpenLatestOrCreate(workDir)
	if err != nil {
		t.Fatalf("OpenLatestOrCreate() first call error = %v", err)
	}
	if reused {
		t.Fatalf("OpenLatestOrCreate() first call reused = true, want false")
	}

	// 模拟应用首次运行后已经把 history 写到了当前 session 文件。
	if err := os.WriteFile(first.HistoryFile, []byte("{\"role\":\"system\",\"content\":\"boot\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", first.HistoryFile, err)
	}

	second, reused, err := OpenLatestOrCreate(workDir)
	if err != nil {
		t.Fatalf("OpenLatestOrCreate() second call error = %v", err)
	}
	if !reused {
		t.Fatalf("OpenLatestOrCreate() second call reused = false, want true")
	}
	if second.ID != first.ID {
		t.Fatalf("second session ID = %q, want %q", second.ID, first.ID)
	}
	if second.HistoryFile != first.HistoryFile {
		t.Fatalf("second history file = %q, want %q", second.HistoryFile, first.HistoryFile)
	}
}

func TestNewPersistsLastSessionID(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := t.TempDir()

	sess, err := New(workDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error = %v", err)
	}

	if len(meta.WorkDirs) != 1 {
		t.Fatalf("len(meta.WorkDirs) = %d, want 1", len(meta.WorkDirs))
	}
	if meta.WorkDirs[0].Path != sess.WorkDir {
		t.Fatalf("meta.WorkDirs[0].Path = %q, want %q", meta.WorkDirs[0].Path, sess.WorkDir)
	}
	if meta.WorkDirs[0].LastSessionID != sess.ID {
		t.Fatalf("meta.WorkDirs[0].LastSessionID = %q, want %q", meta.WorkDirs[0].LastSessionID, sess.ID)
	}
}

func TestContinueReturnsLastSessionFromMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := t.TempDir()

	sess, err := New(workDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	continued, err := Continue(workDir)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}

	if continued.ID != sess.ID {
		t.Fatalf("continued.ID = %q, want %q", continued.ID, sess.ID)
	}
	if continued.WorkDir != sess.WorkDir {
		t.Fatalf("continued.WorkDir = %q, want %q", continued.WorkDir, sess.WorkDir)
	}
	if continued.HistoryFile != sess.HistoryFile {
		t.Fatalf("continued.HistoryFile = %q, want %q", continued.HistoryFile, sess.HistoryFile)
	}
}

func TestContinueReturnsErrorWhenNoPreviousSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := t.TempDir()

	_, err := Continue(workDir)
	if !errors.Is(err, ErrNoPreviousSession) {
		t.Fatalf("Continue() error = %v, want ErrNoPreviousSession", err)
	}
}
