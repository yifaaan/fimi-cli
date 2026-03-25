package agentspec

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadFile(t *testing.T) {
	t.Run("loads minimal v1 spec and resolves prompt path", func(t *testing.T) {
		agentFile, promptFile := writeAgentFixture(t, `
version: 1
agent:
  name: "  Test Agent  "
  system_prompt_path: ./system.md
  tools:
    - " tool.read "
    - ""
    - "tool.write"
`, "  You are a test agent.  \n")

		got, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		want := Spec{
			Extend:           "",
			Name:             "Test Agent",
			SystemPromptPath: promptFile,
			SystemPromptArgs: nil,
			Tools: []string{
				"tool.read",
				"tool.write",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("LoadFile() = %#v, want %#v", got, want)
		}
	})

	t.Run("loads system prompt args", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: " reviewer "
    SCOPE: " app "
  tools:
    - tool.read
`, "You are a ${ROLE} for ${SCOPE}.\n")

		got, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		want := map[string]string{
			"ROLE":  "reviewer",
			"SCOPE": "app",
		}
		if !reflect.DeepEqual(got.SystemPromptArgs, want) {
			t.Fatalf("LoadFile().SystemPromptArgs = %#v, want %#v", got.SystemPromptArgs, want)
		}
	})

	t.Run("loads exclude tools and subagents", func(t *testing.T) {
		dir := t.TempDir()
		agentFile := filepath.Join(dir, "agent.yaml")
		promptFile := filepath.Join(dir, "system.md")
		reviewerFile := filepath.Join(dir, "reviewer.yaml")

		if err := os.WriteFile(agentFile, []byte(`
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - bash
    - read_file
  exclude_tools:
    - " bash "
    - ""
  subagents:
    " reviewer ":
      path: ./reviewer.yaml
      description: " review code "
    "":
      path: ./ignored.yaml
      description: ignored
`), 0o644); err != nil {
			t.Fatalf("WriteFile(agent.yaml) error = %v", err)
		}
		if err := os.WriteFile(promptFile, []byte("You are a test agent.\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(system.md) error = %v", err)
		}
		if err := os.WriteFile(reviewerFile, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(reviewer.yaml) error = %v", err)
		}

		got, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		if !reflect.DeepEqual(got.ExcludeTools, []string{"bash"}) {
			t.Fatalf("LoadFile().ExcludeTools = %#v, want %#v", got.ExcludeTools, []string{"bash"})
		}

		wantSubagents := map[string]SubagentSpec{
			"reviewer": {
				Path:        reviewerFile,
				Description: "review code",
			},
		}
		if !reflect.DeepEqual(got.Subagents, wantSubagents) {
			t.Fatalf("LoadFile().Subagents = %#v, want %#v", got.Subagents, wantSubagents)
		}
	})

	t.Run("resolves relative extend and merges fields", func(t *testing.T) {
		dir := t.TempDir()
		baseAgentFile := filepath.Join(dir, "base.yaml")
		childAgentFile := filepath.Join(dir, "child.yaml")
		promptFile := filepath.Join(dir, "system.md")

		if err := os.WriteFile(baseAgentFile, []byte(`
version: 1
agent:
  name: Base Agent
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: reviewer
  tools:
    - tool.read
`), 0o644); err != nil {
			t.Fatalf("WriteFile(base.yaml) error = %v", err)
		}
		if err := os.WriteFile(childAgentFile, []byte(`
version: 1
agent:
  extend: ./base.yaml
  name: Child Agent
  system_prompt_args:
    SCOPE: app
`), 0o644); err != nil {
			t.Fatalf("WriteFile(child.yaml) error = %v", err)
		}
		if err := os.WriteFile(promptFile, []byte("You are a ${ROLE} for ${SCOPE}.\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(system.md) error = %v", err)
		}

		got, err := LoadFile(childAgentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		want := Spec{
			Extend:           "",
			Name:             "Child Agent",
			SystemPromptPath: promptFile,
			SystemPromptArgs: map[string]string{
				"ROLE":  "reviewer",
				"SCOPE": "app",
			},
			Tools: []string{"tool.read"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("LoadFile() = %#v, want %#v", got, want)
		}
	})

	t.Run("overrides exclude tools and subagents when extending", func(t *testing.T) {
		dir := t.TempDir()
		baseAgentFile := filepath.Join(dir, "base.yaml")
		childAgentFile := filepath.Join(dir, "child.yaml")
		promptFile := filepath.Join(dir, "system.md")
		baseReviewerFile := filepath.Join(dir, "base-reviewer.yaml")
		childCoderFile := filepath.Join(dir, "child-coder.yaml")

		if err := os.WriteFile(baseAgentFile, []byte(`
version: 1
agent:
  name: Base Agent
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: reviewer
  tools:
    - bash
    - read_file
  exclude_tools:
    - bash
  subagents:
    reviewer:
      path: ./base-reviewer.yaml
      description: Review code
`), 0o644); err != nil {
			t.Fatalf("WriteFile(base.yaml) error = %v", err)
		}
		if err := os.WriteFile(childAgentFile, []byte(`
version: 1
agent:
  extend: ./base.yaml
  system_prompt_args:
    SCOPE: app
  exclude_tools:
    - read_file
  subagents:
    coder:
      path: ./child-coder.yaml
      description: Write code
`), 0o644); err != nil {
			t.Fatalf("WriteFile(child.yaml) error = %v", err)
		}
		if err := os.WriteFile(promptFile, []byte("You are a ${ROLE} for ${SCOPE}.\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(system.md) error = %v", err)
		}
		if err := os.WriteFile(baseReviewerFile, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(base-reviewer.yaml) error = %v", err)
		}
		if err := os.WriteFile(childCoderFile, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(child-coder.yaml) error = %v", err)
		}

		got, err := LoadFile(childAgentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		want := Spec{
			Extend:           "",
			Name:             "Base Agent",
			SystemPromptPath: promptFile,
			SystemPromptArgs: map[string]string{
				"ROLE":  "reviewer",
				"SCOPE": "app",
			},
			Tools:        []string{"bash", "read_file"},
			ExcludeTools: []string{"read_file"},
			Subagents: map[string]SubagentSpec{
				"coder": {
					Path:        childCoderFile,
					Description: "Write code",
				},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("LoadFile() = %#v, want %#v", got, want)
		}
	})

	t.Run("defaults missing version to v1", func(t *testing.T) {
		agentFile, promptFile := writeAgentFixture(t, `
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - tool.read
`, "You are a test agent.\n")

		got, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		if got.SystemPromptPath != promptFile {
			t.Fatalf("LoadFile().SystemPromptPath = %q, want %q", got.SystemPromptPath, promptFile)
		}
	})

	t.Run("rejects unsupported version", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 2
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - tool.read
`, "You are a test agent.\n")

		_, err := LoadFile(agentFile)
		if !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("LoadFile() error = %v, want wrapped %v", err, ErrUnsupportedVersion)
		}
	})

	t.Run("rejects extend cycles", func(t *testing.T) {
		dir := t.TempDir()
		firstFile := filepath.Join(dir, "first.yaml")
		secondFile := filepath.Join(dir, "second.yaml")
		promptFile := filepath.Join(dir, "system.md")

		if err := os.WriteFile(firstFile, []byte(`
version: 1
agent:
  extend: ./second.yaml
  name: First Agent
`), 0o644); err != nil {
			t.Fatalf("WriteFile(first.yaml) error = %v", err)
		}
		if err := os.WriteFile(secondFile, []byte(`
version: 1
agent:
  extend: ./first.yaml
  system_prompt_path: ./system.md
  tools:
    - tool.read
`), 0o644); err != nil {
			t.Fatalf("WriteFile(second.yaml) error = %v", err)
		}
		if err := os.WriteFile(promptFile, []byte("You are a test agent.\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(system.md) error = %v", err)
		}

		_, err := LoadFile(firstFile)
		if !errors.Is(err, ErrAgentSpecExtendCycle) {
			t.Fatalf("LoadFile() error = %v, want wrapped %v", err, ErrAgentSpecExtendCycle)
		}
	})

	t.Run("rejects missing required fields", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  system_prompt_path: ./system.md
`, "You are a test agent.\n")

		_, err := LoadFile(agentFile)
		if err == nil {
			t.Fatalf("LoadFile() error = nil, want non-nil")
		}
		if err.Error() != `validate agent spec file "`+agentFile+`": agent.name is required` {
			t.Fatalf("LoadFile() error = %q, want missing-name validation", err.Error())
		}
	})

	t.Run("rejects subagent with empty path", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - tool.read
  subagents:
    reviewer:
      path: " "
      description: Review code
`, "You are a test agent.\n")

		_, err := LoadFile(agentFile)
		if err == nil {
			t.Fatalf("LoadFile() error = nil, want non-nil")
		}
		if err.Error() != `validate agent spec file "`+agentFile+`": agent.subagents.reviewer.path is required` {
			t.Fatalf("LoadFile() error = %q, want missing-subagent-path validation", err.Error())
		}
	})

	t.Run("rejects subagent with empty description", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - tool.read
  subagents:
    reviewer:
      path: ./reviewer.yaml
      description: " "
`, "You are a test agent.\n")

		_, err := LoadFile(agentFile)
		if err == nil {
			t.Fatalf("LoadFile() error = nil, want non-nil")
		}
		if err.Error() != `validate agent spec file "`+agentFile+`": agent.subagents.reviewer.description is required` {
			t.Fatalf("LoadFile() error = %q, want missing-subagent-description validation", err.Error())
		}
	})
}

func TestLoadSystemPrompt(t *testing.T) {
	t.Run("reads prompt without substitutions", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  tools:
    - tool.read
`, "\n  You are a test agent.  \n")

		spec, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		got, err := LoadSystemPrompt(spec)
		if err != nil {
			t.Fatalf("LoadSystemPrompt() error = %v", err)
		}
		if got != "You are a test agent." {
			t.Fatalf("LoadSystemPrompt() = %q, want %q", got, "You are a test agent.")
		}
	})

	t.Run("substitutes explicit prompt args", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: reviewer
    SCOPE: app
  tools:
    - tool.read
`, "\nYou are a ${ROLE} for ${SCOPE}.\n")

		spec, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		got, err := LoadSystemPrompt(spec)
		if err != nil {
			t.Fatalf("LoadSystemPrompt() error = %v", err)
		}
		if got != "You are a reviewer for app." {
			t.Fatalf("LoadSystemPrompt() = %q, want %q", got, "You are a reviewer for app.")
		}
	})

	t.Run("returns error when prompt arg is missing", func(t *testing.T) {
		agentFile, _ := writeAgentFixture(t, `
version: 1
agent:
  name: Test Agent
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: reviewer
  tools:
    - tool.read
`, "\nYou are a ${ROLE} for ${SCOPE}.\n")

		spec, err := LoadFile(agentFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}

		_, err = LoadSystemPrompt(spec)
		if !errors.Is(err, ErrSystemPromptArgMissing) {
			t.Fatalf("LoadSystemPrompt() error = %v, want wrapped %v", err, ErrSystemPromptArgMissing)
		}
	})
}

func writeAgentFixture(t *testing.T, agentYAML string, systemPrompt string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	agentFile := filepath.Join(dir, "agent.yaml")
	promptFile := filepath.Join(dir, "system.md")

	if err := os.WriteFile(agentFile, []byte(agentYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(agent.yaml) error = %v", err)
	}
	if err := os.WriteFile(promptFile, []byte(systemPrompt), 0o644); err != nil {
		t.Fatalf("WriteFile(system.md) error = %v", err)
	}

	return agentFile, promptFile
}
