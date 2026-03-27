package contextstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleUsage     = "_usage"      // token 使用量记录
	RoleCheckpoint = "_checkpoint" // 检查点记录
)

// TextRecord 是当前最小可持久化的历史记录模型。
// 先只支持纯文本内容，后面再扩展多种消息 part。
// ToolCallID 只在 role=tool 时有意义，用于关联工具调用结果。
// ToolCallsJSON 只在 role=assistant 时有意义，存储序列化后的工具调用列表。
type TextRecord struct {
	Role          string `json:"role"`
	Content       string `json:"content"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	ToolCallsJSON string `json:"tool_calls,omitempty"`
}

// UsageRecord 记录 token 使用量。
type UsageRecord struct {
	Role       string `json:"role"`
	TokenCount int    `json:"token_count"`
}

// CheckpointRecord 记录检查点。
type CheckpointRecord struct {
	Role          string `json:"role"`
	ID            int    `json:"id"`
	CreatedAt     string `json:"created_at,omitempty"`
	PromptPreview string `json:"prompt_preview,omitempty"`
}

// CheckpointMetadata 表示创建 checkpoint 时附带的可展示元数据。
type CheckpointMetadata struct {
	CreatedAt     string
	PromptPreview string
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

// RewriteTextRecords 用新的文本记录集合覆盖当前 history。
// 这里不会保留 usage / checkpoint 等元数据；它的语义就是重建对话文本历史。
func (c Context) RewriteTextRecords(records []TextRecord) error {
	if err := os.MkdirAll(filepath.Dir(c.historyFile), 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	var data bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal text record: %w", err)
		}
		data.Write(line)
		data.WriteByte('\n')
	}

	if err := os.WriteFile(c.historyFile, data.Bytes(), 0o644); err != nil {
		return fmt.Errorf("rewrite history file %q: %w", c.historyFile, err)
	}

	return nil
}

// RewriteTextRecordsPreservingBackup 用新的文本记录集合覆盖当前 history。
// 如果当前 history 已存在，会先轮转为通用备份文件，再写入新的文本历史。
func (c Context) RewriteTextRecordsPreservingBackup(records []TextRecord) error {
	return c.RewriteTextRecordsPreservingNamedBackup(records, "")
}

// RewriteTextRecordsPreservingNamedBackup 用新的文本记录集合覆盖当前 history。
// 如果当前 history 已存在，会先按给定标签轮转为备份文件，再写入新的文本历史。
func (c Context) RewriteTextRecordsPreservingNamedBackup(records []TextRecord, backupTag string) error {
	exists, err := c.Exists()
	if err != nil {
		return fmt.Errorf("check history exists: %w", err)
	}
	if exists {
		rotatedPath, err := c.findTaggedRotationPath(backupTag)
		if err != nil {
			return fmt.Errorf("find rotation path: %w", err)
		}
		if err := os.Rename(c.historyFile, rotatedPath); err != nil {
			return fmt.Errorf("rotate history file %q: %w", c.historyFile, err)
		}
	}
	if err := c.RewriteTextRecords(records); err != nil {
		return fmt.Errorf("rewrite text records preserving backup: %w", err)
	}
	return nil
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

		// 跳过元数据记录（_usage, _checkpoint），避免把存储层标记暴露给对话窗口。
		if record.Role == RoleUsage || record.Role == RoleCheckpoint {
			continue
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

// ReadFirstUserRecord 返回 history 中第一条 user 记录（用于快速预览）。
// 如果不存在 user 记录，返回 (TextRecord{}, false, nil)。
func (c Context) ReadFirstUserRecord() (TextRecord, bool, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return TextRecord{}, false, nil
	}
	if err != nil {
		return TextRecord{}, false, fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record TextRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return TextRecord{}, false, fmt.Errorf("decode history line in %q: %w", c.historyFile, err)
		}

		if record.Role == RoleUser {
			return record, true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return TextRecord{}, false, fmt.Errorf("scan history file %q: %w", c.historyFile, err)
	}

	return TextRecord{}, false, nil
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

		// 跳过元数据记录（_usage, _checkpoint）
		if record.Role == RoleUsage || record.Role == RoleCheckpoint {
			continue
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

// AppendUsage 追加 token 使用量记录。
func (c Context) AppendUsage(tokenCount int) error {
	record := UsageRecord{
		Role:       RoleUsage,
		TokenCount: tokenCount,
	}

	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal usage record: %w", err)
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
		return fmt.Errorf("append usage record: %w", err)
	}

	return nil
}

// AppendCheckpoint 追加检查点记录，返回检查点 ID。
func (c Context) AppendCheckpoint() (int, error) {
	return c.AppendCheckpointWithMetadata(CheckpointMetadata{})
}

// AppendCheckpointWithMetadata 追加带元数据的检查点记录，返回检查点 ID。
func (c Context) AppendCheckpointWithMetadata(metadata CheckpointMetadata) (int, error) {
	// 计算下一个检查点 ID
	nextID, err := c.nextCheckpointID()
	if err != nil {
		return 0, fmt.Errorf("get next checkpoint id: %w", err)
	}

	record := CheckpointRecord{
		Role:          RoleCheckpoint,
		ID:            nextID,
		CreatedAt:     strings.TrimSpace(metadata.CreatedAt),
		PromptPreview: strings.TrimSpace(metadata.PromptPreview),
	}

	line, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("marshal checkpoint record: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(c.historyFile), 0o755); err != nil {
		return 0, fmt.Errorf("create history dir: %w", err)
	}

	f, err := os.OpenFile(c.historyFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open history file %q: %w", c.historyFile, err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return 0, fmt.Errorf("append checkpoint record: %w", err)
	}

	return nextID, nil
}

// nextCheckpointID 返回下一个检查点 ID。
func (c Context) nextCheckpointID() (int, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	maxID := -1
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// 尝试解析为 checkpoint 记录
		var record CheckpointRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Role == RoleCheckpoint && record.ID > maxID {
			maxID = record.ID
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan history file: %w", err)
	}

	return maxID + 1, nil
}

// ListCheckpoints 返回当前 history 中的全部检查点记录。
func (c Context) ListCheckpoints() ([]CheckpointRecord, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return []CheckpointRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	checkpoints := []CheckpointRecord{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record CheckpointRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Role != RoleCheckpoint {
			continue
		}
		checkpoints = append(checkpoints, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file: %w", err)
	}
	return checkpoints, nil
}

// ReadUsage 读取最后的 token 使用量。
func (c Context) ReadUsage() (int, error) {
	f, err := os.Open(c.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var lastUsage int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record UsageRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Role == RoleUsage {
			lastUsage = record.TokenCount
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan history file: %w", err)
	}

	return lastUsage, nil
}

// CheckpointCount 返回当前已有的检查点数量。
func (c Context) CheckpointCount() (int, error) {
	nextID, err := c.nextCheckpointID()
	if err != nil {
		return 0, err
	}
	return nextID, nil
}

// RevertToCheckpoint 回滚到指定检查点。
// 该方法会：
// 1. 将当前 history 文件重命名为备份文件
// 2. 从备份文件读取到指定检查点为止的内容，写入新 history 文件
// 3. 返回回滚后恢复的历史记录数量
func (c Context) RevertToCheckpoint(checkpointID int) (int, error) {
	// 验证检查点存在
	count, err := c.CheckpointCount()
	if err != nil {
		return 0, fmt.Errorf("get checkpoint count: %w", err)
	}
	if checkpointID >= count {
		return 0, fmt.Errorf("checkpoint %d does not exist (max: %d)", checkpointID, count-1)
	}

	// 查找可用的备份文件名
	rotatedPath, err := c.findRotationPath()
	if err != nil {
		return 0, fmt.Errorf("find rotation path: %w", err)
	}

	// 重命名当前文件为备份文件
	if err := os.Rename(c.historyFile, rotatedPath); err != nil {
		return 0, fmt.Errorf("rotate history file: %w", err)
	}

	// 从备份文件读取到指定检查点，写入新文件
	recordCount, err := c.rebuildUntilCheckpoint(rotatedPath, checkpointID)
	if err != nil {
		return 0, fmt.Errorf("rebuild history: %w", err)
	}

	return recordCount, nil
}

// findTaggedRotationPath 找到下一个可用的带标签备份文件名。
func (c Context) findTaggedRotationPath(tag string) (string, error) {
	base := c.historyFile
	if tag != "" {
		base = fmt.Sprintf("%s.%s", base, tag)
	}
	for i := 1; i <= 1000; i++ {
		candidate := fmt.Sprintf("%s.%d", base, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available rotation path found")
}

// findRotationPath 找到下一个可用的备份文件名。
func (c Context) findRotationPath() (string, error) {
	return c.findTaggedRotationPath("")
}

// LatestTaggedBackupPath 返回指定标签的最新备份文件路径。
// 如果不存在匹配备份，返回 ("", false, nil)。
func (c Context) LatestTaggedBackupPath(tag string) (string, bool, error) {
	if strings.TrimSpace(tag) == "" {
		return "", false, nil
	}

	pattern := fmt.Sprintf("%s.%s.*", filepath.Base(c.historyFile), tag)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(c.historyFile), pattern))
	if err != nil {
		return "", false, fmt.Errorf("glob tagged backups: %w", err)
	}

	latestPath := ""
	latestIndex := -1
	prefix := fmt.Sprintf("%s.%s.", c.historyFile, tag)
	for _, match := range matches {
		if !strings.HasPrefix(match, prefix) {
			continue
		}
		indexText := strings.TrimPrefix(match, prefix)
		index, err := strconv.Atoi(indexText)
		if err != nil {
			continue
		}
		if index > latestIndex {
			latestIndex = index
			latestPath = match
		}
	}
	if latestIndex == -1 {
		return "", false, nil
	}
	return latestPath, true, nil
}

// rebuildUntilCheckpoint 从备份文件重建 history，直到指定检查点。
func (c Context) rebuildUntilCheckpoint(rotatedPath string, targetCheckpointID int) (int, error) {
	oldFile, err := os.Open(rotatedPath)
	if err != nil {
		return 0, fmt.Errorf("open rotated file: %w", err)
	}
	defer oldFile.Close()

	newFile, err := os.Create(c.historyFile)
	if err != nil {
		return 0, fmt.Errorf("create new history file: %w", err)
	}
	defer newFile.Close()

	recordCount := 0
	scanner := bufio.NewScanner(oldFile)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// 解析行以判断是否为目标检查点
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		// 如果遇到目标检查点，停止复制
		if role, ok := raw["role"].(string); ok && role == RoleCheckpoint {
			id, _ := raw["id"].(float64)
			if int(id) == targetCheckpointID {
				break
			}
		}

		// 写入新文件
		if _, err := newFile.Write(append(line, '\n')); err != nil {
			return 0, fmt.Errorf("write to new history: %w", err)
		}

		// 统计非元数据记录
		if role, ok := raw["role"].(string); ok && role != RoleUsage && role != RoleCheckpoint {
			recordCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan rotated file: %w", err)
	}

	return recordCount, nil
}
