package agentspec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultVersion = 1

var ErrUnsupportedVersion = errors.New("unsupported agent spec version")
var ErrSystemPromptArgMissing = errors.New("system prompt argument is missing")
var ErrAgentSpecExtendCycle = errors.New("agent spec extend cycle detected")

var systemPromptArgPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Spec 表示 Go 运行时当前最小需要的 agent 定义。
// 这里先只保留 runtime 下一步会直接消费的字段。
type Spec struct {
	Extend           string            `yaml:"extend"`
	Name             string            `yaml:"name"`
	SystemPromptPath string            `yaml:"system_prompt_path"`
	SystemPromptArgs map[string]string `yaml:"system_prompt_args"`
	Tools            []string          `yaml:"tools"`
}

type fileEnvelope struct {
	Version int  `yaml:"version"`
	Agent   Spec `yaml:"agent"`
}

// LoadFile 从磁盘读取并解析最小 agent spec。
func LoadFile(path string) (Spec, error) {
	return loadFile(path, make(map[string]struct{}))
}

func loadFile(path string, visited map[string]struct{}) (Spec, error) {
	path = filepath.Clean(path)
	if _, ok := visited[path]; ok {
		return Spec{}, fmt.Errorf("%w: %s", ErrAgentSpecExtendCycle, path)
	}
	visited[path] = struct{}{}
	defer delete(visited, path)

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
	if spec.Extend != "" {
		basePath := resolvePath(filepath.Dir(path), spec.Extend)
		baseSpec, err := loadFile(basePath, visited)
		if err != nil {
			return Spec{}, fmt.Errorf("load base agent spec %q: %w", basePath, err)
		}

		spec = mergeSpec(baseSpec, spec)
	}
	spec.Extend = ""

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

	prompt, err := substituteSystemPromptArgs(strings.TrimSpace(string(data)), spec.SystemPromptArgs)
	if err != nil {
		return "", fmt.Errorf("substitute system prompt file %q: %w", spec.SystemPromptPath, err)
	}

	return prompt, nil
}

func normalizeSpec(spec Spec) Spec {
	spec.Extend = strings.TrimSpace(spec.Extend)
	spec.Name = strings.TrimSpace(spec.Name)
	spec.SystemPromptPath = strings.TrimSpace(spec.SystemPromptPath)
	spec.SystemPromptArgs = normalizeSystemPromptArgs(spec.SystemPromptArgs)
	spec.Tools = normalizeTools(spec.Tools)

	return spec
}

func normalizeSystemPromptArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		normalized[key] = strings.TrimSpace(value)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
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

func mergeSpec(base Spec, override Spec) Spec {
	merged := base
	if override.Name != "" {
		merged.Name = override.Name
	}
	if override.SystemPromptPath != "" {
		merged.SystemPromptPath = override.SystemPromptPath
	}
	if len(override.SystemPromptArgs) > 0 {
		merged.SystemPromptArgs = mergeStringMaps(base.SystemPromptArgs, override.SystemPromptArgs)
	}
	if len(override.Tools) > 0 {
		merged.Tools = override.Tools
	}

	return merged
}

func mergeStringMaps(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	merged := make(map[string]string, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}

	return merged
}

func substituteSystemPromptArgs(prompt string, args map[string]string) (string, error) {
	matches := systemPromptArgPattern.FindAllStringSubmatchIndex(prompt, -1)
	if len(matches) == 0 {
		return prompt, nil
	}

	var builder strings.Builder
	builder.Grow(len(prompt))

	last := 0
	for _, match := range matches {
		start := match[0]
		end := match[1]
		nameStart := match[2]
		nameEnd := match[3]
		name := prompt[nameStart:nameEnd]

		value, ok := args[name]
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrSystemPromptArgMissing, name)
		}

		builder.WriteString(prompt[last:start])
		builder.WriteString(value)
		last = end
	}
	builder.WriteString(prompt[last:])

	return builder.String(), nil
}
