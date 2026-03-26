package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fimi-cli/internal/agentspec"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/printui"
	"fimi-cli/internal/ui/shell"
)

func testLoadedAgent(prompt string) loadedAgent {
	return loadedAgent{
		SystemPrompt: prompt,
		Tools: []tools.Definition{
			{
				Name:        tools.ToolReadFile,
				Kind:        tools.KindFile,
				Description: "Read a file from the workspace.",
			},
		},
	}
}

func testAgentLoader(prompt string) agentLoader {
	return func(workDir string, registry tools.Registry) (loadedAgent, error) {
		return testLoadedAgent(prompt), nil
	}
}

func TestBuildLLMConfig(t *testing.T) {
	cfg := config.Config{
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 5,
		},
	}

	got := buildLLMConfig(cfg)
	if got.HistoryTurnLimit != 5 {
		t.Fatalf("buildLLMConfig().HistoryTurnLimit = %d, want %d", got.HistoryTurnLimit, 5)
	}
}

func TestBuildLLMToolDefinitions(t *testing.T) {
	got := buildLLMToolDefinitions([]tools.Definition{
		{
			Name:        tools.ToolBash,
			Description: "Run a shell command inside the workspace.",
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(buildLLMToolDefinitions()) = %d, want 1", len(got))
	}
	if got[0].Name != tools.ToolBash {
		t.Fatalf("tool name = %q, want %q", got[0].Name, tools.ToolBash)
	}
	if got[0].Description != "Run a shell command inside the workspace." {
		t.Fatalf("tool description = %q, want %q", got[0].Description, "Run a shell command inside the workspace.")
	}
	properties, ok := got[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got[0].Parameters["properties"])
	}
	if _, ok := properties["command"]; !ok {
		t.Fatalf("tool parameters missing %q property", "command")
	}
}

func TestBuildLLMToolDefinitionsForAgentTool(t *testing.T) {
	got := buildLLMToolDefinitions([]tools.Definition{
		{
			Name:        tools.ToolAgent,
			Description: "Run a declared subagent for a focused task.",
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(buildLLMToolDefinitions()) = %d, want 1", len(got))
	}
	properties, ok := got[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got[0].Parameters["properties"])
	}
	for _, name := range []string{"description", "prompt", "subagent_name"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("tool parameters missing %q property", name)
		}
	}
}

func TestBuildLLMToolDefinitionsForThinkTool(t *testing.T) {
	got := buildLLMToolDefinitions([]tools.Definition{
		{
			Name:        tools.ToolThink,
			Description: "Log a private reasoning note without changing workspace state.",
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(buildLLMToolDefinitions()) = %d, want 1", len(got))
	}
	properties, ok := got[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got[0].Parameters["properties"])
	}
	if _, ok := properties["thought"]; !ok {
		t.Fatalf("tool parameters missing %q property", "thought")
	}
}

func TestBuildLLMToolDefinitionsForSetTodoListTool(t *testing.T) {
	got := buildLLMToolDefinitions([]tools.Definition{
		{
			Name:        tools.ToolSetTodoList,
			Description: "Update the whole todo list.",
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(buildLLMToolDefinitions()) = %d, want 1", len(got))
	}
	defs, ok := got[0].Parameters["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters $defs type = %T, want map[string]any", got[0].Parameters["$defs"])
	}
	if _, ok := defs["Todo"]; !ok {
		t.Fatalf("tool parameters missing %q definition", "Todo")
	}
	properties, ok := got[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got[0].Parameters["properties"])
	}
	if _, ok := properties["todos"]; !ok {
		t.Fatalf("tool parameters missing %q property", "todos")
	}
}

func TestBuildLLMToolDefinitionsForSearchWebTool(t *testing.T) {
	got := buildLLMToolDefinitions([]tools.Definition{{
		Name:        tools.ToolSearchWeb,
		Description: "Search the web for recent information and relevant pages.",
	}})

	if len(got) != 1 {
		t.Fatalf("len(buildLLMToolDefinitions()) = %d, want 1", len(got))
	}
	properties, ok := got[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got[0].Parameters["properties"])
	}
	for _, name := range []string{"query", "limit", "include_content"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("tool parameters missing %q property", name)
		}
	}
	required, ok := got[0].Parameters["required"].([]string)
	if !ok {
		t.Fatalf("tool parameters required type = %T, want []string", got[0].Parameters["required"])
	}
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("tool parameters required = %#v, want []string{\"query\"}", required)
	}
}

func TestBuildWebSearcherReturnsNilWhenDisabled(t *testing.T) {
	searcher, err := buildWebSearcher(config.Default())
	if err != nil {
		t.Fatalf("buildWebSearcher() error = %v", err)
	}
	if searcher != nil {
		t.Fatalf("buildWebSearcher() = %#v, want nil", searcher)
	}
}

func TestBuildWebSearcherBuildsDuckDuckGoSearcherWhenEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Web.Enabled = true

	searcher, err := buildWebSearcher(cfg)
	if err != nil {
		t.Fatalf("buildWebSearcher() error = %v", err)
	}
	if searcher == nil {
		t.Fatalf("buildWebSearcher() = nil, want non-nil")
	}
}

func TestBuildRuntimeConfig(t *testing.T) {
	cfg := config.Config{
		LoopControl: config.LoopControl{
			MaxStepsPerRun:    9,
			MaxRetriesPerStep: 4,
		},
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 7,
		},
		DefaultModel: "primary",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider:            config.ProviderTypePlaceholder,
				Model:               "primary",
				ContextWindowTokens: 128000,
			},
		},
	}

	got := buildRuntimeConfig(cfg, loadedAgent{})
	if got.ReplyHistoryTurnLimit != 7 {
		t.Fatalf("buildRuntimeConfig().ReplyHistoryTurnLimit = %d, want %d", got.ReplyHistoryTurnLimit, 7)
	}
	if got.MaxStepsPerRun != 9 {
		t.Fatalf("buildRuntimeConfig().MaxStepsPerRun = %d, want %d", got.MaxStepsPerRun, 9)
	}
	if got.MaxRetriesPerStep != 4 {
		t.Fatalf("buildRuntimeConfig().MaxRetriesPerStep = %d, want %d", got.MaxRetriesPerStep, 4)
	}
	if got.ContextWindowTokens != 128000 {
		t.Fatalf("buildRuntimeConfig().ContextWindowTokens = %d, want %d", got.ContextWindowTokens, 128000)
	}
}

func TestBuildRuntimeInput(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input, testLoadedAgent("You are the configured agent."))
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "custom-model",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimeInputUsesAgentModelOverride(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "actual-default-model",
			},
			"reviewer": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "actual-reviewer-model",
			},
		},
	}
	input := runInput{
		prompt: "fix the test",
	}
	agent := testLoadedAgent("You are the configured agent.")
	agent.Spec.Model = "reviewer"

	got := buildRuntimeInput(cfg, input, agent)
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "actual-reviewer-model",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestApplyRunInputToConfig(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "default-model",
			},
			"fast-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "fast-model",
			},
		},
	}

	got, err := applyRunInputToConfig(cfg, runInput{modelAlias: "fast-model"})
	if err != nil {
		t.Fatalf("applyRunInputToConfig() error = %v", err)
	}
	if got.DefaultModel != "fast-model" {
		t.Fatalf("applyRunInputToConfig().DefaultModel = %q, want %q", got.DefaultModel, "fast-model")
	}
	if cfg.DefaultModel != "default-model" {
		t.Fatalf("original cfg.DefaultModel = %q, want %q", cfg.DefaultModel, "default-model")
	}
}

func TestApplyRunInputToConfigReturnsErrorForUnknownModelAlias(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "default-model",
			},
		},
	}

	_, err := applyRunInputToConfig(cfg, runInput{modelAlias: "missing-model"})
	if err == nil {
		t.Fatalf("applyRunInputToConfig() error = nil, want non-nil")
	}
	if err.Error() != `model "missing-model" not found in config.models` {
		t.Fatalf("applyRunInputToConfig() error = %q, want %q", err.Error(), `model "missing-model" not found in config.models`)
	}
}

func TestSummarizeStartupContentCompactsWhitespaceAndTruncates(t *testing.T) {
	got := summarizeStartupContent("line one\n\n   line two   line three", 18)
	if got != "line one line t..." {
		t.Fatalf("summarizeStartupContent() = %q, want %q", got, "line one line t...")
	}
}

func TestBuildRuntimeInputUsesConfiguredModelName(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "primary",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider: config.ProviderTypeQWEN,
				Model:    "qwen-plus",
			},
		},
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input, testLoadedAgent("You are the configured agent."))
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "qwen-plus",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimeInputFallsBackToModelAliasWhenModelNameEmpty(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "primary",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider: config.ProviderTypeQWEN,
			},
		},
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input, testLoadedAgent("You are the configured agent."))
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "primary",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimeConfigUsesAgentModelOverride(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider:            config.ProviderTypePlaceholder,
				Model:               "default-model",
				ContextWindowTokens: 128000,
			},
			"reviewer": {
				Provider:            config.ProviderTypePlaceholder,
				Model:               "reviewer-model",
				ContextWindowTokens: 64000,
			},
		},
	}
	agent := loadedAgent{
		Spec: agentspec.Spec{
			Model: "reviewer",
		},
	}

	got := buildRuntimeConfig(cfg, agent)
	if got.ContextWindowTokens != 64000 {
		t.Fatalf("buildRuntimeConfig().ContextWindowTokens = %d, want %d", got.ContextWindowTokens, 64000)
	}
}

func TestDefaultAgentFile(t *testing.T) {
	got := defaultAgentFile("/tmp/fimi-project")
	want := filepath.Join("/tmp/fimi-project", defaultAgentsDirName, defaultAgentProfileName, defaultAgentFileName)
	if got != want {
		t.Fatalf("defaultAgentFile() = %q, want %q", got, want)
	}
}

func TestLoadAgentFromWorkDir(t *testing.T) {
	workDir := t.TempDir()
	agentDir := filepath.Dir(defaultAgentFile(workDir))
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", agentDir, err)
	}

	if err := os.WriteFile(defaultAgentFile(workDir), []byte(`
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - bash
    - read_file
`), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "system.md"), []byte("  You are the test agent.  \n"), 0o644); err != nil {
		t.Fatalf("WriteFile(system.md) error = %v", err)
	}

	got, err := loadAgentFromWorkDir(workDir, tools.BuiltinRegistry())
	if err != nil {
		t.Fatalf("loadAgentFromWorkDir() error = %v", err)
	}
	if got.Spec.Name != "Test Agent" {
		t.Fatalf("loadAgentFromWorkDir().Spec.Name = %q, want %q", got.Spec.Name, "Test Agent")
	}
	if got.Spec.SystemPromptPath != filepath.Join(agentDir, "system.md") {
		t.Fatalf("loadAgentFromWorkDir().Spec.SystemPromptPath = %q, want %q", got.Spec.SystemPromptPath, filepath.Join(agentDir, "system.md"))
	}
	if !reflect.DeepEqual(got.Spec.Tools, []string{"bash", "read_file"}) {
		t.Fatalf("loadAgentFromWorkDir().Spec.Tools = %#v, want %#v", got.Spec.Tools, []string{"bash", "read_file"})
	}
	if got.SystemPrompt != "You are the test agent." {
		t.Fatalf("loadAgentFromWorkDir().SystemPrompt = %q, want %q", got.SystemPrompt, "You are the test agent.")
	}
	if len(got.Tools) != 2 {
		t.Fatalf("len(loadAgentFromWorkDir().Tools) = %d, want %d", len(got.Tools), 2)
	}
	if got.Tools[0].Name != tools.ToolBash {
		t.Fatalf("loadAgentFromWorkDir().Tools[0].Name = %q, want %q", got.Tools[0].Name, tools.ToolBash)
	}
	if got.Tools[1].Name != tools.ToolReadFile {
		t.Fatalf("loadAgentFromWorkDir().Tools[1].Name = %q, want %q", got.Tools[1].Name, tools.ToolReadFile)
	}
}

func TestLoadAgentFromWorkDirFiltersExcludedTools(t *testing.T) {
	workDir := t.TempDir()
	agentDir := filepath.Dir(defaultAgentFile(workDir))
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", agentDir, err)
	}

	if err := os.WriteFile(defaultAgentFile(workDir), []byte(`
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - bash
    - read_file
    - grep
  exclude_tools:
    - read_file
    - missing_tool
  subagents:
    reviewer:
      path: ./reviewer.yaml
      description: Review code
`), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "system.md"), []byte("You are the test agent.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(system.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.yaml"), []byte("placeholder\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.yaml) error = %v", err)
	}

	got, err := loadAgentFromWorkDir(workDir, tools.BuiltinRegistry())
	if err != nil {
		t.Fatalf("loadAgentFromWorkDir() error = %v", err)
	}

	if !reflect.DeepEqual(got.Spec.ExcludeTools, []string{"read_file", "missing_tool"}) {
		t.Fatalf("loadAgentFromWorkDir().Spec.ExcludeTools = %#v, want %#v", got.Spec.ExcludeTools, []string{"read_file", "missing_tool"})
	}

	wantSubagents := map[string]agentspec.SubagentSpec{
		"reviewer": {
			Path:        filepath.Join(agentDir, "reviewer.yaml"),
			Description: "Review code",
		},
	}
	if !reflect.DeepEqual(got.Spec.Subagents, wantSubagents) {
		t.Fatalf("loadAgentFromWorkDir().Spec.Subagents = %#v, want %#v", got.Spec.Subagents, wantSubagents)
	}

	if len(got.Tools) != 2 {
		t.Fatalf("len(loadAgentFromWorkDir().Tools) = %d, want %d", len(got.Tools), 2)
	}
	if got.Tools[0].Name != tools.ToolBash {
		t.Fatalf("loadAgentFromWorkDir().Tools[0].Name = %q, want %q", got.Tools[0].Name, tools.ToolBash)
	}
	if got.Tools[1].Name != tools.ToolGrep {
		t.Fatalf("loadAgentFromWorkDir().Tools[1].Name = %q, want %q", got.Tools[1].Name, tools.ToolGrep)
	}
}

func TestLoadAgentFromWorkDirReturnsErrorForUnknownTool(t *testing.T) {
	workDir := t.TempDir()
	agentDir := filepath.Dir(defaultAgentFile(workDir))
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", agentDir, err)
	}

	if err := os.WriteFile(defaultAgentFile(workDir), []byte(`
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - missing_tool
`), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "system.md"), []byte("You are the test agent.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(system.md) error = %v", err)
	}

	_, err := loadAgentFromWorkDir(workDir, tools.BuiltinRegistry())
	if err == nil {
		t.Fatalf("loadAgentFromWorkDir() error = nil, want non-nil")
	}
	if !errors.Is(err, tools.ErrToolNotRegistered) {
		t.Fatalf("loadAgentFromWorkDir() error = %v, want wrapped %v", err, tools.ErrToolNotRegistered)
	}
}

func TestLoadDeclaredSubagent(t *testing.T) {
	dir := t.TempDir()
	subagentFile := filepath.Join(dir, "reviewer.yaml")
	if err := os.WriteFile(subagentFile, []byte(`
version: 1
agent:
  name: Reviewer Agent
  system_prompt_path: ./reviewer.md
  tools:
    - bash
    - read_file
    - grep
  exclude_tools:
    - bash
`), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("  You are the reviewer agent. \n"), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.md) error = %v", err)
	}

	root := loadedAgent{
		Spec: agentspec.Spec{
			Name: "Root Agent",
			Subagents: map[string]agentspec.SubagentSpec{
				"reviewer": {
					Path:        subagentFile,
					Description: "Review code",
				},
			},
		},
	}

	got, err := loadDeclaredSubagent(root, "reviewer", tools.BuiltinRegistry())
	if err != nil {
		t.Fatalf("loadDeclaredSubagent() error = %v", err)
	}
	if got.Spec.Name != "Reviewer Agent" {
		t.Fatalf("loadDeclaredSubagent().Spec.Name = %q, want %q", got.Spec.Name, "Reviewer Agent")
	}
	if got.SystemPrompt != "You are the reviewer agent." {
		t.Fatalf("loadDeclaredSubagent().SystemPrompt = %q, want %q", got.SystemPrompt, "You are the reviewer agent.")
	}
	if !reflect.DeepEqual(got.Spec.ExcludeTools, []string{"bash"}) {
		t.Fatalf("loadDeclaredSubagent().Spec.ExcludeTools = %#v, want %#v", got.Spec.ExcludeTools, []string{"bash"})
	}
	if len(got.Tools) != 2 {
		t.Fatalf("len(loadDeclaredSubagent().Tools) = %d, want %d", len(got.Tools), 2)
	}
	if got.Tools[0].Name != tools.ToolReadFile {
		t.Fatalf("loadDeclaredSubagent().Tools[0].Name = %q, want %q", got.Tools[0].Name, tools.ToolReadFile)
	}
	if got.Tools[1].Name != tools.ToolGrep {
		t.Fatalf("loadDeclaredSubagent().Tools[1].Name = %q, want %q", got.Tools[1].Name, tools.ToolGrep)
	}
}

func TestLoadDeclaredSubagentReturnsErrorForUnknownName(t *testing.T) {
	root := loadedAgent{
		Spec: agentspec.Spec{
			Name:      "Root Agent",
			Subagents: map[string]agentspec.SubagentSpec{},
		},
	}

	_, err := loadDeclaredSubagent(root, "reviewer", tools.BuiltinRegistry())
	if err == nil {
		t.Fatalf("loadDeclaredSubagent() error = nil, want non-nil")
	}
	if !errors.Is(err, ErrSubagentNotDeclared) {
		t.Fatalf("loadDeclaredSubagent() error = %v, want wrapped %v", err, ErrSubagentNotDeclared)
	}
}

func TestLoadDeclaredSubagentReturnsErrorForUnknownTool(t *testing.T) {
	dir := t.TempDir()
	subagentFile := filepath.Join(dir, "reviewer.yaml")
	if err := os.WriteFile(subagentFile, []byte(`
version: 1
agent:
  name: Reviewer Agent
  system_prompt_path: ./reviewer.md
  tools:
    - missing_tool
`), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("You are the reviewer agent.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.md) error = %v", err)
	}

	root := loadedAgent{
		Spec: agentspec.Spec{
			Name: "Root Agent",
			Subagents: map[string]agentspec.SubagentSpec{
				"reviewer": {
					Path:        subagentFile,
					Description: "Review code",
				},
			},
		},
	}

	_, err := loadDeclaredSubagent(root, "reviewer", tools.BuiltinRegistry())
	if err == nil {
		t.Fatalf("loadDeclaredSubagent() error = nil, want non-nil")
	}
	if !errors.Is(err, tools.ErrToolNotRegistered) {
		t.Fatalf("loadDeclaredSubagent() error = %v, want wrapped %v", err, tools.ErrToolNotRegistered)
	}
}

func TestDependenciesRunDeclaredSubagentReturnsFinalAssistantTextAndUsesIsolatedHistory(t *testing.T) {
	workDir := t.TempDir()
	rootHistory := contextstore.New(filepath.Join(workDir, "root-history.jsonl"))
	if err := rootHistory.Append(contextstore.NewUserTextRecord("root prompt")); err != nil {
		t.Fatalf("Append(root user) error = %v", err)
	}
	if err := rootHistory.Append(contextstore.NewAssistantTextRecord("root reply")); err != nil {
		t.Fatalf("Append(root assistant) error = %v", err)
	}

	subagentFile := filepath.Join(workDir, "reviewer.yaml")
	if err := os.WriteFile(subagentFile, []byte(`
version: 1
agent:
  name: Reviewer Agent
  system_prompt_path: ./reviewer.md
  tools:
    - read_file
`), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "reviewer.md"), []byte("You are the reviewer agent.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(reviewer.md) error = %v", err)
	}

	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			return llm.NewPlaceholderClient(), nil
		},
	}
	cfg := config.Config{
		DefaultModel: "test-model",
		Models: map[string]config.ModelConfig{
			"test-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "test-model",
			},
		},
	}
	root := loadedAgent{
		Spec: agentspec.Spec{
			Name: "Root Agent",
			Subagents: map[string]agentspec.SubagentSpec{
				"reviewer": {
					Path:        subagentFile,
					Description: "Review code",
				},
			},
		},
	}

	got, err := deps.runDeclaredSubagent(
		context.Background(),
		cfg,
		root,
		workDir,
		tools.BuiltinRegistry(),
		runtime.ToolCall{
			ID:   "call/1",
			Name: tools.ToolAgent,
		},
		tools.AgentArguments{
			Description:  "review tests",
			Prompt:       "explain the test setup",
			SubagentName: "reviewer",
		},
	)
	if err != nil {
		t.Fatalf("runDeclaredSubagent() error = %v", err)
	}
	if got.Output != "assistant placeholder reply: explain the test setup" {
		t.Fatalf("runDeclaredSubagent().Output = %q, want %q", got.Output, "assistant placeholder reply: explain the test setup")
	}

	rootRecords, err := rootHistory.ReadAll()
	if err != nil {
		t.Fatalf("rootHistory.ReadAll() error = %v", err)
	}
	wantRootRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("root prompt"),
		contextstore.NewAssistantTextRecord("root reply"),
	}
	if !reflect.DeepEqual(rootRecords, wantRootRecords) {
		t.Fatalf("root history records = %#v, want %#v", rootRecords, wantRootRecords)
	}

	subagentHistoryFile, err := subagentHistoryFile(workDir, "reviewer", "call/1")
	if err != nil {
		t.Fatalf("subagentHistoryFile() error = %v", err)
	}
	subagentRecords, err := contextstore.New(subagentHistoryFile).ReadAll()
	if err != nil {
		t.Fatalf("subagent store ReadAll() error = %v", err)
	}
	wantSubagentRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("explain the test setup"),
		contextstore.NewAssistantTextRecord("assistant placeholder reply: explain the test setup"),
	}
	if !reflect.DeepEqual(subagentRecords, wantSubagentRecords) {
		t.Fatalf("subagent history records = %#v, want %#v", subagentRecords, wantSubagentRecords)
	}
}

func TestHelpText(t *testing.T) {
	got := helpText()
	want := "" +
		"Usage:\n" +
		"  fimi [--continue] [--model <alias>] [--help] [prompt...]\n" +
		"  fimi [options] -- [prompt text starting with flags]\n" +
		"\n" +
		"Flags:\n" +
		"  --continue, -C   Continue the previous session for this work dir\n" +
		"  --new-session    Explicitly start a fresh session for this run\n" +
		"  --model <alias>  Override the configured model for this run\n" +
		"  -h, --help       Show this help message\n" +
		"\n" +
		"Prompt Rules:\n" +
		"  --                Stop parsing flags; everything after it is prompt text\n" +
		"  prompt...         Remaining args are joined into the shell's initial prompt\n" +
		"\n" +
		"Examples:\n" +
		"  fimi fix the flaky test\n" +
		"  fimi --continue continue the refactor from the last session\n" +
		"  fimi --model fast-model refactor the session loader\n" +
		"  fimi -- --help should be treated as prompt text\n"

	if got != want {
		t.Fatalf("helpText() = %q, want %q", got, want)
	}
}

func TestPrintHelpWritesHelpText(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer reader.Close()

	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	printHelp()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if string(data) != helpText() {
		t.Fatalf("printHelp() output = %q, want %q", string(data), helpText())
	}
}

func TestParseRunInput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runInput
		wantErr error
	}{
		{
			name: "prompt only",
			args: []string{"fix", "tests"},
			want: runInput{
				prompt: "fix tests",
			},
		},
		{
			name: "force new session flag removed from prompt",
			args: []string{"--new-session", "fix", "tests"},
			want: runInput{
				prompt:          "fix tests",
				forceNewSession: true,
			},
		},
		{
			name: "force new session without prompt",
			args: []string{"--new-session"},
			want: runInput{
				forceNewSession: true,
			},
		},
		{
			name: "continue flag removed from prompt",
			args: []string{"--continue", "fix", "tests"},
			want: runInput{
				prompt:          "fix tests",
				continueSession: true,
			},
		},
		{
			name: "continue short flag",
			args: []string{"-C", "fix"},
			want: runInput{
				prompt:          "fix",
				continueSession: true,
			},
		},
		{
			name: "model override",
			args: []string{"--model", "fast-model", "fix", "tests"},
			want: runInput{
				prompt:     "fix tests",
				modelAlias: "fast-model",
			},
		},
		{
			name: "model override and continue",
			args: []string{"--continue", "--model", "fast-model", "fix"},
			want: runInput{
				prompt:          "fix",
				continueSession: true,
				modelAlias:      "fast-model",
			},
		},
		{
			name: "help long flag",
			args: []string{"--help"},
			want: runInput{
				showHelp: true,
			},
		},
		{
			name: "help short flag",
			args: []string{"-h"},
			want: runInput{
				showHelp: true,
			},
		},
		{
			name: "flag terminator keeps literal flag in prompt",
			args: []string{"--new-session", "--", "--new-session", "fix"},
			want: runInput{
				prompt:          "--new-session fix",
				forceNewSession: true,
			},
		},
		{
			name: "flag terminator keeps literal continue flag in prompt",
			args: []string{"--continue", "--", "--continue", "fix"},
			want: runInput{
				prompt:          "--continue fix",
				continueSession: true,
			},
		},
		{
			name: "flag terminator keeps literal help flag in prompt",
			args: []string{"--", "--help", "fix"},
			want: runInput{
				prompt: "--help fix",
			},
		},
		{
			name: "flag terminator keeps literal model flag in prompt",
			args: []string{"--", "--model", "fast-model", "fix"},
			want: runInput{
				prompt: "--model fast-model fix",
			},
		},
		{
			name:    "unknown flag",
			args:    []string{"--bad-flag", "fix"},
			wantErr: ErrUnknownCLIFlag,
		},
		{
			name:    "removed shell flag is unknown",
			args:    []string{"--shell", "fix"},
			wantErr: ErrUnknownCLIFlag,
		},
		{
			name:    "removed short shell flag is unknown",
			args:    []string{"-i", "fix"},
			wantErr: ErrUnknownCLIFlag,
		},
		{
			name:    "model flag requires value",
			args:    []string{"--model"},
			wantErr: ErrCLIFlagValueRequired,
		},
		{
			name:    "model flag rejects another flag as value",
			args:    []string{"--model", "--new-session", "fix"},
			wantErr: ErrCLIFlagValueRequired,
		},
		{
			name:    "conflicting session flags",
			args:    []string{"--new-session", "--continue", "fix"},
			wantErr: ErrConflictingSessionFlags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunInput(tt.args)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseRunInput() error = %v, want wrapped %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseRunInput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDependenciesRunUsesInjectedProcessDependencies(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var gotWorkDir string
	var gotModelAlias string
	var shellDeps shell.Dependencies
	var shellCalled bool
	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				Models: map[string]config.ModelConfig{
					"custom-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "custom-model",
					},
				},
				HistoryWindow: config.HistoryWindow{
					RuntimeTurns: 2,
					LLMTurns:     1,
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		createSession: func(workDir string) (session.Session, error) {
			gotWorkDir = workDir
			return session.Session{
				ID:          "session-123",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotModelAlias = cfg.DefaultModel
			return llm.NewPlaceholderClient(), nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			shellCalled = true
			shellDeps = deps
			return nil
		},
	}

	err := deps.run([]string{"fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotWorkDir != "/tmp/fimi-project" {
		t.Fatalf("createSession() got workDir = %q, want %q", gotWorkDir, "/tmp/fimi-project")
	}
	if gotModelAlias != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotModelAlias, "custom-model")
	}
	if !shellCalled {
		t.Fatalf("runShellUI() called = false, want true")
	}
	if shellDeps.Runner == nil {
		t.Fatalf("shell deps runner = nil, want non-nil")
	}
	if shellDeps.Store.Path() != historyFile {
		t.Fatalf("shell deps store path = %q, want %q", shellDeps.Store.Path(), historyFile)
	}
	if shellDeps.ModelName != "custom-model" {
		t.Fatalf("shell deps model = %q, want %q", shellDeps.ModelName, "custom-model")
	}
	if shellDeps.StartupInfo.SessionID != "session-123" {
		t.Fatalf("shell startup session = %q, want %q", shellDeps.StartupInfo.SessionID, "session-123")
	}
	if shellDeps.StartupInfo.SessionReused {
		t.Fatalf("shell startup session reused = true, want false")
	}
	if shellDeps.StartupInfo.ConversationDB != historyFile {
		t.Fatalf("shell startup history = %q, want %q", shellDeps.StartupInfo.ConversationDB, historyFile)
	}
	if shellDeps.StartupInfo.LastSummary != "" {
		t.Fatalf("shell startup last summary = %q, want empty for new session", shellDeps.StartupInfo.LastSummary)
	}
	if shellDeps.SystemPrompt != "You are the configured agent." {
		t.Fatalf("shell deps system prompt = %q, want %q", shellDeps.SystemPrompt, "You are the configured agent.")
	}
	if shellDeps.InitialPrompt != "fix tests" {
		t.Fatalf("shell deps initial prompt = %q, want %q", shellDeps.InitialPrompt, "fix tests")
	}

	ctx := contextstore.New(historyFile)
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord(initialRecordContent),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}
}

func TestDependenciesRunPrintsHelpBeforeLoadingConfig(t *testing.T) {
	var loadConfigCalled bool
	var resolveWorkDirCalled bool
	var createSessionCalled bool
	var buildRunnerCalled bool
	var helpCalled bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			loadConfigCalled = true
			return config.Config{}, nil
		},
		resolveWorkDir: func() (string, error) {
			resolveWorkDirCalled = true
			return "", nil
		},
		createSession: func(workDir string) (session.Session, error) {
			createSessionCalled = true
			return session.Session{}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			buildRunnerCalled = true
			return &stubRunner{}, nil
		},
		printHelp: func() {
			helpCalled = true
		},
	}

	err := deps.run([]string{"--help"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !helpCalled {
		t.Fatalf("printHelp() called = false, want true")
	}
	if loadConfigCalled {
		t.Fatalf("loadConfig() called = true, want false")
	}
	if resolveWorkDirCalled {
		t.Fatalf("resolveWorkDir() called = true, want false")
	}
	if createSessionCalled {
		t.Fatalf("createSession() called = true, want false")
	}
	if buildRunnerCalled {
		t.Fatalf("buildRuntimeRunner() called = true, want false")
	}
}

func TestDependenciesRunDelegatesToShellByDefault(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var gotDeps shell.Dependencies
	var printedState bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "default-model",
				Models: map[string]config.ModelConfig{
					"default-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "actual-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		createSession: func(workDir string) (session.Session, error) {
			return session.Session{
				ID:          "session-123",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			return &stubRunner{}, nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			gotDeps = deps
			return nil
		},
		printStartupState: func(
			sess session.Session,
			ctx contextstore.Context,
			state startupState,
			sessionReused bool,
			model string,
		) {
			printedState = true
		},
	}

	if err := deps.run([]string{"fix", "tests"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotDeps.Runner == nil {
		t.Fatalf("shell deps runner = nil, want non-nil")
	}
	if gotDeps.Store.Path() != historyFile {
		t.Fatalf("shell deps store path = %q, want %q", gotDeps.Store.Path(), historyFile)
	}
	if gotDeps.ModelName != "actual-model" {
		t.Fatalf("shell deps model = %q, want %q", gotDeps.ModelName, "actual-model")
	}
	wantHistoryFile, err := session.ShellHistoryFileForWorkDir("/tmp/fimi-project")
	if err != nil {
		t.Fatalf("ShellHistoryFileForWorkDir() error = %v", err)
	}
	if gotDeps.HistoryFile != wantHistoryFile {
		t.Fatalf("shell deps history file = %q, want %q", gotDeps.HistoryFile, wantHistoryFile)
	}
	if gotDeps.StartupInfo.SessionID != "session-123" {
		t.Fatalf("shell startup session = %q, want %q", gotDeps.StartupInfo.SessionID, "session-123")
	}
	if gotDeps.StartupInfo.SessionReused {
		t.Fatalf("shell startup session reused = true, want false")
	}
	if gotDeps.StartupInfo.ModelName != "actual-model" {
		t.Fatalf("shell startup model = %q, want %q", gotDeps.StartupInfo.ModelName, "actual-model")
	}
	if gotDeps.StartupInfo.ConversationDB != historyFile {
		t.Fatalf("shell startup history = %q, want %q", gotDeps.StartupInfo.ConversationDB, historyFile)
	}
	if gotDeps.SystemPrompt != "You are the configured agent." {
		t.Fatalf("shell deps system prompt = %q, want %q", gotDeps.SystemPrompt, "You are the configured agent.")
	}
	if gotDeps.InitialPrompt != "fix tests" {
		t.Fatalf("shell deps initial prompt = %q, want %q", gotDeps.InitialPrompt, "fix tests")
	}
	if printedState {
		t.Fatalf("printStartupState() called = true, want false in shell mode")
	}

	records, err := contextstore.New(historyFile).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord(initialRecordContent),
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}
}

func TestDependenciesRunAppliesModelOverrideToRunnerAndShell(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var gotRunnerCfg config.Config
	var shellModel string

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "default-model",
				Models: map[string]config.ModelConfig{
					"default-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "default-model",
					},
					"fast-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "actual-fast-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		createSession: func(workDir string) (session.Session, error) {
			return session.Session{
				ID:          "session-123",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			gotRunnerCfg = cfg
			return &stubRunner{}, nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			shellModel = deps.ModelName
			return nil
		},
	}

	err := deps.run([]string{"--model", "fast-model", "fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotRunnerCfg.DefaultModel != "fast-model" {
		t.Fatalf("runner cfg.DefaultModel = %q, want %q", gotRunnerCfg.DefaultModel, "fast-model")
	}
	if shellModel != "actual-fast-model" {
		t.Fatalf("shell model = %q, want %q", shellModel, "actual-fast-model")
	}
}

func TestDependenciesRunCreatesNewSessionByDefault(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var createCalled bool
	var gotCreateWorkDir string
	var shellDeps shell.Dependencies
	var shellCalled bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				Models: map[string]config.ModelConfig{
					"custom-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "custom-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		createSession: func(workDir string) (session.Session, error) {
			createCalled = true
			gotCreateWorkDir = workDir
			return session.Session{
				ID:          "session-new",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			return &stubRunner{}, nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			shellCalled = true
			shellDeps = deps
			return nil
		},
	}

	err := deps.run([]string{"fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !createCalled {
		t.Fatalf("createSession() called = false, want true")
	}
	if gotCreateWorkDir != "/tmp/fimi-project" {
		t.Fatalf("createSession() got workDir = %q, want %q", gotCreateWorkDir, "/tmp/fimi-project")
	}
	if !shellCalled {
		t.Fatalf("runShellUI() called = false, want true")
	}
	if shellDeps.Store.Path() != historyFile {
		t.Fatalf("shell deps store path = %q, want %q", shellDeps.Store.Path(), historyFile)
	}
	if shellDeps.StartupInfo.SessionID != "session-new" {
		t.Fatalf("shell startup session = %q, want %q", shellDeps.StartupInfo.SessionID, "session-new")
	}
	if shellDeps.StartupInfo.SessionReused {
		t.Fatalf("shell startup session reused = true, want false")
	}
}

func TestDependenciesRunContinuesSessionWhenRequested(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	store := contextstore.New(historyFile)
	if _, err := store.Bootstrap(buildInitialRecord()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if err := store.Append(contextstore.NewUserTextRecord("continue the refactor")); err != nil {
		t.Fatalf("Append(user) error = %v", err)
	}
	if err := store.Append(contextstore.NewAssistantTextRecord("picked up\nfrom the latest checkpoint")); err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}

	var continueCalled bool
	var createCalled bool
	var gotContinueWorkDir string
	var shellDeps shell.Dependencies
	var shellCalled bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				Models: map[string]config.ModelConfig{
					"custom-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "custom-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		continueSession: func(workDir string) (session.Session, error) {
			continueCalled = true
			gotContinueWorkDir = workDir
			return session.Session{
				ID:          "session-old",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		createSession: func(workDir string) (session.Session, error) {
			createCalled = true
			return session.Session{}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			return &stubRunner{}, nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			shellCalled = true
			shellDeps = deps
			return nil
		},
	}

	err := deps.run([]string{"--continue", "fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if createCalled {
		t.Fatalf("createSession() called = true, want false")
	}
	if !continueCalled {
		t.Fatalf("continueSession() called = false, want true")
	}
	if gotContinueWorkDir != "/tmp/fimi-project" {
		t.Fatalf("continueSession() got workDir = %q, want %q", gotContinueWorkDir, "/tmp/fimi-project")
	}
	if !shellCalled {
		t.Fatalf("runShellUI() called = false, want true")
	}
	if shellDeps.Store.Path() != historyFile {
		t.Fatalf("shell deps store path = %q, want %q", shellDeps.Store.Path(), historyFile)
	}
	if shellDeps.StartupInfo.SessionID != "session-old" {
		t.Fatalf("shell startup session = %q, want %q", shellDeps.StartupInfo.SessionID, "session-old")
	}
	if !shellDeps.StartupInfo.SessionReused {
		t.Fatalf("shell startup session reused = false, want true")
	}
	if shellDeps.StartupInfo.LastRole != contextstore.RoleAssistant {
		t.Fatalf("shell startup last role = %q, want %q", shellDeps.StartupInfo.LastRole, contextstore.RoleAssistant)
	}
	if shellDeps.StartupInfo.LastSummary != "picked up from the latest checkpoint" {
		t.Fatalf("shell startup last summary = %q, want %q", shellDeps.StartupInfo.LastSummary, "picked up from the latest checkpoint")
	}
}

func TestDependenciesRunUsesInjectedRunnerBuilderForShell(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	runner := &stubRunner{}
	var gotCfg config.Config
	var gotShellDeps shell.Dependencies

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				HistoryWindow: config.HistoryWindow{
					RuntimeTurns: 4,
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		loadAgent: testAgentLoader("You are the configured agent."),
		createSession: func(workDir string) (session.Session, error) {
			return session.Session{
				ID:          "session-456",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			gotCfg = cfg
			return runner, nil
		},
		runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
			gotShellDeps = deps
			return nil
		},
	}

	err := deps.run([]string{"fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("runner builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if gotShellDeps.Runner != runner {
		t.Fatalf("shell deps runner = %#v, want injected runner %#v", gotShellDeps.Runner, runner)
	}
	if gotShellDeps.Store.Path() != historyFile {
		t.Fatalf("shell deps store path = %q, want %q", gotShellDeps.Store.Path(), historyFile)
	}
	if gotShellDeps.InitialPrompt != "fix tests" {
		t.Fatalf("shell deps initial prompt = %q, want %q", gotShellDeps.InitialPrompt, "fix tests")
	}
}

func TestDependenciesRunWrapsBoundaryErrors(t *testing.T) {
	errConfigFailed := errors.New("config failed")
	errGetWDFailed := errors.New("getwd failed")
	errParseInputFailed := ErrUnknownCLIFlag
	errFlagValueRequired := ErrCLIFlagValueRequired
	errContinueSessionFailed := errors.New("continue session failed")
	errCreateSessionFailed := errors.New("create session failed")
	errLoadAgentFailed := errors.New("load agent failed")
	errBuildRunnerFailed := errors.New("build runner failed")
	errShellUIFailed := errors.New("shell ui failed")

	tests := []struct {
		name        string
		setup       func(t *testing.T) dependencies
		wantErr     error
		wantErrText string
	}{
		{
			name: "load config",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Config{}, errConfigFailed
					},
				}
			},
			wantErr: errConfigFailed,
		},
		{
			name: "parse input",
			setup: func(t *testing.T) dependencies {
				return dependencies{}
			},
			wantErr: errParseInputFailed,
		},
		{
			name: "flag value required",
			setup: func(t *testing.T) dependencies {
				return dependencies{}
			},
			wantErr: errFlagValueRequired,
		},
		{
			name: "apply model override",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Config{
							DefaultModel: "default-model",
							Models: map[string]config.ModelConfig{
								"default-model": {
									Provider: config.ProviderTypePlaceholder,
									Model:    "default-model",
								},
							},
						}, nil
					},
				}
			},
			wantErrText: `model "missing-model" not found in config.models`,
		},
		{
			name: "resolve work dir",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "", errGetWDFailed
					},
				}
			},
			wantErr: errGetWDFailed,
		},
		{
			name: "continue session",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: testAgentLoader("You are the configured agent."),
					continueSession: func(workDir string) (session.Session, error) {
						return session.Session{}, errContinueSessionFailed
					},
				}
			},
			wantErr: errContinueSessionFailed,
		},
		{
			name: "continue session with no previous session",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: testAgentLoader("You are the configured agent."),
					continueSession: func(workDir string) (session.Session, error) {
						return session.Session{}, fmt.Errorf("%w for work dir %q", session.ErrNoPreviousSession, workDir)
					},
				}
			},
			wantErr:     session.ErrNoPreviousSession,
			wantErrText: `no previous session found for work dir "/tmp/fimi-project"; rerun without --continue to start a new session: no previous session`,
		},
		{
			name: "load agent",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: func(workDir string, registry tools.Registry) (loadedAgent, error) {
						return loadedAgent{}, errLoadAgentFailed
					},
				}
			},
			wantErr: errLoadAgentFailed,
		},
		{
			name: "create session",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: testAgentLoader("You are the configured agent."),
					createSession: func(workDir string) (session.Session, error) {
						return session.Session{}, errCreateSessionFailed
					},
				}
			},
			wantErr: errCreateSessionFailed,
		},
		{
			name: "build runner",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: testAgentLoader("You are the configured agent."),
					createSession: func(workDir string) (session.Session, error) {
						return session.Session{
							ID:          "session-123",
							WorkDir:     workDir,
							HistoryFile: filepath.Join(t.TempDir(), "history.jsonl"),
						}, nil
					},
					buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
						return nil, errBuildRunnerFailed
					},
				}
			},
			wantErr: errBuildRunnerFailed,
		},
		{
			name: "run shell ui",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					loadAgent: testAgentLoader("You are the configured agent."),
					createSession: func(workDir string) (session.Session, error) {
						return session.Session{
							ID:          "session-123",
							WorkDir:     workDir,
							HistoryFile: filepath.Join(t.TempDir(), "history.jsonl"),
						}, nil
					},
					buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
						return &stubRunner{}, nil
					},
					runShellUI: func(ctx context.Context, deps shell.Dependencies) error {
						return errShellUIFailed
					},
				}
			},
			wantErr: errShellUIFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := tt.setup(t)

			args := []string{"fix", "tests"}
			if tt.name == "parse input" {
				args = []string{"--bad-flag", "fix", "tests"}
			}
			if tt.name == "flag value required" {
				args = []string{"--model"}
			}
			if tt.name == "apply model override" {
				args = []string{"--model", "missing-model", "fix", "tests"}
			}
			if tt.name == "create session" {
				args = []string{"--new-session", "fix", "tests"}
			}
			if tt.name == "continue session" {
				args = []string{"--continue", "fix", "tests"}
			}
			if tt.name == "continue session with no previous session" {
				args = []string{"--continue", "fix", "tests"}
			}

			err := deps.run(args)
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("run() error = nil, want %q", tt.wantErrText)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("run() error = %v, want wrapped %v", err, tt.wantErr)
				}
				if err.Error() != tt.wantErrText {
					t.Fatalf("run() error = %q, want %q", err.Error(), tt.wantErrText)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want wrapped %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildRunnerRunsWithWiredPlaceholderEngine(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 2,
			LLMTurns:     1,
		},
	}
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner, err := buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), ctx, runtime.Input{
		Prompt:       "hello",
		Model:        cfg.DefaultModel,
		SystemPrompt: "You are the configured agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// 用户记录由 Run() 在开始时追加，存储在 result.UserRecord
	if result.UserRecord == nil {
		t.Fatalf("Run().UserRecord = nil, want non-nil")
	}
	if *result.UserRecord != contextstore.NewUserTextRecord("hello") {
		t.Fatalf("Run().UserRecord = %#v, want %#v", *result.UserRecord, contextstore.NewUserTextRecord("hello"))
	}

	// Step 只包含 assistant 记录
	wantAssistantRecord := contextstore.NewAssistantTextRecord("assistant placeholder reply: hello")
	if len(result.Steps) != 1 {
		t.Fatalf("len(Run().Steps) = %d, want 1", len(result.Steps))
	}
	if len(result.Steps[0].AppendedRecords) != 1 {
		t.Fatalf("len(Run().Steps[0].AppendedRecords) = %d, want 1", len(result.Steps[0].AppendedRecords))
	}
	if result.Steps[0].AppendedRecords[0] != wantAssistantRecord {
		t.Fatalf("Run().Steps[0].AppendedRecords[0] = %#v, want %#v", result.Steps[0].AppendedRecords[0], wantAssistantRecord)
	}

	// history 文件包含 user + assistant
	wantRecords := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
		wantAssistantRecord,
	}
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}
}

func TestDependenciesBuildRunnerUsesInjectedClientBuilder(t *testing.T) {
	var gotCfg config.Config
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotCfg = cfg
			return llm.NewPlaceholderClient(), nil
		},
	}
	cfg := config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
	}
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	runner, err := deps.buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner() error = %v", err)
	}

	_, err = runner.Run(context.Background(), ctx, runtime.Input{
		Prompt:       "hello",
		Model:        cfg.DefaultModel,
		SystemPrompt: "You are the configured agent.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if gotCfg.Models["custom-model"].Provider != config.ProviderTypePlaceholder {
		t.Fatalf("builder got provider = %q, want %q", gotCfg.Models["custom-model"].Provider, config.ProviderTypePlaceholder)
	}
}

func TestDependenciesBuildRunnerForAgentUsesModelOverride(t *testing.T) {
	var gotCfg config.Config
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotCfg = cfg
			return llm.NewPlaceholderClient(), nil
		},
	}
	cfg := config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
			"reviewer-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "reviewer-model",
			},
		},
	}

	_, err := deps.buildRunnerForAgent(cfg, loadedAgent{
		Spec: agentspec.Spec{
			Model: "reviewer-model",
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("buildRunnerForAgent() error = %v", err)
	}

	if gotCfg.DefaultModel != "reviewer-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "reviewer-model")
	}
}

func TestDependenciesBuildRunnerForAgentRunsWithResolvedAgentTools(t *testing.T) {
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			return llm.NewPlaceholderClient(), nil
		},
	}
	cfg := config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
	}
	runner, err := deps.buildRunnerForAgent(cfg, loadedAgent{
		Tools: []tools.Definition{
			{
				Name: tools.ToolReadFile,
				Kind: tools.KindFile,
			},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("buildRunnerForAgent() error = %v", err)
	}

	result, err := runner.Run(
		context.Background(),
		contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		runtime.Input{
			Prompt:       "hello",
			Model:        "custom-model",
			SystemPrompt: "You are the configured agent.",
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != runtime.RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusFinished)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(result.Steps) = %d, want %d", len(result.Steps), 1)
	}
}

func TestDependenciesRunRuntimeStreamsPrintUIForEventfulRunner(t *testing.T) {
	var out bytes.Buffer
	deps := dependencies{
		buildVisualizer: func() ui.VisualizeFunc {
			return printui.VisualizeText(&out)
		},
	}
	runner := runtime.New(fakeRuntimeEngine{
		reply: runtime.AssistantReply{Text: "assistant reply"},
	}, runtime.Config{})
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	result, err := deps.runRuntime(context.Background(), runner, store, runtime.Input{
		Prompt:       "hello",
		Model:        "test-model",
		SystemPrompt: "You are the configured agent.",
	})
	if err != nil {
		t.Fatalf("runRuntime() error = %v", err)
	}
	if result.Status != runtime.RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusFinished)
	}

	want := "[step 1]\nassistant reply\n"
	if out.String() != want {
		t.Fatalf("print ui output = %q, want %q", out.String(), want)
	}
}

func TestDependenciesRunRuntimeKeepsLegacyRunnerCompatible(t *testing.T) {
	var out bytes.Buffer
	deps := dependencies{
		buildVisualizer: func() ui.VisualizeFunc {
			return printui.VisualizeText(&out)
		},
	}
	runner := &stubRunner{
		result: runtime.Result{
			Status: runtime.RunStatusFinished,
		},
	}
	store := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	result, err := deps.runRuntime(context.Background(), runner, store, runtime.Input{
		Prompt: "hello",
		Model:  "test-model",
	})
	if err != nil {
		t.Fatalf("runRuntime() error = %v", err)
	}
	if result.Status != runtime.RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusFinished)
	}
	if out.Len() != 0 {
		t.Fatalf("print ui output = %q, want empty output for legacy runner", out.String())
	}
}

func TestBuildRunnerReturnsErrorForUnsupportedMode(t *testing.T) {
	_, err := buildRunner(config.Config{
		DefaultModel: "broken",
		Models: map[string]config.ModelConfig{
			"broken": {
				Provider: "custom-provider",
				Model:    "broken-model",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"custom-provider": {
				Type: "unsupported",
			},
		},
	})
	if !errors.Is(err, ErrUnsupportedProviderType) {
		t.Fatalf("buildRunner() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

type stubRunner struct {
	gotCtx          contextstore.Context
	gotInput        runtime.Input
	result          runtime.Result
	err             error
	appendToContext bool
}

func (r *stubRunner) Run(_ context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error) {
	r.gotCtx = store
	r.gotInput = input
	if r.err != nil {
		return runtime.Result{}, r.err
	}

	if r.appendToContext {
		for _, step := range r.result.Steps {
			for _, record := range step.AppendedRecords {
				if err := store.Append(record); err != nil {
					return runtime.Result{}, err
				}
			}
		}
	}

	return r.result, nil
}

type fakeToolCallClient struct {
	replies int
}

func (c *fakeToolCallClient) Reply(request llm.Request) (llm.Response, error) {
	if c.replies == 0 {
		c.replies++
		return llm.Response{
			Text: "I will search the web.",
			ToolCalls: []llm.ToolCall{{
				ID:        "call_search",
				Name:      tools.ToolSearchWeb,
				Arguments: `{"query":"golang duckduckgo","limit":2}`,
			}},
		}, nil
	}

	if len(request.Messages) == 0 {
		return llm.Response{}, errors.New("missing request messages")
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != llm.RoleTool {
		return llm.Response{}, fmt.Errorf("last message role = %q, want %q", last.Role, llm.RoleTool)
	}
	if !strings.Contains(last.Content, "temporary") {
		return llm.Response{}, fmt.Errorf("tool result content = %q, want temporary failure", last.Content)
	}

	return llm.Response{Text: "done"}, nil
}

func TestDependenciesBuildRunnerForAgentSearchWebUsesConfiguredBackend(t *testing.T) {
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			return &fakeToolCallClient{}, nil
		},
	}
	cfg := config.Default()
	cfg.Web.Enabled = true
	cfg.Web.DuckDuckGo.BaseURL = "https://duckduckgo.invalid/html/"
	cfg.DefaultModel = "custom-model"
	cfg.Models = map[string]config.ModelConfig{
		"custom-model": {
			Provider: config.ProviderTypePlaceholder,
			Model:    "custom-model",
		},
	}

	runner, err := deps.buildRunnerForAgent(cfg, loadedAgent{
		Tools: []tools.Definition{{
			Name: tools.ToolSearchWeb,
			Kind: tools.KindUtility,
		}},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("buildRunnerForAgent() error = %v", err)
	}

	_, err = runner.Run(
		context.Background(),
		contextstore.New(filepath.Join(t.TempDir(), "history.jsonl")),
		runtime.Input{
			Prompt:       "search the web",
			Model:        "custom-model",
			SystemPrompt: "You are the configured agent.",
		},
	)
	if err == nil {
		t.Fatalf("Run() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), `execute tool call "search_web"`) {
		t.Fatalf("Run() error = %q, want search_web tool execution error", err.Error())
	}
}

type fakeRuntimeEngine struct {
	reply runtime.AssistantReply
	err   error
}

func (e fakeRuntimeEngine) Reply(ctx context.Context, input runtime.ReplyInput) (runtime.AssistantReply, error) {
	return e.reply, e.err
}
