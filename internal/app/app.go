package app

import (
	"fmt"
	"os"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/session"
)

const (
	initialRecordRole    = "system"
	initialRecordContent = "session initialized"
)

// startupState 聚合启动阶段需要展示的状态信息。
type startupState struct {
	historyExists bool
	historySeeded bool
	historyCount  int
	lastRecord    contextstore.TextRecord
	hasLastRecord bool
}

// Run 是当前应用装配层的最小入口。
// 现在它还不执行 agent，只负责把 CLI 入口稳定下来。
func Run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(args) > 0 {
		return fmt.Errorf("arguments are not supported yet: %v", args)
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current work dir: %w", err)
	}

	sess, err := session.New(workDir)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	ctx := contextstore.New(sess.HistoryFile)
	historySeeded, err := ensureInitialRecord(ctx)
	if err != nil {
		return err
	}

	state, err := loadStartupState(sess, ctx)
	if err != nil {
		return err
	}
	state.historySeeded = historySeeded

	printStartupState(sess, ctx, state)

	_ = cfg

	return nil
}

// buildInitialRecord 构造启动时写入 history 的第一条记录。
func buildInitialRecord() contextstore.TextRecord {
	return contextstore.TextRecord{
		Role:    initialRecordRole,
		Content: initialRecordContent,
	}
}

// ensureInitialRecord 只在 history 为空时写入启动种子记录。
func ensureInitialRecord(ctx contextstore.Context) (bool, error) {
	snapshot, err := ctx.Snapshot()
	if err != nil {
		return false, fmt.Errorf("read history snapshot before bootstrap: %w", err)
	}

	if snapshot.Count > 0 {
		return false, nil
	}

	if err := ctx.Append(buildInitialRecord()); err != nil {
		return false, fmt.Errorf("append initial history record: %w", err)
	}

	return true, nil
}

// loadStartupState 收集启动阶段需要展示的状态信息。
func loadStartupState(sess session.Session, ctx contextstore.Context) (startupState, error) {
	historyExists, err := sess.HistoryExists()
	if err != nil {
		return startupState{}, fmt.Errorf("check history file existence: %w", err)
	}

	snapshot, err := ctx.Snapshot()
	if err != nil {
		return startupState{}, fmt.Errorf("read history snapshot: %w", err)
	}

	return startupState{
		historyExists: historyExists,
		historyCount:  snapshot.Count,
		lastRecord:    snapshot.LastRecord,
		hasLastRecord: snapshot.HasLastRecord,
	}, nil
}

// printStartupState 统一输出当前启动阶段的关键信息。
func printStartupState(
	sess session.Session,
	ctx contextstore.Context,
	state startupState,
) {
	fmt.Printf("session: %s\n", sess.ID)
	fmt.Printf("history: %s\n", ctx.Path())
	fmt.Printf("history exists: %t\n", state.historyExists)
	fmt.Printf("history seeded: %t\n", state.historySeeded)
	fmt.Printf("history records: %d\n", state.historyCount)
	if state.hasLastRecord {
		fmt.Printf("last history role: %s\n", state.lastRecord.Role)
		fmt.Printf("last history content: %s\n", state.lastRecord.Content)
	}
}
