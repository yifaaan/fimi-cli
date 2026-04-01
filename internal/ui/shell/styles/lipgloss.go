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
			Bold(true)

	UserLabelStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Bold(true)

	UserBubbleStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorPanelBg).
			Padding(0, 1)

	// 助手消息样式
	AssistantStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	AssistantBulletStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	AssistantBubbleStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(ColorBorder).
				PaddingLeft(1)

	AssistantLabelStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	// 系统消息样式
	SystemStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	SystemNoticeStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Background(ColorPanelBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	// 错误消息样式
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	ErrorNoticeStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(ColorErrorBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorError).
				Padding(0, 1)

	// 工具名称样式
	ToolNameStyle = lipgloss.NewStyle().
			Foreground(ColorTool).
			Bold(true)

	ActivityTitleStyle = lipgloss.NewStyle().
				Foreground(ColorTitle).
				Bold(true)

	ActivityDetailStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ActivityPreviewStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	ActivityCardStyle = lipgloss.NewStyle().
				Background(ColorSubtleBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	ActivityBadgeBaseStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Bold(true)

	ActivityPreviewLabelStyle = lipgloss.NewStyle().
					Foreground(ColorMuted).
					Bold(true)

	PreviewFooterStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	PreviewHintStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Background(ColorPanelBg).
				Padding(0, 1)

	ActivityPendingStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ActivityRunningStyle = lipgloss.NewStyle().
				Foreground(ColorAccent)

	ActivityCompletedStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ActivityFailedStyle = lipgloss.NewStyle().
				Foreground(ColorError)

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
			Foreground(ColorWhite)

	// 输入提示符样式
	PromptStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	ComposerBoxStyle = lipgloss.NewStyle().
				Background(ColorPanelBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	ComposerHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Bold(true)

	ComposerPlaceholderStyle = lipgloss.NewStyle().
					Foreground(ColorMuted)

	ComposerHintStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ComposerCursorStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	ComposerTextStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	DropdownBoxStyle = lipgloss.NewStyle().
				Background(ColorSubtleBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	DropdownTitleStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Bold(true)

	DropdownSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorBlack).
				Background(ColorPrimary).
				Padding(0, 1)

	DropdownOptionStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	DropdownMetaStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

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
				Foreground(ColorWhite).
				Background(ColorToolBg)

	ToolDiffAddedStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(Color("22"))

	ToolDiffRemovedStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(ColorErrorBg)

	ToolDiffHunkStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(Color("24")).
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

	ApprovalCardStyle = lipgloss.NewStyle().
				Background(ColorSubtleBg).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	// 状态栏样式
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorSubtleBg).
			Padding(0, 1)

	// 帮助文本样式
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// 启动横幅样式
	BannerStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	BannerBoxStyle = lipgloss.NewStyle().
			Background(ColorPanelBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	BannerMetaStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	BannerMetaChipStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Background(ColorSubtleBg).
				Padding(0, 1).
				MarginRight(1)

	BannerSummaryStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Background(ColorSubtleBg).
				Padding(0, 1)

	BannerHintStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// 会话信息样式
	SessionStyle = lipgloss.NewStyle().
			Foreground(ColorInfo)

	// 模型名称样式
	ModelStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	LiveStatusStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorSubtleBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)
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
