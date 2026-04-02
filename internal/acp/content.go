package acp

import (
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func buildACPContentItems(content []runtimeevents.RichContent, fallbackText string) []ToolCallContentItem {
	if len(content) == 0 {
		return buildTextContentItems(fallbackText)
	}

	items := make([]ToolCallContentItem, 0, len(content)+1)
	if fallbackText != "" && hasNonTextContent(content) {
		items = append(items, ToolCallContentItem{
			Type:    "text",
			Content: ContentBlock{Type: "text", Text: fallbackText},
		})
	}

	for _, item := range content {
		block := ContentBlock{Type: item.Type, Text: item.Text, MIMEType: item.MIMEType, Data: item.Data}
		itemType := item.Type
		if itemType == "" {
			itemType = "text"
			block.Type = "text"
		}
		items = append(items, ToolCallContentItem{
			Type:    itemType,
			Content: block,
		})
	}

	return items
}

func buildTextContentItems(text string) []ToolCallContentItem {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	return []ToolCallContentItem{{
		Type:    "text",
		Content: ContentBlock{Type: "text", Text: text},
	}}
}

func hasNonTextContent(content []runtimeevents.RichContent) bool {
	for _, item := range content {
		if item.Type != "" && item.Type != "text" {
			return true
		}
	}
	return false
}

func firstNonEmptyToolOutput(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			const maxOutputLen = 10000
			if len(value) > maxOutputLen {
				return value[:maxOutputLen] + "\n... (truncated)"
			}
			return value
		}
	}
	return ""
}
