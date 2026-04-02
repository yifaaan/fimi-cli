package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"fimi-cli/internal/mcp"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

// NewMCPToolHandler creates a HandlerFunc that delegates to an MCP tool.
func NewMCPToolHandler(client *mcp.Client, tool mcp.Tool) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, _ Definition) (runtime.ToolExecution, error) {
		// Parse arguments JSON into a map
		var args map[string]any
		if call.Arguments != "" {
			if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
				return runtime.ToolExecution{}, markRefused(fmt.Errorf("parse tool arguments: %w", err))
			}
		}

		// Call the MCP tool
		result, err := client.CallTool(ctx, tool.Name, args)
		if err != nil {
			// MCP errors are temporary (network/server issues), not refused
			return runtime.ToolExecution{}, markTemporary(err)
		}

		output := convertMCPResultToString(result)
		content := convertMCPResultToRichContent(result)

		return runtime.ToolExecution{
			Call:          call,
			Output:        output,
			DisplayOutput: output,
			Content:       content,
		}, nil
	}
}

// convertMCPResultToString converts MCP tool result content to a single string.
func convertMCPResultToString(result *mcp.ToolResult) string {
	var sb string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			sb += c.Text
		case mcp.ImageContent:
			sb += fmt.Sprintf("\n[Image: %s, %d bytes]", c.MIMEType, len(c.Data))
		case mcp.AudioContent:
			sb += fmt.Sprintf("\n[Audio: %s, %d bytes]", c.MIMEType, len(c.Data))
		}
	}

	// Add error prefix if the tool reported an error
	if result.IsError {
		return "[Tool Error] " + sb
	}
	return sb
}

func convertMCPResultToRichContent(result *mcp.ToolResult) []runtimeevents.RichContent {
	if result == nil || len(result.Content) == 0 {
		return nil
	}

	content := make([]runtimeevents.RichContent, 0, len(result.Content))
	for _, item := range result.Content {
		switch c := item.(type) {
		case mcp.TextContent:
			content = append(content, runtimeevents.RichContent{
				Type: "text",
				Text: c.Text,
			})
		case mcp.ImageContent:
			content = append(content, runtimeevents.RichContent{
				Type:     "image",
				MIMEType: c.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(c.Data),
			})
		case mcp.AudioContent:
			content = append(content, runtimeevents.RichContent{
				Type:     "audio",
				MIMEType: c.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(c.Data),
			})
		}
	}

	return content
}
