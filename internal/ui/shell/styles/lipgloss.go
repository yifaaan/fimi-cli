package styles

import "github.com/charmbracelet/lipgloss"

// 预定义样式 - 使用 lipgloss.NewStyle() 创建可复用的样式

var (
	// 标题样式
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorTitle)

	// 用户消息样式
	UserStyle = lipgloss.NewStyle().
			Foreground(ColorUser).
			Background(ColorInputBg).
			Padding(0, 1)

	UserBubbleStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorInputBg).
			Padding(1, 2)

	// 助手消息样式
	AssistantStyle = lipgloss.NewStyle().
			Foreground(ColorAssistant).
			Bold(true)

	AssistantBulletStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	// 系统消息样式
	SystemStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// 错误消息样式
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	// 工具名称样式
	ToolNameStyle = lipgloss.NewStyle().
			Foreground(ColorTool).
			Bold(true)

	ActivityTitleStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true)

	ActivityDetailStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ActivityPreviewStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	TranscriptDividerStyle = lipgloss.NewStyle().
				Foreground(ColorBorder)

	ElapsedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// 步骤指示器样式
	StepStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	// 上下文使用率样式 - 根据百分比返回不同颜色
	ContextStyle = func(usagePercent int) lipgloss.Style {
		var color Color
		switch {
		case usagePercent < 50:
			color = ColorSuccess
		case usagePercent < 75:
			color = ColorWarning
		default:
			color = ColorError
		}
		return lipgloss.NewStyle().Foreground(color)
	}

	// 输入框样式
	InputStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// 输入提示符样式
	PromptStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// 边框样式
	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	// 工具卡片样式
	ToolCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorTool).
			Padding(0, 1)

	// 工具参数框样式
	ToolArgsStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	// 工具输出框样式
	ToolOutputStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Padding(0, 1)

	// 代码编辑 diff 样式
	ToolEditSummaryStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true)

	ToolDiffContextStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	ToolDiffAddedStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess)

	ToolDiffRemovedStyle = lipgloss.NewStyle().
				Foreground(ColorError)

	ToolDiffHunkStyle = lipgloss.NewStyle().
				Foreground(ColorInfo).
				Bold(true)

	ApprovalTitleStyle = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)

	ApprovalSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorBlack).
				Background(ColorInfo).
				Bold(true)

	ApprovalOptionStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	// 状态栏样式
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorPrimary).
			Padding(0, 1)

	// 帮助文本样式
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// 启动横幅样式
	BannerStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// 会话信息样式
	SessionStyle = lipgloss.NewStyle().
			Foreground(ColorInfo)

	// 模型名称样式
	ModelStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)
)

// Toast styles
var (
	ToastInfoStyle = lipgloss.NewStyle().
			Foreground(ColorInfo).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorInfo).
			Padding(0, 1)

	ToastWarningStyle = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	ToastErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(0, 1)

	ToastSuccessStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSuccess).
				Padding(0, 1)
)
