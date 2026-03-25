package session

import (
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
