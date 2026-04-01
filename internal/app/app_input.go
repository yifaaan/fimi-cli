package app

import (
	"fmt"
	"strings"

	"fimi-cli/internal/config"
)

// runInput 表示当前 CLI 入口解析出的最小输入结果。
type runInput struct {
	prompt          string
	forceNewSession bool
	continueSession bool
	modelAlias      string
	outputMode      string // "shell"（默认）, "text", "stream-json"
	showHelp        bool
	yolo            bool
}

// parseRunInput 把 CLI 参数折叠成应用层输入。
// 这里先手写最小解析逻辑，只识别 app 层已经承诺支持的 flag。
func parseRunInput(args []string) (runInput, error) {
	promptParts := make([]string, 0, len(args))
	input := runInput{}
	parseFlags := true

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if parseFlags && arg == "--" {
			parseFlags = false
			continue
		}
		if parseFlags && arg == "--new-session" {
			input.forceNewSession = true
			continue
		}
		if parseFlags && (arg == "--continue" || arg == "-C") {
			input.continueSession = true
			continue
		}
		if parseFlags && (arg == "--yolo" || arg == "--dangerously-skip-permissions") {
			input.yolo = true
			continue
		}
		if parseFlags && (arg == "--help" || arg == "-h") {
			input.showHelp = true
			continue
		}
		if parseFlags && arg == "--model" {
			if i+1 >= len(args) {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			value := strings.TrimSpace(args[i+1])
			if value == "" || strings.HasPrefix(value, "-") {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			input.modelAlias = value
			i++
			continue
		}
		if parseFlags && arg == "--output" {
			if i+1 >= len(args) {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			value := strings.TrimSpace(args[i+1])
			if value == "" || strings.HasPrefix(value, "-") {
				return runInput{}, fmt.Errorf("%w: %s", ErrCLIFlagValueRequired, arg)
			}

			switch value {
			case "text", "stream-json", "shell":
				input.outputMode = value
			default:
				return runInput{}, fmt.Errorf("invalid --output value %q: must be text, stream-json, or shell", value)
			}
			i++
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "-") {
			return runInput{}, fmt.Errorf("%w: %s", ErrUnknownCLIFlag, arg)
		}

		promptParts = append(promptParts, arg)
	}

	input.prompt = strings.TrimSpace(strings.Join(promptParts, " "))
	if input.forceNewSession && input.continueSession {
		return runInput{}, ErrConflictingSessionFlags
	}

	return runInput{
		prompt:          input.prompt,
		forceNewSession: input.forceNewSession,
		continueSession: input.continueSession,
		modelAlias:      input.modelAlias,
		outputMode:      input.outputMode,
		showHelp:        input.showHelp,
		yolo:            input.yolo,
	}, nil
}

// applyRunInputToConfig 把一次运行的 CLI 覆盖项折叠进当前进程内有效配置。
// 这里只改本次运行视图，不修改磁盘配置文件。
func applyRunInputToConfig(cfg config.Config, input runInput) (config.Config, error) {
	if input.modelAlias == "" {
		return cfg, nil
	}
	if _, ok := cfg.Models[input.modelAlias]; !ok {
		return config.Config{}, fmt.Errorf("model %q not found in config.models", input.modelAlias)
	}

	cfg.DefaultModel = input.modelAlias
	return cfg, nil
}
