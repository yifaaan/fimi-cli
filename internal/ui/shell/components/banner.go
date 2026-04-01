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
	var lines []string
	lines = append(lines, styles.TitleStyle.Render(buildBannerTitle(info)))
	lines = append(lines, styles.BannerMetaStyle.Render(buildBannerSubtitle(info)))

	if metaLine := renderBannerMetaLine(info); metaLine != "" {
		lines = append(lines, metaLine)
	}

	if summary := renderBannerSummary(info); summary != "" {
		lines = append(lines, styles.BannerSummaryStyle.Render(summary))
	}

	lines = append(lines, styles.BannerHintStyle.Render("Enter send | /help /clear /exit | Ctrl+C quit"))
	return styles.BannerBoxStyle.Render(strings.Join(lines, "\n"))
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
		return "interactive coding shell"
	}
	return strings.Join(parts, " · ")
}

func renderBannerMetaLine(info BannerInfo) string {
	var chips []string

	if workDir := formatBannerPath(info.WorkDir); workDir != "" {
		chips = append(chips, styles.BannerMetaChipStyle.Render(workDir))
	}
	if model := strings.TrimSpace(info.ModelName); model != "" {
		chips = append(chips, styles.BannerMetaChipStyle.Render(model))
	}
	if info.SessionID != "" {
		sessionText := "new " + shortSessionID(info.SessionID)
		if info.SessionReused {
			sessionText = "resume " + shortSessionID(info.SessionID)
		}
		chips = append(chips, styles.BannerMetaChipStyle.Render(sessionText))
	}
	if strings.TrimSpace(info.AppVersion) == "dev" {
		chips = append(chips, styles.BannerMetaChipStyle.Render("dev build"))
	}

	if len(chips) == 0 {
		return ""
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, chips...)
}

func renderBannerSummary(info BannerInfo) string {
	if info.LastSummary == "" {
		return ""
	}

	role := strings.TrimSpace(info.LastRole)
	if role == "" {
		role = "last"
	}
	if role == "user" {
		return ""
	}

	// 摘要只做展示截断，不改变真实上下文数据。
	return fmt.Sprintf("%s: %s", role, truncateBannerText(info.LastSummary, 96))
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
