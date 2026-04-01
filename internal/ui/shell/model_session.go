package shell

import (
	"fmt"
	"strings"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// handleResumeListResult 处理 session 列表查询结果。
func (m Model) handleResumeListResult(msg ResumeListMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error listing sessions: %v", msg.Err),
		})
		return m, nil
	}

	if len(msg.Sessions) == 0 {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "No sessions found for this work directory.",
		})
		return m, nil
	}

	m.mode = ModeSessionSelect
	m.sessionList = msg.Sessions
	m.selectedSession = 0
	return m, nil
}

// handleSessionSelectKeyPress 处理 session 选择模式的键盘输入。
func (m Model) handleSessionSelectKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	availableHeight := m.height - 8
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisible := availableHeight / 2
	if maxVisible < 1 {
		maxVisible = 1
	}

	total := len(m.sessionList)
	oldScrollOffset := m.sessionScrollOffset

	switch keyStr {
	case "up", "k":
		if m.selectedSession > 0 {
			m.selectedSession--
			if m.selectedSession < m.sessionScrollOffset {
				m.sessionScrollOffset = m.selectedSession
			}
		}
	case "down", "j":
		if m.selectedSession < total-1 {
			m.selectedSession++
			if m.selectedSession >= m.sessionScrollOffset+maxVisible {
				m.sessionScrollOffset = m.selectedSession - maxVisible + 1
			}
		}
	case "enter":
		return m.handleResumeSwitch(m.sessionList[m.selectedSession].ID)
	case "ctrl+d":
		if len(m.sessionList) == 0 {
			return m, nil
		}
		return m, m.deleteSelectedSession()
	case "esc", "q":
		return m.finishSessionSelection(), nil
	}

	if m.sessionScrollOffset != oldScrollOffset {
		return m, tea.ClearScreen
	}
	return m, nil
}

// renderSessionSelectView 渲染 session 选择界面。
func (m Model) renderSessionSelectView() string {
	var sections []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	sections = append(sections, titleStyle.Render("Select a session to resume"))
	sections = append(sections, "")

	availableHeight := m.height - 6
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisibleSessions := availableHeight / 2
	if maxVisibleSessions < 1 {
		maxVisibleSessions = 1
	}

	totalSessions := len(m.sessionList)
	scrollOffset := m.sessionScrollOffset
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > totalSessions-maxVisibleSessions && totalSessions > maxVisibleSessions {
		scrollOffset = totalSessions - maxVisibleSessions
	}

	startIdx := scrollOffset
	endIdx := scrollOffset + maxVisibleSessions
	if endIdx > totalSessions {
		endIdx = totalSessions
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)
	selectedMetaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	for i := startIdx; i < endIdx; i++ {
		s := m.sessionList[i]
		preview := s.Preview
		if preview == "" {
			preview = "..."
		}

		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		idPreview := fmt.Sprintf("%s  %s", shortID, preview)
		metaLine := fmt.Sprintf("    %s  %s", formatTime(s.LastModified), formatFileSize(s.FileSize))

		var block string
		displayNum := i + 1
		if i == m.selectedSession {
			line1 := selectedStyle.Render(fmt.Sprintf("[%d] > %s", displayNum, idPreview))
			line2 := selectedMetaStyle.Render(fmt.Sprintf("      %s", metaLine))
			block = fmt.Sprintf("%s\n%s", line1, line2)
		} else {
			block = fmt.Sprintf("[%d] %s\n    %s", displayNum, idPreview, metaLine)
		}
		sections = append(sections, block)
	}

	sections = append(sections, "")

	statusText := fmt.Sprintf("Session %d/%d", m.selectedSession+1, totalSessions)
	if totalSessions > maxVisibleSessions {
		statusText += fmt.Sprintf(" (showing %d-%d)", startIdx+1, endIdx)
	}
	statusStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, statusStyle.Render(statusText))

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, helpStyle.Render("↑/↓ or j/k to navigate, Enter to select, Ctrl+D to delete, Esc/q to cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// formatFileSize 返回友好的文件大小描述（kB / MB / GB）。
func formatFileSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}

// handleResumeSwitchResult 处理 session 切换结果。
func (m Model) handleResumeSwitchResult(msg ResumeSwitchMsg) (tea.Model, tea.Cmd) {
	m = m.finishSessionSelection()

	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error loading session: %v", msg.Err),
		})
		return m, nil
	}

	newStore := contextstore.New(msg.Session.HistoryFile)
	m.deps.Store = newStore
	m.deps.StartupInfo.SessionID = msg.Session.ID
	m.deps.StartupInfo.SessionReused = true
	m.deps.StartupInfo.ConversationDB = msg.Session.HistoryFile

	m.output = m.output.Clear()
	for _, line := range transcriptLineModelsFromRecords(msg.Records) {
		m.output = m.output.AppendLine(line)
	}

	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: fmt.Sprintf("Switched to session %s", msg.Session.ID[:8]),
	})

	return m, nil
}

func (m Model) finishSessionSelection() Model {
	m.mode = ModeIdle
	m.sessionList = nil
	m.selectedSession = 0
	m.sessionScrollOffset = 0
	return m
}

// deleteSelectedSession 返回一个删除选中 session 的命令。
func (m Model) deleteSelectedSession() tea.Cmd {
	if len(m.sessionList) == 0 || m.selectedSession >= len(m.sessionList) {
		return nil
	}

	sessionID := m.sessionList[m.selectedSession].ID
	workDir := m.deps.WorkDir

	return func() tea.Msg {
		err := session.DeleteSession(workDir, sessionID)
		return SessionDeleteMsg{SessionID: sessionID, Err: err}
	}
}

// handleSessionDeleteResult 处理 session 删除结果。
func (m Model) handleSessionDeleteResult(msg SessionDeleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error deleting session: %v", msg.Err),
		})
		return m, nil
	}

	newList := make([]session.SessionInfo, 0, len(m.sessionList))
	for _, s := range m.sessionList {
		if s.ID != msg.SessionID {
			newList = append(newList, s)
		}
	}

	if len(newList) == 0 {
		m = m.finishSessionSelection()
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "Session deleted. No more sessions available.",
		})
		return m, nil
	}

	m.sessionList = newList
	if m.selectedSession >= len(newList) {
		m.selectedSession = len(newList) - 1
	}

	availableHeight := m.height - 8
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisible := availableHeight / 2
	if maxVisible < 1 {
		maxVisible = 1
	}
	if m.sessionScrollOffset > m.selectedSession {
		m.sessionScrollOffset = m.selectedSession
	}
	if m.sessionScrollOffset > len(newList)-maxVisible && len(newList) > maxVisible {
		m.sessionScrollOffset = len(newList) - maxVisible
	}

	return m, tea.ClearScreen
}

// handleResumeList 处理 /resume 命令（无参数），列出可用 session。
func (m Model) handleResumeList() (tea.Model, tea.Cmd) {
	if m.deps.WorkDir == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: work directory not set",
		})
		return m, nil
	}

	return m, func() tea.Msg {
		sessions, err := session.ListSessions(m.deps.WorkDir)
		return ResumeListMsg{Sessions: sessions, Err: err}
	}
}

// handleResumeSwitch 处理 /resume <id> 命令，切换到指定 session。
func (m Model) handleResumeSwitch(sessionID string) (tea.Model, tea.Cmd) {
	if m.deps.WorkDir == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: work directory not set",
		})
		return m, nil
	}
	if sessionID == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: session ID is required",
		})
		return m, nil
	}

	return m, func() tea.Msg {
		sess, err := resolveSessionForSwitch(m.deps.WorkDir, sessionID)
		if err != nil {
			return ResumeSwitchMsg{Err: err}
		}

		records, _ := contextstore.New(sess.HistoryFile).ReadAll()
		return ResumeSwitchMsg{Session: sess, Records: records}
	}
}

func resolveSessionForSwitch(workDir, sessionID string) (session.Session, error) {
	sess, err := session.LoadSession(workDir, sessionID)
	if err == nil {
		return sess, nil
	}

	sessions, listErr := session.ListSessions(workDir)
	if listErr != nil {
		return session.Session{}, err
	}
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, sessionID) {
			return session.Session{
				ID:          s.ID,
				WorkDir:     s.WorkDir,
				HistoryFile: s.HistoryFile,
			}, nil
		}
	}

	return session.Session{}, err
}
