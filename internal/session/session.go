package session

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppStateDirName    = "fimi"
	SessionsDirName    = "sessions"
	HistoryFileExtName = ".jsonl"
)

// Session 表示某个工作目录上的一次 agent 会话。
type Session struct {
	ID          string
	WorkDir     string
	HistoryFile string
}

// Dir 返回会话状态根目录。
func Dir() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, AppStateDirName), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}

	return filepath.Join(homeDir, ".local", "state", AppStateDirName), nil
}

// New 为工作目录创建一个新的 session。
func New(workDir string) (Session, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return Session{}, fmt.Errorf("resolve work dir %q: %w", workDir, err)
	}

	sessionRoot, err := Dir()
	if err != nil {
		return Session{}, err
	}

	workDirHash := md5.Sum([]byte(absWorkDir))
	workDirSessionsDir := filepath.Join(sessionRoot, SessionsDirName, hex.EncodeToString(workDirHash[:]))
	if err := os.MkdirAll(workDirSessionsDir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions dir %q: %w", workDirSessionsDir, err)
	}

	sessionID, err := newID()
	if err != nil {
		return Session{}, err
	}

	historyFile := filepath.Join(workDirSessionsDir, sessionID+HistoryFileExtName)

	return Session{
		ID:          sessionID,
		WorkDir:     absWorkDir,
		HistoryFile: historyFile,
	}, nil
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return hex.EncodeToString(buf[:]), nil
}
