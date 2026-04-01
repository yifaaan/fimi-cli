package shell

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/changelog"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/tools"
	"fimi-cli/internal/ui/shell/completer"
	"fimi-cli/internal/ui/shell/styles"
	"fimi-cli/internal/wire"

	tea "github.com/charmbracelet/bubbletea"
)

// handleCommand 处理 slash 命令。
func (m Model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return m, nil
	}

	name := fields[0]
	args := fields[1:]

	switch name {
	case "/exit", "/quit":
		return m, tea.Quit
	case "/help":
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: helpText(),
		})
		return m, nil
	case "/clear":
		m.output = m.output.Clear()
		return m, nil
	case "/compact":
		return m.handleCompactCommand()
	case "/init":
		return m.handleInitCommand()
	case "/rewind":
		return m.handleRewindList()
	case "/version":
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: versionText(m.deps.StartupInfo.AppVersion),
		})
		return m, nil
	case "/release-notes":
		entries := changelog.ParseReleases(changelog.Content, 5)
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: releaseNotesText(entries),
		})
		return m, nil
	case "/resume":
		if len(args) > 0 {
			return m.handleResumeSwitch(args[0])
		}
		return m.handleResumeList()
	case "/task":
		return m.handleTaskCommand(args)
	case "/setup":
		return m.enterSetupMode()
	case "/reload":
		return m.handleReloadCommand()
	default:
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Unknown command: %s", cmd),
		})
		return m, nil
	}
}

func (m Model) handleTaskCommand(args []string) (tea.Model, tea.Cmd) {
	if m.deps.TaskManager == nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: background task manager not available",
		})
		return m, nil
	}

	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == "list"):
		tasks := m.deps.TaskManager.List()
		content := "No background tasks for this session."
		if len(tasks) > 0 {
			content = formatTaskListOutput(tasks)
		}
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: content,
		})
		return m, nil

	case len(args) == 1:
		task, err := m.deps.TaskManager.Status(args[0])
		if err != nil {
			m.output = m.output.AppendLine(TranscriptLine{
				Type:    LineTypeError,
				Content: fmt.Sprintf("error reading background task: %v", err),
			})
			return m, nil
		}
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: formatTaskDetailOutput(task),
		})
		return m, nil

	case len(args) == 2 && args[0] == "kill":
		if err := m.deps.TaskManager.Kill(args[1]); err != nil {
			m.output = m.output.AppendLine(TranscriptLine{
				Type:    LineTypeError,
				Content: fmt.Sprintf("error killing background task: %v", err),
			})
			return m, nil
		}
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("Killed background task %s", args[1]),
		})
		return m, nil

	default:
		m.output = m.output.AppendLine(TranscriptLine{
			Type: LineTypeError,
			Content: strings.Join([]string{
				"usage:",
				"  /task",
				"  /task <id>",
				"  /task kill <id>",
			}, "\n"),
		})
		return m, nil
	}
}

// handleReloadCommand reloads configuration and refreshes the file index.
func (m Model) handleReloadCommand() (tea.Model, tea.Cmd) {
	cfg, err := config.Load()
	msg := "Configuration reloaded."
	if err != nil {
		msg = fmt.Sprintf("Failed to reload config: %v", err)
	} else {
		m.deps.ModelName = cfg.DefaultModel
	}
	if m.fileIndexer != nil {
		m.fileIndexer = completer.NewFileIndexer(m.deps.WorkDir)
	}
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: msg,
	})
	return m, nil
}

type shellActionSpec struct {
	CommandText string
	StatusText  string
	Prompt      string
}

func (m Model) handleCompactCommand() (tea.Model, tea.Cmd) {
	return m.startShellAction(compactActionSpec())
}

func (m Model) startShellAction(spec shellActionSpec) (tea.Model, tea.Cmd) {
	spec.CommandText = strings.TrimSpace(spec.CommandText)
	spec.Prompt = strings.TrimSpace(spec.Prompt)
	if spec.CommandText == "" || spec.Prompt == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: shell action requires non-empty command text and prompt",
		})
		return m, nil
	}

	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m = m.prepareRuntimeExecution()

	if m.history != nil {
		_ = m.history.Append(spec.CommandText)
	}
	m.input.AppendHistory(spec.CommandText)

	m.input = m.input.Clear()
	m.resetFileCompletion()
	m.mode = ModeThinking
	m.activeShellActionCommand = spec.CommandText
	m.runtimeStartedAt = time.Now()

	// 将 system notice 和 user command 添加到 pending blocks
	m.output = m.output.SetPending([]TranscriptBlock{
		{
			Kind: BlockKindSystemNotice,
			Text: spec.StatusText,
		},
		{
			Kind:     BlockKindUserPrompt,
			UserText: spec.CommandText,
		},
	})

	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
		m.startRuntimeExecution(m.deps.Store, spec.Prompt),
	)
}

func compactActionSpec() shellActionSpec {
	return shellActionSpec{
		CommandText: "/compact",
		StatusText:  "Compacting conversation context into a structured working summary...",
		Prompt: strings.Join([]string{
			"Compact the current conversation context into a concise working summary.",
			"Return the result using exactly these section headings:",
			"- Current goal",
			"- Constraints",
			"- Decisions",
			"- Open tasks",
			"- Next step",
			"Keep each section brief and concrete.",
			"Preserve the user's goals, current constraints, important decisions, open tasks, and the next concrete step so work can continue efficiently in the same session.",
		}, "\n"),
	}
}

func (m Model) handleInitCommand() (tea.Model, tea.Cmd) {
	spec := initActionSpec()

	tmpFile, err := os.CreateTemp("", "fimi-init-*.jsonl")
	if err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error creating temp context: %v", err),
		})
		return m, nil
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m = m.prepareRuntimeExecution()
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: spec.StatusText,
	})

	m.input = m.input.Clear()
	m.resetFileCompletion()
	m.mode = ModeThinking
	m.activeShellActionCommand = spec.CommandText
	m.initTempFile = tmpPath
	m.runtimeStartedAt = time.Now()
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
		m.startRuntimeExecution(contextstore.New(tmpPath), spec.Prompt),
	)
}

func initActionSpec() shellActionSpec {
	return shellActionSpec{
		CommandText: "/init",
		StatusText:  "Analyzing project and generating AGENTS.md...",
		Prompt: strings.Join([]string{
			"You are a software engineering expert. Explore the current project directory to understand the project's architecture and main details.",
			"",
			"Task requirements:",
			"1. Analyze the project structure and identify key configuration files (such as pyproject.toml, package.json, go.mod, Cargo.toml, etc.).",
			"2. Understand the project's technology stack, build process and runtime architecture.",
			"3. Identify how the code is organized and main module divisions.",
			"4. Discover project-specific development conventions, testing strategies, and deployment processes.",
			"",
			"After the exploration, write a thorough summary into `AGENTS.md` file in the project root. Refer to what is already in the file when you do so.",
			"",
			"`AGENTS.md` is a file intended to be read by AI coding agents. Expect the reader knows nothing about the project.",
			"Compose this file according to the actual project content. Do not make assumptions or generalizations.",
			"Ensure the information is accurate and useful. Use the natural language that is mainly used in the project's comments and documentation.",
			"",
			"Suggested sections:",
			"- Project overview",
			"- Build and test commands",
			"- Code style guidelines",
			"- Testing instructions",
			"- Security considerations",
		}, "\n"),
	}
}

// startRuntimeExecution 返回一个启动 runtime 执行的命令。
// store 参数允许调用方选择在哪个上下文中执行（当前会话或隔离临时存储）。
func (m Model) startRuntimeExecution(store contextstore.Context, prompt string) tea.Cmd {
	return func() tea.Msg {
		baseCtx := m.deps.RunContext
		if baseCtx == nil {
			baseCtx = context.Background()
		}

		ctx := wire.WithCurrent(baseCtx, m.wire)
		ctx = approval.WithContext(ctx, approval.New(m.deps.Yolo))

		result, err := m.deps.Runner.Run(ctx, store, runtime.Input{
			Prompt:       prompt,
			Model:        m.deps.ModelName,
			SystemPrompt: m.deps.SystemPrompt,
		})

		return RuntimeCompleteMsg{Result: result, Err: err}
	}
}

func (m Model) prepareRuntimeExecution() Model {
	if m.wire != nil {
		m.wire.Shutdown()
	}
	m.wire = wire.New(0)
	m.pendingApprovals = make(map[string]*wire.ApprovalRequest)
	m.commitLateRuntimeEvents = false
	return m
}

// renderLiveStatus 渲染实时状态区域。
func (m Model) renderLiveStatus() string {
	statusText := m.renderLiveStatusText()
	if statusText == "" {
		return ""
	}
	return transcriptBodyIndent() + styles.LiveStatusStyle.Render(m.runtime.SpinnerView()+" "+statusText)
}

func (m Model) renderLiveStatusText() string {
	if (m.mode != ModeThinking && m.mode != ModeStreaming) || m.runtimeStartedAt.IsZero() {
		return ""
	}
	elapsed := formatWorkingElapsed(time.Since(m.runtimeStartedAt))
	if retry := m.runtime.Retry; retry != nil {
		return "Working (" + elapsed + ") · " + formatRetryLiveStatusText(*retry)
	}
	return "Working (" + elapsed + ")"
}

func formatRetryLiveStatusText(retry runtimeevents.RetryStatus) string {
	seconds := math.Max(0, float64(retry.NextDelayMS)/1000)
	return fmt.Sprintf("Retrying in %.1fs (next attempt %d/%d)...", seconds, retry.Attempt+1, retry.MaxAttempts)
}

func formatTaskListOutput(tasks []tools.TaskResult) string {
	lines := []string{"Background tasks:"}
	for _, task := range tasks {
		lines = append(lines, fmt.Sprintf(
			"- %s [%s] %s (%s)",
			task.ID,
			task.Status,
			truncateTaskCommand(task.Command),
			task.Duration.Round(time.Millisecond),
		))
	}

	return strings.Join(lines, "\n")
}

func formatTaskDetailOutput(task tools.TaskResult) string {
	lines := []string{
		fmt.Sprintf("Task %s [%s]", task.ID, task.Status),
		fmt.Sprintf("Command: %s", task.Command),
		fmt.Sprintf("Duration: %s", task.Duration.Round(time.Millisecond)),
	}
	if task.ExitCode != 0 {
		lines = append(lines, fmt.Sprintf("Exit code: %d", task.ExitCode))
	}
	if task.Stdout != "" {
		lines = append(lines, "STDOUT:")
		lines = append(lines, task.Stdout)
	}
	if task.Stderr != "" {
		lines = append(lines, "STDERR:")
		lines = append(lines, task.Stderr)
	}

	return strings.Join(lines, "\n")
}

func truncateTaskCommand(command string) string {
	command = strings.TrimSpace(command)
	runes := []rune(command)
	if len(runes) <= 40 {
		return command
	}

	return string(runes[:37]) + "..."
}

func formatWorkingElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d.Round(time.Second).Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	remainingSeconds := seconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %02ds", minutes, remainingSeconds)
	}
	hours := minutes / 60
	remainingMinutes := minutes % 60
	return fmt.Sprintf("%dh %02dm %02ds", hours, remainingMinutes, remainingSeconds)
}
