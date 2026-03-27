package mcp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps an MCP client session for tool discovery and execution.
type Client struct {
	name    string
	client  *mcp.Client
	session *mcp.ClientSession
	tools   []Tool
}

// NewClient creates a new MCP client connected to a server via stdio transport.
// The server process is spawned immediately and tools are discovered on connection.
func NewClient(ctx context.Context, name string, command string, args []string, env map[string]string) (*Client, error) {
	// Build the command
	cmd := exec.CommandContext(ctx, command, args...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Create MCP client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "fimi-cli",
		Version: "0.1.0",
	}, nil)

	// Connect via stdio transport
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP server %q: %w", name, err)
	}

	// List tools
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("list tools from MCP server %q: %w", name, err)
	}

	// Convert tools to our type
	tools := make([]Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		tool := Tool{
			Name:        t.Name,
			Description: t.Description,
		}
		if t.InputSchema != nil {
			if m, ok := t.InputSchema.(map[string]any); ok {
				tool.InputSchema = m
			}
		}
		tools = append(tools, tool)
	}

	return &Client{
		name:    name,
		client:  client,
		session: session,
		tools:   tools,
	}, nil
}

// Close closes the MCP client session.
func (c *Client) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// Tools returns the list of tools discovered from this server.
func (c *Client) Tools() []Tool {
	return c.tools
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", name, err)
	}

	// Convert content
	content := make([]Content, 0, len(result.Content))
	for _, c := range result.Content {
		switch ct := c.(type) {
		case *mcp.TextContent:
			content = append(content, TextContent{Text: ct.Text})
		case *mcp.ImageContent:
			content = append(content, ImageContent{
				Data:     ct.Data,
				MIMEType: ct.MIMEType,
			})
		case *mcp.AudioContent:
			content = append(content, AudioContent{
				Data:     ct.Data,
				MIMEType: ct.MIMEType,
			})
		}
	}

	return &ToolResult{
		Content: content,
		IsError: result.IsError,
	}, nil
}