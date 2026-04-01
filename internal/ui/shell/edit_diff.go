package shell

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"fimi-cli/internal/ui/shell/styles"
)

const (
	editDiffPreviewLines  = 8
	editDiffExpandedLines = 40
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
	Summary string
	Lines   []editDiffLine
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
		Summary: summary,
		Lines:   make([]editDiffLine, 0, len(rawLines)-1),
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

func renderEditDiffToolResult(content string, expanded bool) (string, bool) {
	preview, ok := parseEditDiffPreview(content)
	if !ok {
		return "", false
	}

	limit := editDiffPreviewLines
	actionHint := "Ctrl+O to expand"
	if expanded {
		limit = editDiffExpandedLines
		actionHint = "Ctrl+O to collapse"
	}

	lines := []string{styles.ToolEditSummaryStyle.Render("  ⎿  " + preview.Summary)}
	visible := preview.Lines
	hidden := 0
	if len(visible) > limit {
		hidden = len(visible) - limit
		visible = visible[:limit]
	}

	for _, line := range visible {
		lines = append(lines, renderEditDiffLine(line))
	}

	if hidden > 0 {
		lines = append(lines, styles.HelpStyle.Render(fmt.Sprintf("     ... %d more diff lines hidden (%s)", hidden, actionHint)))
	} else {
		lines = append(lines, styles.HelpStyle.Render("     ("+actionHint+")"))
	}

	return strings.Join(lines, "\n"), true
}

func renderEditDiffLine(line editDiffLine) string {
	switch line.Kind {
	case editDiffLineHunk:
		return styles.ToolDiffHunkStyle.Render("     " + line.Raw)
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
