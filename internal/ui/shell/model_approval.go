package shell

import (
	"fimi-cli/internal/wire"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) handleApprovalKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "ctrl+p":
		m.approvalSelection--
		if m.approvalSelection < 0 {
			m.approvalSelection = 2
		}
		m.syncApprovalSelection()
		return m, nil
	case "down", "ctrl+n":
		m.approvalSelection++
		if m.approvalSelection > 2 {
			m.approvalSelection = 0
		}
		m.syncApprovalSelection()
		return m, nil
	case "enter":
		return m.resolveFirstPending(approvalResponseForSelection(m.approvalSelection))
	default:
		return m, nil
	}
}

func approvalResponseForSelection(selection int) wire.ApprovalResponse {
	switch selection {
	case 0:
		return wire.ApprovalApprove
	case 1:
		return wire.ApprovalApproveForSession
	default:
		return wire.ApprovalReject
	}
}

func (m *Model) syncApprovalSelection() {
	if req := m.currentApprovalRequest(); req != nil {
		m.runtime = m.runtime.UpdateApprovalSelection(req.ID, m.approvalSelection)
		m.output = m.output.SetPending(m.mergeInitialPendingBlocks(m.runtime.ToBlocks()))
	}
}

// resolveFirstPending resolves the first pending approval request.
func (m Model) resolveFirstPending(resp wire.ApprovalResponse) (tea.Model, tea.Cmd) {
	if req := m.currentApprovalRequest(); req != nil {
		return m.resolveApproval(req.ID, resp)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}

// renderApprovalView returns the main transcript-first layout.
func (m Model) renderApprovalView() string {
	leadingSections, trailingSections := m.mainViewLayoutSections()
	var sections []string
	sections = append(sections, leadingSections...)
	if outputView := m.renderOutputForLayout(leadingSections, trailingSections); outputView != "" {
		sections = append(sections, outputView)
	}
	sections = append(sections, trailingSections...)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) currentApprovalRequest() *wire.ApprovalRequest {
	for _, r := range m.pendingApprovals {
		return r
	}
	return nil
}

// resolveApproval completes an approval request.
func (m Model) resolveApproval(id string, resp wire.ApprovalResponse) (Model, tea.Cmd) {
	if req, ok := m.pendingApprovals[id]; ok {
		req.Resolve(resp)
		delete(m.pendingApprovals, id)
	}
	m.runtime = m.runtime.ResolveApproval(id, resp)
	m.output = m.output.SetPending(m.mergeInitialPendingBlocks(m.runtime.ToBlocks()))

	if next := m.currentApprovalRequest(); next != nil {
		m.approvalSelection = 0
		m.mode = ModeApprovalPrompt
		m.runtime = m.runtime.UpdateApprovalSelection(next.ID, 0)
		m.output = m.output.SetPending(m.mergeInitialPendingBlocks(m.runtime.ToBlocks()))
		return m, nil
	}

	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}

func (m Model) approvalViewTrailingSections(_ *wire.ApprovalRequest) []string {
	_, trailingSections := m.mainViewLayoutSections()
	return trailingSections
}
