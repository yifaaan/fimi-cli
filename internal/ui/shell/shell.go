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

	tea "github.com/charmbracelet/bubbletea"
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
	Runner         Runner
	Store          contextstore.Context
	Input          io.Reader
	Output         io.Writer
	ErrOutput      io.Writer
	HistoryFile    string
	EditorFactory  lineEditorFactory
	ModelName      string
	SystemPrompt   string
	InitialPrompt  string
	InitialRecords []contextstore.TextRecord
	StartupInfo    StartupInfo
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
	LastRole       string
	LastSummary    string
}

// Run 启动交互式 shell。
// 在 TTY 模式下使用 Bubble Tea UI，否则使用简单的 transcript 模式。
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

	interactiveTTY, fallbackReason := interactiveTTYStatus(input, output)
	if !interactiveTTY && fallbackReason != "" {
		if _, err := fmt.Fprintf(
			errOutput,
			"shell ui disabled: %s; falling back to text mode\n",
			fallbackReason,
		); err != nil {
			return fmt.Errorf("write shell ui fallback reason: %w", err)
		}
	}

	// 加载历史记录
	history, err := loadHistoryStore(deps.HistoryFile)
	if err != nil {
		fmt.Fprintf(output, "shell history unavailable: %v\n", err)
	}

	// 在 TTY 模式下使用 Bubble Tea UI
	if interactiveTTY {
		return runBubbleTeaMode(ctx, deps, &history)
	}

	// 非交互模式使用传统的 liner/transcript
	return runLinerMode(ctx, deps, &history)
}

// runBubbleTeaMode 使用 Bubble Tea 框架运行 shell。
func runBubbleTeaMode(ctx context.Context, deps Dependencies, history *historyStore) error {
	// 创建 Bubble Tea 模型
	model := NewModel(deps, history)

	// 创建 Bubble Tea 程序
	// 使用 WithAltScreen 让 Bubble Tea 使用备用屏幕缓冲区
	// 这样退出时会恢复之前的终端内容
	p := tea.NewProgram(
		model,
		tea.WithInput(deps.Input),
		tea.WithOutput(deps.Output),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// 在 goroutine 中运行，以便处理 context 取消
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		p.Quit()
		return ctx.Err()
	}
}

// runLinerMode 使用传统的 liner-based UI 运行 shell（非交互模式）。
func runLinerMode(ctx context.Context, deps Dependencies, history *historyStore) error {
	input := deps.Input
	if input == nil {
		input = strings.NewReader("")
	}

	output := deps.Output
	if output == nil {
		output = io.Discard
	}

	display := newDisplay(output, false)

	editorFactory := deps.EditorFactory
	if editorFactory == nil {
		editorFactory = newScannerEditor
	}
	editor, err := editorFactory(input, output, history.Entries())
	if err != nil {
		return fmt.Errorf("create shell line editor: %w", err)
	}
	defer editor.Close()

	if err := displayStartupBanner(display, deps.StartupInfo); err != nil {
		return err
	}
	if err := appendInitialTranscript(display, deps.InitialRecords); err != nil {
		return err
	}

	if err := runPrompt(ctx, deps, display, editor, history, false, strings.TrimSpace(deps.InitialPrompt)); err != nil {
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

		exit, err := dispatchCommand(ctx, deps, display, editor, history, false, line)
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

func appendInitialTranscript(display *display, records []contextstore.TextRecord) error {
	lines := transcriptLinesFromRecords(records)
	if len(lines) == 0 {
		return nil
	}

	return display.AppendTranscriptLines(lines)
}

func transcriptLinesFromRecords(records []contextstore.TextRecord) []string {
	models := transcriptLineModelsFromRecords(records)
	lines := make([]string, 0, len(models)*2)
	for _, line := range models {
		switch line.Type {
		case LineTypeUser:
			lines = append(lines, "[user]")
			lines = append(lines, splitPreservingEmpty(line.Content)...)
		case LineTypeAssistant:
			lines = append(lines, "[assistant]")
			lines = append(lines, splitPreservingEmpty(line.Content)...)
		case LineTypeToolCall:
			lines = append(lines, "[tool] "+line.Content)
		case LineTypeToolResult:
			lines = append(lines, "[tool result]")
			lines = append(lines, splitPreservingEmpty(line.Content)...)
		}
	}

	return lines
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
	if info.LastSummary != "" {
		if info.LastRole != "" {
			lines = append(lines, fmt.Sprintf("  last: %s: %s", info.LastRole, info.LastSummary))
		} else {
			lines = append(lines, fmt.Sprintf("  last: %s", info.LastSummary))
		}
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
