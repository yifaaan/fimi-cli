package shell

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/x/ansi"
)

var editDiffHunkHeaderRE = regexp.MustCompile(`@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@`)

type editDiffLineKind int

const (
	editDiffLineContext editDiffLineKind = iota
	editDiffLineAdd
	editDiffLineRemove
	editDiffLineHunk
)

type editDiffLine struct {
	Kind    editDiffLineKind
	Raw     string
	Text    string
	OldLine int
	NewLine int
}

type editDiffPreview struct {
	Lines []editDiffLine
}

func parseEditDiffPreview(content string) (editDiffPreview, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return editDiffPreview{}, false
	}

	rawLines := strings.Split(content, "\n")
	if len(rawLines) < 2 {
		return editDiffPreview{}, false
	}

	summary := strings.TrimSpace(rawLines[0])
	if !strings.HasPrefix(summary, "Edited ") {
		return editDiffPreview{}, false
	}

	preview := editDiffPreview{
		Lines: make([]editDiffLine, 0, len(rawLines)-1),
	}

	var oldLine int
	var newLine int
	hasDiff := false

	for _, raw := range rawLines[1:] {
		switch {
		case strings.HasPrefix(raw, "@@"):
			oldLine, newLine = parseEditDiffHunkHeader(raw)
			preview.Lines = append(preview.Lines, editDiffLine{
				Kind: editDiffLineHunk,
				Raw:  raw,
			})
			hasDiff = true
		case strings.HasPrefix(raw, " "):
			preview.Lines = append(preview.Lines, editDiffLine{
				Kind:    editDiffLineContext,
				Raw:     raw,
				Text:    raw[1:],
				OldLine: oldLine,
				NewLine: newLine,
			})
			oldLine++
			newLine++
			hasDiff = true
		case strings.HasPrefix(raw, "-"):
			preview.Lines = append(preview.Lines, editDiffLine{
				Kind:    editDiffLineRemove,
				Raw:     raw,
				Text:    raw[1:],
				OldLine: oldLine,
			})
			oldLine++
			hasDiff = true
		case strings.HasPrefix(raw, "+"):
			preview.Lines = append(preview.Lines, editDiffLine{
				Kind:    editDiffLineAdd,
				Raw:     raw,
				Text:    raw[1:],
				NewLine: newLine,
			})
			newLine++
			hasDiff = true
		}
	}

	return preview, hasDiff
}

func parseEditDiffHunkHeader(raw string) (oldLine int, newLine int) {
	matches := editDiffHunkHeaderRE.FindStringSubmatch(raw)
	if matches == nil {
		return 0, 0
	}

	oldLine, _ = strconv.Atoi(matches[1])
	newLine, _ = strconv.Atoi(matches[2])
	return oldLine, newLine
}

func renderEditDiffPreviewBody(summary string, content string, expanded bool) (string, bool) {
	return renderEditDiffPreviewBodyWithWidth(summary, content, expanded, 0)
}

func renderEditDiffPreviewBodyWithWidth(summary string, content string, expanded bool, width int) (string, bool) {
	summary = strings.TrimSpace(summary)
	content = strings.TrimSpace(content)
	if summary == "" || content == "" {
		return "", false
	}

	preview, ok := parseEditDiffPreview(summary + "\n" + content)
	if !ok {
		return "", false
	}

	actionHint := previewToggleHint(false)
	if expanded {
		actionHint = previewToggleHint(true)
	}

	visible, hidden := truncateEditDiffLines(preview.Lines, expanded)
	lines := make([]string, 0, len(visible)+1)
	lineNumberWidth := editDiffLineNumberWidth(preview.Lines)
	seenHunk := false

	for _, line := range visible {
		rendered, ok := renderEditDiffLine(line, lineNumberWidth, seenHunk, width)
		if line.Kind == editDiffLineHunk {
			seenHunk = true
		}
		if !ok {
			continue
		}
		lines = append(lines, rendered)
	}

	if hidden > 0 {
		lines = append(lines, renderPreviewFooter("     ", PreviewKindDiff, hidden, "diff lines", actionHint))
	} else if expanded {
		lines = append(lines, renderPreviewFooter("     ", PreviewKindDiff, 0, "", actionHint))
	}

	return strings.Join(lines, "\n"), true
}

func renderEditDiffLine(line editDiffLine, lineNumberWidth int, seenHunk bool, width int) (string, bool) {
	switch line.Kind {
	case editDiffLineHunk:
		if !seenHunk {
			return "", false
		}
		return styles.ToolDiffHunkStyle.Render(formatDiffSpacerLine(lineNumberWidth, "⋮")), true
	case editDiffLineAdd:
		return styles.ToolDiffAddedStyle.Render(renderWrappedDiffNumberedLine("+", line.NewLine, line.Text, lineNumberWidth, width)), true
	case editDiffLineRemove:
		return styles.ToolDiffRemovedStyle.Render(renderWrappedDiffNumberedLine("-", line.OldLine, line.Text, lineNumberWidth, width)), true
	default:
		lineNo := line.NewLine
		if lineNo <= 0 {
			lineNo = line.OldLine
		}
		return styles.ToolDiffContextStyle.Render(renderWrappedDiffNumberedLine(" ", lineNo, line.Text, lineNumberWidth, width)), true
	}
}

func previewToggleHint(expanded bool) string {
	if expanded {
		return "Ctrl+O collapse"
	}
	return "Ctrl+O expand"
}

func formatDiffNumberedLine(prefix string, lineNo int, text string, lineNumberWidth int) string {
	if lineNumberWidth < 1 {
		lineNumberWidth = 1
	}
	return fmt.Sprintf("     %*s %s%s", lineNumberWidth, formatDiffLineNo(lineNo), prefix, text)
}

func formatDiffSpacerLine(lineNumberWidth int, marker string) string {
	if lineNumberWidth < 1 {
		lineNumberWidth = 1
	}
	return fmt.Sprintf("     %*s %s", lineNumberWidth, "", marker)
}

func renderWrappedDiffNumberedLine(prefix string, lineNo int, text string, lineNumberWidth int, width int) string {
	firstPrefix := formatDiffLinePrefix(prefix, lineNo, lineNumberWidth)
	if width <= 0 {
		return firstPrefix + text
	}

	contentWidth := width - ansi.StringWidthWc(firstPrefix)
	if contentWidth < 1 {
		contentWidth = 1
	}

	wrapped := ansi.HardwrapWc(text, contentWidth, true)
	segments := []string{""}
	if wrapped != "" {
		segments = strings.Split(wrapped, "\n")
	}

	continuationPrefix := strings.Repeat(" ", ansi.StringWidthWc(firstPrefix))
	lines := make([]string, 0, len(segments))
	for idx, segment := range segments {
		if idx == 0 {
			lines = append(lines, firstPrefix+segment)
			continue
		}
		lines = append(lines, continuationPrefix+segment)
	}

	return strings.Join(lines, "\n")
}

func formatDiffLinePrefix(prefix string, lineNo int, lineNumberWidth int) string {
	if lineNumberWidth < 1 {
		lineNumberWidth = 1
	}
	return fmt.Sprintf("     %*s %s", lineNumberWidth, formatDiffLineNo(lineNo), prefix)
}

func formatDiffLineNo(lineNo int) string {
	if lineNo <= 0 {
		return ""
	}

	return strconv.Itoa(lineNo)
}

func editDiffLineNumberWidth(lines []editDiffLine) int {
	maxLineNo := 0
	for _, line := range lines {
		switch line.Kind {
		case editDiffLineAdd:
			if line.NewLine > maxLineNo {
				maxLineNo = line.NewLine
			}
		case editDiffLineRemove:
			if line.OldLine > maxLineNo {
				maxLineNo = line.OldLine
			}
		case editDiffLineContext:
			if line.NewLine > maxLineNo {
				maxLineNo = line.NewLine
			}
			if line.NewLine <= 0 && line.OldLine > maxLineNo {
				maxLineNo = line.OldLine
			}
		}
	}

	if maxLineNo <= 0 {
		return 1
	}

	return len(strconv.Itoa(maxLineNo))
}

func truncateEditDiffLines(lines []editDiffLine, expanded bool) ([]editDiffLine, int) {
	totalVisible := countVisibleEditDiffLines(lines)
	limit := diffPreviewLineLimit
	if expanded {
		limit = expandedPreviewLineLimit
	}
	if totalVisible <= limit {
		return append([]editDiffLine(nil), lines...), 0
	}

	truncated := make([]editDiffLine, 0, len(lines))
	visibleCount := 0
	seenHunk := false
	for _, line := range lines {
		lineVisible := editDiffRenderedLineCount(line, seenHunk)
		if line.Kind == editDiffLineHunk {
			seenHunk = true
		}
		if lineVisible == 0 {
			truncated = append(truncated, line)
			continue
		}
		if visibleCount+lineVisible > limit {
			break
		}
		truncated = append(truncated, line)
		visibleCount += lineVisible
	}

	return truncated, totalVisible - visibleCount
}

func countVisibleEditDiffLines(lines []editDiffLine) int {
	total := 0
	seenHunk := false
	for _, line := range lines {
		total += editDiffRenderedLineCount(line, seenHunk)
		if line.Kind == editDiffLineHunk {
			seenHunk = true
		}
	}
	return total
}

func editDiffRenderedLineCount(line editDiffLine, seenHunk bool) int {
	if line.Kind == editDiffLineHunk && !seenHunk {
		return 0
	}
	return 1
}
