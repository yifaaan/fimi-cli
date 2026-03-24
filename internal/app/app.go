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

type configLoader func() (config.Config, error)
type workDirResolver func() (string, error)
type sessionOpener func(workDir string) (session.Session, bool, error)
type sessionCreator func(workDir string) (session.Session, error)
type llmClientBuilder func(cfg config.Config) (llm.Client, error)
type runtimeRunnerBuilder func(cfg config.Config) (runtimeRunner, error)
type startupStatePrinter func(
	sess session.Session,
	ctx contextstore.Context,
	state startupState,
	sessionReused bool,
	model string,
)

// runtimeRunner 是 app 对 runtime 的最小消费边界。
// 在消费方定义接口，避免 app 依赖 runtime 的具体装配细节。
type runtimeRunner interface {
	Run(ctx contextstore.Context, input runtime.Input) (runtime.Result, error)
}

// dependencies 表示 app 装配层当前持有的可替换依赖。
// 这些依赖都属于进程边界或适配器装配，收进来之后 Run 才容易测试。
type dependencies struct {
	loadConfig         configLoader
	resolveWorkDir     workDirResolver
	openSession        sessionOpener
	createSession      sessionCreator
	buildLLMClient     llmClientBuilder
	buildRuntimeRunner runtimeRunnerBuilder
	printStartupState  startupStatePrinter
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
	return defaultDependencies().run(args)
}

func (d dependencies) run(args []string) error {
	loadConfig := d.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	input := parseRunInput(args)

	resolveWorkDir := d.resolveWorkDir
	if resolveWorkDir == nil {
		resolveWorkDir = os.Getwd
	}

	workDir, err := resolveWorkDir()
	if err != nil {
		return fmt.Errorf("get current work dir: %w", err)
	}

	sess, sessionReused, err := d.openRunSession(workDir, input)
	if err != nil {
		return err
	}

	ctx := contextstore.New(sess.HistoryFile)
	state, err := bootstrapStartupState(ctx)
	if err != nil {
		return err
	}

	runner, err := d.buildRunner(cfg)
	if err != nil {
		return err
	}

	runResult, err := runner.Run(ctx, buildRuntimeInput(cfg, input))
	if err != nil {
		return fmt.Errorf("run runtime: %w", err)
	}

	state = applyRuntimeResult(state, runResult)

	printState := d.printStartupState
	if printState == nil {
		printState = printStartupState
	}
	printState(sess, ctx, state, sessionReused, resolveRuntimeModelName(cfg))

	return nil
}

// runInput 表示当前 CLI 入口解析出的最小输入结果。
type runInput struct {
	prompt          string
	forceNewSession bool
}

// parseRunInput 把 CLI 参数折叠成应用层输入。
// 这里先手写最小解析逻辑，只识别 app 层已经承诺支持的 flag。
func parseRunInput(args []string) runInput {
	promptParts := make([]string, 0, len(args))
	input := runInput{}

	for _, arg := range args {
		if arg == "--new-session" {
			input.forceNewSession = true
			continue
		}

		promptParts = append(promptParts, arg)
	}

	input.prompt = strings.TrimSpace(strings.Join(promptParts, " "))

	return runInput{
		prompt:          input.prompt,
		forceNewSession: input.forceNewSession,
	}
}

func defaultDependencies() dependencies {
	return dependencies{
		loadConfig:        config.Load,
		resolveWorkDir:    os.Getwd,
		openSession:       session.OpenLatestOrCreate,
		createSession:     session.New,
		buildLLMClient:    buildLLMClientFromConfig,
		printStartupState: printStartupState,
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
		Model:        resolveRuntimeModelName(cfg),
		SystemPrompt: cfg.SystemPrompt,
	}
}

// resolveRuntimeModelName 把逻辑模型选择折叠成 runtime 真正要发送的模型名。
func resolveRuntimeModelName(cfg config.Config) string {
	modelCfg, err := resolveConfiguredModel(cfg)
	if err == nil && modelCfg.Model != "" {
		return modelCfg.Model
	}

	return cfg.DefaultModel
}

// buildEngine 负责装配当前默认的 llm engine。
func (d dependencies) buildEngine(cfg config.Config) (llm.Engine, error) {
	buildClient := d.buildLLMClient
	if buildClient == nil {
		buildClient = buildLLMClientFromConfig
	}

	client, err := buildClient(cfg)
	if err != nil {
		return llm.Engine{}, fmt.Errorf("build llm client: %w", err)
	}

	return llm.NewEngine(client, buildLLMConfig(cfg)), nil
}

// buildRunner 负责装配一次 runtime 执行所需的核心依赖。
func (d dependencies) buildRunner(cfg config.Config) (runtimeRunner, error) {
	if d.buildRuntimeRunner != nil {
		return d.buildRuntimeRunner(cfg)
	}

	engine, err := d.buildEngine(cfg)
	if err != nil {
		return nil, err
	}

	return runtime.New(engine, buildRuntimeConfig(cfg)), nil
}

func buildEngine(cfg config.Config) (llm.Engine, error) {
	return defaultDependencies().buildEngine(cfg)
}

func buildRunner(cfg config.Config) (runtimeRunner, error) {
	return defaultDependencies().buildRunner(cfg)
}

// openRunSession 根据当前应用输入决定 session 获取策略。
// 是否复用旧 session 属于 app 层决策，而不是 session 包内部规则。
func (d dependencies) openRunSession(workDir string, input runInput) (session.Session, bool, error) {
	if input.forceNewSession {
		createSession := d.createSession
		if createSession == nil {
			createSession = session.New
		}

		sess, err := createSession(workDir)
		if err != nil {
			return session.Session{}, false, fmt.Errorf("create session: %w", err)
		}

		return sess, false, nil
	}

	openSession := d.openSession
	if openSession == nil {
		openSession = session.OpenLatestOrCreate
	}

	sess, reused, err := openSession(workDir)
	if err != nil {
		return session.Session{}, false, fmt.Errorf("open session: %w", err)
	}

	return sess, reused, nil
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
