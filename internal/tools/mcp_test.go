package tools

import (
	"testing"

	"fimi-cli/internal/mcp"
)

func TestConvertMCPResultToRichContentPreservesStructuredPayloads(t *testing.T) {
	result := &mcp.ToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Text: "hello"},
			mcp.ImageContent{MIMEType: "image/png", Data: []byte("img")},
			mcp.AudioContent{MIMEType: "audio/wav", Data: []byte("aud")},
		},
	}

	got := convertMCPResultToRichContent(result)
	if len(got) != 3 {
		t.Fatalf("len(convertMCPResultToRichContent()) = %d, want 3", len(got))
	}

	if got[0].Type != "text" || got[0].Text != "hello" {
		t.Fatalf("text content = %#v, want text payload", got[0])
	}
	if got[1].Type != "image" || got[1].MIMEType != "image/png" || got[1].Data != "aW1n" {
		t.Fatalf("image content = %#v, want base64-preserved image payload", got[1])
	}
	if got[2].Type != "audio" || got[2].MIMEType != "audio/wav" || got[2].Data != "YXVk" {
		t.Fatalf("audio content = %#v, want base64-preserved audio payload", got[2])
	}
}
