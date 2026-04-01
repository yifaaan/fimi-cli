package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"fimi-cli/internal/runtime"
)

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

		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for patch: %w", targetAbs, err)
		}

		originalData, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for patch: %w", targetAbs, err)
		}

		patchedContent, hunksApplied, err := applyUnifiedDiff(string(originalData), args.Diff)
		if err != nil {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %v", ErrToolPatchFailed, err))
		}
		if patchedContent == string(originalData) {
			return runtime.ToolExecution{}, markRefused(errors.New("no changes were made by the patch"))
		}

		if err := os.WriteFile(targetAbs, []byte(patchedContent), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write patched file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("applied %d hunk(s) to %s", hunksApplied, targetRel),
		}, nil
	}
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

// applyUnifiedDiff 将 unified diff 应用到原始内容上。
// 返回修改后的内容、应用的 hunk 数量、以及可能的错误。
func applyUnifiedDiff(original string, diff string) (string, int, error) {
	hunks, err := parseUnifiedDiff(diff)
	if err != nil {
		return "", 0, err
	}
	if len(hunks) == 0 {
		return "", 0, errors.New("no valid hunks found in diff")
	}

	lines := splitLinesKeepEnds(original)
	for i := len(hunks) - 1; i >= 0; i-- {
		lines, err = applyHunk(lines, hunks[i])
		if err != nil {
			return "", 0, fmt.Errorf("apply hunk %d: %w", i+1, err)
		}
	}

	return strings.Join(lines, ""), len(hunks), nil
}

type unifiedHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []string
}

func parseUnifiedDiff(diff string) ([]unifiedHunk, error) {
	diffLines := strings.Split(diff, "\n")

	var hunks []unifiedHunk
	var currentHunk *unifiedHunk
	inHunk := false

	for _, line := range diffLines {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			currentHunk = &hunk
			inHunk = true
			continue
		}
		if !inHunk || currentHunk == nil || len(line) == 0 {
			continue
		}

		prefix := line[0]
		if prefix == ' ' || prefix == '-' || prefix == '+' {
			currentHunk.lines = append(currentHunk.lines, line)
		}
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, nil
}

func parseHunkHeader(line string) (unifiedHunk, error) {
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

func applyHunk(lines []string, hunk unifiedHunk) ([]string, error) {
	startIdx := hunk.oldStart - 1
	if startIdx < 0 || startIdx > len(lines) {
		return nil, fmt.Errorf("invalid old start line %d (file has %d lines)", hunk.oldStart, len(lines))
	}

	contextIdx := startIdx
	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]
		if prefix != ' ' && prefix != '-' {
			continue
		}
		if contextIdx >= len(lines) {
			return nil, fmt.Errorf("unexpected end of file at line %d", contextIdx+1)
		}

		expected := strings.TrimRight(content, "\r\n")
		actual := strings.TrimRight(lines[contextIdx], "\r\n")
		if expected != actual {
			return nil, fmt.Errorf("line %d mismatch: expected %q, got %q", contextIdx+1, expected, actual)
		}
		contextIdx++
	}

	result := make([]string, 0, len(lines))
	result = append(result, lines[:startIdx]...)

	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]

		switch prefix {
		case ' ':
			result = append(result, lines[startIdx])
			startIdx++
		case '-':
			startIdx++
		case '+':
			if !strings.HasSuffix(content, "\n") && !strings.HasSuffix(content, "\r") {
				content += "\n"
			}
			result = append(result, content)
		}
	}

	result = append(result, lines[startIdx:]...)
	return result, nil
}
