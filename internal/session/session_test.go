package session

import (
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
