// Package styles 提供 Bubble Tea UI 的样式定义。
package styles

import "github.com/charmbracelet/lipgloss"

// Color 定义 ANSI 颜色常量。
// 使用 lipgloss.Color 类型，支持 16色、256色和 True Color。
type Color = lipgloss.Color

// 基础色 - 使用 ANSI 16 色以保证兼容性
var (
	// 前景色
	ColorBlack   Color = "0"
	ColorRed     Color = "1"
	ColorGreen   Color = "2"
	ColorYellow  Color = "3"
	ColorBlue    Color = "4"
	ColorMagenta Color = "5"
	ColorCyan    Color = "6"
	ColorWhite   Color = "7"

	// 高亮前景色
	ColorBrightBlack   Color = "8"
	ColorBrightRed     Color = "9"
	ColorBrightGreen   Color = "10"
	ColorBrightYellow  Color = "11"
	ColorBrightBlue    Color = "12"
	ColorBrightMagenta Color = "13"
	ColorBrightCyan    Color = "14"
	ColorBrightWhite   Color = "15"
)

// 语义色 - 用于传达状态信息
var (
	// 成功状态
	ColorSuccess Color = ColorGreen

	// 警告状态
	ColorWarning Color = ColorYellow

	// 错误状态
	ColorError Color = ColorRed

	// 信息状态
	ColorInfo Color = ColorCyan
)

// 角色色 - 用于区分消息来源
var (
	// 用户消息
	ColorUser Color = ColorBrightCyan

	// 助手消息
	ColorAssistant Color = ColorBrightGreen

	// 工具调用
	ColorTool Color = ColorBrightYellow

	// 系统消息
	ColorSystem Color = ColorBrightBlack
)

// UI 元素色
var (
	// 主色调 - 参考 Codex 的冷色强调
	ColorPrimary Color = "14"

	// 次要色调
	ColorSecondary Color = "12"

	// 边框色
	ColorBorder Color = "240"

	// 标题色
	ColorTitle Color = ColorBrightWhite

	// 暗淡文本（如时间戳、提示）
	ColorMuted Color = "245"

	// 强调文本
	ColorAccent Color = "81"
)

// 背景色
var (
	// 主面板背景
	ColorPanelBg Color = "236"

	// 次级面板背景
	ColorSubtleBg Color = "237"

	// 输入框背景
	ColorInputBg Color = ColorPanelBg

	// 工具卡片背景
	ColorToolBg Color = ColorSubtleBg

	// 错误背景
	ColorErrorBg Color = "52" // 256色：深红
)
