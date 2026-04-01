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

func TestApprovalRequestRendersInlineTranscriptBlock(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.width = 80
	model.height = 20
	model.input.width = 80
	model.output.width = 80

	updatedModel, cmd := model.Update(approvalRequestMsg{Request: &wire.ApprovalRequest{
		ID:          "req-1",
		Action:      "bash",
		Description: "run go test ./internal/ui/shell",
	}})
	if cmd != nil {
		t.Fatalf("Update(approvalRequestMsg) cmd = %#v, want nil", cmd)
	}

	updated := updatedModel.(Model)
	if updated.mode != ModeApprovalPrompt {
		t.Fatalf("mode = %v, want %v", updated.mode, ModeApprovalPrompt)
	}
	if len(updated.output.pending) != 1 {
		t.Fatalf("pending blocks = %d, want 1", len(updated.output.pending))
	}
	block := updated.output.pending[0]
	if block.Kind != BlockKindApproval {
		t.Fatalf("pending block = %#v, want approval block", block)
	}
	if block.Approval.Selected != 0 {
		t.Fatalf("approval selection = %d, want 0", block.Approval.Selected)
	}

	view := updated.View()
	for _, want := range []string{
		"Approval required",
		"bash",
		"run go test ./internal/ui/shell",
		"Approve for session",
		"fimi>",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}

func TestApprovalRequestKeepsSubmittedUserPromptPending(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	submittedModel, _ := model.handleSubmit("what is your name?")
	submitted := submittedModel.(Model)

	updatedModel, cmd := submitted.Update(approvalRequestMsg{Request: &wire.ApprovalRequest{
		ID:          "req-1",
		Action:      "bash",
		Description: "run go test ./internal/ui/shell",
	}})
	if cmd != nil {
		t.Fatalf("Update(approvalRequestMsg) cmd = %#v, want nil", cmd)
	}

	updated := updatedModel.(Model)
	if len(updated.output.pending) != 2 {
		t.Fatalf("pending blocks = %d, want 2", len(updated.output.pending))
	}
	if updated.output.pending[0].Kind != BlockKindUserPrompt || updated.output.pending[0].UserText != "what is your name?" {
		t.Fatalf("first pending block = %#v, want submitted user prompt", updated.output.pending[0])
	}
	if updated.output.pending[1].Kind != BlockKindApproval {
		t.Fatalf("second pending block = %#v, want approval block", updated.output.pending[1])
	}
}

func TestHandleApprovalKeyPressUpdatesInlineSelectionAndResolvedState(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.width = 80
	model.height = 20
	model.input.width = 80
	model.output.width = 80
	model.mode = ModeApprovalPrompt
	model.pendingApprovals["req-1"] = &wire.ApprovalRequest{
		ID:          "req-1",
		Action:      "write_file",
		Description: "update PLAN.md",
	}
	model.runtime = model.runtime.ApplyApprovalRequest(model.pendingApprovals["req-1"], 0)
	model.output = model.output.SetPending(model.runtime.ToBlocks())

	updatedModel, cmd := model.handleApprovalKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("handleApprovalKeyPress(down) cmd = %#v, want nil", cmd)
	}
	updated := updatedModel.(Model)
	if updated.approvalSelection != 1 {
		t.Fatalf("approvalSelection = %d, want 1", updated.approvalSelection)
	}
	if updated.output.pending[0].Approval.Selected != 1 {
		t.Fatalf("pending approval selection = %d, want 1", updated.output.pending[0].Approval.Selected)
	}

	resolved, cmd := updated.resolveApproval("req-1", wire.ApprovalApproveForSession)
	if cmd == nil {
		t.Fatal("resolveApproval() cmd = nil, want wire receive loop")
	}
	if resolved.mode != ModeThinking {
		t.Fatalf("mode = %v, want %v", resolved.mode, ModeThinking)
	}
	if len(resolved.output.pending) != 1 {
		t.Fatalf("pending blocks after resolve = %d, want 1", len(resolved.output.pending))
	}
	block := resolved.output.pending[0]
	if block.Kind != BlockKindApproval || block.Approval.Status != ApprovalStatusApprovedForSession {
		t.Fatalf("resolved approval block = %#v, want approved-for-session status", block)
	}
	view := resolved.View()
	if !strings.Contains(view, "Approved for session") {
		t.Fatalf("resolved View() = %q, want resolved approval status", view)
	}
	if strings.Contains(view, "> Approve for session") {
		t.Fatalf("resolved View() = %q, want options hidden after resolution", view)
	}
}
