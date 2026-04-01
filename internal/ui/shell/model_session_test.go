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
	for _, want := range []string{"12345678", "fix parser panic", "2.0 kB"} {
		if !strings.Contains(view, want) {
			t.Fatalf("renderSessionSelectView() missing %q in %q", want, view)
		}
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
	if len(gotModel.output.blocks) != 1 {
		t.Fatalf("output blocks = %d, want 1", len(gotModel.output.blocks))
	}
	if gotModel.output.blocks[0].Kind != BlockKindSystemNotice || gotModel.output.blocks[0].Text != "Session deleted. No more sessions available." {
		t.Fatalf("output block = %#v, want deletion notice", gotModel.output.blocks[0])
	}
}
