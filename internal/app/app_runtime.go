package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/dmail"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/mcp"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/printui"
	"fimi-cli/internal/ui/shell"
	"fimi-cli/internal/webfetch"
	"fimi-cli/internal/websearch"
)

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
		ReplyHistoryTurnLimit:       cfg.HistoryWindow.RuntimeTurns,
		MaxStepsPerRun:              cfg.LoopControl.MaxStepsPerRun,
		MaxAdditionalRetriesPerStep: cfg.LoopControl.MaxAdditionalRetriesPerStep,
		ContextWindowTokens:         resolveContextWindowTokens(modelCfg, err),
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

func (d dependencies) buildEngineForAgentWithTools(cfg config.Config, agent loadedAgent, definitions []tools.Definition) (llm.Engine, error) {
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
	llmCfg.Tools = buildLLMToolDefinitions(definitions)

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

	var denwaRenji *dmail.DenwaRenji
	if containsTool(agent.Tools, tools.ToolSendDMail) {
		denwaRenji = dmail.NewDenwaRenji()
		toolHandlers[tools.ToolSendDMail] = tools.NewSendDMailHandler(denwaRenji)
	}

	bgMgr := tools.NewBackgroundManager()
	toolExecutor := tools.NewBuiltinExecutor(
		allTools,
		workDir,
		bgMgr,
		tools.WithExtraHandlers(toolHandlers),
	)

	runner := runtime.NewWithToolExecutor(engine, toolExecutor, buildRuntimeConfig(cfg, agent))
	if denwaRenji != nil {
		runner = runner.WithDMailer(denwaRenji)
	}

	return &mcpAwareRunner{
		Runner:     runner,
		mcpManager: mcpManager,
		bgMgr:      bgMgr,
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

// mcpAwareRunner wraps a runtime.Runner and owns the process-level resources it uses.
type mcpAwareRunner struct {
	runtime.Runner
	mcpManager *mcp.Manager
	bgMgr      *tools.BackgroundManager
}

func (r *mcpAwareRunner) Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error) {
	return r.Runner.Run(ctx, store, input)
}

// Inner 返回内嵌的 runtime.Runner。
func (r *mcpAwareRunner) Inner() runtime.Runner { return r.Runner }

func (r *mcpAwareRunner) Close() {
	if r.bgMgr != nil {
		r.bgMgr.Close()
		r.bgMgr = nil
	}
	if r.mcpManager != nil {
		_ = r.mcpManager.Close()
		r.mcpManager = nil
	}
}

func (r *mcpAwareRunner) BackgroundTaskManager() shell.TaskManager {
	return r.bgMgr
}

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
	return webfetch.NewHTTPFetcher(webfetch.HTTPFetcherConfig{})
}

func (d dependencies) runRuntime(
	ctx context.Context,
	runner runtimeRunner,
	store contextstore.Context,
	input runtime.Input,
) (runtime.Result, error) {
	return ui.Run(ctx, runner.Run, store, input, d.resolveVisualizer("text"))
}

func (d dependencies) resolveVisualizer(mode string) ui.VisualizeFunc {
	buildVisualizer := d.buildVisualizer
	if buildVisualizer == nil {
		buildVisualizer = defaultRuntimeVisualizer
	}

	return buildVisualizer(mode, os.Stdout)
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
