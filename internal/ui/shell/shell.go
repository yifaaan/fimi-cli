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
	"fimi-cli/internal/ui/printui"
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

	errOutput := deps.ErrOutput
	if errOutput == nil {
		errOutput = output
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if err := runPrompt(ctx, deps, output, errOutput, strings.TrimSpace(deps.InitialPrompt)); err != nil {
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

		exit, err := dispatchCommand(ctx, deps, output, errOutput, line)
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
	output io.Writer,
	errOutput io.Writer,
	line string,
) (bool, error) {
	switch line {
	case "/exit":
		return true, nil
	case "/help":
		_, err := fmt.Fprintln(output, helpText())
		return false, err
	case "/clear":
		_, err := fmt.Fprint(output, clearScreenANSI)
		return false, err
	default:
		if strings.HasPrefix(line, "/") {
			if _, err := fmt.Fprintf(errOutput, "unknown command: %s\n", line); err != nil {
				return false, fmt.Errorf("write shell error: %w", err)
			}

			return false, nil
		}

		if err := runPrompt(ctx, deps, output, errOutput, line); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			if _, writeErr := fmt.Fprintf(errOutput, "run error: %v\n", err); writeErr != nil {
				return false, fmt.Errorf("write shell error: %w", writeErr)
			}
		}

		return false, nil
	}
}

func runPrompt(
	ctx context.Context,
	deps Dependencies,
	output io.Writer,
	errOutput io.Writer,
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
		printui.VisualizeText(output),
	)
	if err != nil {
		return err
	}
	if result.Status == runtime.RunStatusFailed {
		return fmt.Errorf("runtime finished with status %q", result.Status)
	}

	_ = errOutput
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
