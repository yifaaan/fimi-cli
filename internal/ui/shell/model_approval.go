package shell

import (
	"fmt"

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
		return m, nil
	case "down", "ctrl+n":
		m.approvalSelection++
		if m.approvalSelection > 2 {
			m.approvalSelection = 0
		}
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

// resolveFirstPending resolves the first pending approval request.
func (m Model) resolveFirstPending(resp wire.ApprovalResponse) (tea.Model, tea.Cmd) {
	for id := range m.pendingApprovals {
		return m.resolveApproval(id, resp)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}

// renderApprovalView renders the full-screen approval prompt.
func (m Model) renderApprovalView() string {
	req := m.currentApprovalRequest()
	if req == nil {
		return ""
	}

	trailingSections := m.approvalViewTrailingSections(req)

	var sections []string
	outputView := m.renderOutputForLayout(nil, trailingSections)
	if outputView != "" {
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

func (m Model) approvalViewTrailingSections(req *wire.ApprovalRequest) []string {
	var trailingSections []string

	panelWidth := min(m.width-4, 60)
	if panelWidth < 30 {
		panelWidth = 30
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6B6B")).
		Bold(true)

	actionLabel := fmt.Sprintf("  %s requires approval", req.Action)
	trailingSections = append(trailingSections, headerStyle.Render(actionLabel))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA"))
	trailingSections = append(trailingSections, descStyle.Render("  "+req.Description))
	trailingSections = append(trailingSections, "")

	options := []struct {
		label string
		icon  string
	}{
		{"Approve", "✓"},
		{"Approve for session", "✓"},
		{"Reject", "✗"},
	}

	for i, opt := range options {
		if i == m.approvalSelection {
			selected := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#4A90D9")).
				Bold(true).
				Padding(0, 1).
				Width(panelWidth)
			trailingSections = append(trailingSections, selected.Render(fmt.Sprintf(" > %s %s", opt.icon, opt.label)))
			continue
		}

		normal := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Padding(0, 1).
			Width(panelWidth)
		trailingSections = append(trailingSections, normal.Render(fmt.Sprintf("   %s %s", opt.icon, opt.label)))
	}

	trailingSections = append(trailingSections, "")

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555"))
	trailingSections = append(trailingSections, helpStyle.Render("  ↑/↓ select · Enter confirm"))

	trailingSections = append(trailingSections, m.input.View())

	if statusBar := m.renderStatusBar(); statusBar != "" {
		trailingSections = append(trailingSections, statusBar)
	}

	return trailingSections
}

// resolveApproval completes an approval request.
func (m Model) resolveApproval(id string, resp wire.ApprovalResponse) (Model, tea.Cmd) {
	if req, ok := m.pendingApprovals[id]; ok {
		req.Resolve(resp)
		delete(m.pendingApprovals, id)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}
