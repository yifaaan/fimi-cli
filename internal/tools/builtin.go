package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fimi-cli/internal/runtime"
)

var ErrToolArgumentsInvalid = errors.New("tool arguments are invalid")
var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolPathRequired = errors.New("tool path is required")
var ErrToolPathOutsideWorkspace = errors.New("tool path escapes workspace")

type bashArguments struct {
	Command string `json:"command"`
}

type readFileArguments struct {
	Path string `json:"path"`
}

// NewBuiltinExecutor 返回带内建 handler 的最小工具执行器。
// 当前先接通 read_file 和 bash 两个基础只读能力。
func NewBuiltinExecutor(definitions []Definition, workDir string) Executor {
	return NewExecutor(definitions, builtinHandlers(workDir))
}

func builtinHandlers(workDir string) map[string]HandlerFunc {
	return map[string]HandlerFunc{
		ToolBash:     newBashHandler(workDir),
		ToolReadFile: newReadFileHandler(workDir),
	}
}

func newBashHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		cmd := exec.Command("bash", "-lc", args.Command)
		if strings.TrimSpace(workDir) != "" {
			cmd.Dir = workDir
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("run bash command: %w", err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: string(output),
		}, nil
	}
}

func newReadFileHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReadFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		path, err := resolveWorkspacePath(workDir, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q: %w", path, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: string(data),
		}, nil
	}
}

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return bashArguments{}, ErrToolCommandRequired
	}

	return args, nil
}

func decodeReadFileArguments(raw string) (readFileArguments, error) {
	var args readFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return readFileArguments{}, fmt.Errorf("%w: decode read_file arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return readFileArguments{}, ErrToolPathRequired
	}

	return args, nil
}

func resolveWorkspacePath(workDir string, target string) (string, error) {
	root := workDir
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(rootAbs, targetPath)
	}

	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve tool path %q: %w", target, err)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("relativize tool path %q: %w", targetAbs, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrToolPathOutsideWorkspace, target)
	}

	return targetAbs, nil
}
