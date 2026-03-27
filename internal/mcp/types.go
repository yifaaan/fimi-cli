package mcp

// Tool describes one MCP tool discovered from a server.
// InputSchema is a JSON Schema object.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ToolResult struct {
	Content []Content
	IsError bool
}

type Content interface {
	contentMarker()
}

type TextContent struct {
	Text string
}

func (TextContent) contentMarker() {}

type ImageContent struct {
	Data     []byte
	MIMEType string
}

func (ImageContent) contentMarker() {}

type AudioContent struct {
	Data     []byte
	MIMEType string
}

func (AudioContent) contentMarker() {}
