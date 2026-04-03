package components

import (
	"fmt"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/lipgloss"
)

// StatusBar 表示状态栏的内容。
type StatusBar struct {
	// 上下文使用率 (0-100)
	ContextUsage int
	// 当前模式
	Mode string
	// 模型名称
	ModelName string
	// 当前步骤
	Step int
	// 宽度
	Width int
}

// RenderStatusBar 渲染状态栏。
func RenderStatusBar(bar StatusBar) string {
	var parts []string

	// 模式指示器
	if bar.Mode != "" {
		modeStyle := lipgloss.NewStyle().
			Foreground(styles.ColorInfo).
			Bold(true)
		parts = append(parts, modeStyle.Render(bar.Mode))
	}

	// 步骤指示器
	if bar.Step > 0 {
		stepStyle := lipgloss.NewStyle().
			Foreground(styles.ColorAccent)
		parts = append(parts, stepStyle.Render(fmt.Sprintf("Step %d", bar.Step)))
	}

	// 上下文使用率
	if bar.ContextUsage > 0 {
		var color lipgloss.Color
		switch {
		case bar.ContextUsage < 50:
			color = styles.ColorSuccess
		case bar.ContextUsage < 75:
			color = styles.ColorWarning
		default:
			color = styles.ColorError
		}
		ctxStyle := lipgloss.NewStyle().Foreground(color)
		parts = append(parts, ctxStyle.Render(fmt.Sprintf("Context: %d%%", bar.ContextUsage)))
	}

	// 模型名称
	if bar.ModelName != "" {
		modelStyle := lipgloss.NewStyle().
			Foreground(styles.ColorMuted)
		parts = append(parts, modelStyle.Render(bar.ModelName))
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// 状态栏背景
	statusBarStyle := lipgloss.NewStyle().
		Foreground(styles.ColorWhite).
		Padding(0, 1).
		Width(bar.Width)

	return statusBarStyle.Render(content)
}
