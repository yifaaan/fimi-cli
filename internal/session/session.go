package session

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	AppStateDirName      = "fimi"
	SessionsDirName      = "sessions"
	HistoryFileExtName   = ".jsonl"
	ShellHistoryFileName = "shell_history.txt"
)

var ErrNoPreviousSession = errors.New("no previous session")

// SessionInfo 表示一个 session 的基本信息，用于列出和选择 session。
type SessionInfo struct {
	ID           string
	WorkDir      string
	HistoryFile  string
	MessageCount int
	LastModified time.Time
	FileSize     int64 // 历史文件大小（字节）
}

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

// ShellHistoryFileForWorkDir 返回某个工作目录对应的 shell 输入历史文件路径。
// 它按工作目录维度存储，而不是按 session 维度存储，这样同一仓库下的 shell 交互历史可以复用。
func ShellHistoryFileForWorkDir(workDir string) (string, error) {
	_, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(workDirSessionsDir, ShellHistoryFileName), nil
}

// New 为工作目录创建一个新的 session。
func New(workDir string) (Session, error) {
	absWorkDir, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return Session{}, err
	}

	return newSession(absWorkDir, workDirSessionsDir)
}

// Continue 按 metadata 中记录的 last_session_id 恢复工作目录对应的 session。
// 这里故意不再根据 history 文件修改时间猜测“最近会话”，避免后续被子 session 干扰。
func Continue(workDir string) (Session, error) {
	absWorkDir, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return Session{}, err
	}

	lastSessionID, err := lastSessionIDForWorkDir(absWorkDir)
	if err != nil {
		return Session{}, err
	}
	if lastSessionID == "" {
		return Session{}, fmt.Errorf("%w for work dir %q", ErrNoPreviousSession, absWorkDir)
	}

	return Session{
		ID:          lastSessionID,
		WorkDir:     absWorkDir,
		HistoryFile: HistoryFileForSession(workDirSessionsDir, lastSessionID),
	}, nil
}

func newSession(absWorkDir, workDirSessionsDir string) (Session, error) {
	if err := os.MkdirAll(workDirSessionsDir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions dir %q: %w", workDirSessionsDir, err)
	}

	sessionID, err := newID()
	if err != nil {
		return Session{}, err
	}

	historyFile := HistoryFileForSession(workDirSessionsDir, sessionID)

	sess := Session{
		ID:          sessionID,
		WorkDir:     absWorkDir,
		HistoryFile: historyFile,
	}

	// 先在 metadata 中记录最后一次显式创建的新 session，
	// 后续 continue 语义将基于这个索引，而不是 history 文件 mtime。
	if err := setLastSessionID(absWorkDir, sessionID); err != nil {
		return Session{}, fmt.Errorf("persist session metadata: %w", err)
	}

	return sess, nil
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return hex.EncodeToString(buf[:]), nil
}

// ListSessions 列出指定工作目录的所有 session。
// 返回按最后修改时间倒序排列的 session 列表（最新的在前）。
func ListSessions(workDir string) ([]SessionInfo, error) {
	absWorkDir, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return nil, err
	}

	// 如果目录不存在，返回空列表
	if _, err := os.Stat(workDirSessionsDir); os.IsNotExist(err) {
		return []SessionInfo{}, nil
	}

	entries, err := os.ReadDir(workDirSessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions directory %q: %w", workDirSessionsDir, err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 只处理 .jsonl 文件，且排除 shell_history.txt
		if !strings.HasSuffix(name, HistoryFileExtName) || name == ShellHistoryFileName {
			continue
		}

		// 从文件名提取 session ID（去掉 .jsonl 后缀）
		sessionID := strings.TrimSuffix(name, HistoryFileExtName)
		if sessionID == "" {
			continue
		}

		historyFile := filepath.Join(workDirSessionsDir, name)
		info, err := entry.Info()
		if err != nil {
			continue // 跳过无法获取信息的文件
		}

		// 统计消息数量
		messageCount := countMessagesInHistoryFile(historyFile)

		sessions = append(sessions, SessionInfo{
			ID:           sessionID,
			WorkDir:      absWorkDir,
			HistoryFile:  historyFile,
			MessageCount: messageCount,
			LastModified: info.ModTime(),
			FileSize:     info.Size(),
		})
	}

	// 按最后修改时间倒序排列
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	return sessions, nil
}

// countMessagesInHistoryFile 统计历史文件中的消息记录数量。
// 只统计 role 不是特殊内部角色的记录。
func countMessagesInHistoryFile(historyFile string) int {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 简单检查：如果不是内部角色标记，计为消息
		// 实际格式是 JSON，这里简化处理，只检查是否包含 role 字段
		if strings.Contains(line, `"role"`) {
			// 排除内部角色
			if !strings.Contains(line, `"_usage"`) &&
				!strings.Contains(line, `"_checkpoint"`) &&
				!strings.Contains(line, `"_system"`) {
				count++
			}
		}
	}

	return count
}

// LoadSession 加载指定 ID 的 session。
// 不依赖 metadata 中的 last_session_id，直接根据 sessionID 构造 Session。
func LoadSession(workDir, sessionID string) (Session, error) {
	if sessionID == "" {
		return Session{}, errors.New("session ID is required")
	}

	absWorkDir, workDirSessionsDir, err := DirForWorkDir(workDir)
	if err != nil {
		return Session{}, err
	}

	historyFile := HistoryFileForSession(workDirSessionsDir, sessionID)

	// 验证文件是否存在
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		return Session{}, fmt.Errorf("session %q not found: %w", sessionID, ErrNoPreviousSession)
	}

	return Session{
		ID:          sessionID,
		WorkDir:     absWorkDir,
		HistoryFile: historyFile,
	}, nil
}
