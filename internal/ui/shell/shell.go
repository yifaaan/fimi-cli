package shell

import (
	"context"
	"errors"
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
	HistoryFile   string
	EditorFactory lineEditorFactory
	ModelName     string
	SystemPrompt  string
	InitialPrompt string
	StartupInfo   StartupInfo
}

type eventSinkCapableRunner interface {
	WithEventSink(sink runtimeevents.Sink) runtime.Runner
}

// StartupInfo 描述 shell 首屏需要展示的启动上下文。
type StartupInfo struct {
	SessionID      string
	SessionReused  bool
	ModelName      string
	ConversationDB string
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

	interactiveTTY := supportsInteractiveTTY(input, output)
	display := newDisplay(output, interactiveTTY)
	history, err := loadHistoryStore(deps.HistoryFile)
	if err != nil {
		if appendErr := display.AppendTranscriptLines([]string{
			fmt.Sprintf("shell history unavailable: %v", err),
		}); appendErr != nil {
			return appendErr
		}
	}

	editorFactory := deps.EditorFactory
	if editorFactory == nil {
		if interactiveTTY {
			editorFactory = newLinerEditor
		} else {
			editorFactory = newScannerEditor
		}
	}
	editor, err := editorFactory(input, output, history.Entries())
	if err != nil {
		return fmt.Errorf("create shell line editor: %w", err)
	}
	defer editor.Close()

	if err := displayStartupBanner(display, deps.StartupInfo); err != nil {
		return err
	}

	if err := runPrompt(ctx, deps, display, editor, &history, interactiveTTY, strings.TrimSpace(deps.InitialPrompt)); err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line, err := editor.ReadLine(promptText)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, ErrLineReadAborted) {
			if err := display.AppendTranscriptLines([]string{"[interrupted]"}); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read shell input: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		exit, err := dispatchCommand(ctx, deps, display, editor, &history, interactiveTTY, line)
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
	editor lineEditor,
	history *historyStore,
	interactiveTTY bool,
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

		if err := runPrompt(ctx, deps, display, editor, history, interactiveTTY, line); err != nil {
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
	editor lineEditor,
	history *historyStore,
	interactiveTTY bool,
	prompt string,
) error {
	if prompt == "" {
		return nil
	}
	if editor != nil {
		editor.AppendHistory(prompt)
	}
	if history != nil {
		if err := history.Append(prompt); err != nil {
			if appendErr := display.AppendTranscriptLines([]string{
				fmt.Sprintf("shell history unavailable: %v", err),
			}); appendErr != nil {
				return appendErr
			}
		}
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
		visualizeForMode(display, interactiveTTY),
	)
	if err != nil {
		return err
	}
	if result.Status == runtime.RunStatusFailed {
		return fmt.Errorf("runtime finished with status %q", result.Status)
	}

	return nil
}

func visualizeForMode(display *display, interactiveTTY bool) ui.VisualizeFunc {
	if interactiveTTY {
		return visualizeLive(display)
	}

	return visualizeTranscript(display)
}

func displayStartupBanner(display *display, info StartupInfo) error {
	lines := startupBannerLines(info)
	if len(lines) == 0 {
		return nil
	}

	return display.AppendTranscriptLines(lines)
}

func startupBannerLines(info StartupInfo) []string {
	if info == (StartupInfo{}) {
		return nil
	}

	lines := []string{"Shell session"}
	if info.SessionID != "" {
		lines = append(lines, fmt.Sprintf("  session: %s", info.SessionID))
	}
	if info.SessionReused {
		lines = append(lines, "  mode: continue")
	} else {
		lines = append(lines, "  mode: new")
	}
	if info.ModelName != "" {
		lines = append(lines, fmt.Sprintf("  model: %s", info.ModelName))
	}
	if info.ConversationDB != "" {
		lines = append(lines, fmt.Sprintf("  history: %s", info.ConversationDB))
	}
	lines = append(lines, "  commands: /help /clear /exit")

	return lines
}

func helpText() string {
	return strings.Join([]string{
		"Shell commands:",
		"  /help   Show available shell commands",
		"  /clear  Clear the terminal transcript",
		"  /exit   Exit shell mode",
	}, "\n")
}
