package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fimi-cli/internal/runtime"
)

const DefaultBashCommandTimeout = 30 * time.Second

var ErrToolArgumentsInvalid = errors.New("tool arguments are invalid")
var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolCommandTimedOut = errors.New("tool command timed out")
var ErrToolPathRequired = errors.New("tool path is required")
var ErrToolPatternRequired = errors.New("tool pattern is required")
var ErrToolReplaceOldRequired = errors.New("tool replace old text is required")
var ErrToolReplaceTargetMissing = errors.New("tool replace target not found")
var ErrToolReplaceTargetNotUnique = errors.New("tool replace target is not unique")
var ErrToolPathOutsideWorkspace = errors.New("tool path escapes workspace")
var ErrToolPatternOutsideWorkspace = errors.New("tool pattern escapes workspace")

type bashArguments struct {
	Command string `json:"command"`
}

type globArguments struct {
	Pattern string `json:"pattern"`
}

type grepArguments struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type readFileArguments struct {
	Path string `json:"path"`
}

type writeFileArguments struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type replaceFileArguments struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// NewBuiltinExecutor 返回带内建 handler 的最小工具执行器。
// 当前先接通最小可用的一组本地工具能力。
func NewBuiltinExecutor(definitions []Definition, workDir string) Executor {
	return NewExecutor(definitions, builtinHandlers(workDir))
}

func builtinHandlers(workDir string) map[string]HandlerFunc {
	return map[string]HandlerFunc{
		ToolBash:        newBashHandler(workDir),
		ToolGlob:        newGlobHandler(workDir),
		ToolGrep:        newGrepHandler(workDir),
		ToolReadFile:    newReadFileHandler(workDir),
		ToolWriteFile:   newWriteFileHandler(workDir),
		ToolReplaceFile: newReplaceFileHandler(workDir),
	}
}

func newBashHandler(workDir string) HandlerFunc {
	return newBashHandlerWithTimeout(workDir, DefaultBashCommandTimeout)
}

func newBashHandlerWithTimeout(workDir string, timeout time.Duration) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
		if strings.TrimSpace(workDir) != "" {
			cmd.Dir = workDir
		}

		output, err := cmd.CombinedOutput()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return runtime.ToolExecution{}, fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command)
		}
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

func newGlobHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeGlobArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		pattern, err := normalizeWorkspacePattern(args.Pattern)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		matches, err := findGlobMatches(rootAbs, pattern)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: strings.Join(matches, "\n"),
		}, nil
	}
}

func newGrepHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeGrepArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		expression, err := regexp.Compile(args.Pattern)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("%w: compile grep pattern: %v", ErrToolArgumentsInvalid, err)
		}

		matches, err := findGrepMatches(rootAbs, targetAbs, expression)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: strings.Join(matches, "\n"),
		}, nil
	}
}

func newWriteFileHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeWriteFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// 写工具先采用“覆盖写入”语义，并自动补父目录，后面再单独引入 replace 这类更细粒度操作。
		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("create parent dir for %q: %w", targetAbs, err)
		}
		if err := os.WriteFile(targetAbs, []byte(args.Content), 0o644); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("wrote %d bytes to %s", len([]byte(args.Content)), targetRel),
		}, nil
	}
}

func newReplaceFileHandler(workDir string) HandlerFunc {
	return func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReplaceFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		data, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for replace: %w", targetAbs, err)
		}
		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for replace: %w", targetAbs, err)
		}

		content := string(data)
		matchCount := strings.Count(content, args.Old)
		switch {
		case matchCount == 0:
			return runtime.ToolExecution{}, fmt.Errorf("%w: %s", ErrToolReplaceTargetMissing, targetRel)
		case matchCount > 1:
			return runtime.ToolExecution{}, fmt.Errorf("%w: %s", ErrToolReplaceTargetNotUnique, targetRel)
		}

		replaced := strings.Replace(content, args.Old, args.New, 1)
		if err := os.WriteFile(targetAbs, []byte(replaced), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write replaced file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("replaced 1 occurrence in %s", targetRel),
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

func decodeGlobArguments(raw string) (globArguments, error) {
	var args globArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return globArguments{}, fmt.Errorf("%w: decode glob arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return globArguments{}, ErrToolPatternRequired
	}

	return args, nil
}

func decodeGrepArguments(raw string) (grepArguments, error) {
	var args grepArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return grepArguments{}, fmt.Errorf("%w: decode grep arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return grepArguments{}, ErrToolPatternRequired
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
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

func decodeWriteFileArguments(raw string) (writeFileArguments, error) {
	var args writeFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return writeFileArguments{}, fmt.Errorf("%w: decode write_file arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return writeFileArguments{}, ErrToolPathRequired
	}

	return args, nil
}

func decodeReplaceFileArguments(raw string) (replaceFileArguments, error) {
	var args replaceFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return replaceFileArguments{}, fmt.Errorf("%w: decode replace_file arguments: %v", ErrToolArgumentsInvalid, err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return replaceFileArguments{}, ErrToolPathRequired
	}
	if args.Old == "" {
		return replaceFileArguments{}, ErrToolReplaceOldRequired
	}

	return args, nil
}

func resolveWorkspaceRoot(workDir string) (string, error) {
	root := workDir
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	return rootAbs, nil
}

func resolveWorkspacePath(workDir string, target string) (string, error) {
	rootAbs, err := resolveWorkspaceRoot(workDir)
	if err != nil {
		return "", err
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

func relativeWorkspacePath(rootAbs string, targetAbs string) (string, error) {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("relativize tool path %q: %w", targetAbs, err)
	}

	return filepath.ToSlash(rel), nil
}

func normalizeWorkspacePattern(raw string) (string, error) {
	pattern := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	if pattern == "." {
		return pattern, nil
	}
	if path.IsAbs(pattern) || pattern == ".." || strings.HasPrefix(pattern, "../") {
		return "", fmt.Errorf("%w: %s", ErrToolPatternOutsideWorkspace, raw)
	}

	return pattern, nil
}

func findGlobMatches(rootAbs string, pattern string) ([]string, error) {
	matches := make([]string, 0)
	err := filepath.WalkDir(rootAbs, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(rootAbs, current)
		if err != nil {
			return fmt.Errorf("relativize path %q: %w", current, err)
		}
		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)
		ok, err := matchWorkspacePattern(pattern, rel)
		if err != nil {
			return fmt.Errorf("%w: match glob pattern %q: %v", ErrToolArgumentsInvalid, pattern, err)
		}
		if ok {
			matches = append(matches, rel)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace for glob: %w", err)
	}

	return matches, nil
}

func matchWorkspacePattern(pattern string, target string) (bool, error) {
	if pattern == "." {
		return target == ".", nil
	}

	patternSegments := splitSlashPath(pattern)
	targetSegments := splitSlashPath(target)

	return matchWorkspacePatternSegments(patternSegments, targetSegments)
}

func matchWorkspacePatternSegments(pattern []string, target []string) (bool, error) {
	if len(pattern) == 0 {
		return len(target) == 0, nil
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(target); i++ {
			ok, err := matchWorkspacePatternSegments(pattern[1:], target[i:])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}

		return false, nil
	}
	if len(target) == 0 {
		return false, nil
	}

	ok, err := path.Match(pattern[0], target[0])
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	return matchWorkspacePatternSegments(pattern[1:], target[1:])
}

func splitSlashPath(raw string) []string {
	if raw == "" || raw == "." {
		return nil
	}

	return strings.Split(raw, "/")
}

func findGrepMatches(rootAbs string, targetAbs string, expression *regexp.Regexp) ([]string, error) {
	info, err := os.Stat(targetAbs)
	if err != nil {
		return nil, fmt.Errorf("stat grep path %q: %w", targetAbs, err)
	}

	if !info.IsDir() {
		return grepFile(rootAbs, targetAbs, expression)
	}

	matches := make([]string, 0)
	err = filepath.WalkDir(targetAbs, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		fileMatches, err := grepFile(rootAbs, current, expression)
		if err != nil {
			return err
		}
		matches = append(matches, fileMatches...)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace for grep: %w", err)
	}

	return matches, nil
}

func grepFile(rootAbs string, filePath string, expression *regexp.Regexp) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open grep file %q: %w", filePath, err)
	}
	defer file.Close()

	rel, err := filepath.Rel(rootAbs, filePath)
	if err != nil {
		return nil, fmt.Errorf("relativize grep file %q: %w", filePath, err)
	}
	rel = filepath.ToSlash(rel)

	matches := make([]string, 0)
	scanner := bufio.NewScanner(file)
	// 允许较长源码行，避免默认 64K 上限过早打断搜索。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !expression.MatchString(line) {
			continue
		}

		matches = append(matches, rel+":"+strconv.Itoa(lineNumber)+":"+line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan grep file %q: %w", filePath, err)
	}

	return matches, nil
}
