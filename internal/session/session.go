package session

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// HistoryExists 返回当前 session 的 history file 是否存在。
func (s Session) HistoryExists() (bool, error) {
	_, err := os.Stat(s.HistoryFile)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat history file %q: %w", s.HistoryFile, err)
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

// DirForWorkDir 返回某个工作目录对应的 sessions 子目录。
// 同时返回归一化后的绝对工作目录，避免调用方重复解析。
func DirForWorkDir(workDir string) (string, string, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve work dir %q: %w", workDir, err)
	}

	sessionRoot, err := Dir()
	if err != nil {
		return "", "", err
	}

	workDirHash := md5.Sum([]byte(absWorkDir))
	workDirSessionsDir := filepath.Join(
		sessionRoot,
		SessionsDirName,
		hex.EncodeToString(workDirHash[:]),
	)

	return absWorkDir, workDirSessionsDir, nil
}

// HistoryFileForSession 返回某个 session 对应的 history file 路径。
func HistoryFileForSession(sessionsDir, sessionID string) string {
	return filepath.Join(sessionsDir, sessionID+HistoryFileExtName)
}

// New 为工作目录创建一个新的 session。
func New(workDir string) (Session, error) {
	absWorkDir, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return Session{}, err
	}

	if err := os.MkdirAll(workDirSessionsDir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions dir %q: %w", workDirSessionsDir, err)
	}

	sessionID, err := newID()
	if err != nil {
		return Session{}, err
	}

	historyFile := HistoryFileForSession(workDirSessionsDir, sessionID)

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
