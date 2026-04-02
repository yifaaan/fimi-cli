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

func TestBuildACPContentItemsFallsBackToTextForEmptyContent(t *testing.T) {
	items := buildACPContentItems(nil, "preview")
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Type != "text" {
		t.Fatalf("items[0].Type = %q, want text", items[0].Type)
	}
	if items[0].Content.Text != "preview" {
		t.Fatalf("items[0].Content.Text = %q, want preview", items[0].Content.Text)
	}
}

func TestBuildACPContentItemsPrependsFallbackForNonTextContent(t *testing.T) {
	items := buildACPContentItems([]runtimeevents.RichContent{{Type: "image", MIMEType: "image/png", Data: "aGVsbG8="}}, "preview")
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Type != "text" {
		t.Fatalf("items[0].Type = %q, want text", items[0].Type)
	}
	if items[0].Content.Text != "preview" {
		t.Fatalf("items[0].Content.Text = %q, want preview", items[0].Content.Text)
	}
	if items[1].Type != "image" {
		t.Fatalf("items[1].Type = %q, want image", items[1].Type)
	}
	if items[1].Content.MIMEType != "image/png" {
		t.Fatalf("items[1].Content.MIMEType = %q, want image/png", items[1].Content.MIMEType)
	}
}

func TestBuildACPContentItemsDoesNotDuplicateFallbackForTextContent(t *testing.T) {
	items := buildACPContentItems([]runtimeevents.RichContent{{Type: "text", Text: "hello"}}, "preview")
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Type != "text" {
		t.Fatalf("items[0].Type = %q, want text", items[0].Type)
	}
	if items[0].Content.Text != "hello" {
		t.Fatalf("items[0].Content.Text = %q, want hello", items[0].Content.Text)
	}
}

func TestFirstNonEmptyToolOutputPrefersDisplayOutputAndTruncatesLongOutput(t *testing.T) {
	longOutput := strings.Repeat("x", 10001)

	got := firstNonEmptyToolOutput("preview", longOutput)
	if got != "preview" {
		t.Fatalf("firstNonEmptyToolOutput() = %q, want preview", got)
	}

	got = firstNonEmptyToolOutput("  ", longOutput)
	if !strings.HasSuffix(got, "\n... (truncated)") {
		t.Fatalf("firstNonEmptyToolOutput() suffix = %q, want truncation marker", got)
	}
	if len(got) != 10016 {
		t.Fatalf("len(firstNonEmptyToolOutput()) = %d, want 10016", len(got))
	}
}

func TestSessionAccessorsAndCancellation(t *testing.T) {
	acpSession, _ := newTestACPSession(t)

	if acpSession.CurrentModelID() != "test-model" {
		t.Fatalf("CurrentModelID() = %q, want test-model", acpSession.CurrentModelID())
	}

	acpSession.SetModelID("other-model")
	if acpSession.CurrentModelID() != "other-model" {
		t.Fatalf("CurrentModelID() after SetModelID = %q, want other-model", acpSession.CurrentModelID())
	}

	cancelled := 0
	acpSession.SetCancel(func() { cancelled++ })
	acpSession.Cancel()
	acpSession.Cancel()
	if cancelled != 1 {
		t.Fatalf("cancelled = %d, want 1", cancelled)
	}

	if !strings.HasSuffix(acpSession.HistoryFile(), ".jsonl") {
		t.Fatalf("HistoryFile() = %q, want .jsonl suffix", acpSession.HistoryFile())
	}
}
