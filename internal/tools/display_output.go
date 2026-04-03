package tools

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const toolPreviewLineLimit = 8

type previewSection struct {
	Label   string
	Content string
}

func buildInlinePreview(title string, content string) string {
	title = strings.TrimSpace(title)
	lines, hidden := clipPreviewLines(content, toolPreviewLineLimit)

	var parts []string
	if title != "" {
		parts = append(parts, title)
	}
	parts = append(parts, lines...)
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("... +%d lines", hidden))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildSectionedPreview(title string, sections ...previewSection) string {
	var parts []string
	if title = strings.TrimSpace(title); title != "" {
		parts = append(parts, title)
	}

	for _, section := range sections {
		label := strings.TrimSpace(section.Label)
		lines, hidden := clipPreviewLines(section.Content, toolPreviewLineLimit)
		if label == "" && len(lines) == 0 && hidden == 0 {
			continue
		}
		if label != "" {
			parts = append(parts, label+":")
		}
		parts = append(parts, lines...)
		if hidden > 0 {
			parts = append(parts, fmt.Sprintf("... +%d lines", hidden))
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildReplaceDisplayOutput(path string, oldText string, newText string) string {
	summary := fmt.Sprintf("Updated %s (+%d -%d)", strings.TrimSpace(path), nonEmptyLineCount(newText), nonEmptyLineCount(oldText))
	lines := make([]string, 0, nonEmptyLineCount(oldText)+nonEmptyLineCount(newText))
	lines = appendPrefixedLines(lines, "-", oldText)
	lines = appendPrefixedLines(lines, "+", newText)
	return buildInlinePreview(summary, strings.Join(lines, "\n"))
}

func appendPrefixedLines(dst []string, prefix string, text string) []string {
	for _, line := range previewLines(text) {
		dst = append(dst, prefix+line)
	}
	return dst
}

func clipPreviewLines(text string, limit int) ([]string, int) {
	lines := previewLines(text)
	if limit <= 0 || len(lines) <= limit {
		return lines, 0
	}
	return lines[:limit], len(lines) - limit
}

func previewLines(text string) []string {
	text = ansi.Strip(text)
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func nonEmptyLineCount(text string) int {
	return len(previewLines(text))
}
