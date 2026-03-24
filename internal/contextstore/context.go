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
	RoleTool      = "tool"
)

// TextRecord 是当前最小可持久化的历史记录模型。
// 先只支持纯文本内容，后面再扩展多种消息 part。
// ToolCallID 只在 role=tool 时有意义，用于关联工具调用结果。
// ToolCallsJSON 只在 role=assistant 时有意义，存储序列化后的工具调用列表。
type TextRecord struct {
	Role         string `json:"role"`
	Content      string `json:"content"`
	ToolCallID   string `json:"tool_call_id,omitempty"`
	ToolCallsJSON string `json:"tool_calls,omitempty"`
}

// Snapshot 表示某个 history 文件当前的读取结果摘要。
type Snapshot struct {
	Count         int
	LastRecord    TextRecord
	HasLastRecord bool
}

// BootstrapResult 表示初始化 history 后的结果。
type BootstrapResult struct {
	HistoryExists bool
	HistorySeeded bool
	Snapshot      Snapshot
}

// Context 管理某个 history file 的追加写入。
type Context struct {
	historyFile string
}

const readAllRecords = -1

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

// NewToolResultRecord 为工具调用结果创建记录。
// toolCallID 用于关联回之前的工具调用。
func NewToolResultRecord(toolCallID, content string) TextRecord {
	return TextRecord{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Content:    content,
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
	return c.readRecords(readAllRecords)
}

// ReadRecent 读取最近若干条文本记录。
// 这里仍然顺序扫描整个 JSONL 文件，但把内存占用限制在 limit 内。
func (c Context) ReadRecent(limit int) ([]TextRecord, error) {
	if limit <= 0 {
		return []TextRecord{}, nil
	}

	return c.readRecords(limit)
}

// ReadRecentTurns 按 user 轮次读取最近若干段对话历史。
// 返回结果会从最近窗口里的第一条 user 记录开始，避免以孤立 assistant 开头。
func (c Context) ReadRecentTurns(limit int) ([]TextRecord, error) {
	if limit <= 0 {
		return []TextRecord{}, nil
	}

	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return []TextRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	records := make([]TextRecord, 0, limit*2)
	userCount := 0

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
		if record.Role == RoleUser {
			userCount++
		}

		records = dropLeadingNonUserRecords(records)
		for userCount > limit && len(records) > 0 {
			if records[0].Role == RoleUser {
				userCount--
			}
			records = records[1:]
			records = dropLeadingNonUserRecords(records)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file %q: %w", c.historyFile, err)
	}

	return records, nil
}

func (c Context) readRecords(limit int) ([]TextRecord, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return []TextRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	records := make([]TextRecord, 0)
	if limit > 0 {
		records = make([]TextRecord, 0, limit)
	}

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
		if limit == readAllRecords {
			records = append(records, record)
			continue
		}

		// 只保留尾部窗口，避免随着 history 增长而无限累积内存。
		if len(records) < limit {
			records = append(records, record)
			continue
		}

		records = append(records[1:], record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file %q: %w", c.historyFile, err)
	}

	return records, nil
}

func dropLeadingNonUserRecords(records []TextRecord) []TextRecord {
	for len(records) > 0 && records[0].Role != RoleUser {
		records = records[1:]
	}

	return records
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

// Bootstrap 确保 history 至少包含一条初始记录。
// 如果 history 为空，就写入 initialRecord，并返回初始化后的摘要。
func (c Context) Bootstrap(initialRecord TextRecord) (BootstrapResult, error) {
	historyExists, err := c.Exists()
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("check history file existence: %w", err)
	}

	snapshot, err := c.Snapshot()
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("read history snapshot before bootstrap: %w", err)
	}

	result := BootstrapResult{
		HistoryExists: historyExists,
		HistorySeeded: false,
		Snapshot:      snapshot,
	}
	if snapshot.Count != 0 {
		return result, nil
	}

	if err := c.Append(initialRecord); err != nil {
		return BootstrapResult{}, fmt.Errorf("append initial history record: %w", err)
	}

	return BootstrapResult{
		HistoryExists: true,
		HistorySeeded: true,
		Snapshot: Snapshot{
			Count:         1,
			LastRecord:    initialRecord,
			HasLastRecord: true,
		},
	}, nil
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
