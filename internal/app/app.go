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
)

type llmClientBuilder func(mode string) (llm.Client, error)

// dependencies 表示 app 装配层当前持有的可替换依赖。
// 这里先只暴露 LLM client builder，后面接真实 provider 时不用改 runtime。
type dependencies struct {
	buildLLMClient llmClientBuilder
}

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
	deps := defaultDependencies()

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

	runner, err := deps.buildRunner(cfg)
	if err != nil {
		return err
	}

	runResult, err := runner.Run(ctx, buildRuntimeInput(cfg, input))
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

func defaultDependencies() dependencies {
	return dependencies{
		buildLLMClient: llm.BuildClient,
	}
}

// buildLLMConfig 把全局配置映射为 llm 模块自己的最小配置。
func buildLLMConfig(cfg config.Config) llm.Config {
	return llm.Config{
		HistoryTurnLimit: cfg.HistoryWindow.LLMTurns,
	}
}

// buildRuntimeConfig 把全局配置映射为 runtime 模块自己的最小配置。
func buildRuntimeConfig(cfg config.Config) runtime.Config {
	return runtime.Config{
		ReplyHistoryTurnLimit: cfg.HistoryWindow.RuntimeTurns,
	}
}

// buildRuntimeInput 把应用输入和全局配置映射为单次 runtime 调用输入。
func buildRuntimeInput(cfg config.Config, input runInput) runtime.Input {
	return runtime.Input{
		Prompt:       input.prompt,
		Model:        cfg.DefaultModel,
		SystemPrompt: cfg.SystemPrompt,
	}
}

// buildEngine 负责装配当前默认的 llm engine。
func (d dependencies) buildEngine(cfg config.Config) (llm.Engine, error) {
	buildClient := d.buildLLMClient
	if buildClient == nil {
		buildClient = llm.BuildClient
	}

	client, err := buildClient(cfg.EngineMode)
	if err != nil {
		return llm.Engine{}, fmt.Errorf("build llm client: %w", err)
	}

	return llm.NewEngine(client, buildLLMConfig(cfg)), nil
}

// buildRunner 负责装配一次 runtime 执行所需的核心依赖。
func (d dependencies) buildRunner(cfg config.Config) (runtime.Runner, error) {
	engine, err := d.buildEngine(cfg)
	if err != nil {
		return runtime.Runner{}, err
	}

	return runtime.New(engine, buildRuntimeConfig(cfg)), nil
}

func buildEngine(cfg config.Config) (llm.Engine, error) {
	return defaultDependencies().buildEngine(cfg)
}

func buildRunner(cfg config.Config) (runtime.Runner, error) {
	return defaultDependencies().buildRunner(cfg)
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
