package app

import (
	"fmt"
	"strings"
)

// printHelp 输出当前 CLI 入口支持的最小帮助信息。
func printHelp() {
	fmt.Print(helpText())
}

// helpText 返回当前 CLI 入口支持的最小帮助文本。
func helpText() string {
	lines := make([]string, 0, 16)
	for _, section := range helpSections() {
		lines = append(lines, helpSectionLines(section.title, section.lines)...)
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

type helpSection struct {
	title string
	lines []string
}

func helpSections() []helpSection {
	return []helpSection{
		{title: "Usage", lines: helpUsageLines()},
		{title: "Flags", lines: helpFlagLines()},
		{title: "Prompt Rules", lines: helpPromptRuleLines()},
		{title: "Examples", lines: helpExampleLines()},
	}
}

func helpSectionLines(title string, lines []string) []string {
	section := make([]string, 0, len(lines)+1)
	section = append(section, title+":")
	section = append(section, lines...)

	return section
}

func helpUsageLines() []string {
	return []string{
		"  fimi [--continue] [--model <alias>] [--output <mode>] [--help] [prompt...]",
		"  fimi [options] -- [prompt text starting with flags]",
	}
}

func helpFlagLines() []string {
	return []string{
		"  --continue, -C   Continue the previous session for this work dir",
		"  --new-session    Explicitly start a fresh session for this run",
		"  --model <alias>  Override the configured model for this run",
		"  --output <mode>  Output mode: shell (default), text, stream-json",
		"  --yolo           Skip all tool approval prompts",
		"  -h, --help       Show this help message",
	}
}

func helpPromptRuleLines() []string {
	return []string{
		"  --                Stop parsing flags; everything after it is prompt text",
		"  prompt...         Remaining args are joined into the shell's initial prompt",
	}
}

func helpExampleLines() []string {
	return []string{
		"  fimi fix the flaky test",
		"  fimi --continue continue the refactor from the last session",
		"  fimi --model fast-model refactor the session loader",
		"  fimi -- --help should be treated as prompt text",
	}
}
