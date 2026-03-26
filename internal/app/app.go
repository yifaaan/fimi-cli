package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fimi-cli/internal/agentspec"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/printui"
	"fimi-cli/internal/ui/shell"
	"fimi-cli/internal/websearch"
)

const (
	initialRecordContent    = "session initialized"
	defaultAgentsDirName    = "agents"
	defaultAgentProfileName = "default"
	defaultAgentFileName    = "agent.yaml"
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
type runtimeVisualizerBuilder func() ui.VisualizeFunc
type shellUIRunner func(ctx context.Context, deps shell.Dependencies) error
type agentLoader func(workDir string, registry tools.Registry) (loadedAgent, error)
type toolRegistryBuilder func() tools.Registry
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
	buildToolRegistry  toolRegistryBuilder
	continueSession    sessionContinuer
	createSession      sessionCreator
	buildLLMClient     llmClientBuilder
	buildRuntimeRunner runtimeRunnerBuilder
	buildVisualizer    runtimeVisualizerBuilder
	runShellUI         shellUIRunner
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
// 当前它会完成配置、默认 agent、session 与 runtime 的装配。
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

	registry := d.resolveToolRegistry()

	agent, err := d.loadRunAgent(workDir, registry)
	if err != nil {
		return err
	}

	return d.runShell(context.Background(), cfg, agent, workDir, input)
}

// runInput 表示当前 CLI 入口解析出的最小输入结果。
type runInput struct {
	prompt          string
	forceNewSession bool
	continueSession bool
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
		loadAgent:         loadAgentFromWorkDir,
		buildToolRegistry: tools.BuiltinRegistry,
		continueSession:   session.Continue,
		createSession:     session.New,
		buildLLMClient:    buildLLMClientFromConfig,
		buildVisualizer:   defaultRuntimeVisualizer,
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

func buildLLMToolDefinitions(definitions []tools.Definition) []llm.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}

	toolDefs := make([]llm.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  toolParametersSchema(definition.Name),
		})
	}

	return toolDefs
}

func toolParametersSchema(name string) map[string]any {
	switch name {
	case tools.ToolAgent:
		return objectSchema(requiredProperties(
			schemaProperty("description", "string", "Short task description for the subagent."),
			schemaProperty("prompt", "string", "Detailed task prompt for the subagent."),
			schemaProperty("subagent_name", "string", "Declared subagent name to run."),
		))
	case tools.ToolThink:
		return objectSchema(requiredProperties(
			schemaProperty("thought", "string", "Private reasoning note to log for the current step."),
		))
	case tools.ToolSetTodoList:
		return map[string]any{
			"type": "object",
			"$defs": map[string]any{
				"Todo": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type":        "string",
							"description": "The title of the todo",
							"minLength":   1,
						},
						"status": map[string]any{
							"type":        "string",
							"description": "The status of the todo",
							"enum":        []string{"Pending", "In Progress", "Done"},
						},
					},
					"required": []string{"title", "status"},
				},
			},
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "The updated todo list",
					"items": map[string]any{
						"$ref": "#/$defs/Todo",
					},
				},
			},
			"required": []string{"todos"},
		}
	case tools.ToolBash:
		return objectSchema(requiredProperties(
			schemaProperty("command", "string", "Shell command to run inside the workspace."),
		))
	case tools.ToolSearchWeb:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query to run on the web.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return.",
					"minimum":     1,
					"maximum":     20,
					"default":     5,
				},
				"include_content": map[string]any{
					"type":        "boolean",
					"description": "Include fetched page content when the backend can provide it.",
					"default":     false,
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		}
	case tools.ToolReadFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to read."),
		))
	case tools.ToolGlob:
		return objectSchema(requiredProperties(
			schemaProperty("pattern", "string", "Glob pattern relative to the workspace root."),
		))
	case tools.ToolGrep:
		return objectSchema(requiredProperties(
			schemaProperty("pattern", "string", "Regular expression to search for."),
			schemaProperty("path", "string", "Workspace-relative file or directory path to search."),
		))
	case tools.ToolWriteFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to write."),
			schemaProperty("content", "string", "Full file contents to write."),
		))
	case tools.ToolReplaceFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to edit."),
			schemaProperty("old", "string", "Exact text to replace."),
			schemaProperty("new", "string", "Replacement text."),
		))
	case tools.ToolPatchFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to patch."),
			schemaProperty("diff", "string", "Unified diff patch content."),
		))
	default:
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": true,
		}
	}
}

func requiredProperties(properties ...schemaEntry) []schemaEntry {
	return properties
}

type schemaEntry struct {
	name   string
	schema map[string]any
}

func schemaProperty(name, typeName, description string) schemaEntry {
	return schemaEntry{
		name: name,
		schema: map[string]any{
			"type":        typeName,
			"description": description,
		},
	}
}

func objectSchema(entries []schemaEntry) map[string]any {
	properties := make(map[string]any, len(entries))
	required := make([]string, 0, len(entries))
	for _, entry := range entries {
		properties[entry.name] = entry.schema
		required = append(required, entry.name)
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
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
	llmCfg.Tools = buildLLMToolDefinitions(agent.Tools)

	return llm.NewEngine(client, llmCfg), nil
}

// buildRunner 负责装配一次 runtime 执行所需的核心依赖。
func (d dependencies) buildRunner(cfg config.Config) (runtimeRunner, error) {
	return d.buildRunnerForAgent(cfg, loadedAgent{}, "")
}

// buildRunnerForAgent 负责把当前 agent 的工具能力一起装配进 runtime。
func (d dependencies) buildRunnerForAgent(cfg config.Config, agent loadedAgent, workDir string) (runtimeRunner, error) {
	cfg = resolveModelOverride(cfg, agent)

	if d.buildRuntimeRunner != nil {
		return d.buildRuntimeRunner(cfg)
	}

	engine, err := d.buildEngineForAgent(cfg, agent)
	if err != nil {
		return nil, err
	}

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

	toolExecutor := tools.NewBuiltinExecutorWithExtraHandlers(
		agent.Tools,
		workDir,
		toolHandlers,
	)

	return runtime.NewWithToolExecutor(engine, toolExecutor, buildRuntimeConfig(cfg, agent)), nil
}


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

type eventSinkCapableRunner interface {
	WithEventSink(sink runtimeevents.Sink) runtime.Runner
}

func (d dependencies) runRuntime(
	ctx context.Context,
	runner runtimeRunner,
	store contextstore.Context,
	input runtime.Input,
) (runtime.Result, error) {
	buildVisualizer := d.buildVisualizer
	if buildVisualizer == nil {
		buildVisualizer = defaultRuntimeVisualizer
	}

	visualize := buildVisualizer()

	return ui.Run(
		ctx,
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			eventfulRunner, ok := runner.(eventSinkCapableRunner)
			if !ok {
				return runner.Run(ctx, store, input)
			}

			return eventfulRunner.WithEventSink(sink).Run(ctx, store, input)
		},
		visualize,
	)
}


func defaultRuntimeVisualizer() ui.VisualizeFunc {
	return printui.VisualizeText(os.Stdout)
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
	historyTurnLimit := cfg.HistoryWindow.RuntimeTurns
	if historyTurnLimit <= 0 {
		historyTurnLimit = config.DefaultRuntimeTurns
	}
	initialRecords, err := store.ReadRecentTurns(historyTurnLimit)
	if err != nil {
		return fmt.Errorf("read recent turns for shell startup: %w", err)
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}

	runShellUI := d.runShellUI
	if runShellUI == nil {
		runShellUI = shell.Run
	}

	historyFile, err := session.ShellHistoryFileForWorkDir(sess.WorkDir)
	if err != nil {
		return fmt.Errorf("resolve shell history file: %w", err)
	}

	return runShellUI(ctx, shell.Dependencies{
		Runner:         runner,
		Store:          store,
		Input:          os.Stdin,
		Output:         os.Stdout,
		ErrOutput:      os.Stderr,
		HistoryFile:    historyFile,
		ModelName:      resolveRuntimeModelName(cfg),
		SystemPrompt:   agent.SystemPrompt,
		WorkDir:        sess.WorkDir,
		InitialPrompt:  input.prompt,
		InitialRecords: initialRecords,
		StartupInfo: shell.StartupInfo{
			SessionID:      sess.ID,
			SessionReused:  sessionReused,
			ModelName:      resolveRuntimeModelName(cfg),
			ConversationDB: sess.HistoryFile,
			LastRole:       startupLastRole(state),
			LastSummary:    startupLastSummary(state),
		},
	})
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
	buildRegistry := d.buildToolRegistry
	if buildRegistry == nil {
		buildRegistry = tools.BuiltinRegistry
	}

	return buildRegistry()
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

	result, err := runner.Run(ctx, contextstore.New(historyFile), runtime.Input{
		Prompt:       args.Prompt,
		Model:        resolveRuntimeModelName(cfg),
		SystemPrompt: subagent.SystemPrompt,
	})
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("run subagent %q: %w", args.SubagentName, err)
	}

	return runtime.ToolExecution{
		Call:   call,
		Output: finalAssistantText(result),
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
		"  fimi [--continue] [--model <alias>] [--help] [prompt...]",
		"  fimi [options] -- [prompt text starting with flags]",
	}
}

func helpFlagLines() []string {
	return []string{
		"  --continue, -C   Continue the previous session for this work dir",
		"  --new-session    Explicitly start a fresh session for this run",
		"  --model <alias>  Override the configured model for this run",
		"  -h, --help       Show this help message",
	}
}

func helpPromptRuleLines() []string {
	return []string{
		"  --                Stop parsing flags; everything after it is prompt text",
		"  prompt...         Remaining args are joined into the shell's initial prompt",
	}
}

func helpExampleLines() []string {
	return []string{
		"  fimi fix the flaky test",
		"  fimi --continue continue the refactor from the last session",
		"  fimi --model fast-model refactor the session loader",
		"  fimi -- --help should be treated as prompt text",
	}
}
