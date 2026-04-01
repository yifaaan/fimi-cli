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

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

func newPatchFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodePatchFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "patch_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
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

		addedLines, removedLines := patchDiffStats(args.Diff)
		summary := buildPatchSummary(targetRel, addedLines, removedLines, hunksApplied)

		return runtime.ToolExecution{
			Call:          call,
			Output:        summary,
			DisplayOutput: buildPatchDisplayOutput(summary, args.Diff),
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
	for i, hunk := range hunks {
		lines, err = applyHunk(lines, hunk)
		if err != nil {
			return "", 0, fmt.Errorf("apply hunk %d: %w", i+1, err)
		}
	}

	return strings.Join(lines, ""), len(hunks), nil
}

type unifiedHunk struct {
	oldStart       int
	oldCount       int
	newStart       int
	newCount       int
	hasLineNumbers bool
	lines          []string
}

func parseUnifiedDiff(diff string) ([]unifiedHunk, error) {
	diffLines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")

	var hunks []unifiedHunk
	var currentHunk *unifiedHunk
	inHunk := false

	for _, line := range diffLines {
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

		if inHunk && currentHunk != nil {
			if len(line) == 0 || line == `\ No newline at end of file` {
				continue
			}

			prefix := line[0]
			if prefix == ' ' || prefix == '-' || prefix == '+' {
				currentHunk.lines = append(currentHunk.lines, line)
				continue
			}
		}

		if isPatchMetadataLine(line) {
			continue
		}
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, nil
}

func parseHunkHeader(line string) (unifiedHunk, error) {
	re := regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@(?:.*)?$`)
	matches := re.FindStringSubmatch(line)
	if matches != nil {
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
			oldStart:       oldStart,
			oldCount:       oldCount,
			newStart:       newStart,
			newCount:       newCount,
			hasLineNumbers: true,
			lines:          make([]string, 0),
		}, nil
	}

	if strings.HasPrefix(strings.TrimSpace(line), "@@") {
		return unifiedHunk{
			lines: make([]string, 0),
		}, nil
	}

	return unifiedHunk{}, errors.New("invalid hunk header format")
}

func applyHunk(lines []string, hunk unifiedHunk) ([]string, error) {
	startIdx, err := locateHunkStart(lines, hunk)
	if err != nil {
		return nil, err
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

func locateHunkStart(lines []string, hunk unifiedHunk) (int, error) {
	oldLines := oldHunkLines(hunk)
	if len(oldLines) == 0 {
		if !hunk.hasLineNumbers {
			return 0, errors.New("cannot locate hunk without context lines")
		}

		startIdx := hunk.oldStart - 1
		if startIdx < 0 || startIdx > len(lines) {
			return 0, fmt.Errorf("invalid old start line %d (file has %d lines)", hunk.oldStart, len(lines))
		}

		return startIdx, nil
	}

	matches := findMatchingLineRanges(lines, oldLines)
	if len(matches) == 0 {
		if hunk.hasLineNumbers {
			return 0, fmt.Errorf("no matching context found near line %d", hunk.oldStart)
		}
		return 0, errors.New("no matching context found for hunk")
	}

	if hunk.hasLineNumbers {
		preferred := hunk.oldStart - 1
		best := matches[0]
		bestDistance := absInt(best - preferred)
		for _, idx := range matches {
			if idx == preferred {
				return idx, nil
			}
			if distance := absInt(idx - preferred); distance < bestDistance {
				best = idx
				bestDistance = distance
			}
		}
		return best, nil
	}

	if len(matches) > 1 {
		return 0, errors.New("hunk context is ambiguous; add more surrounding context")
	}

	return matches[0], nil
}

func oldHunkLines(hunk unifiedHunk) []string {
	lines := make([]string, 0, len(hunk.lines))
	for _, hunkLine := range hunk.lines {
		if len(hunkLine) == 0 {
			continue
		}

		switch hunkLine[0] {
		case ' ', '-':
			lines = append(lines, strings.TrimRight(hunkLine[1:], "\r\n"))
		}
	}

	return lines
}

func findMatchingLineRanges(lines []string, target []string) []int {
	if len(target) == 0 || len(lines) < len(target) {
		return nil
	}

	matches := make([]int, 0, 1)
	for start := 0; start+len(target) <= len(lines); start++ {
		ok := true
		for i := range target {
			if strings.TrimRight(lines[start+i], "\r\n") != target[i] {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, start)
		}
	}

	return matches
}

func isPatchMetadataLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}

	switch {
	case strings.HasPrefix(trimmed, "```"),
		strings.HasPrefix(trimmed, "*** Begin Patch"),
		strings.HasPrefix(trimmed, "*** End Patch"),
		strings.HasPrefix(trimmed, "*** Update File:"),
		strings.HasPrefix(trimmed, "*** End of File"),
		strings.HasPrefix(trimmed, "diff --git "),
		strings.HasPrefix(trimmed, "index "),
		strings.HasPrefix(trimmed, "--- "),
		strings.HasPrefix(trimmed, "+++ "):
		return true
	default:
		return false
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

func patchDiffStats(diff string) (added int, removed int) {
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	return added, removed
}

func buildPatchSummary(path string, added int, removed int, hunksApplied int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "file"
	}

	if added == 0 && removed == 0 {
		return fmt.Sprintf("Edited %s (%d hunk(s))", path, hunksApplied)
	}

	return fmt.Sprintf("Edited %s (+%d -%d)", path, added, removed)
}

func buildPatchDisplayOutput(summary string, diff string) string {
	summary = strings.TrimSpace(summary)
	lines := normalizedPatchLines(diff)
	if len(lines) == 0 {
		return summary
	}

	var b strings.Builder
	b.WriteString(summary)
	b.WriteByte('\n')
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

func normalizedPatchLines(diff string) []string {
	rawLines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		switch {
		case strings.HasPrefix(line, "@@"),
			strings.HasPrefix(line, " "),
			strings.HasPrefix(line, "+"),
			strings.HasPrefix(line, "-"):
			if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
				continue
			}
			lines = append(lines, line)
		}
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}
