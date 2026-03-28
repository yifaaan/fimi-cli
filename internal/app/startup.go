package app

import (
	"fmt"
	"os"
	"strings"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui/shell"
)

// startupState 聚合启动阶段需要展示的状态信息。
type startupState struct {
	historyExists bool
	historySeeded bool
	historyCount  int
	lastRecord    contextstore.TextRecord
	hasLastRecord bool
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
	for _, step := range result.Steps {
		for _, record := range step.AppendedRecords {
			state = advanceStartupState(state, record)
		}
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

func startupLastRole(state startupState) string {
	if !shouldShowStartupLastRecord(state) {
		return ""
	}

	return state.lastRecord.Role
}

func startupLastSummary(state startupState) string {
	if !shouldShowStartupLastRecord(state) {
		return ""
	}

	return summarizeStartupContent(state.lastRecord.Content, 80)
}

func buildShellStartupInfo(
	sess session.Session,
	state startupState,
	sessionReused bool,
	model string,
) shell.StartupInfo {
	return shell.StartupInfo{
		SessionID:      sess.ID,
		SessionReused:  sessionReused,
		ModelName:      model,
		AppVersion:     resolveAppVersion(),
		ConversationDB: sess.HistoryFile,
		LastRole:       startupLastRole(state),
		LastSummary:    startupLastSummary(state),
	}
}

func loadShellInitialRecords(cfg config.Config, store contextstore.Context) ([]contextstore.TextRecord, error) {
	historyTurnLimit := cfg.HistoryWindow.RuntimeTurns
	if historyTurnLimit <= 0 {
		historyTurnLimit = config.DefaultRuntimeTurns
	}

	records, err := store.ReadRecentTurns(historyTurnLimit)
	if err != nil {
		return nil, fmt.Errorf("read recent turns for shell startup: %w", err)
	}

	return records, nil
}

func buildShellDependencies(
	runner runtimeRunner,
	store contextstore.Context,
	agent loadedAgent,
	sess session.Session,
	input runInput,
	modelName string,
	historyFile string,
	initialRecords []contextstore.TextRecord,
	startupInfo shell.StartupInfo,
) shell.Dependencies {
	return shell.Dependencies{
		Runner:         runner,
		Store:          store,
		Input:          os.Stdin,
		Output:         os.Stdout,
		ErrOutput:      os.Stderr,
		HistoryFile:    historyFile,
		ModelName:      modelName,
		SystemPrompt:   agent.SystemPrompt,
		WorkDir:        sess.WorkDir,
		InitialPrompt:  input.prompt,
		InitialRecords: initialRecords,
		StartupInfo:    startupInfo,
	}
}

func shouldShowStartupLastRecord(state startupState) bool {
	if !state.hasLastRecord {
		return false
	}
	if state.lastRecord.Role == contextstore.RoleSystem && state.lastRecord.Content == initialRecordContent {
		return false
	}

	return true
}

func summarizeStartupContent(content string, maxLen int) string {
	compact := strings.Join(strings.Fields(content), " ")
	if maxLen <= 0 || len(compact) <= maxLen {
		return compact
	}
	if maxLen <= 3 {
		return compact[:maxLen]
	}

	return compact[:maxLen-3] + "..."
}
