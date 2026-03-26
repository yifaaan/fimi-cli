package shell

import (
	"context"
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Mode 表示 UI 的当前操作模式。
type Mode int

const (
	// ModeIdle 空闲状态，等待用户输入
	ModeIdle Mode = iota
	// ModeThinking AI 正在处理（显示 spinner）
	ModeThinking
	// ModeStreaming AI 响应正在流式输出
	ModeStreaming
)

// Model 是 Bubble Tea 的根模型。
// 它组合了输入、输出和运行时三个子模型。
type Model struct {
	// 子模型 (组合模式)
	input   InputModel
	output  OutputModel
	runtime RuntimeModel

	// 共享状态
	width  int
	height int
	mode   Mode
	err    error

	// 是否显示启动横幅
	showBanner bool

	// 依赖
	deps    Dependencies
	history *historyStore

	// 运行时事件通道 (由 Run() 创建)
	eventsCh <-chan runtimeevents.Event
	// runtime 已结束，但仍需等待尾部事件排空后再切回 idle。
	pendingCompletion *RuntimeCompleteMsg
}

// NewModel 创建一个新的 Bubble Tea 模型。
func NewModel(deps Dependencies, history *historyStore) Model {
	// 如果有启动信息，显示横幅
	showBanner := deps.StartupInfo != (StartupInfo{})

	output := NewOutputModel()
	for _, line := range transcriptLineModelsFromRecords(deps.InitialRecords) {
		output = output.AppendLine(line)
	}

	return Model{
		input:      NewInputModel(),
		output:     output,
		runtime:    NewRuntimeModel(),
		mode:       ModeIdle,
		showBanner: showBanner,
		deps:       deps,
		history:    history,
	}
}

// Init 实现 tea.Model 接口。
// 返回初始命令，包括 spinner 动画和事件监听。
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.runtime.SpinnerCmd(),
	)
}

// Update 实现 tea.Model 接口。
// 处理所有传入的消息并更新模型状态。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		var outputCmd tea.Cmd
		m.output, outputCmd = m.output.Update(msg, m.width, m.height)
		if outputCmd != nil {
			cmds = append(cmds, outputCmd)
		}
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		var outputCmd tea.Cmd
		m.output, outputCmd = m.output.Update(msg, m.width, m.height)
		return m, outputCmd

	case spinner.TickMsg:
		if m.mode == ModeIdle {
			return m, nil
		}
		var cmd tea.Cmd
		m.runtime, cmd = m.runtime.UpdateSpinner(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// 更新子模型的尺寸
		var inputCmd, outputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg, m.width)
		m.output, outputCmd = m.output.Update(msg, m.width, m.height)
		return m, tea.Batch(inputCmd, outputCmd)

	case RuntimeEventMsg:
		return m.handleRuntimeEvents(RuntimeEventsMsg{
			Events: []runtimeevents.Event{msg.Event},
		})

	case RuntimeEventsMsg:
		return m.handleRuntimeEvents(msg)

	case InputSubmitMsg:
		return m.handleSubmit(msg.Text)

	case RuntimeCompleteMsg:
		// 先等待事件通道排空，再进入最终完成态，避免尾部流式内容丢失。
		if m.eventsCh != nil {
			complete := msg
			m.pendingCompletion = &complete
			return m, nil
		}
		m = m.finishRuntime(msg)
		return m, nil

	case ClearMsg:
		m.output = m.output.Clear()
		return m, nil

	case ErrorMsg:
		m.err = msg.Err
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// View 实现 tea.Model 接口。
// 渲染整个 UI。
func (m Model) View() string {
	var sections []string

	// 0. 启动横幅（只显示一次）
	if m.showBanner {
		banner := m.renderBanner()
		sections = append(sections, banner)
		sections = append(sections, "") // 空行
	}

	// 1. 输出区域 (可滚动的 transcript)
	outputView := m.output.View()
	if outputView != "" {
		sections = append(sections, outputView)
	}

	// 2. 实时状态区域 (spinner + 工具信息)
	if m.mode != ModeIdle {
		liveStatus := m.renderLiveStatus()
		if liveStatus != "" {
			sections = append(sections, liveStatus)
		}
	}

	// 3. 输入区域
	sections = append(sections, m.input.View())

	// 4. 状态栏
	statusBar := m.renderStatusBar()
	if statusBar != "" {
		sections = append(sections, statusBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderBanner 渲染启动横幅。
func (m Model) renderBanner() string {
	info := m.deps.StartupInfo
	var lines []string

	// 标题
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	lines = append(lines, titleStyle.Render("Shell Session"))

	// 会话信息
	if info.SessionID != "" {
		sessionStyle := lipgloss.NewStyle().Foreground(styles.ColorInfo)
		modeText := "new"
		if info.SessionReused {
			modeText = "continue"
		}
		lines = append(lines, fmt.Sprintf("  session: %s (%s)", sessionStyle.Render(info.SessionID[:8]), modeText))
	}

	// 模型名称
	if info.ModelName != "" {
		modelStyle := lipgloss.NewStyle().Foreground(styles.ColorAccent)
		lines = append(lines, fmt.Sprintf("  model: %s", modelStyle.Render(info.ModelName)))
	}

	// 历史数据库
	if info.ConversationDB != "" {
		lines = append(lines, fmt.Sprintf("  history: %s", info.ConversationDB))
	}

	// 最后一条消息
	if info.LastSummary != "" {
		role := info.LastRole
		if role == "" {
			role = "last"
		}
		summary := info.LastSummary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", role, styles.SystemStyle.Render(summary)))
	}

	// 可用命令
	lines = append(lines, "")
	helpStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)
	lines = append(lines, helpStyle.Render("  commands: /help /clear /exit"))

	return strings.Join(lines, "\n")
}

// handleKeyPress 处理键盘输入。
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 全局快捷键
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		if m.mode == ModeIdle {
			return m, tea.Quit
		}
		// 如果正在运行，发送中断信号
		return m, nil

	case "ctrl+l":
		// 清屏
		m.output = m.output.Clear()
		return m, nil

	case "ctrl+o":
		// 切换工具结果折叠状态
		var toggled bool
		m.output, toggled = m.output.ToggleExpand()
		_ = toggled // 忽略返回值，即使没有切换也刷新界面
		return m, nil
	}

	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m, nil
	}

	// 如果正在处理，忽略大部分输入
	if m.mode != ModeIdle {
		return m, nil
	}

	// 转发给输入模型处理
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg, m.width)
	return m, cmd
}

// handleSubmit 处理用户提交的 prompt。
func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}

	// 隐藏启动横幅
	m.showBanner = false

	// 处理 slash 命令
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}

	// 追加到 transcript
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeUser,
		Content: text,
	})

	// 保存历史
	if m.history != nil {
		_ = m.history.Append(text)
	}
	m.input.AppendHistory(text)

	// 清空输入并进入 thinking 模式
	m.input = m.input.Clear()
	m.mode = ModeThinking

	// 启动 runtime 执行
	eventsCh := make(chan runtimeevents.Event, 256)
	m.eventsCh = eventsCh

	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		waitForRuntimeEvents(eventsCh),
		m.startRuntimeExecution(text, eventsCh),
	)
}

func (m Model) handleRuntimeEvents(msg RuntimeEventsMsg) (tea.Model, tea.Cmd) {
	// 已完成后迟到的事件批次不能再污染 UI。
	if m.eventsCh == nil && !msg.Closed {
		return m, nil
	}

	for _, event := range msg.Events {
		if stepBegin, ok := event.(runtimeevents.StepBegin); ok && stepBegin.Number > 1 {
			m.output = m.output.FlushPending()
		}
		m.runtime = m.runtime.ApplyEvent(event)
		m.mode = ModeStreaming
		// pending 持有“当前 step 的完整快照”。
		// 遇到新的 StepBegin 时，上一个 step 已先 flush 到 transcript。
		m.output = m.output.SetPending(m.runtime.ToLines())
	}

	if msg.Closed {
		m.eventsCh = nil
		if m.pendingCompletion != nil {
			m = m.finishRuntime(*m.pendingCompletion)
		}
		return m, nil
	}

	if m.eventsCh == nil {
		return m, nil
	}

	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		waitForRuntimeEvents(m.eventsCh),
	)
}

func (m Model) finishRuntime(msg RuntimeCompleteMsg) Model {
	m.mode = ModeIdle
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m.eventsCh = nil
	m.pendingCompletion = nil
	if msg.Err != nil {
		m.err = msg.Err
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error: %v", msg.Err),
		})
	}

	return m
}

// handleCommand 处理 slash 命令。
func (m Model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
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
	default:
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Unknown command: %s", cmd),
		})
		return m, nil
	}
}

// startRuntimeExecution 返回一个启动 runtime 执行的命令。
func (m Model) startRuntimeExecution(prompt string, eventsCh chan runtimeevents.Event) tea.Cmd {
	return func() tea.Msg {
		// 创建事件 sink
		sink := runtimeevents.SinkFunc(func(ctx context.Context, event runtimeevents.Event) error {
			select {
			case eventsCh <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		runner := m.deps.Runner
		if eventfulRunner, ok := runner.(eventSinkCapableRunner); ok {
			runner = eventfulRunner.WithEventSink(sink)
		}

		// 执行 runtime
		result, err := runner.Run(context.Background(), m.deps.Store, runtime.Input{
			Prompt:       prompt,
			Model:        m.deps.ModelName,
			SystemPrompt: m.deps.SystemPrompt,
		})

		close(eventsCh)

		return RuntimeCompleteMsg{Result: result, Err: err}
	}
}

// renderLiveStatus 渲染实时状态区域。
func (m Model) renderLiveStatus() string {
	var parts []string

	// Spinner
	spinner := m.runtime.SpinnerView()
	if spinner != "" {
		parts = append(parts, spinner)
	}

	// 工具信息
	if toolCard := m.runtime.ToolCardView(m.width - 4); toolCard != "" {
		parts = append(parts, toolCard)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderStatusBar 渲染状态栏。
func (m Model) renderStatusBar() string {
	var parts []string

	// 上下文使用率
	if m.runtime.ContextUsage > 0 {
		pct := int(m.runtime.ContextUsage * 100)
		ctxText := styles.ContextStyle(pct).Render(fmt.Sprintf("Context: %d%%", pct))
		parts = append(parts, ctxText)
	}

	// 模式指示器
	switch m.mode {
	case ModeThinking:
		parts = append(parts, styles.SystemStyle.Render("Thinking..."))
	case ModeStreaming:
		parts = append(parts, styles.SystemStyle.Render("Streaming..."))
	}

	// 模型名称
	if m.deps.ModelName != "" {
		parts = append(parts, styles.ModelStyle.Render(m.deps.ModelName))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
