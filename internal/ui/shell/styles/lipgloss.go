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

	// 助手消息样式
	AssistantStyle = lipgloss.NewStyle().
			Foreground(ColorAssistant).
			Bold(true)

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
