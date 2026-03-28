package acp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fimi-cli/internal/config"
	"fimi-cli/internal/session"
)

type recordedNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func newTestServer(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	var out bytes.Buffer
	conn := NewFramedConn(bytes.NewReader(nil), &out)
	server := NewServer(conn, config.Default(), nil)
	return server, &out
}

func createPersistedSession(t *testing.T, workDir string) session.Session {
	t.Helper()

	sess, err := session.New(workDir)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}
	if err := os.WriteFile(sess.HistoryFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(history) error = %v", err)
	}

	return sess
}

func TestHandleNewSessionRegistersSessionAndSendsNotification(t *testing.T) {
	server, out := newTestServer(t)
	workDir := t.TempDir()

	params, err := json.Marshal(NewSessionParams{CWD: workDir})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handleNewSession("1", params)
	if err != nil {
		t.Fatalf("handleNewSession() error = %v", err)
	}

	result, ok := got.(NewSessionResult)
	if !ok {
		t.Fatalf("handleNewSession() result type = %T, want NewSessionResult", got)
	}
	if result.SessionID == "" {
		t.Fatalf("handleNewSession().SessionID = empty, want non-empty")
	}
	if result.Modes.CurrentModeID != "default" {
		t.Fatalf("handleNewSession().Modes.CurrentModeID = %q, want %q", result.Modes.CurrentModeID, "default")
	}
	if result.Models.CurrentModelID != config.Default().DefaultModel {
		t.Fatalf("handleNewSession().Models.CurrentModelID = %q, want %q", result.Models.CurrentModelID, config.Default().DefaultModel)
	}
	if _, ok := server.sessions[result.SessionID]; !ok {
		t.Fatalf("server.sessions[%q] missing", result.SessionID)
	}

	var notification recordedNotification
	if err := json.Unmarshal(out.Bytes(), &notification); err != nil {
		t.Fatalf("json.Unmarshal(notification) error = %v", err)
	}
	if notification.Method != "session/update" {
		t.Fatalf("notification.Method = %q, want %q", notification.Method, "session/update")
	}

	var update SessionUpdateNotification
	if err := json.Unmarshal(notification.Params, &update); err != nil {
		t.Fatalf("json.Unmarshal(notification params) error = %v", err)
	}
	if update.SessionID != result.SessionID {
		t.Fatalf("notification.SessionID = %q, want %q", update.SessionID, result.SessionID)
	}
}

func TestHandleListSessionsUsesResolvedCWD(t *testing.T) {
	server, _ := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)

	params, err := json.Marshal(ListSessionsParams{CWD: workDir})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handleListSessions("1", params)
	if err != nil {
		t.Fatalf("handleListSessions() error = %v", err)
	}

	result, ok := got.(ListSessionsResult)
	if !ok {
		t.Fatalf("handleListSessions() result type = %T, want ListSessionsResult", got)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("len(handleListSessions().Sessions) = %d, want 1", len(result.Sessions))
	}
	if result.Sessions[0].SessionID != sess.ID {
		t.Fatalf("handleListSessions().Sessions[0].SessionID = %q, want %q", result.Sessions[0].SessionID, sess.ID)
	}

	wantCWD, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if result.Sessions[0].CWD != wantCWD {
		t.Fatalf("handleListSessions().Sessions[0].CWD = %q, want %q", result.Sessions[0].CWD, wantCWD)
	}
}

func TestHandleResumeSessionRegistersLoadedSession(t *testing.T) {
	server, _ := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)

	params, err := json.Marshal(ResumeSessionParams{CWD: workDir, SessionID: sess.ID})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handleResumeSession("1", params)
	if err != nil {
		t.Fatalf("handleResumeSession() error = %v", err)
	}

	result, ok := got.(ResumeSessionResult)
	if !ok {
		t.Fatalf("handleResumeSession() result type = %T, want ResumeSessionResult", got)
	}
	if result.Modes.CurrentModeID != "default" {
		t.Fatalf("handleResumeSession().Modes.CurrentModeID = %q, want %q", result.Modes.CurrentModeID, "default")
	}
	if result.Models.CurrentModelID != config.Default().DefaultModel {
		t.Fatalf("handleResumeSession().Models.CurrentModelID = %q, want %q", result.Models.CurrentModelID, config.Default().DefaultModel)
	}
	if _, ok := server.sessions[sess.ID]; !ok {
		t.Fatalf("server.sessions[%q] missing after resume", sess.ID)
	}
}

func TestHandleLoadSessionDelegatesToResumeSession(t *testing.T) {
	server, _ := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)

	params, err := json.Marshal(ResumeSessionParams{CWD: workDir, SessionID: sess.ID})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handleLoadSession("1", params)
	if err != nil {
		t.Fatalf("handleLoadSession() error = %v", err)
	}

	result, ok := got.(ResumeSessionResult)
	if !ok {
		t.Fatalf("handleLoadSession() result type = %T, want ResumeSessionResult", got)
	}
	if result.Models.CurrentModelID != config.Default().DefaultModel {
		t.Fatalf("handleLoadSession().Models.CurrentModelID = %q, want %q", result.Models.CurrentModelID, config.Default().DefaultModel)
	}
}
