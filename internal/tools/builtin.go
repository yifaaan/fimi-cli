package tools

import (
	"bufio"
	"bytes"
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
var ErrToolPatchDiffRequired = errors.New("tool patch diff is required")
var ErrToolPatchFailed = errors.New("failed to apply patch")

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

type patchFileArguments struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

// NewBuiltinExecutor 返回带内建 handler 的最小工具执行器。
// 当前先接通最小可用的一组本地工具能力。
// shaper 用于对工具输出进行塑形，防止超大输出消耗模型上下文。
func NewBuiltinExecutor(definitions []Definition, workDir string) Executor {
	return NewBuiltinExecutorWithShaper(definitions, workDir, NewOutputShaper())
}

// NewBuiltinExecutorWithShaper 创建带自定义塑形器的执行器。
func NewBuiltinExecutorWithShaper(definitions []Definition, workDir string, shaper OutputShaper) Executor {
	return NewExecutor(definitions, builtinHandlers(workDir, shaper))
}

func builtinHandlers(workDir string, shaper OutputShaper) map[string]HandlerFunc {
	return map[string]HandlerFunc{
		ToolBash:        newBashHandler(workDir, shaper),
		ToolGlob:        newGlobHandler(workDir, shaper),
		ToolGrep:        newGrepHandler(workDir, shaper),
		ToolReadFile:    newReadFileHandler(workDir, shaper),
		ToolWriteFile:   newWriteFileHandler(workDir),
		ToolReplaceFile: newReplaceFileHandler(workDir),
		ToolPatchFile:   newPatchFileHandler(workDir),
	}
}

func newBashHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return newBashHandlerWithTimeout(workDir, shaper, DefaultBashCommandTimeout)
}

func newBashHandlerWithTimeout(workDir string, shaper OutputShaper, timeout time.Duration) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// 使用传入的 ctx 作为父 context，这样外部取消也能中断 bash 执行
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
		if strings.TrimSpace(workDir) != "" {
			cmd.Dir = workDir
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
		}
		if err != nil && !isExitError(err) {
			return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
		}

		// 对 stdout 进行塑形
		rawStdout := stdout.String()
		shapedStdout := shaper.Shape(rawStdout)

		// 对 stderr 进行塑形（使用相同的限制）
		rawStderr := stderr.String()
		shapedStderr := shaper.Shape(rawStderr)

		// 构建最终输出：stdout + stderr + 截断提示
		var outputParts []string
		outputParts = append(outputParts, shapedStdout.Output)
		if shapedStderr.Output != "" {
			outputParts = append(outputParts, "STDERR:", shapedStderr.Output)
		}

		// 添加截断提示
		var truncationMsgs []string
		if shapedStdout.Message != "" {
			truncationMsgs = append(truncationMsgs, "stdout: "+shapedStdout.Message)
		}
		if shapedStderr.Message != "" {
			truncationMsgs = append(truncationMsgs, "stderr: "+shapedStderr.Message)
		}
		if len(truncationMsgs) > 0 {
			outputParts = append(outputParts, "\n["+strings.Join(truncationMsgs, "; ")+"]")
		}

		return runtime.ToolExecution{
			Call:     call,
			Output:   strings.Join(outputParts, "\n"),
			Stdout:   shapedStdout.Output,
			Stderr:   shapedStderr.Output,
			ExitCode: exitCodeFromError(err),
		}, nil
	}
}

func newReadFileHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
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

		// 对文件内容进行塑形
		rawContent := string(data)
		shaped := shaper.Shape(rawContent)

		// 构建最终输出
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
		}, nil
	}
}

func newGlobHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
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

		// 对匹配结果进行塑形
		rawOutput := strings.Join(matches, "\n")
		shaped := shaper.Shape(rawOutput)

		// 构建最终输出
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
		}, nil
	}
}

func newGrepHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
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
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: compile grep pattern: %v", ErrToolArgumentsInvalid, err))
		}

		matches, err := findGrepMatches(rootAbs, targetAbs, expression)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// 对匹配结果进行塑形
		rawOutput := strings.Join(matches, "\n")
		shaped := shaper.Shape(rawOutput)

		// 构建最终输出
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
		}, nil
	}
}

func newWriteFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
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

		// 写工具先采用"覆盖写入"语义，并自动补父目录，后面再单独引入 replace 这类更细粒度操作。
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
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
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
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s", ErrToolReplaceTargetMissing, targetRel))
		case matchCount > 1:
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s", ErrToolReplaceTargetNotUnique, targetRel))
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

func newPatchFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodePatchFileArguments(call.Arguments)
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

		// 检查文件存在
		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for patch: %w", targetAbs, err)
		}

		// 读取原始内容
		originalData, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for patch: %w", targetAbs, err)
		}

		// 解析并应用 patch
		patchedContent, hunksApplied, err := applyUnifiedDiff(string(originalData), args.Diff)
		if err != nil {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %v", ErrToolPatchFailed, err))
		}

		// 检查是否有实际变化
		if patchedContent == string(originalData) {
			return runtime.ToolExecution{}, markRefused(errors.New("no changes were made by the patch"))
		}

		// 写入修改后的内容
		if err := os.WriteFile(targetAbs, []byte(patchedContent), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write patched file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("applied %d hunk(s) to %s", hunksApplied, targetRel),
		}, nil
	}
}

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, markRefused(fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Command) == "" {
		return bashArguments{}, markRefused(ErrToolCommandRequired)
	}

	return args, nil
}

func decodeGlobArguments(raw string) (globArguments, error) {
	var args globArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return globArguments{}, markRefused(fmt.Errorf("%w: decode glob arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return globArguments{}, markRefused(ErrToolPatternRequired)
	}

	return args, nil
}

func decodeGrepArguments(raw string) (grepArguments, error) {
	var args grepArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return grepArguments{}, markRefused(fmt.Errorf("%w: decode grep arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return grepArguments{}, markRefused(ErrToolPatternRequired)
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
	}

	return args, nil
}

func decodeReadFileArguments(raw string) (readFileArguments, error) {
	var args readFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return readFileArguments{}, markRefused(fmt.Errorf("%w: decode read_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return readFileArguments{}, markRefused(ErrToolPathRequired)
	}

	return args, nil
}

func decodeWriteFileArguments(raw string) (writeFileArguments, error) {
	var args writeFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return writeFileArguments{}, markRefused(fmt.Errorf("%w: decode write_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return writeFileArguments{}, markRefused(ErrToolPathRequired)
	}

	return args, nil
}

func decodeReplaceFileArguments(raw string) (replaceFileArguments, error) {
	var args replaceFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return replaceFileArguments{}, markRefused(fmt.Errorf("%w: decode replace_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return replaceFileArguments{}, markRefused(ErrToolPathRequired)
	}
	if args.Old == "" {
		return replaceFileArguments{}, markRefused(ErrToolReplaceOldRequired)
	}

	return args, nil
}

func decodePatchFileArguments(raw string) (patchFileArguments, error) {
	var args patchFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return patchFileArguments{}, markRefused(fmt.Errorf("%w: decode patch_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return patchFileArguments{}, markRefused(ErrToolPathRequired)
	}
	if strings.TrimSpace(args.Diff) == "" {
		return patchFileArguments{}, markRefused(ErrToolPatchDiffRequired)
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
		return "", markRefused(fmt.Errorf("%w: %s", ErrToolPathOutsideWorkspace, target))
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

func isExitError(err error) bool {
	if err == nil {
		return false
	}

	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}

func normalizeWorkspacePattern(raw string) (string, error) {
	pattern := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	if pattern == "." {
		return pattern, nil
	}
	if path.IsAbs(pattern) || pattern == ".." || strings.HasPrefix(pattern, "../") {
		return "", markRefused(fmt.Errorf("%w: %s", ErrToolPatternOutsideWorkspace, raw))
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
			return markRefused(fmt.Errorf("%w: match glob pattern %q: %v", ErrToolArgumentsInvalid, pattern, err))
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

// applyUnifiedDiff 将 unified diff 应用到原始内容上。
// 返回修改后的内容、应用的 hunk 数量、以及可能的错误。
func applyUnifiedDiff(original string, diff string) (string, int, error) {
	// 解析 diff 为 hunk 列表
	hunks, err := parseUnifiedDiff(diff)
	if err != nil {
		return "", 0, err
	}

	if len(hunks) == 0 {
		return "", 0, errors.New("no valid hunks found in diff")
	}

	// 将原始内容按行分割（保留换行符信息）
	lines := splitLinesKeepEnds(original)

	// 从后向前应用 hunk，避免行号偏移问题
	for i := len(hunks) - 1; i >= 0; i-- {
		hunk := hunks[i]
		lines, err = applyHunk(lines, hunk)
		if err != nil {
			return "", 0, fmt.Errorf("apply hunk %d: %w", i+1, err)
		}
	}

	result := strings.Join(lines, "")
	return result, len(hunks), nil
}

// unifiedHunk 表示一个 unified diff hunk
type unifiedHunk struct {
	oldStart int      // 旧文件起始行号（1-based）
	oldCount int      // 旧文件行数
	newStart int      // 新文件起始行号（1-based）
	newCount int      // 新文件行数
	lines    []string // hunk 中的所有行（包含前缀字符）
}

// parseUnifiedDiff 解析 unified diff 格式的字符串
func parseUnifiedDiff(diff string) ([]unifiedHunk, error) {
	diffLines := strings.Split(diff, "\n")

	var hunks []unifiedHunk
	var currentHunk *unifiedHunk

	inHunk := false

	for _, line := range diffLines {
		// 跳过文件头
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		// 检测 hunk 头
		if strings.HasPrefix(line, "@@") {
			// 保存之前的 hunk
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			// 解析 hunk 头：@@ -oldStart,oldCount +newStart,newCount @@
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			currentHunk = &hunk
			inHunk = true
			continue
		}

		// 收集 hunk 内容
		if inHunk && currentHunk != nil {
			// 只收集有意义的行（以空格、-、+ 开头的行）
			if len(line) > 0 {
				prefix := line[0]
				if prefix == ' ' || prefix == '-' || prefix == '+' {
					currentHunk.lines = append(currentHunk.lines, line)
				}
			}
		}
	}

	// 保存最后一个 hunk
	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, nil
}

// parseHunkHeader 解析 hunk 头：@@ -oldStart,oldCount +newStart,newCount @@
func parseHunkHeader(line string) (unifiedHunk, error) {
	// 格式：@@ -start,count +start,count @@ optional text
	re := regexp.MustCompile(`@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return unifiedHunk{}, errors.New("invalid hunk header format")
	}

	oldStart, _ := strconv.Atoi(matches[1])
	oldCount := 1
	if matches[2] != "" {
		oldCount, _ = strconv.Atoi(matches[2])
	}

	newStart, _ := strconv.Atoi(matches[3])
	newCount := 1
	if matches[4] != "" {
		newCount, _ = strconv.Atoi(matches[4])
	}

	return unifiedHunk{
		oldStart: oldStart,
		oldCount: oldCount,
		newStart: newStart,
		newCount: newCount,
		lines:    make([]string, 0),
	}, nil
}

// applyHunk 将单个 hunk 应用到行列表
func applyHunk(lines []string, hunk unifiedHunk) ([]string, error) {
	// 转换为 0-based 索引
	startIdx := hunk.oldStart - 1

	// 验证起始位置
	if startIdx < 0 || startIdx > len(lines) {
		return nil, fmt.Errorf("invalid old start line %d (file has %d lines)", hunk.oldStart, len(lines))
	}

	// 验证上下文行匹配
	contextIdx := startIdx
	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]

		if prefix == ' ' || prefix == '-' {
			// 验证上下文和删除行是否匹配
			if contextIdx >= len(lines) {
				return nil, fmt.Errorf("unexpected end of file at line %d", contextIdx+1)
			}

			// 比较：diff 中可能没有换行符，但文件行保留原始换行符
			// 需要两边都去掉换行符后再比较
			expected := strings.TrimRight(content, "\r\n")
			actual := strings.TrimRight(lines[contextIdx], "\r\n")
			if expected != actual {
				return nil, fmt.Errorf("line %d mismatch: expected %q, got %q", contextIdx+1, expected, actual)
			}
			contextIdx++
		}
	}

	// 构建新的行列表
	result := make([]string, 0, len(lines))
	result = append(result, lines[:startIdx]...) // 前面的行保持不变

	// 处理 hunk 内容
	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]

		switch prefix {
		case ' ':
			// 上下文行：保留原行（包括换行符）
			result = append(result, lines[startIdx])
			startIdx++
		case '-':
			// 删除行：跳过原行
			startIdx++
		case '+':
			// 添加行：插入新行
			// 如果内容没有换行符，添加一个
			if !strings.HasSuffix(content, "\n") && !strings.HasSuffix(content, "\r") {
				content += "\n"
			}
			result = append(result, content)
		}
	}

	// 添加剩余的行
	result = append(result, lines[startIdx:]...)

	return result, nil
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
