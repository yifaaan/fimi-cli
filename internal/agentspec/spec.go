package agentspec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultVersion = 1

var ErrUnsupportedVersion = errors.New("unsupported agent spec version")

// Spec 表示 Go 运行时当前最小需要的 agent 定义。
// 这里先只保留 runtime 下一步会直接消费的字段。
type Spec struct {
	Name             string   `yaml:"name"`
	SystemPromptPath string   `yaml:"system_prompt_path"`
	Tools            []string `yaml:"tools"`
}

type fileEnvelope struct {
	Version int  `yaml:"version"`
	Agent   Spec `yaml:"agent"`
}

// LoadFile 从磁盘读取并解析最小 agent spec。
func LoadFile(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read agent spec file %q: %w", path, err)
	}

	var envelope fileEnvelope
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return Spec{}, fmt.Errorf("decode agent spec file %q: %w", path, err)
	}

	if envelope.Version == 0 {
		envelope.Version = DefaultVersion
	}
	if envelope.Version != DefaultVersion {
		return Spec{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, envelope.Version)
	}

	spec := normalizeSpec(envelope.Agent)
	spec.SystemPromptPath = resolvePath(filepath.Dir(path), spec.SystemPromptPath)

	if err := validateSpec(spec); err != nil {
		return Spec{}, fmt.Errorf("validate agent spec file %q: %w", path, err)
	}

	return spec, nil
}

// LoadSystemPrompt 读取 agent 绑定的 system prompt 文件。
func LoadSystemPrompt(spec Spec) (string, error) {
	data, err := os.ReadFile(spec.SystemPromptPath)
	if err != nil {
		return "", fmt.Errorf("read system prompt file %q: %w", spec.SystemPromptPath, err)
	}

	return strings.TrimSpace(string(data)), nil
}

func normalizeSpec(spec Spec) Spec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.SystemPromptPath = strings.TrimSpace(spec.SystemPromptPath)
	spec.Tools = normalizeTools(spec.Tools)

	return spec
}

func normalizeTools(tools []string) []string {
	normalized := make([]string, 0, len(tools))
	for _, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}

		normalized = append(normalized, tool)
	}

	return normalized
}

func resolvePath(baseDir string, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Clean(filepath.Join(baseDir, path))
}

func validateSpec(spec Spec) error {
	if spec.Name == "" {
		return errors.New("agent.name is required")
	}
	if spec.SystemPromptPath == "" {
		return errors.New("agent.system_prompt_path is required")
	}
	if len(spec.Tools) == 0 {
		return errors.New("agent.tools is required")
	}

	return nil
}
