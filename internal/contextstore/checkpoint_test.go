package contextstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendUsage(t *testing.T) {
	dir := t.TempDir()
	historyFile := filepath.Join(dir, "history.jsonl")
	ctx := New(historyFile)

	// 初始应该返回 0
	usage, err := ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage failed: %v", err)
	}
	if usage != 0 {
		t.Errorf("expected 0 usage, got %d", usage)
	}

	// 追加使用量
	if err := ctx.AppendUsage(100); err != nil {
		t.Fatalf("AppendUsage failed: %v", err)
	}

	usage, err = ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage failed: %v", err)
	}
	if usage != 100 {
		t.Errorf("expected 100 usage, got %d", usage)
	}

	// 再追加一次
	if err := ctx.AppendUsage(200); err != nil {
		t.Fatalf("AppendUsage failed: %v", err)
	}

	usage, err = ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage failed: %v", err)
	}
	if usage != 200 {
		t.Errorf("expected 200 usage (last value), got %d", usage)
	}
}

func TestAppendCheckpoint(t *testing.T) {
	dir := t.TempDir()
	historyFile := filepath.Join(dir, "history.jsonl")
	ctx := New(historyFile)

	// 第一个检查点应该是 ID 0
	id, err := ctx.AppendCheckpoint()
	if err != nil {
		t.Fatalf("AppendCheckpoint failed: %v", err)
	}
	if id != 0 {
		t.Errorf("expected checkpoint id 0, got %d", id)
	}

	// 第二个检查点应该是 ID 1
	id, err = ctx.AppendCheckpoint()
	if err != nil {
		t.Fatalf("AppendCheckpoint failed: %v", err)
	}
	if id != 1 {
		t.Errorf("expected checkpoint id 1, got %d", id)
	}

	// 检查点数量应该是 2
	count, err := ctx.CheckpointCount()
	if err != nil {
		t.Fatalf("CheckpointCount failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 checkpoints, got %d", count)
	}
}

func TestRevertToCheckpoint(t *testing.T) {
	dir := t.TempDir()
	historyFile := filepath.Join(dir, "history.jsonl")
	ctx := New(historyFile)

	// 写入初始内容
	if err := ctx.Append(NewSystemTextRecord("system prompt")); err != nil {
		t.Fatalf("Append system failed: %v", err)
	}
	if err := ctx.Append(NewUserTextRecord("user message 1")); err != nil {
		t.Fatalf("Append user 1 failed: %v", err)
	}
	if err := ctx.Append(NewAssistantTextRecord("assistant reply 1")); err != nil {
		t.Fatalf("Append assistant 1 failed: %v", err)
	}

	// 创建检查点 0
	id, err := ctx.AppendCheckpoint()
	if err != nil {
		t.Fatalf("AppendCheckpoint failed: %v", err)
	}
	if id != 0 {
		t.Errorf("expected checkpoint id 0, got %d", id)
	}

	// 继续追加内容
	if err := ctx.Append(NewUserTextRecord("user message 2")); err != nil {
		t.Fatalf("Append user 2 failed: %v", err)
	}
	if err := ctx.Append(NewAssistantTextRecord("assistant reply 2")); err != nil {
		t.Fatalf("Append assistant 2 failed: %v", err)
	}
	if err := ctx.AppendUsage(500); err != nil {
		t.Fatalf("AppendUsage failed: %v", err)
	}

	// 创建检查点 1
	id, err = ctx.AppendCheckpoint()
	if err != nil {
		t.Fatalf("AppendCheckpoint failed: %v", err)
	}
	if id != 1 {
		t.Errorf("expected checkpoint id 1, got %d", id)
	}

	// 再追加一些内容
	if err := ctx.Append(NewUserTextRecord("user message 3")); err != nil {
		t.Fatalf("Append user 3 failed: %v", err)
	}

	// 验证当前有 6 条记录（system + 3 users + 2 assistants）
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(records) != 6 {
		t.Errorf("expected 6 records before revert, got %d", len(records))
	}

	// 回滚到检查点 0
	recordCount, err := ctx.RevertToCheckpoint(0)
	if err != nil {
		t.Fatalf("RevertToCheckpoint failed: %v", err)
	}
	t.Logf("Reverted to checkpoint 0, restored %d records", recordCount)

	// 应该只剩 3 条记录（system + user1 + assistant1）
	records, err = ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records after revert, got %d", len(records))
		for i, r := range records {
			t.Logf("record %d: role=%s, content=%s", i, r.Role, r.Content)
		}
	}

	// 验证备份文件存在
	backupPath := historyFile + ".1"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("expected backup file to exist")
	}
}

func TestRevertToCheckpoint_InvalidID(t *testing.T) {
	dir := t.TempDir()
	historyFile := filepath.Join(dir, "history.jsonl")
	ctx := New(historyFile)

	// 没有检查点时尝试回滚
	_, err := ctx.RevertToCheckpoint(0)
	if err == nil {
		t.Error("expected error for non-existent checkpoint")
	}

	// 创建一个检查点
	_, _ = ctx.AppendCheckpoint()

	// 尝试回滚到不存在的检查点
	_, err = ctx.RevertToCheckpoint(99)
	if err == nil {
		t.Error("expected error for non-existent checkpoint")
	}
}

func TestUsageAndCheckpointInterleaved(t *testing.T) {
	dir := t.TempDir()
	historyFile := filepath.Join(dir, "history.jsonl")
	ctx := New(historyFile)

	// 混合追加消息、使用量和检查点
	_ = ctx.Append(NewUserTextRecord("msg1"))
	_ = ctx.AppendUsage(100)
	_, _ = ctx.AppendCheckpoint() // ID 0
	_ = ctx.Append(NewAssistantTextRecord("reply1"))
	_ = ctx.AppendUsage(200)
	_, _ = ctx.AppendCheckpoint() // ID 1
	_ = ctx.Append(NewUserTextRecord("msg2"))

	// 验证最终使用量
	usage, err := ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage failed: %v", err)
	}
	if usage != 200 {
		t.Errorf("expected usage 200, got %d", usage)
	}

	// 回滚到检查点 1（删除检查点 1 及之后的内容）
	recordCount, err := ctx.RevertToCheckpoint(1)
	if err != nil {
		t.Fatalf("RevertToCheckpoint failed: %v", err)
	}
	t.Logf("Reverted to checkpoint 1, restored %d records", recordCount)

	// 使用量应该是 200（检查点 1 之前的最后一个 usage）
	usage, err = ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage after revert failed: %v", err)
	}
	if usage != 200 {
		t.Errorf("expected usage 200 after revert to checkpoint 1, got %d", usage)
	}

	// 回滚到检查点 0（删除检查点 0 及之后的内容）
	recordCount, err = ctx.RevertToCheckpoint(0)
	if err != nil {
		t.Fatalf("RevertToCheckpoint failed: %v", err)
	}
	t.Logf("Reverted to checkpoint 0, restored %d records", recordCount)

	// 使用量应该是 100（检查点 0 之前的最后一个 usage）
	usage, err = ctx.ReadUsage()
	if err != nil {
		t.Fatalf("ReadUsage after revert failed: %v", err)
	}
	if usage != 100 {
		t.Errorf("expected usage 100 after revert to checkpoint 0, got %d", usage)
	}
}