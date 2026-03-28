package tools

import (
	"strings"
)

// 默认输出限制常量
// 与 Python 参考实现保持一致
const (
	DefaultMaxChars      = 50000 // 单次工具输出的最大字符数
	DefaultMaxLineLength = 2000  // 单行的最大字符数
)

// OutputShaper 负责对工具输出进行塑形和截断。
// 1. 先按行截断（如果单行超过 maxLineLength）
// 2. 再按总字符数截断（如果总输出超过 maxChars）
type OutputShaper struct {
	maxChars      int
	maxLineLength int
	truncateLine  string // 单行截断标记
	truncateTotal string // 整体截断提示
}

// NewOutputShaper 创建默认配置的输出塑形器。
func NewOutputShaper() OutputShaper {
	return NewOutputShaperWithLimits(DefaultMaxChars, DefaultMaxLineLength)
}

// NewOutputShaperWithLimits 创建自定义限制的输出塑形器。
// 如果 maxLineLength <= 0，则不限制行长度。
func NewOutputShaperWithLimits(maxChars, maxLineLength int) OutputShaper {
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	if maxLineLength <= 0 {
		maxLineLength = DefaultMaxLineLength
	}

	return OutputShaper{
		maxChars:      maxChars,
		maxLineLength: maxLineLength,
		truncateLine:  "[...truncated]",
		truncateTotal: "Output is truncated to fit in the message.",
	}
}

// ShapeResult 包含塑形后的输出和元信息。
type ShapeResult struct {
	Output            string // 塑形后的输出内容
	Message           string // 附加说明（如截断提示）
	WasLineTruncated  bool   // 是否有行被截断
	WasTotalTruncated bool   // 是否整体被截断
}

// Shape 对原始输出进行塑形，返回截断后的结果。
func (s OutputShaper) Shape(raw string) ShapeResult {
	if raw == "" {
		return ShapeResult{Output: ""}
	}

	// 1. 按行处理：每行单独截断
	lines := splitLinesKeepEnds(raw)
	wasLineTruncated := false
	var processedLines []string

	for _, line := range lines {
		if len(line) > s.maxLineLength {
			line = s.truncateLineKeepNewline(line, s.maxLineLength)
			wasLineTruncated = true
		}
		processedLines = append(processedLines, line)
	}

	// 2. 按总字符数截断
	totalOutput := strings.Join(processedLines, "")
	wasTotalTruncated := false

	if len(totalOutput) > s.maxChars {
		totalOutput = totalOutput[:s.maxChars]
		wasTotalTruncated = true
	}

	// 3. 构建结果
	result := ShapeResult{
		Output:            totalOutput,
		WasLineTruncated:  wasLineTruncated,
		WasTotalTruncated: wasTotalTruncated,
	}

	// 4. 添加截断提示
	if wasLineTruncated || wasTotalTruncated {
		result.Message = s.truncateTotal
	}

	return result
}

// splitLinesKeepEnds 分割字符串为行，保留换行符。
// 这样处理后的行可以直接拼接还原。
func splitLinesKeepEnds(s string) []string {
	if s == "" {
		return nil
	}

	var lines []string
	start := 0

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			// 包含换行符
			lines = append(lines, s[start:i+1])
			start = i + 1
		case '\r':
			// 处理 \r\n 或单独的 \r
			if i+1 < len(s) && s[i+1] == '\n' {
				lines = append(lines, s[start:i+2])
				start = i + 2
				i++ // 跳过 \n
			} else {
				lines = append(lines, s[start:i+1])
				start = i + 1
			}
		}
	}

	// 处理最后一行（没有换行符结尾的情况）
	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

// truncateLineKeepNewline 截断单行，但保留末尾的换行符。
func (s OutputShaper) truncateLineKeepNewline(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}

	// 检查末尾是否有换行符
	lineBreak := ""
	if strings.HasSuffix(line, "\r\n") {
		lineBreak = "\r\n"
	} else if strings.HasSuffix(line, "\n") {
		lineBreak = "\n"
	} else if strings.HasSuffix(line, "\r") {
		lineBreak = "\r"
	}

	// 计算可用空间：确保至少能放下截断标记
	marker := s.truncateLine
	available := maxLen - len(marker) - len(lineBreak)
	if available <= 0 {
		// 空间太小，只能返回截断标记
		return marker + lineBreak
	}

	return line[:available] + marker + lineBreak
}
