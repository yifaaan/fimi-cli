package tools

import (
	"strings"
	"testing"
)

func TestOutputShaper_Shape_Empty(t *testing.T) {
	shaper := NewOutputShaper()
	result := shaper.Shape("")

	if result.Output != "" {
		t.Errorf("expected empty output, got %q", result.Output)
	}
	if result.Message != "" {
		t.Errorf("expected no message for empty input, got %q", result.Message)
	}
	if result.WasLineTruncated || result.WasTotalTruncated {
		t.Error("expected no truncation for empty input")
	}
}

func TestOutputShaper_Shape_SmallOutput(t *testing.T) {
	shaper := NewOutputShaperWithLimits(100, 50)
	input := "hello world\nsecond line"
	result := shaper.Shape(input)

	if result.Output != input {
		t.Errorf("expected unchanged output, got %q", result.Output)
	}
	if result.Message != "" {
		t.Errorf("expected no message for small output, got %q", result.Message)
	}
	if result.WasLineTruncated || result.WasTotalTruncated {
		t.Error("expected no truncation for small output")
	}
}

func TestOutputShaper_Shape_LineTruncation(t *testing.T) {
	// 设置较小的行长度限制
	shaper := NewOutputShaperWithLimits(1000, 20)

	// 构造一个超过行长度限制的输入
	longLine := strings.Repeat("a", 50) + "\n"
	shortLine := "short line\n"
	input := longLine + shortLine

	result := shaper.Shape(input)

	// 检查长行被截断
	if !result.WasLineTruncated {
		t.Error("expected line truncation")
	}

	// 检查输出包含截断标记
	if !strings.Contains(result.Output, "[...truncated]") {
		t.Errorf("expected truncation marker in output, got %q", result.Output)
	}

	// 检查换行符被保留
	if !strings.Contains(result.Output, "\n") {
		t.Error("expected newline to be preserved")
	}
}

func TestOutputShaper_Shape_TotalTruncation(t *testing.T) {
	// 设置较小的总字符限制
	shaper := NewOutputShaperWithLimits(50, 1000)

	// 构造一个超过总字符限制的输入
	input := strings.Repeat("abcdefghij", 10) // 100 chars

	result := shaper.Shape(input)

	// 检查总字符截断
	if !result.WasTotalTruncated {
		t.Error("expected total truncation")
	}

	// 检查输出长度
	if len(result.Output) > 50 {
		t.Errorf("expected output <= 50 chars, got %d", len(result.Output))
	}

	// 检查有截断提示
	if result.Message == "" {
		t.Error("expected truncation message")
	}
}

func TestOutputShaper_Shape_BothTruncations(t *testing.T) {
	// 同时设置行长度和总字符限制
	// 使用多行输入，让行截断和总字符截断都有意义
	shaper := NewOutputShaperWithLimits(25, 10)

	// 构造多行输入：每行都超过行长度限制
	// 截断后每行约 10 字符，多行加起来超过 25 字符限制
	input := strings.Repeat("a", 20) + "\n" +
		strings.Repeat("b", 20) + "\n" +
		strings.Repeat("c", 20) + "\n"

	result := shaper.Shape(input)

	// 行截断应该发生
	if !result.WasLineTruncated {
		t.Error("expected line truncation")
	}

	// 总字符截断也应该发生
	if !result.WasTotalTruncated {
		t.Error("expected total truncation")
	}

	// 最终输出应该不超过总字符限制
	if len(result.Output) > 25 {
		t.Errorf("expected output <= 25 chars, got %d", len(result.Output))
	}
}

func TestOutputShaper_Shape_PreserveNewlines(t *testing.T) {
	shaper := NewOutputShaperWithLimits(100, 10)

	tests := []struct {
		name     string
		input    string
		wantNl   string // 期望保留的换行符类型
	}{
		{
			name:   "unix newline",
			input:  "aaaaaaaaaaaa\n", // 12 a's + \n, exceeds line limit
			wantNl: "\n",
		},
		{
			name:   "windows newline",
			input:  "aaaaaaaaaaaa\r\n", // 12 a's + \r\n
			wantNl: "\r\n",
		},
		{
			name:   "old mac newline",
			input:  "aaaaaaaaaaaa\r",
			wantNl: "\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shaper.Shape(tt.input)

			if !strings.HasSuffix(result.Output, tt.wantNl) {
				t.Errorf("expected output to end with %q, got %q", tt.wantNl, result.Output)
			}
		})
	}
}

func TestOutputShaper_Shape_MultiLine(t *testing.T) {
	shaper := NewOutputShaperWithLimits(100, 10)

	// 多行输入，有些行超长，有些不超
	input := "short\n" +
		strings.Repeat("b", 20) + "\n" + // 超长行
		"end\n"

	result := shaper.Shape(input)

	// 检查短行保持不变
	if !strings.Contains(result.Output, "short\n") {
		t.Error("expected short line to be preserved")
	}

	// 检查长行被截断
	if !strings.Contains(result.Output, "[...truncated]") {
		t.Error("expected truncation marker in long line")
	}

	// 检查最后一行保留
	if !strings.Contains(result.Output, "end") {
		t.Error("expected last line to be preserved")
	}
}

func TestSplitLinesKeepEnds(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "single line no newline",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "single line with newline",
			input:    "hello\n",
			expected: []string{"hello\n"},
		},
		{
			name:     "two lines",
			input:    "hello\nworld\n",
			expected: []string{"hello\n", "world\n"},
		},
		{
			name:     "windows newlines",
			input:    "hello\r\nworld\r\n",
			expected: []string{"hello\r\n", "world\r\n"},
		},
		{
			name:     "mixed newlines",
			input:    "hello\nworld\r\nlast",
			expected: []string{"hello\n", "world\r\n", "last"},
		},
		{
			name:     "last line no newline",
			input:    "hello\nworld",
			expected: []string{"hello\n", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitLinesKeepEnds(tt.input)

			if len(lines) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d", len(tt.expected), len(lines))
				return
			}

			for i, line := range lines {
				if line != tt.expected[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.expected[i], line)
				}
			}
		})
	}
}