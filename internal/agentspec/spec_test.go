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
			Name:             "Test Agent",
			SystemPromptPath: promptFile,
			Tools: []string{
				"tool.read",
				"tool.write",
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
}

func TestLoadSystemPrompt(t *testing.T) {
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
