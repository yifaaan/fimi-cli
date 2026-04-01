package shell

import (
	"strings"
	"testing"

	"fimi-cli/internal/wire"

	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalResponseForSelection(t *testing.T) {
	tests := []struct {
		name      string
		selection int
		want      wire.ApprovalResponse
	}{
		{name: "approve", selection: 0, want: wire.ApprovalApprove},
		{name: "approve for session", selection: 1, want: wire.ApprovalApproveForSession},
		{name: "reject default", selection: 2, want: wire.ApprovalReject},
		{name: "reject fallback", selection: 99, want: wire.ApprovalReject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := approvalResponseForSelection(tt.selection); got != tt.want {
				t.Fatalf("approvalResponseForSelection(%d) = %q, want %q", tt.selection, got, tt.want)
			}
		})
	}
}

func TestHandleApprovalKeyPressWrapsSelection(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeApprovalPrompt

	updatedModel, cmd := model.handleApprovalKeyPress(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("handleApprovalKeyPress(up) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.approvalSelection != 2 {
		t.Fatalf("approvalSelection after up = %d, want 2", updated.approvalSelection)
	}

	updatedModel, cmd = updated.handleApprovalKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("handleApprovalKeyPress(down) cmd = %#v, want nil", cmd)
	}
	updated = updatedModel.(Model)
	if updated.approvalSelection != 0 {
		t.Fatalf("approvalSelection after down = %d, want 0", updated.approvalSelection)
	}
}

func TestHandleKeyPressDelegatesApprovalMode(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeApprovalPrompt

	updatedModel, cmd := model.handleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("handleKeyPress(down) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.approvalSelection != 1 {
		t.Fatalf("approvalSelection = %d, want 1", updated.approvalSelection)
	}
}

func TestResolveFirstPendingWithoutRequestsResumesThinking(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeApprovalPrompt

	updatedModel, cmd := model.resolveFirstPending(wire.ApprovalApprove)
	if cmd == nil {
		t.Fatal("resolveFirstPending() cmd = nil, want wire receive loop")
	}
	updated := updatedModel.(Model)
	if updated.mode != ModeThinking {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeThinking)
	}
}

func TestRenderApprovalViewShowsRequestDetails(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeApprovalPrompt
	model.width = 80
	model.height = 20
	model.pendingApprovals["req-1"] = &wire.ApprovalRequest{
		ID:          "req-1",
		Action:      "bash_execute",
		Description: "run go test ./...",
	}

	view := model.renderApprovalView()
	for _, want := range []string{
		"bash_execute requires approval",
		"run go test ./...",
		"Approve for session",
		"Reject",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("renderApprovalView() missing %q in %q", want, view)
		}
	}
}
