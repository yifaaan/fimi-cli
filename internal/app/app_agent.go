package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"fimi-cli/internal/agentspec"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
	"fimi-cli/internal/tools"
)

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
	defer closeRuntimeRunner(runner)

	result, err := runSubagentOnce(ctx, runner, historyFile, buildRuntimePromptInput(cfg, subagent, args.Prompt))
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("run subagent %q: %w", args.SubagentName, err)
	}
	if result.Status == runtime.RunStatusMaxSteps {
		return runtime.ToolExecution{}, fmt.Errorf(
			"subagent %q reached max steps. Please try splitting the task into smaller subtasks",
			args.SubagentName,
		)
	}

	output := finalAssistantText(result)
	if output == "" {
		return runtime.ToolExecution{}, fmt.Errorf(
			"subagent %q did not produce any output", args.SubagentName,
		)
	}

	if len(output) < subagentMinResponseLen {
		contResult, contErr := runSubagentOnce(ctx, runner, historyFile, buildRuntimePromptInput(cfg, subagent, subagentContinuePrompt))
		if contErr == nil && contResult.Status != runtime.RunStatusFailed {
			if continued := finalAssistantText(contResult); continued != "" {
				output = continued
			}
		}
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
