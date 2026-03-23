package app

import (
	"fmt"
	"os"
	"strings"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
)

const (
	initialRecordContent = "session initialized"
	defaultSystemPrompt  = "You are fimi, a coding agent."
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

	sess, sessionReused, err := session.OpenLatestOrCreate(workDir)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}

	ctx := contextstore.New(sess.HistoryFile)
	state, err := bootstrapStartupState(ctx)
	if err != nil {
		return err
	}

	runner := runtime.New(llm.NewPlaceholderEngine())

	runResult, err := runner.Run(ctx, runtime.Input{
		Prompt:       input.prompt,
		Model:        cfg.DefaultModel,
		SystemPrompt: defaultSystemPrompt,
	})
	if err != nil {
		return fmt.Errorf("run runtime: %w", err)
	}

	state = applyRuntimeResult(state, runResult)

	printStartupState(sess, ctx, state, sessionReused, cfg.DefaultModel)

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

// advanceStartupState 根据刚写入的记录推进启动阶段的内存状态。
func advanceStartupState(
	state startupState,
	record contextstore.TextRecord,
) startupState {
	state.historyExists = true
	state.historyCount++
	state.lastRecord = record
	state.hasLastRecord = true

	return state
}

// buildInitialRecord 构造启动时写入 history 的第一条记录。
func buildInitialRecord() contextstore.TextRecord {
	return contextstore.NewSystemTextRecord(initialRecordContent)
}

// applyRuntimeResult 把 runtime 的输出折叠回当前启动阶段状态。
func applyRuntimeResult(state startupState, result runtime.Result) startupState {
	for _, record := range result.AppendedRecords {
		state = advanceStartupState(state, record)
	}

	return state
}

// bootstrapStartupState 统一完成启动期的 history 初始化与状态收集。
func bootstrapStartupState(ctx contextstore.Context) (startupState, error) {
	result, err := ctx.Bootstrap(buildInitialRecord())
	if err != nil {
		return startupState{}, fmt.Errorf("bootstrap history: %w", err)
	}

	return startupState{
		historyExists: result.HistoryExists,
		historySeeded: result.HistorySeeded,
		historyCount:  result.Snapshot.Count,
		lastRecord:    result.Snapshot.LastRecord,
		hasLastRecord: result.Snapshot.HasLastRecord,
	}, nil
}

// printStartupState 统一输出当前启动阶段的关键信息。
func printStartupState(
	sess session.Session,
	ctx contextstore.Context,
	state startupState,
	sessionReused bool,
	model string,
) {
	fmt.Printf("session: %s\n", sess.ID)
	fmt.Printf("session reused: %t\n", sessionReused)
	fmt.Printf("model: %s\n", model)
	fmt.Printf("history: %s\n", ctx.Path())
	fmt.Printf("history exists: %t\n", state.historyExists)
	fmt.Printf("history seeded: %t\n", state.historySeeded)
	fmt.Printf("history records: %d\n", state.historyCount)
	if state.hasLastRecord {
		fmt.Printf("last history role: %s\n", state.lastRecord.Role)
		fmt.Printf("last history content: %s\n", state.lastRecord.Content)
	}
}
