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

// Value 返回当前输入值。
func (m InputModel) Value() string {
	return m.value
}

// Clear 清空输入并重置状态。
func (m InputModel) Clear() InputModel {
	m.value = ""
	m.historyIdx = -1
	return m
}

// SetValue 设置输入值。
func (m InputModel) SetValue(v string) InputModel {
	m.value = v
	m.historyIdx = -1
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
		// 输入字符
		m.value += string(msg.Runes)
		m.historyIdx = -1

	case tea.KeySpace:
		// Bubble Tea 会把空格作为独立按键类型发送。
		m.value += " "
		m.historyIdx = -1

	case tea.KeyBackspace:
		// 删除最后一个 Unicode 字符，而不是最后一个字节。
		if len(m.value) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.value)
			if size > 0 {
				m.value = m.value[:len(m.value)-size]
			}
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
			}
		}

	case tea.KeyDown:
		// 向下导航历史
		if m.historyIdx != -1 {
			m.historyIdx++
			if m.historyIdx >= len(m.history) {
				m.historyIdx = -1
				m.value = ""
			} else {
				m.value = m.history[m.historyIdx]
			}
		}
	}

	return m, nil
}

// View 渲染输入区域。
func (m InputModel) View() string {
	// 输入提示符
	prompt := styles.PromptStyle.Render("fimi> ")

	// 输入值
	inputValue := m.value
	if inputValue == "" {
		// 显示占位符
		inputValue = styles.HelpStyle.Render("Type your message...")
	}

	// 光标
	cursor := "▌"

	// 组合
	inputLine := lipgloss.JoinHorizontal(lipgloss.Top, prompt, inputValue, cursor)

	// 如果没有宽度信息，直接返回输入行
	if m.width <= 2 {
		return inputLine
	}

	// 输入框容器
	inputBox := styles.BorderStyle.
		Width(m.width - 2).
		Render(inputLine)

	return inputBox
}