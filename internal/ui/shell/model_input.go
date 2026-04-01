package shell

import (
	"strings"
	"unicode/utf8"

	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InputModel 管理用户输入区域的状态。
// 目前使用简单的单行输入，后续可以升级为多行 textarea。
type InputModel struct {
	// 当前输入值
	value string
	// 光标位置（字节偏移），0 <= cursorPos <= len(value)
	cursorPos int
	// 历史记录（用于上下箭头导航）
	history []string
	// 当前历史索引（-1 表示不在历史导航中）
	historyIdx int
	// 输入框宽度
	width int
	// 是否聚焦
	focused bool
}

// NewInputModel 创建一个新的输入模型。
func NewInputModel() InputModel {
	return InputModel{
		history:    make([]string, 0),
		historyIdx: -1,
		focused:    true,
	}
}

// NewInputModelWithHistory 创建一个预加载历史记录的输入模型。
func NewInputModelWithHistory(entries []string) InputModel {
	return InputModel{
		history:    entries,
		historyIdx: -1,
		focused:    true,
	}
}

// Value 返回当前输入值。
func (m InputModel) Value() string {
	return m.value
}

// CursorPos 返回当前光标位置（字节偏移）。
func (m InputModel) CursorPos() int {
	return m.cursorPos
}

// Clear 清空输入并重置状态。
func (m InputModel) Clear() InputModel {
	m.value = ""
	m.cursorPos = 0
	m.historyIdx = -1
	return m
}

// SetValue 设置输入值。
func (m InputModel) SetValue(v string) InputModel {
	m.value = v
	m.cursorPos = len(v)
	m.historyIdx = -1
	return m
}

// InsertAtCursor 在当前光标位置插入文本，并将光标移动到插入文本之后。
func (m InputModel) InsertAtCursor(text string) InputModel {
	m.value = m.value[:m.cursorPos] + text + m.value[m.cursorPos:]
	m.cursorPos += len(text)
	m.historyIdx = -1
	return m
}

// DeleteRange 删除 [start, end) 范围的字节，并调整光标位置。
func (m InputModel) DeleteRange(start, end int) InputModel {
	if start < 0 {
		start = 0
	}
	if end > len(m.value) {
		end = len(m.value)
	}
	if start >= end {
		return m
	}
	m.value = m.value[:start] + m.value[end:]
	if m.cursorPos > end {
		m.cursorPos -= end - start
	} else if m.cursorPos > start {
		m.cursorPos = start
	}
	return m
}

// AppendHistory 添加一条历史记录。
func (m *InputModel) AppendHistory(entry string) {
	if strings.TrimSpace(entry) == "" {
		return
	}
	m.history = append(m.history, entry)
}

// Update 处理消息并更新状态。
func (m InputModel) Update(msg tea.Msg, width int) (InputModel, tea.Cmd) {
	m.width = width

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

// handleKeyPress 处理键盘输入。
func (m InputModel) handleKeyPress(msg tea.KeyMsg) (InputModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		runes := string(msg.Runes)
		m.value = m.value[:m.cursorPos] + runes + m.value[m.cursorPos:]
		m.cursorPos += len(runes)
		m.historyIdx = -1

	case tea.KeySpace:
		m.value = m.value[:m.cursorPos] + " " + m.value[m.cursorPos:]
		m.cursorPos++
		m.historyIdx = -1

	case tea.KeyBackspace:
		if m.cursorPos > 0 {
			_, size := utf8.DecodeLastRuneInString(m.value[:m.cursorPos])
			m.value = m.value[:m.cursorPos-size] + m.value[m.cursorPos:]
			m.cursorPos -= size
		}

	case tea.KeyDelete:
		if m.cursorPos < len(m.value) {
			_, size := utf8.DecodeRuneInString(m.value[m.cursorPos:])
			m.value = m.value[:m.cursorPos] + m.value[m.cursorPos+size:]
		}

	case tea.KeyLeft:
		if m.cursorPos > 0 {
			_, size := utf8.DecodeLastRuneInString(m.value[:m.cursorPos])
			m.cursorPos -= size
		}

	case tea.KeyRight:
		if m.cursorPos < len(m.value) {
			_, size := utf8.DecodeRuneInString(m.value[m.cursorPos:])
			m.cursorPos += size
		}

	case tea.KeyEnter:
		// 提交输入
		if m.value != "" {
			return m, func() tea.Msg {
				return InputSubmitMsg{Text: m.value}
			}
		}

	case tea.KeyUp:
		// 向上导航历史
		if len(m.history) > 0 {
			if m.historyIdx == -1 {
				m.historyIdx = len(m.history) - 1
			} else if m.historyIdx > 0 {
				m.historyIdx--
			}
			if m.historyIdx >= 0 && m.historyIdx < len(m.history) {
				m.value = m.history[m.historyIdx]
				m.cursorPos = len(m.value)
			}
		}

	case tea.KeyDown:
		// 向下导航历史
		if m.historyIdx != -1 {
			m.historyIdx++
			if m.historyIdx >= len(m.history) {
				m.historyIdx = -1
				m.value = ""
				m.cursorPos = 0
			} else {
				m.value = m.history[m.historyIdx]
				m.cursorPos = len(m.value)
			}
		}
	}

	return m, nil
}

// View 渲染输入区域。
func (m InputModel) View() string {
	width := m.width
	if width <= 0 {
		width = defaultRenderWidth
	}
	if width < 32 {
		width = 32
	}
	bodyWidth := messageBodyWidth(width)

	before := styles.ComposerTextStyle.Render(m.value[:m.cursorPos])
	after := styles.ComposerTextStyle.Render(m.value[m.cursorPos:])
	cursor := styles.ComposerCursorStyle.Render("|")

	var content string
	if m.value == "" {
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			styles.PromptStyle.Render("> "),
			styles.ComposerPlaceholderStyle.Render("Ask fimi to do anything"),
			cursor,
		)
	} else {
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			styles.PromptStyle.Render("> "),
			before,
			cursor,
			after,
		)
	}

	footer := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styles.ComposerHeaderStyle.Render("Message"),
		styles.ComposerHintStyle.Render(" · Enter send · Up/Down history · @ files · / commands"),
	)

	body := lipgloss.JoinVertical(lipgloss.Left, content, footer)

	return transcriptBodyIndent() + styles.ComposerBoxStyle.Width(bodyWidth).Render(body)
}
