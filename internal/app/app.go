package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"fimi-cli/internal/agentspec"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/shell"
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

type closeableRuntimeRunner interface {
	Close()
}

type backgroundTaskManagingRunner interface {
	BackgroundTaskManager() shell.TaskManager
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

func closeRuntimeRunner(runner runtimeRunner) {
	if runner == nil {
		return
	}

	closeable, ok := runner.(closeableRuntimeRunner)
	if !ok {
		return
	}

	closeable.Close()
}

func backgroundTaskManagerFromRunner(runner runtimeRunner) shell.TaskManager {
	if runner == nil {
		return nil
	}

	taskManaging, ok := runner.(backgroundTaskManagingRunner)
	if !ok {
		return nil
	}

	return taskManaging.BackgroundTaskManager()
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
