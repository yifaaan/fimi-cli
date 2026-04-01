package shell

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"fimi-cli/internal/ui/shell/styles"
)

const (
	editDiffCollapsedContextLines = 1
)

var editDiffHunkHeaderRE = regexp.MustCompile(`@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@`)

type editDiffLineKind int

const (
	editDiffLineContext editDiffLineKind = iota
	editDiffLineAdd
	editDiffLineRemove
	editDiffLineHunk
	editDiffLineOmitted
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
	summary = strings.TrimSpace(summary)
	content = strings.TrimSpace(content)
	if summary == "" || content == "" {
		return "", false
	}

	preview, ok := parseEditDiffPreview(summary + "\n" + content)
	if !ok {
		return "", false
	}

	actionHint := "Ctrl+O to expand"
	if expanded {
		actionHint = "Ctrl+O to collapse"
	}

	visible, hidden := collapseEditDiffContext(preview.Lines, expanded)
	lines := make([]string, 0, len(visible)+1)

	for _, line := range visible {
		lines = append(lines, renderEditDiffLine(line))
	}

	if hidden > 0 {
		lines = append(lines, styles.HelpStyle.Render(fmt.Sprintf("     ... %d unchanged lines hidden (%s)", hidden, actionHint)))
	} else if expanded {
		lines = append(lines, styles.HelpStyle.Render("     ("+actionHint+")"))
	}

	return strings.Join(lines, "\n"), true
}

func renderEditDiffLine(line editDiffLine) string {
	switch line.Kind {
	case editDiffLineHunk:
		return styles.ToolDiffHunkStyle.Render("     " + line.Raw)
	case editDiffLineOmitted:
		return styles.HelpStyle.Render("     ...")
	case editDiffLineAdd:
		return styles.ToolDiffAddedStyle.Render(formatDiffNumberedLine("+", line.NewLine, line.Text))
	case editDiffLineRemove:
		return styles.ToolDiffRemovedStyle.Render(formatDiffNumberedLine("-", line.OldLine, line.Text))
	default:
		lineNo := line.NewLine
		if lineNo <= 0 {
			lineNo = line.OldLine
		}
		return styles.ToolDiffContextStyle.Render(formatDiffNumberedLine(" ", lineNo, line.Text))
	}
}

func formatDiffNumberedLine(prefix string, lineNo int, text string) string {
	if lineNo > 0 {
		return fmt.Sprintf("     %s%4d %s", prefix, lineNo, text)
	}

	return fmt.Sprintf("     %s     %s", prefix, text)
}

func collapseEditDiffContext(lines []editDiffLine, expanded bool) ([]editDiffLine, int) {
	if expanded {
		return append([]editDiffLine(nil), lines...), 0
	}

	collapsed := make([]editDiffLine, 0, len(lines))
	hidden := 0

	for i := 0; i < len(lines); {
		if lines[i].Kind != editDiffLineContext {
			collapsed = append(collapsed, lines[i])
			i++
			continue
		}

		j := i
		for j < len(lines) && lines[j].Kind == editDiffLineContext {
			j++
		}

		run := lines[i:j]
		kept, omitted := collapseEditDiffContextRun(lines, i, j)
		collapsed = append(collapsed, kept...)
		hidden += omitted
		i = j
		_ = run
	}

	return collapsed, hidden
}

func collapseEditDiffContextRun(lines []editDiffLine, start int, end int) ([]editDiffLine, int) {
	run := lines[start:end]
	if len(run) <= editDiffCollapsedContextLines*2 {
		return append([]editDiffLine(nil), run...), 0
	}

	prevKind := editDiffLineKind(-1)
	if start > 0 {
		prevKind = lines[start-1].Kind
	}
	nextKind := editDiffLineKind(-1)
	if end < len(lines) {
		nextKind = lines[end].Kind
	}

	keepHead := editDiffCollapsedContextLines
	keepTail := editDiffCollapsedContextLines

	if prevKind == editDiffLineHunk || prevKind == -1 {
		keepHead = 0
	}
	if nextKind == editDiffLineHunk || nextKind == -1 {
		keepTail = 0
	}
	if keepHead == 0 && keepTail == 0 {
		keepTail = editDiffCollapsedContextLines
	}

	if len(run) <= keepHead+keepTail {
		return append([]editDiffLine(nil), run...), 0
	}

	collapsed := make([]editDiffLine, 0, keepHead+keepTail+1)
	if keepHead > 0 {
		collapsed = append(collapsed, run[:keepHead]...)
	}
	collapsed = append(collapsed, editDiffLine{Kind: editDiffLineOmitted})
	if keepTail > 0 {
		collapsed = append(collapsed, run[len(run)-keepTail:]...)
	}
	return collapsed, len(run) - keepHead - keepTail
}
