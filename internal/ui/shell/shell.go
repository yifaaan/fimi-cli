package shell

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"fimi-cli/internal/changelog"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// helpText 返回 shell 的帮助文本。
func helpText() string {
	lines := []string{
		"Available commands:",
		"  /help           Show this help message",
		"  /clear          Clear the screen",
		"  /compact        Compact conversation context",
		"  /init           Generate AGENTS.md for the project",
		"  /rewind         List available rewind checkpoints",
		"  /version        Show version information",
		"  /release-notes  Show release notes",
		"  /exit, /quit    Exit the shell",
		"  /resume         List available sessions",
		"  /resume <id>    Switch to a specific session",
		"  /setup          Setup LLM provider and model",
		"  /reload         Reload configuration",
		"",
		"Keyboard shortcuts:",
		"  Ctrl+C/Ctrl+D   Exit (when idle)",
		"  Ctrl+L          Clear screen",
		"  Ctrl+O          Toggle tool result expansion",
	}
	return strings.Join(lines, "\n")
}

// formatTime 返回相对于现在的友好时间描述。
func formatTime(t time.Time) string {
	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%d min ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	case duration < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// releaseNotesText formats parsed release entries for display.
func releaseNotesText(entries []changelog.ReleaseEntry) string {
	if len(entries) == 0 {
		return "No release notes available."
	}

	var lines []string
	lines = append(lines, "Release Notes:")
	lines = append(lines, "")

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("## [%s] - %s", entry.Version, entry.Date))
		for _, bullet := range entry.Bullets {
			lines = append(lines, "  - "+bullet)
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// versionText returns the version display string.
func versionText(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return "fimi-cli dev build"
	}
	return "fimi-cli v" + strings.TrimPrefix(version, "v")
}

const promptText = "fimi> "

// Runner 定义 shell 对 runtime 的最小依赖边界。
type Runner interface {
	Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error)
}

// Dependencies 描述 shell REPL 运行需要的最小装配输入。
type Dependencies struct {
	Runner         Runner
	Store          contextstore.Context
	Input          io.Reader
	Output         io.Writer
	ErrOutput      io.Writer
	HistoryFile    string
	ModelName      string
	SystemPrompt   string
	WorkDir        string
	InitialPrompt  string
	InitialRecords []contextstore.TextRecord
	StartupInfo    StartupInfo
	Yolo           bool
}

// StartupInfo 描述 shell 首屏需要展示的启动上下文。
type StartupInfo struct {
	SessionID      string
	SessionReused  bool
	ModelName      string
	AppVersion     string
	ConversationDB string
	LastRole       string
	LastSummary    string
}

// Run 启动交互式 shell（仅 Bubble Tea 模式）。
func Run(ctx context.Context, deps Dependencies) error {
	if deps.Runner == nil {
		return fmt.Errorf("shell runner is required")
	}

	input := deps.Input
	if input == nil {
		input = strings.NewReader("")
	}

	output := deps.Output
	if output == nil {
		output = io.Discard
	}

	// 加载历史记录
	history, err := loadHistoryStore(deps.HistoryFile)
	if err != nil {
		fmt.Fprintf(output, "shell history unavailable: %v\n", err)
	}

	// 创建 Bubble Tea 模型
	model := NewModel(deps, &history)

	// 创建 Bubble Tea 程序
	// 不使用 alt screen 和鼠标捕获，这样终端原生的文本选择和滚轮翻页都可以正常工作。
	p := tea.NewProgram(
		model,
		tea.WithInput(input),
		tea.WithOutput(output),
	)

	// 在 goroutine 中运行，以便处理 context 取消
	done := make(chan error, 1)
	go func() {
		_, runErr := p.Run()
		done <- runErr
	}()

	var runResult error
	select {
	case runResult = <-done:
	case <-ctx.Done():
		p.Quit()
		runResult = ctx.Err()
	}

	// 退出时打印恢复提示
	if deps.StartupInfo.SessionID != "" {
		shortID := deps.StartupInfo.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Fprintf(output, "\nTo resume this session, run:\n  fimi -resume %s\n", shortID)
	}

	return runResult
}
