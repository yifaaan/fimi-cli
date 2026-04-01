package shell

import (
	"fmt"
	"strings"

	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// truncateString 截断字符串到指定长度（使用 rune 正确处理 UTF-8）。
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

func (m Model) handleRewindList() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		checkpoints, err := m.deps.Store.ListCheckpoints()
		return CheckpointListMsg{Checkpoints: checkpoints, Err: err}
	}
}

func (m Model) handleCheckpointListResult(msg CheckpointListMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error listing rewind checkpoints: %v", msg.Err),
		})
		return m, nil
	}

	if len(msg.Checkpoints) == 0 {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "No rewind checkpoints found for this session.",
		})
		return m, nil
	}

	m.mode = ModeCheckpointSelect
	m.checkpointList = msg.Checkpoints
	m.selectedCheckpoint = 0
	m.checkpointScrollOffset = 0
	return m, nil
}

func (m Model) handleCheckpointSelectKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	availableHeight := m.height - 8
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisible := availableHeight / 2
	if maxVisible < 1 {
		maxVisible = 1
	}

	total := len(m.checkpointList)
	oldScrollOffset := m.checkpointScrollOffset

	switch keyStr {
	case "up", "k":
		if m.selectedCheckpoint > 0 {
			m.selectedCheckpoint--
			if m.selectedCheckpoint < m.checkpointScrollOffset {
				m.checkpointScrollOffset = m.selectedCheckpoint
			}
		}
	case "down", "j":
		if m.selectedCheckpoint < total-1 {
			m.selectedCheckpoint++
			if m.selectedCheckpoint >= m.checkpointScrollOffset+maxVisible {
				m.checkpointScrollOffset = m.selectedCheckpoint - maxVisible + 1
			}
		}
	case "enter":
		return m.rewindToSelectedCheckpoint(), nil
	case "esc", "q":
		return m.finishCheckpointSelection(), nil
	}

	if m.checkpointScrollOffset != oldScrollOffset {
		return m, tea.ClearScreen
	}
	return m, nil
}

func (m Model) renderCheckpointSelectView() string {
	var sections []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	sections = append(sections, titleStyle.Render("Select a checkpoint to rewind"))
	sections = append(sections, "")

	availableHeight := m.height - 6
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisibleCheckpoints := availableHeight / 2
	if maxVisibleCheckpoints < 1 {
		maxVisibleCheckpoints = 1
	}

	totalCheckpoints := len(m.checkpointList)
	scrollOffset := m.checkpointScrollOffset
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > totalCheckpoints-maxVisibleCheckpoints && totalCheckpoints > maxVisibleCheckpoints {
		scrollOffset = totalCheckpoints - maxVisibleCheckpoints
	}

	startIdx := scrollOffset
	endIdx := scrollOffset + maxVisibleCheckpoints
	if endIdx > totalCheckpoints {
		endIdx = totalCheckpoints
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)
	selectedMetaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	for i := startIdx; i < endIdx; i++ {
		checkpoint := m.checkpointList[i]
		label := strings.TrimSpace(checkpoint.PromptPreview)
		if label == "" {
			label = fmt.Sprintf("checkpoint %d", checkpoint.ID)
		}

		metaLine := fmt.Sprintf("ID %d", checkpoint.ID)
		if createdAt := strings.TrimSpace(checkpoint.CreatedAt); createdAt != "" {
			metaLine = fmt.Sprintf("ID %d  %s", checkpoint.ID, createdAt)
		}

		var block string
		displayNum := i + 1
		if i == m.selectedCheckpoint {
			line1 := selectedStyle.Render(fmt.Sprintf("[%d] > %s", displayNum, truncateString(label, 70)))
			line2 := selectedMetaStyle.Render(fmt.Sprintf("      %s", metaLine))
			block = fmt.Sprintf("%s\n%s", line1, line2)
		} else {
			block = fmt.Sprintf("[%d] %s\n    %s", displayNum, truncateString(label, 70), metaLine)
		}
		sections = append(sections, block)
	}

	sections = append(sections, "")

	statusText := fmt.Sprintf("Checkpoint %d/%d", m.selectedCheckpoint+1, totalCheckpoints)
	if totalCheckpoints > maxVisibleCheckpoints {
		statusText += fmt.Sprintf(" (showing %d-%d)", startIdx+1, endIdx)
	}
	statusStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, statusStyle.Render(statusText))

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, helpStyle.Render("↑/↓ or j/k to navigate, Enter to select, Esc/q to cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func rewindedNoticeText(checkpointID int) string {
	return fmt.Sprintf("Conversation rewound to checkpoint %d.", checkpointID)
}

func (m Model) finishCheckpointSelection() Model {
	m.mode = ModeIdle
	m.checkpointList = nil
	m.selectedCheckpoint = 0
	m.checkpointScrollOffset = 0
	return m
}

func (m Model) rewindToSelectedCheckpoint() Model {
	if len(m.checkpointList) == 0 || m.selectedCheckpoint >= len(m.checkpointList) {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error rewinding conversation: no checkpoint selected",
		})
		return m.finishCheckpointSelection()
	}

	checkpoint := m.checkpointList[m.selectedCheckpoint]
	if _, err := m.deps.Store.RevertToCheckpoint(checkpoint.ID); err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error rewinding conversation: %v", err),
		})
		return m.finishCheckpointSelection()
	}

	records, err := m.deps.Store.ReadAll()
	if err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error reading rewound conversation: %v", err),
		})
		return m.finishCheckpointSelection()
	}

	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m.activeShellActionCommand = ""
	m = m.finishCheckpointSelection()
	m = m.rebuildOutputFromRecords(records)
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: rewindedNoticeText(checkpoint.ID),
	})
	return m
}
