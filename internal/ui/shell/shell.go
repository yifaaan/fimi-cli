package shell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui"
)

const (
	promptText      = "fimi> "
	clearScreenANSI = "\033[H\033[2J"
)

// Runner 定义 shell 对 runtime 的最小依赖边界。
type Runner interface {
	Run(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error)
}

// Dependencies 描述 shell REPL 运行需要的最小装配输入。
type Dependencies struct {
	Runner        Runner
	Store         contextstore.Context
	Input         io.Reader
	Output        io.Writer
	ErrOutput     io.Writer
	ModelName     string
	SystemPrompt  string
	InitialPrompt string
}

type eventSinkCapableRunner interface {
	WithEventSink(sink runtimeevents.Sink) runtime.Runner
}

// Run 启动最小交互式 shell。
func Run(ctx context.Context, deps Dependencies) error {
	if deps.Runner == nil {
		return fmt.Errorf("shell runner is required")
	}

	input := deps.Input
	if input == nil {
		input = strings.NewReader("")
	}

	output := deps.Output
	if output == nil {
		output = io.Discard
	}

	display := newDisplay(output)

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if err := runPrompt(ctx, deps, display, strings.TrimSpace(deps.InitialPrompt)); err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if _, err := fmt.Fprint(output, promptText); err != nil {
			return fmt.Errorf("write shell prompt: %w", err)
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read shell input: %w", err)
			}

			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		exit, err := dispatchCommand(ctx, deps, display, line)
		if err != nil {
			return err
		}
		if exit {
			return nil
		}
	}
}

func dispatchCommand(
	ctx context.Context,
	deps Dependencies,
	display *display,
	line string,
) (bool, error) {
	switch line {
	case "/exit":
		return true, nil
	case "/help":
		return false, display.AppendTranscriptText(helpText())
	case "/clear":
		return false, display.Clear()
	default:
		if strings.HasPrefix(line, "/") {
			return false, display.AppendTranscriptLines([]string{
				fmt.Sprintf("unknown command: %s", line),
			})
		}

		if err := runPrompt(ctx, deps, display, line); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, display.AppendTranscriptLines([]string{
				fmt.Sprintf("run error: %v", err),
			})
		}

		return false, nil
	}
}

func runPrompt(
	ctx context.Context,
	deps Dependencies,
	display *display,
	prompt string,
) error {
	if prompt == "" {
		return nil
	}

	result, err := ui.Run(
		ctx,
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			if eventfulRunner, ok := deps.Runner.(eventSinkCapableRunner); ok {
				return eventfulRunner.WithEventSink(sink).Run(ctx, deps.Store, runtime.Input{
					Prompt:       prompt,
					Model:        deps.ModelName,
					SystemPrompt: deps.SystemPrompt,
				})
			}

			return deps.Runner.Run(ctx, deps.Store, runtime.Input{
				Prompt:       prompt,
				Model:        deps.ModelName,
				SystemPrompt: deps.SystemPrompt,
			})
		},
		visualizeLive(display),
	)
	if err != nil {
		return err
	}
	if result.Status == runtime.RunStatusFailed {
		return fmt.Errorf("runtime finished with status %q", result.Status)
	}

	return nil
}

func helpText() string {
	return strings.Join([]string{
		"Shell commands:",
		"  /help   Show available shell commands",
		"  /clear  Clear the terminal transcript",
		"  /exit   Exit shell mode",
	}, "\n")
}
