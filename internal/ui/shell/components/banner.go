package components

import (
	"fmt"
	"strings"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/lipgloss"
)

// BannerInfo 包含启动横幅的信息。
type BannerInfo struct {
	SessionID      string
	SessionReused  bool
	ModelName      string
	ConversationDB string
	LastRole       string
	LastSummary    string
	Width          int
}

// RenderBanner 渲染启动横幅。
func RenderBanner(info BannerInfo) string {
	var lines []string

	// 标题
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	lines = append(lines, titleStyle.Render("Shell Session"))
	lines = append(lines, "")

	// 会话信息
	if info.SessionID != "" {
		sessionStyle := lipgloss.NewStyle().
			Foreground(styles.ColorInfo)
		modeText := "new"
		if info.SessionReused {
			modeText = "continue"
		}
		lines = append(lines, fmt.Sprintf("  session: %s", sessionStyle.Render(info.SessionID)))
		lines = append(lines, fmt.Sprintf("  mode: %s", modeText))
	}

	// 模型名称
	if info.ModelName != "" {
		modelStyle := lipgloss.NewStyle().
			Foreground(styles.ColorAccent)
		lines = append(lines, fmt.Sprintf("  model: %s", modelStyle.Render(info.ModelName)))
	}

	// 历史数据库
	if info.ConversationDB != "" {
		lines = append(lines, fmt.Sprintf("  history: %s", info.ConversationDB))
	}

	// 最后一条消息
	if info.LastSummary != "" {
		role := info.LastRole
		if role == "" {
			role = "last"
		}
		// 截断过长的摘要
		summary := info.LastSummary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", role, summary))
	}

	// 可用命令
	lines = append(lines, "")
	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	lines = append(lines, helpStyle.Render("  commands: /help /clear /exit"))

	// 边框容器
	content := strings.Join(lines, "\n")
	bannerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorder).
		Padding(1, 2).
		Width(info.Width)

	return bannerStyle.Render(content)
}
