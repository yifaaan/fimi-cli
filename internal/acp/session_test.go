package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/wire"
)

func newTestACPSession(t *testing.T) (*Session, *bytes.Buffer) {
	t.Helper()

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	workDir := t.TempDir()
	sess, err := session.New(workDir)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	var out bytes.Buffer
	conn := NewFramedConn(bytes.NewReader(nil), &out)
	return NewSession(sess, conn, "test-model"), &out
}

func TestSessionVisualizeWireSendsApprovalRequestAndResolvesIt(t *testing.T) {
	acpSession, out := newTestACPSession(t)
	messages := make(chan wire.Message, 1)
	done := make(chan error, 1)

	go func() {
		done <- acpSession.VisualizeWire()(context.Background(), messages)
	}()

	req := &wire.ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "call-1",
		Action:      "bash",
		Description: "ls",
	}

	waitDone := make(chan wire.ApprovalResponse, 1)
	go func() {
		resp, err := req.Wait(context.Background())
		if err != nil {
			t.Errorf("ApprovalRequest.Wait() error = %v", err)
			return
		}
		waitDone <- resp
	}()

	messages <- req
	close(messages)

	if err := <-done; err != nil {
		t.Fatalf("VisualizeWire() error = %v", err)
	}

	if !strings.Contains(out.String(), "\"session_update\":\"approval_request\"") {
		t.Fatalf("approval notification missing from %q", out.String())
	}
	if !strings.Contains(out.String(), "\"tool_call_id\":\"call-1\"") {
		t.Fatalf("approval notification missing tool_call_id in %q", out.String())
	}

	if err := acpSession.ResolveApproval("approval-1", wire.ApprovalApprove); err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}

	resp := <-waitDone
	if resp != wire.ApprovalApprove {
		t.Fatalf("approval response = %q, want %q", resp, wire.ApprovalApprove)
	}
}

func TestSessionVisualizeWireProjectsToolCallPartAndRichResult(t *testing.T) {
	acpSession, out := newTestACPSession(t)
	messages := make(chan wire.Message, 3)

	messages <- wire.EventMessage{Event: runtimeevents.ToolCallPart{
		ToolCallID: "call-1",
		Delta:      "{\"command\":\"ls\"}",
	}}
	messages <- wire.EventMessage{Event: runtimeevents.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		Subtitle: "ls",
	}}
	messages <- wire.EventMessage{Event: runtimeevents.ToolResult{
		ToolCallID:    "call-1",
		ToolName:      "mcp_tool",
		DisplayOutput: "preview",
		Content: []runtimeevents.RichContent{
			{Type: "image", MIMEType: "image/png", Data: "aGVsbG8="},
		},
	}}
	close(messages)

	if err := acpSession.VisualizeWire()(context.Background(), messages); err != nil {
		t.Fatalf("VisualizeWire() error = %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("notification count = %d, want 3", len(lines))
	}

	var first, second, third recordedNotification
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("json.Unmarshal(first) error = %v", err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("json.Unmarshal(second) error = %v", err)
	}
	if err := json.Unmarshal(lines[2], &third); err != nil {
		t.Fatalf("json.Unmarshal(third) error = %v", err)
	}

	if !strings.Contains(string(first.Params), "\"session_update\":\"tool_call_start\"") {
		t.Fatalf("first update = %s, want tool_call_start", first.Params)
	}
	if !strings.Contains(string(first.Params), "\\\"command\\\":\\\"ls\\\"") {
		t.Fatalf("first update missing tool delta in %s", first.Params)
	}
	if !strings.Contains(string(second.Params), "\"title\":\"bash(ls)\"") {
		t.Fatalf("second update missing resolved title in %s", second.Params)
	}
	if !strings.Contains(string(third.Params), "\"mime_type\":\"image/png\"") {
		t.Fatalf("third update missing image mime type in %s", third.Params)
	}
	if !strings.Contains(string(third.Params), "\"data\":\"aGVsbG8=\"") {
		t.Fatalf("third update missing image data in %s", third.Params)
	}
}
