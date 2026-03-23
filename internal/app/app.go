package app

import (
	"fmt"
	"os"
	"strings"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/session"
)

const (
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

	input := parseRunInput(args)

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current work dir: %w", err)
	}

	sess, err := session.New(workDir)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	ctx := contextstore.New(sess.HistoryFile)
	state, err := bootstrapStartupState(ctx)
	if err != nil {
		return err
	}

	state, err = applyRunInput(ctx, state, input)
	if err != nil {
		return err
	}

	printStartupState(sess, ctx, state)

	_ = cfg

	return nil
}

// runInput 表示当前 CLI 入口解析出的最小输入结果。
type runInput struct {
	prompt string
}

// parseRunInput 把 CLI 参数折叠成一段原始 prompt 文本。
func parseRunInput(args []string) runInput {
	return runInput{
		prompt: strings.TrimSpace(strings.Join(args, " ")),
	}
}

// applyRunInput 把解析后的 CLI 输入写入当前 session history。
func applyRunInput(
	ctx contextstore.Context,
	state startupState,
	input runInput,
) (startupState, error) {
	if input.prompt == "" {
		return state, nil
	}

	record := buildPromptRecord(input.prompt)
	if err := ctx.Append(record); err != nil {
		return startupState{}, fmt.Errorf("append prompt record: %w", err)
	}

	state.historyExists = true
	state.historyCount++
	state.lastRecord = record
	state.hasLastRecord = true

	return state, nil
}

// buildInitialRecord 构造启动时写入 history 的第一条记录。
func buildInitialRecord() contextstore.TextRecord {
	return contextstore.NewSystemTextRecord(initialRecordContent)
}

// buildPromptRecord 构造用户输入对应的最小 history 记录。
func buildPromptRecord(prompt string) contextstore.TextRecord {
	return contextstore.NewUserTextRecord(prompt)
}

// bootstrapStartupState 统一完成启动期的 history 初始化与状态收集。
func bootstrapStartupState(ctx contextstore.Context) (startupState, error) {
	historyExists, err := ctx.Exists()
	if err != nil {
		return startupState{}, fmt.Errorf("check history file existence: %w", err)
	}

	snapshot, err := ctx.Snapshot()
	if err != nil {
		return startupState{}, fmt.Errorf("read history snapshot before bootstrap: %w", err)
	}

	historySeeded := false
	if snapshot.Count == 0 {
		initialRecord := buildInitialRecord()
		if err := ctx.Append(initialRecord); err != nil {
			return startupState{}, fmt.Errorf("append initial history record: %w", err)
		}

		historyExists = true
		historySeeded = true
		snapshot = contextstore.Snapshot{
			Count:         1,
			LastRecord:    initialRecord,
			HasLastRecord: true,
		}
	}

	return startupState{
		historyExists: historyExists,
		historySeeded: historySeeded,
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
