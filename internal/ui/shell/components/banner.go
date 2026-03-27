package components

import (
	"fmt"
	"os"
	"strings"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/lipgloss"
)

// BannerInfo 包含启动横幅的信息。
type BannerInfo struct {
	SessionID      string
	SessionReused  bool
	ModelName      string
	AppVersion     string
	ConversationDB string
	LastRole       string
	LastSummary    string
	WorkDir        string
}

// RenderBanner 渲染启动横幅。
func RenderBanner(info BannerInfo) string {
	logoStyle := lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	titleStyle := lipgloss.NewStyle().Foreground(styles.ColorTitle).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(styles.ColorAccent)
	pathStyle := lipgloss.NewStyle().Foreground(styles.ColorInfo)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)
	summaryStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)

	var lines []string
	lines = append(lines, logoStyle.Render("▐▛███▜▌")+"   "+titleStyle.Render(buildBannerTitle(info)))
	lines = append(lines, logoStyle.Render("▝▜█████▛▘")+"  "+metaStyle.Render(buildBannerSubtitle(info)))

	if workDir := formatBannerPath(info.WorkDir); workDir != "" {
		lines = append(lines, logoStyle.Render("  ▘▘ ▝▝")+"    "+pathStyle.Render(workDir))
	}

	if info.SessionID != "" {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("session: %s", shortSessionID(info.SessionID))))
	}

	if info.LastSummary != "" {
		role := strings.TrimSpace(info.LastRole)
		if role == "" {
			role = "last"
		}
		// 摘要只做展示截断，不改变真实上下文数据。
		lines = append(lines, summaryStyle.Render(fmt.Sprintf("%s: %s", role, truncateBannerText(info.LastSummary, 72))))
	}

	lines = append(lines, mutedStyle.Render("commands: /help /clear /exit"))
	return strings.Join(lines, "\n")
}

func buildBannerTitle(info BannerInfo) string {
	version := strings.TrimSpace(info.AppVersion)
	if version == "" {
		return "fimi-cli"
	}
	return "fimi-cli v" + strings.TrimPrefix(version, "v")
}

func buildBannerSubtitle(info BannerInfo) string {
	var parts []string
	if model := strings.TrimSpace(info.ModelName); model != "" {
		parts = append(parts, model)
	}
	if info.SessionID != "" {
		modeText := "new session"
		if info.SessionReused {
			modeText = "continue session"
		}
		parts = append(parts, modeText)
	}
	if strings.TrimSpace(info.AppVersion) == "dev" {
		parts = append(parts, "dev build")
	}
	if len(parts) == 0 {
		return "interactive shell"
	}
	return strings.Join(parts, " · ")
}

func shortSessionID(sessionID string) string {
	if len(sessionID) <= 12 {
		return sessionID
	}
	return sessionID[:12]
}

func formatBannerPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func truncateBannerText(text string, maxLen int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if maxLen <= 3 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen-3]) + "..."
}
