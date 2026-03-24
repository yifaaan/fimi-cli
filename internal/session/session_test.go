package session

import (
	"errors"
	"testing"
)

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
