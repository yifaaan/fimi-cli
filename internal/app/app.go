package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fimi-cli/internal/acp"
	"fimi-cli/internal/agentspec"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/mcp"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/printui"
	"fimi-cli/internal/ui/shell"
	"fimi-cli/internal/webfetch"
	"fimi-cli/internal/websearch"
)

const (
	initialRecordContent    = "session initialized"
	defaultAgentsDirName    = "agents"
	defaultAgentProfileName = "default"
	defaultAgentFileName    = "agent.yaml"

	// 子代理回复过短时，用 continuation prompt 要求更详细的输出
	subagentContinuePrompt = "Your previous response was too brief. Please provide a more comprehensive summary that includes:\n\n1. Specific technical details and implementations\n2. Complete code examples if relevant\n3. Detailed findings and analysis\n4. All important information that should be aware of by the caller"
	subagentMinResponseLen = 200
)

var ErrUnknownCLIFlag = errors.New("unknown cli flag")
var ErrCLIFlagValueRequired = errors.New("cli flag requires a value")
var ErrConflictingSessionFlags = errors.New("conflicting session flags")
var ErrSubagentNotDeclared = errors.New("subagent is not declared")

type configLoader func() (config.Config, error)
type workDirResolver func() (string, error)
type sessionContinuer func(workDir string) (session.Session, error)
type sessionCreator func(workDir string) (session.Session, error)
type llmClientBuilder func(cfg config.Config) (llm.Client, error)
type runtimeRunnerBuilder func(cfg config.Config) (runtimeRunner, error)
type runtimeVisualizerBuilder func(mode string, w io.Writer) ui.VisualizeFunc
type shellUIRunner func(ctx context.Context, deps shell.Dependencies) error
type agentLoader func(workDir string, registry tools.Registry) (loadedAgent, error)

// runtimeRunner 是 app 对 runtime 的最小消费边界。
// 在消费方定义接口，避免 app 依赖 runtime 的具体装配细节。
type runtimeRunner interface {
	Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error)
}

// loadedAgent 表示 app 当前一次运行实际解析出的 agent 视图。
// 这里保留最小字段，避免 app 过早持有 tools 等后续阶段才会消费的内容。
type loadedAgent struct {
	Spec         agentspec.Spec
	SystemPrompt string
	Tools        []tools.Definition
}

// dependencies 表示 app 装配层当前持有的可替换依赖。
// 这些依赖都属于进程边界或适配器装配，收进来之后 Run 才容易测试。
type dependencies struct {
	loadConfig         configLoader
	resolveWorkDir     workDirResolver
	loadAgent          agentLoader
	continueSession    sessionContinuer
	createSession      sessionCreator
	buildLLMClient     llmClientBuilder
	buildRuntimeRunner runtimeRunnerBuilder
	buildVisualizer    runtimeVisualizerBuilder
	runShellUI         shellUIRunner
	buildMCPTools      mcpToolBuilder
}

// Run 是当前应用装配层的最小入口。
// 当前它会完成配置、默认 agent、session 与 runtime 的装配。
func Run(args []string) error {
	return defaultDependencies().run(args)
}

func (d dependencies) run(args []string) error {
	// 检查是否是 ACP 子命令
	if len(args) > 0 && args[0] == "acp" {
		return d.runACP(context.Background())
	}

	input, err := parseRunInput(args)
	if err != nil {
		return err
	}
	if input.showHelp {
		printHelp()
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

	registry := d.resolveToolRegistry()

	agent, err := d.loadRunAgent(workDir, registry)
	if err != nil {
		return err
	}

	// 非 shell 模式（text / stream-json）走一次性打印流程
	if input.outputMode != "" && input.outputMode != "shell" {
		return d.runPrint(context.Background(), cfg, agent, workDir, input)
	}

	return d.runShell(context.Background(), cfg, agent, workDir, input)
}

// runInput 表示当前 CLI 入口解析出的最小输入结果。
type runInput struct {
	prompt          string
	forceNewSession bool
	continueSession bool
	modelAlias      string
	outputMode      string // "shell"（默认）, "text", "stream-json"
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
		if parseFlags && (arg == "--continue" || arg == "-C") {
			input.continueSession = true
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
		if parseFlags && arg == "--output" {
			if i+1 >= len(args) {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			value := strings.TrimSpace(args[i+1])
			if value == "" || strings.HasPrefix(value, "-") {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			switch value {
			case "text", "stream-json", "shell":
				input.outputMode = value
			default:
				return runInput{}, fmt.Errorf("invalid --output value %q: must be text, stream-json, or shell", value)
			}
			i++
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "-") {
			return runInput{}, fmt.Errorf("%w: %s", ErrUnknownCLIFlag, arg)
		}

		promptParts = append(promptParts, arg)
	}

	input.prompt = strings.TrimSpace(strings.Join(promptParts, " "))
	if input.forceNewSession && input.continueSession {
		return runInput{}, ErrConflictingSessionFlags
	}

	return runInput{
		prompt:          input.prompt,
		forceNewSession: input.forceNewSession,
		continueSession: input.continueSession,
		modelAlias:      input.modelAlias,
		outputMode:      input.outputMode,
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
		loadConfig:      config.Load,
		resolveWorkDir:  os.Getwd,
		loadAgent:       loadAgentFromWorkDir,
		continueSession: session.Continue,
		createSession:   session.New,
		buildLLMClient:  buildLLMClientFromConfig,
		buildVisualizer: defaultRuntimeVisualizer,
	}
}

// buildLLMConfig 把全局配置映射为 llm 模块自己的最小配置。
func buildLLMConfig(cfg config.Config) llm.Config {
	return llm.Config{
		HistoryTurnLimit: cfg.HistoryWindow.LLMTurns,
	}
}

func buildLLMToolDefinitions(definitions []tools.Definition) []llm.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}

	toolDefs := make([]llm.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		// Use InputSchema if provided (MCP tools), otherwise use builtin schema
		params := definition.InputSchema
		if params == nil {
			params = tools.ToolParametersSchema(definition.Name)
		}
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  params,
		})
	}

	return toolDefs
}

// buildRuntimeConfig 把全局配置映射为 runtime 模块自己的最小配置。
func buildRuntimeConfig(cfg config.Config, agent loadedAgent) runtime.Config {
	modelCfg, err := resolveConfiguredModel(resolveModelOverride(cfg, agent))

	return runtime.Config{
		ReplyHistoryTurnLimit: cfg.HistoryWindow.RuntimeTurns,
		MaxStepsPerRun:        cfg.LoopControl.MaxStepsPerRun,
		MaxRetriesPerStep:     cfg.LoopControl.MaxRetriesPerStep,
		ContextWindowTokens:   resolveContextWindowTokens(modelCfg, err),
	}
}

func resolveContextWindowTokens(modelCfg config.ModelConfig, err error) int {
	if err != nil {
		return 0
	}

	return modelCfg.ContextWindowTokens
}

// buildRuntimeInput 把应用输入、模型选择和 agent prompt 映射为单次 runtime 调用输入。
func buildRuntimeInput(cfg config.Config, input runInput, agent loadedAgent) runtime.Input {
	cfg = resolveModelOverride(cfg, agent)

	return runtime.Input{
		Prompt:       input.prompt,
		Model:        resolveRuntimeModelName(cfg),
		SystemPrompt: agent.SystemPrompt,
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

func resolveModelOverride(cfg config.Config, agent loadedAgent) config.Config {
	modelAlias := strings.TrimSpace(agent.Spec.Model)
	if modelAlias == "" {
		return cfg
	}

	cfg.DefaultModel = modelAlias

	return cfg
}

// buildEngine 负责装配当前默认的 llm engine。
func (d dependencies) buildEngine(cfg config.Config) (llm.Engine, error) {
	return d.buildEngineForAgent(cfg, loadedAgent{})
}

func (d dependencies) buildEngineForAgent(cfg config.Config, agent loadedAgent) (llm.Engine, error) {
	return d.buildEngineForAgentWithTools(cfg, agent, agent.Tools)
}

func (d dependencies) buildEngineForAgentWithTools(cfg config.Config, agent loadedAgent, tools []tools.Definition) (llm.Engine, error) {
	cfg = resolveModelOverride(cfg, agent)

	buildClient := d.buildLLMClient
	if buildClient == nil {
		buildClient = buildLLMClientFromConfig
	}

	client, err := buildClient(cfg)
	if err != nil {
		return llm.Engine{}, fmt.Errorf("build llm client: %w", err)
	}

	llmCfg := buildLLMConfig(cfg)
	llmCfg.Tools = buildLLMToolDefinitions(tools)

	return llm.NewEngine(client, llmCfg), nil
}

// buildRunner 负责装配一次 runtime 执行所需的核心依赖。
func (d dependencies) buildRunner(cfg config.Config) (runtimeRunner, error) {
	return d.buildRunnerForAgent(cfg, loadedAgent{}, "")
}

// buildRunnerForAgent 负责把当前 agent 的工具能力一起装配进 runtime。
func (d dependencies) buildRunnerForAgent(cfg config.Config, agent loadedAgent, workDir string) (runtimeRunner, error) {
	cfg = resolveModelOverride(cfg, agent)

	mcpManager := d.buildMCPManager(cfg)
	allTools := mergeAgentAndMCPTools(agent.Tools, mcpManager.Tools())

	if d.buildRuntimeRunner != nil {
		return d.buildRuntimeRunner(cfg)
	}

	engine, err := d.buildEngineForAgentWithTools(cfg, agent, allTools)
	if err != nil {
		return nil, err
	}

	toolHandlers, err := d.buildRunnerToolHandlers(cfg, agent, workDir, mcpManager)
	if err != nil {
		return nil, err
	}

	toolExecutor := tools.NewBuiltinExecutor(
		allTools,
		workDir,
		nil, // TODO: wire BackgroundManager here
		tools.WithExtraHandlers(toolHandlers),
	)

	runner := runtime.NewWithToolExecutor(engine, toolExecutor, buildRuntimeConfig(cfg, agent))

	return &mcpAwareRunner{
		Runner:     runner,
		mcpManager: mcpManager,
	}, nil
}

func (d dependencies) buildMCPManager(cfg config.Config) *mcp.Manager {
	if d.buildMCPTools != nil {
		return d.buildMCPTools(cfg.MCP)
	}

	return mcp.NewManager(context.Background(), cfg.MCP)
}

func mergeAgentAndMCPTools(agentTools []tools.Definition, mcpTools []mcp.Tool) []tools.Definition {
	allTools := make([]tools.Definition, 0, len(agentTools)+len(mcpTools))
	allTools = append(allTools, agentTools...)
	for _, tool := range mcpTools {
		allTools = append(allTools, tools.Definition{
			Name:        tool.Name,
			Kind:        tools.KindUtility,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	return allTools
}

func (d dependencies) buildRunnerToolHandlers(cfg config.Config, agent loadedAgent, workDir string, mcpManager *mcp.Manager) (map[string]tools.HandlerFunc, error) {
	registry := d.resolveToolRegistry()
	toolHandlers := map[string]tools.HandlerFunc{
		tools.ToolAgent: d.newAgentToolHandler(cfg, agent, workDir, registry),
	}
	if containsTool(agent.Tools, tools.ToolSearchWeb) {
		searcher, err := buildWebSearcher(cfg)
		if err != nil {
			return nil, fmt.Errorf("build web searcher: %w", err)
		}
		toolHandlers[tools.ToolSearchWeb] = tools.NewSearchWebHandler(searcher, tools.NewOutputShaper())
	}
	if containsTool(agent.Tools, tools.ToolFetchURL) {
		fetcher, err := buildURLFetcher()
		if err != nil {
			return nil, fmt.Errorf("build url fetcher: %w", err)
		}
		toolHandlers[tools.ToolFetchURL] = tools.NewFetchURLHandler(fetcher, tools.NewOutputShaper())
	}
	for _, tool := range mcpManager.Tools() {
		client := mcpManager.ClientForTool(tool.Name)
		if client == nil {
			continue
		}
		toolHandlers[tool.Name] = tools.NewMCPToolHandler(client, tool)
	}

	return toolHandlers, nil
}

// mcpAwareRunner wraps a runtime.Runner to close the MCP manager after the run.
type mcpAwareRunner struct {
	runtime.Runner
	mcpManager *mcp.Manager
}

func (r *mcpAwareRunner) Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error) {
	defer func() {
		if r.mcpManager != nil {
			_ = r.mcpManager.Close()
		}
	}()
	return r.Runner.Run(ctx, store, input)
}

// Inner 返回内嵌的 runtime.Runner。
func (r *mcpAwareRunner) Inner() runtime.Runner { return r.Runner }

// buildMCPTools is a dependency injection point for testing.
type mcpToolBuilder func(cfg config.MCPConfig) *mcp.Manager

func buildEngine(cfg config.Config) (llm.Engine, error) {
	return defaultDependencies().buildEngine(cfg)
}

func buildRunner(cfg config.Config) (runtimeRunner, error) {
	return defaultDependencies().buildRunner(cfg)
}

func buildWebSearcher(cfg config.Config) (tools.WebSearcher, error) {
	if !cfg.Web.Enabled {
		return nil, nil
	}
	if cfg.Web.SearchBackend != config.DefaultWebSearchBackend {
		return nil, fmt.Errorf("unsupported web search backend: %s", cfg.Web.SearchBackend)
	}

	return websearch.NewDuckDuckGoSearcher(websearch.DuckDuckGoConfig{
		BaseURL:   cfg.Web.DuckDuckGo.BaseURL,
		UserAgent: cfg.Web.DuckDuckGo.UserAgent,
	})
}

func buildURLFetcher() (tools.URLFetcher, error) {
	// URL fetcher 使用内建默认配置，暂不暴露用户配置入口。
	return webfetch.NewHTTPFetcher(webfetch.HTTPFetcherConfig{})
}

func runWithEventSink(runner runtimeRunner, store contextstore.Context, input runtime.Input) ui.RunFunc {
	return func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
		eventfulRunner, ok := runner.(runtime.EventSinkCapableRunner)
		if !ok {
			return runner.Run(ctx, store, input)
		}

		return eventfulRunner.WithEventSink(sink).Run(ctx, store, input)
	}
}

func (d dependencies) runRuntime(
	ctx context.Context,
	runner runtimeRunner,
	store contextstore.Context,
	input runtime.Input,
) (runtime.Result, error) {
	return ui.Run(ctx, runWithEventSink(runner, store, input), d.resolveVisualizer("text"))
}

func resolvePrintPrompt(input runInput) (string, error) {
	prompt := input.prompt

	if prompt == "" {
		var stdinPrompt string
		_, err := fmt.Fscanln(os.Stdin, &stdinPrompt)
		if err != nil && err.Error() != "expected newline" {
			return "", fmt.Errorf("read prompt from stdin: %w", err)
		}
		prompt = stdinPrompt
	}

	if prompt == "" {
		return "", fmt.Errorf("no prompt provided; pass as argument or via stdin")
	}

	return prompt, nil
}

func (d dependencies) preparePrintStore(workDir, prompt string) (contextstore.Context, error) {
	createSession := d.createSession
	if createSession == nil {
		createSession = session.New
	}

	sess, err := createSession(workDir)
	if err != nil {
		return contextstore.Context{}, fmt.Errorf("create print session: %w", err)
	}

	store := contextstore.New(sess.HistoryFile)
	if err := store.Append(contextstore.NewUserTextRecord(prompt)); err != nil {
		return contextstore.Context{}, fmt.Errorf("append user prompt to history: %w", err)
	}

	return store, nil
}

func buildRuntimePromptInput(cfg config.Config, agent loadedAgent, prompt string) runtime.Input {
	return runtime.Input{
		Prompt:       prompt,
		Model:        resolveRuntimeModelName(cfg),
		SystemPrompt: agent.SystemPrompt,
	}
}

func runSubagentOnce(
	ctx context.Context,
	runner runtimeRunner,
	historyFile string,
	input runtime.Input,
) (runtime.Result, error) {
	return runner.Run(ctx, contextstore.New(historyFile), input)
}

func (d dependencies) resolveVisualizer(mode string) ui.VisualizeFunc {
	buildVisualizer := d.buildVisualizer
	if buildVisualizer == nil {
		buildVisualizer = defaultRuntimeVisualizer
	}

	return buildVisualizer(mode, os.Stdout)
}

func (d dependencies) resolveShellUIRunner() shellUIRunner {
	runShellUI := d.runShellUI
	if runShellUI == nil {
		runShellUI = shell.Run
	}

	return runShellUI
}

func defaultRuntimeVisualizer(mode string, w io.Writer) ui.VisualizeFunc {
	if w == nil {
		w = os.Stdout
	}
	if mode == "stream-json" {
		return printui.VisualizeStreamJSON(w)
	}
	return printui.VisualizeText(w)
}

func containsTool(definitions []tools.Definition, name string) bool {
	for _, definition := range definitions {
		if definition.Name == name {
			return true
		}
	}

	return false
}

func (d dependencies) runShell(
	ctx context.Context,
	cfg config.Config,
	agent loadedAgent,
	workDir string,
	input runInput,
) error {
	cfg = resolveModelOverride(cfg, agent)

	sess, sessionReused, err := d.openRunSession(workDir, input)
	if err != nil {
		return err
	}

	store := contextstore.New(sess.HistoryFile)
	state, err := bootstrapStartupState(store)
	if err != nil {
		return err
	}
	initialRecords, err := loadShellInitialRecords(cfg, store)
	if err != nil {
		return err
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}

	historyFile, err := session.ShellHistoryFileForWorkDir(sess.WorkDir)
	if err != nil {
		return fmt.Errorf("resolve shell history file: %w", err)
	}
	modelName := resolveRuntimeModelName(cfg)

	return d.resolveShellUIRunner()(ctx, buildShellDependencies(
		runner,
		store,
		agent,
		sess,
		input,
		modelName,
		historyFile,
		initialRecords,
		buildShellStartupInfo(sess, state, sessionReused, modelName),
	))
}

// runPrint 处理 text / stream-json 模式的单次执行。
// prompt 从参数读取，如果没有则尝试从 stdin 读取一行。
func (d dependencies) runPrint(
	ctx context.Context,
	cfg config.Config,
	agent loadedAgent,
	workDir string,
	input runInput,
) error {
	prompt, err := resolvePrintPrompt(input)
	if err != nil {
		return err
	}

	cfg = resolveModelOverride(cfg, agent)

	store, err := d.preparePrintStore(workDir, prompt)
	if err != nil {
		return err
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}

	runtimeInput := buildRuntimePromptInput(cfg, agent, prompt)

	_, err = ui.Run(ctx, runWithEventSink(runner, store, runtimeInput), d.resolveVisualizer(input.outputMode))

	return err
}

// loadRunAgent 负责解析当前运行使用的默认 agent。
func (d dependencies) loadRunAgent(workDir string, registry tools.Registry) (loadedAgent, error) {
	loadAgent := d.loadAgent
	if loadAgent == nil {
		loadAgent = loadAgentFromWorkDir
	}

	agent, err := loadAgent(workDir, registry)
	if err != nil {
		return loadedAgent{}, fmt.Errorf("load agent: %w", err)
	}

	return agent, nil
}

func (d dependencies) resolveToolRegistry() tools.Registry {
	return tools.BuiltinRegistry()
}

// runACP 启动 ACP JSON-RPC 服务器。
func (d dependencies) runACP(ctx context.Context) error {
	loadConfig := d.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	conn := acp.NewFramedConn(os.Stdin, os.Stdout)

	resolveWorkDir := d.resolveWorkDir
	if resolveWorkDir == nil {
		resolveWorkDir = os.Getwd
	}

	workDir, err := resolveWorkDir()
	if err != nil {
		return fmt.Errorf("get work dir: %w", err)
	}

	registry := d.resolveToolRegistry()
	agent, err := d.loadRunAgent(workDir, registry)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 每次 prompt 调用时创建新的 runner，并直接在这里组装 ACP 运行闭包。
	runFn := func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
		r, err := d.buildRunnerForAgent(cfg, agent, workDir)
		if err != nil {
			return runtime.Result{}, fmt.Errorf("build ACP runner: %w", err)
		}

		return ui.Run(ctx, runWithEventSink(r, store, input), visualize)
	}

	server := acp.NewServer(conn, cfg, runFn)
	return server.Serve(ctx)
}

// defaultAgentFile 返回工作目录下的默认 agent 文件位置。
func defaultAgentFile(workDir string) string {
	return filepath.Join(
		workDir,
		defaultAgentsDirName,
		defaultAgentProfileName,
		defaultAgentFileName,
	)
}

// loadAgentFromWorkDir 从当前工作目录加载默认 agent。
func loadAgentFromWorkDir(workDir string, registry tools.Registry) (loadedAgent, error) {
	agentFile := defaultAgentFile(workDir)

	agent, err := loadAgentFromFile(agentFile, registry)
	if err != nil {
		return loadedAgent{}, fmt.Errorf("load default agent file %q: %w", agentFile, err)
	}

	return agent, nil
}

func loadDeclaredSubagent(
	root loadedAgent,
	subagentName string,
	registry tools.Registry,
) (loadedAgent, error) {
	subagentName = strings.TrimSpace(subagentName)
	subagentSpec, ok := root.Spec.Subagents[subagentName]
	if !ok {
		return loadedAgent{}, fmt.Errorf("%w: %s", ErrSubagentNotDeclared, subagentName)
	}

	agent, err := loadAgentFromFile(subagentSpec.Path, registry)
	if err != nil {
		return loadedAgent{}, fmt.Errorf(
			"load subagent %q for agent %q: %w",
			subagentName,
			root.Spec.Name,
			err,
		)
	}

	return agent, nil
}

func (d dependencies) newAgentToolHandler(
	cfg config.Config,
	rootAgent loadedAgent,
	workDir string,
	registry tools.Registry,
) tools.HandlerFunc {
	return tools.NewAgentToolHandler(func(
		ctx context.Context,
		call runtime.ToolCall,
		args tools.AgentArguments,
		definition tools.Definition,
	) (runtime.ToolExecution, error) {
		return d.runDeclaredSubagent(ctx, cfg, rootAgent, workDir, registry, call, args)
	})
}

func (d dependencies) runDeclaredSubagent(
	ctx context.Context,
	cfg config.Config,
	rootAgent loadedAgent,
	workDir string,
	registry tools.Registry,
	call runtime.ToolCall,
	args tools.AgentArguments,
) (runtime.ToolExecution, error) {
	subagent, err := loadDeclaredSubagent(rootAgent, args.SubagentName, registry)
	if err != nil {
		return runtime.ToolExecution{}, err
	}

	historyFile, err := subagentHistoryFile(workDir, args.SubagentName, call.ID)
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("resolve subagent history file: %w", err)
	}

	runner, err := d.buildRunnerForAgent(cfg, subagent, workDir)
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("build subagent runner: %w", err)
	}

	result, err := runSubagentOnce(ctx, runner, historyFile, buildRuntimePromptInput(cfg, subagent, args.Prompt))
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("run subagent %q: %w", args.SubagentName, err)
	}

	// 步数耗尽时提示拆分任务
	if result.Status == runtime.RunStatusMaxSteps {
		return runtime.ToolExecution{}, fmt.Errorf(
			"subagent %q reached max steps. Please try splitting the task into smaller subtasks",
			args.SubagentName,
		)
	}

	output := finalAssistantText(result)

	// 无输出时的兜底错误
	if output == "" {
		return runtime.ToolExecution{}, fmt.Errorf(
			"subagent %q did not produce any output", args.SubagentName,
		)
	}

	// 回复过短时追加一轮 continuation prompt，让子代理输出更多细节
	if len(output) < subagentMinResponseLen {
		contResult, contErr := runSubagentOnce(ctx, runner, historyFile, buildRuntimePromptInput(cfg, subagent, subagentContinuePrompt))
		if contErr == nil && contResult.Status != runtime.RunStatusFailed {
			if continued := finalAssistantText(contResult); continued != "" {
				output = continued
			}
		}
		// continuation 失败时静默忽略，使用原始短回复
	}

	return runtime.ToolExecution{
		Call:   call,
		Output: output,
	}, nil
}

func loadAgentFromFile(agentFile string, registry tools.Registry) (loadedAgent, error) {
	spec, err := agentspec.LoadFile(agentFile)
	if err != nil {
		return loadedAgent{}, err
	}
	systemPrompt, err := agentspec.LoadSystemPrompt(spec)
	if err != nil {
		return loadedAgent{}, fmt.Errorf("load system prompt for agent %q: %w", spec.Name, err)
	}

	resolvedTools, err := registry.ResolveAll(spec.Tools)
	if err != nil {
		return loadedAgent{}, fmt.Errorf("resolve tools for agent %q: %w", spec.Name, err)
	}
	resolvedTools = filterExcludedTools(resolvedTools, spec.ExcludeTools)

	return loadedAgent{
		Spec:         spec,
		SystemPrompt: systemPrompt,
		Tools:        resolvedTools,
	}, nil
}

func filterExcludedTools(resolved []tools.Definition, excluded []string) []tools.Definition {
	if len(resolved) == 0 || len(excluded) == 0 {
		return resolved
	}

	excludedSet := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		excludedSet[name] = struct{}{}
	}

	filtered := make([]tools.Definition, 0, len(resolved))
	for _, definition := range resolved {
		if _, skip := excludedSet[definition.Name]; skip {
			continue
		}
		filtered = append(filtered, definition)
	}

	return filtered
}

func subagentHistoryFile(workDir, subagentName, toolCallID string) (string, error) {
	_, sessionsDir, err := session.DirForWorkDir(workDir)
	if err != nil {
		return "", err
	}

	baseName := sanitizeSubagentPathComponent(strings.TrimSpace(toolCallID))
	if baseName == "" {
		baseName = sanitizeSubagentPathComponent(strings.TrimSpace(subagentName))
	}
	if baseName == "" {
		baseName = "subagent"
	}

	return filepath.Join(sessionsDir, "subagents", baseName+session.HistoryFileExtName), nil
}

func sanitizeSubagentPathComponent(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	return strings.Trim(builder.String(), "_")
}

func finalAssistantText(result runtime.Result) string {
	for i := len(result.Steps) - 1; i >= 0; i-- {
		text := strings.TrimSpace(result.Steps[i].AssistantText)
		if text != "" {
			return text
		}
	}

	return ""
}

// openRunSession 根据当前应用输入决定 session 获取策略。
// 是否复用旧 session 属于 app 层决策，而不是 session 包内部规则。
func (d dependencies) openRunSession(workDir string, input runInput) (session.Session, bool, error) {
	if input.continueSession {
		continueSession := d.continueSession
		if continueSession == nil {
			continueSession = session.Continue
		}

		sess, err := continueSession(workDir)
		if err != nil {
			return session.Session{}, false, renderContinueSessionError(workDir, err)
		}

		return sess, true, nil
	}

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

func renderContinueSessionError(workDir string, err error) error {
	if errors.Is(err, session.ErrNoPreviousSession) {
		return fmt.Errorf(
			"no previous session found for work dir %q; rerun without --continue to start a new session: %w",
			workDir,
			session.ErrNoPreviousSession,
		)
	}

	return fmt.Errorf("continue session: %w", err)
}
