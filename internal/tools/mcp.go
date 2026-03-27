package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"fimi-cli/internal/mcp"
	"fimi-cli/internal/runtime"
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

		// Convert result to string output
		output := convertMCPResultToString(result)

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
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