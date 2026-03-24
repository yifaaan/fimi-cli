package app

import (
	"errors"
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

var ErrUnknownCLIFlag = errors.New("unknown cli flag")
var ErrCLIFlagValueRequired = errors.New("cli flag requires a value")

type configLoader func() (config.Config, error)
type workDirResolver func() (string, error)
type sessionOpener func(workDir string) (session.Session, bool, error)
type sessionCreator func(workDir string) (session.Session, error)
type llmClientBuilder func(cfg config.Config) (llm.Client, error)
type runtimeRunnerBuilder func(cfg config.Config) (runtimeRunner, error)
type helpPrinter func()
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
	printHelp          helpPrinter
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
	input, err := parseRunInput(args)
	if err != nil {
		return err
	}
	if input.showHelp {
		help := d.printHelp
		if help == nil {
			help = printHelp
		}
		help()
		return nil
	}

	loadConfig := d.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg, err = applyRunInputToConfig(cfg, input)
	if err != nil {
		return err
	}

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
	modelAlias      string
	showHelp        bool
}

// parseRunInput 把 CLI 参数折叠成应用层输入。
// 这里先手写最小解析逻辑，只识别 app 层已经承诺支持的 flag。
func parseRunInput(args []string) (runInput, error) {
	promptParts := make([]string, 0, len(args))
	input := runInput{}
	parseFlags := true

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if parseFlags && arg == "--" {
			// `--` 之后的内容全部按 prompt 字面量处理，避免和 CLI flag 冲突。
			parseFlags = false
			continue
		}

		if parseFlags && arg == "--new-session" {
			input.forceNewSession = true
			continue
		}
		if parseFlags && (arg == "--help" || arg == "-h") {
			input.showHelp = true
			continue
		}
		if parseFlags && arg == "--model" {
			if i+1 >= len(args) {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			value := strings.TrimSpace(args[i+1])
			if value == "" || strings.HasPrefix(value, "-") {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			input.modelAlias = value
			i++
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "-") {
			return runInput{}, fmt.Errorf("%w: %s", ErrUnknownCLIFlag, arg)
		}

		promptParts = append(promptParts, arg)
	}

	input.prompt = strings.TrimSpace(strings.Join(promptParts, " "))

	return runInput{
		prompt:          input.prompt,
		forceNewSession: input.forceNewSession,
		modelAlias:      input.modelAlias,
		showHelp:        input.showHelp,
	}, nil
}

// applyRunInputToConfig 把一次运行的 CLI 覆盖项折叠进当前进程内有效配置。
// 这里只改本次运行视图，不修改磁盘配置文件。
func applyRunInputToConfig(cfg config.Config, input runInput) (config.Config, error) {
	if input.modelAlias == "" {
		return cfg, nil
	}

	if _, ok := cfg.Models[input.modelAlias]; !ok {
		return config.Config{}, fmt.Errorf("model %q not found in config.models", input.modelAlias)
	}

	cfg.DefaultModel = input.modelAlias

	return cfg, nil
}

func defaultDependencies() dependencies {
	return dependencies{
		loadConfig:        config.Load,
		resolveWorkDir:    os.Getwd,
		openSession:       session.OpenLatestOrCreate,
		createSession:     session.New,
		buildLLMClient:    buildLLMClientFromConfig,
		printHelp:         printHelp,
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

// printHelp 输出当前 CLI 入口支持的最小帮助信息。
func printHelp() {
	fmt.Print(helpText())
}

// helpText 返回当前 CLI 入口支持的最小帮助文本。
func helpText() string {
	lines := make([]string, 0, 16)
	for _, section := range helpSections() {
		lines = append(lines, helpSectionLines(section.title, section.lines)...)
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

type helpSection struct {
	title string
	lines []string
}

func helpSections() []helpSection {
	return []helpSection{
		{title: "Usage", lines: helpUsageLines()},
		{title: "Flags", lines: helpFlagLines()},
		{title: "Prompt Rules", lines: helpPromptRuleLines()},
		{title: "Examples", lines: helpExampleLines()},
	}
}

func helpSectionLines(title string, lines []string) []string {
	section := make([]string, 0, len(lines)+1)
	section = append(section, title+":")
	section = append(section, lines...)

	return section
}

func helpUsageLines() []string {
	return []string{
		"  fimi [--new-session] [--model <alias>] [--help] [prompt...]",
		"  fimi [options] -- [prompt text starting with flags]",
	}
}

func helpFlagLines() []string {
	return []string{
		"  --new-session    Start a fresh session for this run",
		"  --model <alias>  Override the configured model for this run",
		"  -h, --help       Show this help message",
	}
}

func helpPromptRuleLines() []string {
	return []string{
		"  --                Stop parsing flags; everything after it is prompt text",
		"  prompt...         Remaining args are joined into one prompt string",
	}
}

func helpExampleLines() []string {
	return []string{
		"  fimi --new-session fix the flaky test",
		"  fimi --new-session --model fast-model refactor the session loader",
		"  fimi -- --help should be treated as prompt text",
	}
}
