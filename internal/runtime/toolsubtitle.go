package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolCallSubtitle 根据工具调用参数生成人类可读的简短摘要。
// 逻辑上属于工具特有的展示知识，但因为 runtime.ToolCall 类型定义在此，
// 暂时放在 runtime 包中以避免循环依赖。
// 当 tools 包不再依赖 runtime 类型时，可以移到 tools 包。
func ToolCallSubtitle(call ToolCall) string {
	switch call.Name {
	case "agent":
		return agentToolSubtitle(call.Arguments)
	case "bash":
		return bashToolSubtitle(call.Arguments)
	case "read_file":
		return readFileToolSubtitle(call.Arguments)
	case "glob":
		return globToolSubtitle(call.Arguments)
	case "grep":
		return grepToolSubtitle(call.Arguments)
	case "write_file":
		return writeFileToolSubtitle(call.Arguments)
	case "replace_file":
		return replaceFileToolSubtitle(call.Arguments)
	case "patch_file":
		return patchFileToolSubtitle(call.Arguments)
	case "think":
		return thinkToolSubtitle(call.Arguments)
	case "set_todo_list":
		return setTodoListToolSubtitle(call.Arguments)
	default:
		return ""
	}
}

func agentToolSubtitle(raw string) string {
	var args struct {
		SubagentName string `json:"subagent_name"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if name := strings.TrimSpace(args.SubagentName); name != "" {
		return "Ran subagent " + name
	}

	return "Ran subagent"
}

func bashToolSubtitle(raw string) string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if command := strings.TrimSpace(args.Command); command != "" {
		return "Ran " + command
	}

	return "Ran command"
}

func readFileToolSubtitle(raw string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if path := strings.TrimSpace(args.Path); path != "" {
		return "Read " + path
	}

	return "Read file"
}

func globToolSubtitle(raw string) string {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if pattern := strings.TrimSpace(args.Pattern); pattern != "" {
		return "Matched " + pattern
	}

	return "Matched paths"
}

func grepToolSubtitle(raw string) string {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}

	pattern := strings.TrimSpace(args.Pattern)
	path := strings.TrimSpace(args.Path)
	switch {
	case pattern != "" && path != "":
		return fmt.Sprintf("Searched %s for %q", path, pattern)
	case pattern != "":
		return fmt.Sprintf("Searched for %q", pattern)
	case path != "":
		return "Searched " + path
	default:
		return "Searched files"
	}
}

func writeFileToolSubtitle(raw string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if path := strings.TrimSpace(args.Path); path != "" {
		return "Wrote " + path
	}

	return "Wrote file"
}

func replaceFileToolSubtitle(raw string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if path := strings.TrimSpace(args.Path); path != "" {
		return "Updated " + path
	}

	return "Updated file"
}

func patchFileToolSubtitle(raw string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if path := strings.TrimSpace(args.Path); path != "" {
		return "Patched " + path
	}

	return "Applied patch"
}

func thinkToolSubtitle(raw string) string {
	var args struct {
		Thought string `json:"thought"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}

	if thought := strings.TrimSpace(args.Thought); thought != "" {
		return "Thought: " + thought
	}

	return "Thought"
}

func setTodoListToolSubtitle(raw string) string {
	var args struct {
		Todos []struct {
			Title string `json:"title"`
		} `json:"todos"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return ""
	}
	if len(args.Todos) == 0 {
		return "Updated todo list"
	}
	if len(args.Todos) == 1 {
		title := strings.TrimSpace(args.Todos[0].Title)
		if title == "" {
			return "Updated todo list (1 item)"
		}
		return "Updated todo list: " + title
	}

	return fmt.Sprintf("Updated todo list (%d items)", len(args.Todos))
}
