package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellHistoryFileForWorkDirUsesWorkspaceSessionDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")
	got, err := ShellHistoryFileForWorkDir(workDir)
	if err != nil {
		t.Fatalf("ShellHistoryFileForWorkDir() error = %v", err)
	}

	_, sessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		t.Fatalf("DirForWorkDir() error = %v", err)
	}

	want := filepath.Join(sessionsDir, ShellHistoryFileName)
	if got != want {
		t.Fatalf("ShellHistoryFileForWorkDir() = %q, want %q", got, want)
	}
}

func TestListSessionsReturnsEmptyWhenNoDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")
	got, err := ListSessions(workDir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListSessions() len = %d, want 0", len(got))
	}
}

func TestListSessionsFindsJSONLHistoryFiles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")
	absWorkDir, sessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		t.Fatalf("DirForWorkDir() error = %v", err)
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// 两个 session history
	if err := os.WriteFile(filepath.Join(sessionsDir, "a"+HistoryFileExtName), []byte("{\"role\":\"user\",\"content\":\"hi\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "b"+HistoryFileExtName), []byte("{\"role\":\"assistant\",\"content\":\"ok\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// shell 输入历史，不应该算 session
	if err := os.WriteFile(filepath.Join(sessionsDir, ShellHistoryFileName), []byte("/help\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ListSessions(workDir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListSessions() len = %d, want 2", len(got))
	}
	for _, s := range got {
		if s.WorkDir != absWorkDir {
			t.Fatalf("ListSessions() WorkDir = %q, want %q", s.WorkDir, absWorkDir)
		}
		if s.HistoryFile == "" {
			t.Fatalf("ListSessions() HistoryFile is empty")
		}
		if s.ID == "" {
			t.Fatalf("ListSessions() ID is empty")
		}
	}
}

func TestLoadSessionErrorsOnMissingID(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")
	if _, err := LoadSession(workDir, ""); err == nil {
		t.Fatalf("LoadSession() expected error")
	}
}

func TestLoadSessionLoadsExistingHistoryFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	workDir := filepath.Join(t.TempDir(), "repo")
	absWorkDir, sessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		t.Fatalf("DirForWorkDir() error = %v", err)
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	sessionID := "abc123"
	historyFile := HistoryFileForSession(sessionsDir, sessionID)
	if err := os.WriteFile(historyFile, []byte("{\"role\":\"user\",\"content\":\"hi\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sess, err := LoadSession(workDir, sessionID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if sess.ID != sessionID {
		t.Fatalf("LoadSession() ID = %q, want %q", sess.ID, sessionID)
	}
	if sess.WorkDir != absWorkDir {
		t.Fatalf("LoadSession() WorkDir = %q, want %q", sess.WorkDir, absWorkDir)
	}
	if sess.HistoryFile != historyFile {
		t.Fatalf("LoadSession() HistoryFile = %q, want %q", sess.HistoryFile, historyFile)
	}
}
