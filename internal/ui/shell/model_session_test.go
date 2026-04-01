package shell

import (
	"strings"
	"testing"
	"time"

	"fimi-cli/internal/session"
)

func TestRenderSessionSelectViewUsesPreviewText(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeSessionSelect
	model.height = 12
	model.sessionList = []session.SessionInfo{
		{
			ID:           "1234567890abcdef",
			Preview:      "fix parser panic",
			FileSize:     2048,
			LastModified: time.Now().Add(-2 * time.Hour),
		},
	}

	view := model.renderSessionSelectView()
	if !strings.Contains(view, "12345678") {
		t.Fatalf("renderSessionSelectView() = %q, want short session ID", view)
	}
	if !strings.Contains(view, "fix parser panic") {
		t.Fatalf("renderSessionSelectView() = %q, want preview text", view)
	}
	if !strings.Contains(view, "2.0 kB") {
		t.Fatalf("renderSessionSelectView() = %q, want formatted file size", view)
	}
}

func TestHandleSessionDeleteResultRemovesLastSession(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeSessionSelect
	model.sessionList = []session.SessionInfo{{ID: "session-1"}}

	updated, cmd := model.handleSessionDeleteResult(SessionDeleteMsg{SessionID: "session-1"})
	if cmd != nil {
		t.Fatalf("handleSessionDeleteResult() cmd = %#v, want nil", cmd)
	}

	gotModel := updated.(Model)
	if gotModel.mode != ModeIdle {
		t.Fatalf("mode = %v, want %v", gotModel.mode, ModeIdle)
	}
	if gotModel.sessionList != nil {
		t.Fatalf("sessionList = %#v, want nil", gotModel.sessionList)
	}
	if len(gotModel.output.lines) != 1 {
		t.Fatalf("output lines = %d, want 1", len(gotModel.output.lines))
	}
	if gotModel.output.lines[0].Content != "Session deleted. No more sessions available." {
		t.Fatalf("output line = %q, want deletion notice", gotModel.output.lines[0].Content)
	}
}
