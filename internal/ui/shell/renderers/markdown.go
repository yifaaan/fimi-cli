// Package renderers 提供内容渲染器。
package renderers

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// MarkdownRenderer 包装 glamour 渲染器，用于渲染 Markdown 内容。
type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// NewMarkdownRenderer 创建一个新的 Markdown 渲染器。
func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	// 使用 dark 主题作为基础
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	return &MarkdownRenderer{
		renderer: renderer,
		width:    width,
	}, nil
}

// Render 渲染 Markdown 文本为 ANSI 格式化文本。
func (r *MarkdownRenderer) Render(markdown string) string {
	if r.renderer == nil {
		return markdown
	}

	rendered, err := r.renderer.Render(markdown)
	if err != nil {
		// 渲染失败时返回原始文本
		return markdown
	}

	// glamour 会在末尾添加换行符，需要去除
	return strings.TrimSpace(rendered)
}

// SetWidth 更新渲染器宽度。
func (r *MarkdownRenderer) SetWidth(width int) error {
	if r.width == width {
		return nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	r.renderer = renderer
	r.width = width
	return nil
}

// RenderCodeBlock 渲染代码块，带语法高亮。
func (r *MarkdownRenderer) RenderCodeBlock(code, language string) string {
	// 使用 glamour 渲染代码块
	md := "```" + language + "\n" + code + "\n```"
	return r.Render(md)
}

// RenderInlineCode 渲染行内代码。
func RenderInlineCode(code string) string {
	// 使用 lipgloss 创建行内代码样式
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	return style.Render(code)
}

// RenderBold 渲染粗体文本。
func RenderBold(text string) string {
	return lipgloss.NewStyle().Bold(true).Render(text)
}

// RenderItalic 渲染斜体文本。
func RenderItalic(text string) string {
	return lipgloss.NewStyle().Italic(true).Render(text)
}

// RenderLink 渲染链接。
func RenderLink(text, url string) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")). // cyan
		Underline(true)

	return style.Render(text) + " " + lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")). // gray
		Render("("+url+")")
}
