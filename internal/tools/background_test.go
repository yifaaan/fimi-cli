package tools

import (
	"strings"
	"testing"
	"time"
)

func TestBackgroundManagerStartReturnsTaskID(t *testing.T) {
	mgr := NewBackgroundManager()
	id, err := mgr.Start("echo hello", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if id == "" {
		t.Fatalf("Start() returned empty task ID")
	}
	if !strings.HasPrefix(id, "bg-") {
		t.Fatalf("Start() ID = %q, want prefix %q", id, "bg-")
	}

	mgr.Close()
}

func TestBackgroundManagerStatusTracksRunningTask(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("sleep 10", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.ID != id {
		t.Fatalf("Status().ID = %q, want %q", result.ID, id)
	}
	if result.Status != BGStatusRunning {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusRunning)
	}
	if result.Command != "sleep 10" {
		t.Fatalf("Status().Command = %q, want %q", result.Command, "sleep 10")
	}
}

func TestBackgroundManagerStatusReturnsCompletedTask(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("printf 'output data'", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 等待进程结束
	time.Sleep(500 * time.Millisecond)

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Status != BGStatusDone {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusDone)
	}
	if !strings.Contains(result.Stdout, "output data") {
		t.Fatalf("Status().Stdout = %q, want to contain %q", result.Stdout, "output data")
	}
	if result.ExitCode != 0 {
		t.Fatalf("Status().ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestBackgroundManagerStatusReturnsFailedTask(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("exit 42", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 等待进程结束
	time.Sleep(500 * time.Millisecond)

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Status != BGStatusFailed {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusFailed)
	}
	if result.ExitCode != 42 {
		t.Fatalf("Status().ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestBackgroundManagerStatusReturnsTimedOutTask(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("sleep 10", t.TempDir(), 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 等待超时触发
	time.Sleep(200 * time.Millisecond)

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Status != BGStatusTimedOut {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusTimedOut)
	}
}

func TestBackgroundManagerStatusRejectsUnknownTaskID(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	_, err := mgr.Status("bg-999")
	if err == nil {
		t.Fatalf("Status() error = nil, want error for unknown task ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Status() error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestBackgroundManagerKillTerminatesRunningTask(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("sleep 30", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	err = mgr.Kill(id)
	if err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Status != BGStatusKilled {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusKilled)
	}
}

func TestBackgroundManagerKillPreservesKilledStatusAfterProcessExit(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id, err := mgr.Start("sleep 30", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := mgr.Kill(id); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	result, err := mgr.Status(id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Status != BGStatusKilled {
		t.Fatalf("Status().Status = %q, want %q", result.Status, BGStatusKilled)
	}
}

func TestBackgroundManagerCloseTerminatesAllRunningTasks(t *testing.T) {
	mgr := NewBackgroundManager()

	id1, _ := mgr.Start("sleep 30", t.TempDir(), 0)
	id2, _ := mgr.Start("sleep 30", t.TempDir(), 0)

	mgr.Close()

	for _, id := range []string{id1, id2} {
		result, err := mgr.Status(id)
		if err != nil {
			t.Fatalf("Status(%q) error = %v", id, err)
		}
		if result.Status != BGStatusKilled {
			t.Fatalf("Status(%q).Status = %q, want %q", id, result.Status, BGStatusKilled)
		}
	}
}

func TestBackgroundManagerSequentialIDs(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id1, _ := mgr.Start("true", t.TempDir(), 0)
	id2, _ := mgr.Start("true", t.TempDir(), 0)

	if id1 == id2 {
		t.Fatalf("consecutive task IDs should differ: %q == %q", id1, id2)
	}
	if id1 != "bg-1" {
		t.Fatalf("first ID = %q, want %q", id1, "bg-1")
	}
	if id2 != "bg-2" {
		t.Fatalf("second ID = %q, want %q", id2, "bg-2")
	}
}

func TestBackgroundManagerListReturnsNewestTasksFirst(t *testing.T) {
	mgr := NewBackgroundManager()
	defer mgr.Close()

	id1, err := mgr.Start("sleep 30", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	id2, err := mgr.Start("sleep 30", t.TempDir(), 0)
	if err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}

	got := mgr.List()
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].ID != id2 {
		t.Fatalf("List()[0].ID = %q, want %q", got[0].ID, id2)
	}
	if got[1].ID != id1 {
		t.Fatalf("List()[1].ID = %q, want %q", got[1].ID, id1)
	}
}
