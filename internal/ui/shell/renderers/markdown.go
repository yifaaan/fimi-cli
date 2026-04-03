// Package renderers 提供内容渲染器。
package renderers

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

const markdownStyle = "dark"

// MarkdownRenderer 包装 glamour 渲染器，用于渲染 Markdown 内容。
type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// NewMarkdownRenderer 创建一个新的 Markdown 渲染器。
func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	if width < 1 {
		width = 1
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyle),
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
	markdown = sanitizeMarkdown(markdown)
	if markdown == "" {
		return ""
	}
	if r.renderer == nil {
		return markdown
	}

	rendered, err := r.renderer.Render(markdown)
	if err != nil {
		return markdown
	}

	return strings.TrimRight(rendered, "\r\n")
}

// SetWidth 更新渲染器宽度。
func (r *MarkdownRenderer) SetWidth(width int) error {
	if width < 1 {
		width = 1
	}
	if r.width == width {
		return nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyle),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	r.renderer = renderer
	r.width = width
	return nil
}

func SanitizeMarkdown(markdown string) string {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")
	markdown = stripANSI(markdown)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, markdown)
}

func sanitizeMarkdown(markdown string) string {
	return strings.TrimSpace(SanitizeMarkdown(markdown))
}

func stripANSI(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] != 0x1b {
			b.WriteByte(text[i])
			continue
		}
		if i+1 >= len(text) {
			break
		}
		switch text[i+1] {
		case '[':
			i += 2
			for i < len(text) {
				c := text[i]
				if c >= 0x40 && c <= 0x7e {
					break
				}
				i++
			}
		case ']':
			i += 2
			for i < len(text) {
				if text[i] == 0x07 {
					break
				}
				if text[i] == 0x1b && i+1 < len(text) && text[i+1] == '\\' {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return b.String()
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
		Foreground(lipgloss.Color("252")).
		Underline(true).
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
