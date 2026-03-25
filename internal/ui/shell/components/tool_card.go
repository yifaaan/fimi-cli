// Package components 提供可复用的 UI 组件。
package components

import (
	"fmt"
	"strings"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/lipgloss"
)

// ToolStatus 表示工具调用的状态。
type ToolStatus int

const (
	// ToolStatusPending 等待执行
	ToolStatusPending ToolStatus = iota
	// ToolStatusRunning 正在执行
	ToolStatusRunning
	// ToolStatusCompleted 执行完成
	ToolStatusCompleted
	// ToolStatusError 执行失败
	ToolStatusError
)

// ToolCard 包含工具调用的可视化信息。
type ToolCard struct {
	Name   string
	Status ToolStatus
	Args   string
	Output string
	Width  int
}

// RenderToolCard 渲染工具调用卡片。
func RenderToolCard(card ToolCard) string {
	var b strings.Builder

	// 状态图标和颜色
	var icon string
	var color lipgloss.Color
	switch card.Status {
	case ToolStatusRunning:
		icon = " "
		color = styles.ColorWarning
	case ToolStatusCompleted:
		icon = " "
		color = styles.ColorSuccess
	case ToolStatusError:
		icon = " "
		color = styles.ColorError
	default:
		icon = " "
		color = styles.ColorMuted
	}

	// 工具名称样式
	nameStyle := lipgloss.NewStyle().
		Foreground(color).
		Bold(true)

	// 头部：图标 + 工具名
	header := fmt.Sprintf("%s %s", icon, nameStyle.Render(card.Name))
	b.WriteString(header)
	b.WriteString("\n")

	// 参数区域
	if card.Args != "" {
		argsBox := lipgloss.NewStyle().
			Foreground(styles.ColorMuted).
			Padding(0, 1).
			Width(card.Width - 4).
			Render(truncate(card.Args, card.Width-6))
		b.WriteString(argsBox)
		b.WriteString("\n")
	}

	// 输出区域（如果已完成）
	if card.Status == ToolStatusCompleted || card.Status == ToolStatusError {
		if card.Output != "" {
			outputStyle := lipgloss.NewStyle().
				Foreground(styles.ColorWhite).
				Padding(0, 1)
			if card.Status == ToolStatusError {
				outputStyle = outputStyle.Foreground(styles.ColorError)
			}
			outputBox := outputStyle.
				Width(card.Width - 4).
				Render(truncate(card.Output, card.Width-6))
			b.WriteString(outputBox)
		}
	}

	// 卡片边框
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(card.Width)

	return cardStyle.Render(strings.TrimSpace(b.String()))
}

// truncate 截断字符串到指定长度。
func truncate(s string, maxLen int) string {
	// 处理多行文本，只显示前几行
	lines := strings.Split(s, "\n")
	if len(lines) > 5 {
		lines = lines[:5]
		lines = append(lines, "...")
	}
	s = strings.Join(lines, "\n")

	// 按字节截断
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
