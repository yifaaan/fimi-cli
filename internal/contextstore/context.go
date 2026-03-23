package contextstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// TextRecord 是当前最小可持久化的历史记录模型。
// 先只支持纯文本内容，后面再扩展多种消息 part。
type TextRecord struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Snapshot 表示某个 history 文件当前的读取结果摘要。
type Snapshot struct {
	Count         int
	LastRecord    TextRecord
	HasLastRecord bool
}

// Context 管理某个 history file 的追加写入。
type Context struct {
	historyFile string
}

// New 为给定 history file 创建一个最小上下文存储。
func New(historyFile string) Context {
	return Context{historyFile: historyFile}
}

// NewSystemTextRecord 为系统消息创建最小文本记录。
func NewSystemTextRecord(content string) TextRecord {
	return TextRecord{
		Role:    RoleSystem,
		Content: content,
	}
}

// NewUserTextRecord 为用户消息创建最小文本记录。
func NewUserTextRecord(content string) TextRecord {
	return TextRecord{
		Role:    RoleUser,
		Content: content,
	}
}

// NewAssistantTextRecord 为 assistant 消息创建最小文本记录。
func NewAssistantTextRecord(content string) TextRecord {
	return TextRecord{
		Role:    RoleAssistant,
		Content: content,
	}
}

// Path 返回当前上下文绑定的 history file 路径。
func (c Context) Path() string {
	return c.historyFile
}

// Exists 返回当前上下文绑定的 history file 是否存在。
func (c Context) Exists() (bool, error) {
	_, err := os.Stat(c.historyFile)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat history file %q: %w", c.historyFile, err)
}

// Append 以 JSONL 形式向 history file 追加一条记录。
func (c Context) Append(record TextRecord) error {
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal text record: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(c.historyFile), 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	f, err := os.OpenFile(c.historyFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append history file %q: %w", c.historyFile, err)
	}

	return nil
}

// AppendText 是追加文本记录的便捷方法。
func (c Context) AppendText(role, content string) error {
	return c.Append(TextRecord{
		Role:    role,
		Content: content,
	})
}

// ReadAll 读取 history file 中的全部文本记录。
// 如果文件还不存在，返回空结果而不是报错。
func (c Context) ReadAll() ([]TextRecord, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return []TextRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	records := make([]TextRecord, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record TextRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("decode history line in %q: %w", c.historyFile, err)
		}
		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file %q: %w", c.historyFile, err)
	}

	return records, nil
}

// Last 返回最后一条文本记录。
// bool 为 false 表示当前 history 里还没有任何记录。
func (c Context) Last() (TextRecord, bool, error) {
	records, err := c.ReadAll()
	if err != nil {
		return TextRecord{}, false, err
	}

	if len(records) == 0 {
		return TextRecord{}, false, nil
	}

	return records[len(records)-1], true, nil
}

// Count 返回当前 history 中的记录总数。
func (c Context) Count() (int, error) {
	records, err := c.ReadAll()
	if err != nil {
		return 0, err
	}

	return len(records), nil
}

// Snapshot 返回 history 的数量和最后一条记录。
func (c Context) Snapshot() (Snapshot, error) {
	records, err := c.ReadAll()
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Count: len(records),
	}
	if len(records) == 0 {
		return snapshot, nil
	}

	snapshot.LastRecord = records[len(records)-1]
	snapshot.HasLastRecord = true

	return snapshot, nil
}
