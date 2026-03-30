package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/changelog"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui/shell/components"
	"fimi-cli/internal/ui/shell/styles"
	"fimi-cli/internal/wire"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
	// ModeSessionSelect 选择 session 的交互模式
	ModeSessionSelect
	// ModeCheckpointSelect 选择 checkpoint 的交互模式
	ModeCheckpointSelect
	// ModeCommandSelect 选择命令的交互模式
	ModeCommandSelect
	// ModeSetup Interactive setup wizard
	ModeSetup
	// ModeApprovalPrompt Waiting for approval decision
	ModeApprovalPrompt
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

	// 当前正在运行的 shell 动作命令字，用于在完成态识别后处理。
	activeShellActionCommand string

	// /init 隔离执行的临时历史文件路径，完成后清理。
	initTempFile string

	// Session 选择相关状态
	sessionList         []session.SessionInfo
	selectedSession     int
	sessionScrollOffset int // 滚动偏移量
	inSessionSelect     bool

	// Checkpoint 选择相关状态
	checkpointList         []contextstore.CheckpointRecord
	selectedCheckpoint     int
	checkpointScrollOffset int

	// Command 建议相关状态
	showCommandSuggestions bool
	selectedSuggestion     int

	// Setup wizard state (active when mode == ModeSetup)
	setupState SetupState

	// Wire for bidirectional communication with runtime
	wire             *wire.Wire
	pendingApprovals map[string]*wire.ApprovalRequest
}

// CommandInfo 表示一个可用的命令。
type CommandInfo struct {
	Name        string
	Description string
}

// setupPhase represents phases in the setup wizard.
type setupPhase int

const (
	setupPhaseWelcome setupPhase = iota
	setupPhaseProviderSelect
	setupPhaseAPIKeyInput
	setupPhaseModelSelect
	setupPhaseSave
)

// SetupState holds temporary state during setup wizard.
type SetupState struct {
	phase            setupPhase
	selectedProvider string
	selectedModel    string
	apiKeyInput      string
	config           config.Config
}

// availableCommands 返回所有可用的命令列表。
func availableCommands() []CommandInfo {
	return []CommandInfo{
		{Name: "/help", Description: "Show this help message"},
		{Name: "/clear", Description: "Clear the screen"},
		{Name: "/compact", Description: "Compact conversation context"},
		{Name: "/rewind", Description: "List available rewind checkpoints"},
		{Name: "/version", Description: "Show version information"},
		{Name: "/release-notes", Description: "Show release notes"},
		{Name: "/exit", Description: "Exit the shell"},
		{Name: "/quit", Description: "Exit the shell"},
		{Name: "/init", Description: "Generate AGENTS.md for the project"},
		{Name: "/setup", Description: "Setup LLM provider and model"},
		{Name: "/resume", Description: "List available sessions"},
	}
}

// NewModel 创建一个新的 Bubble Tea 模型。
func NewModel(deps Dependencies, history *historyStore) Model {
	// 如果有启动信息，显示横幅
	showBanner := deps.StartupInfo != (StartupInfo{})

	// Create wire for runtime communication
	w := wire.New(0)

	output := NewOutputModel()
	for _, line := range transcriptLineModelsFromRecords(deps.InitialRecords) {
		output = output.AppendLine(line)
	}

	return Model{
		input:            NewInputModel(),
		output:           output,
		runtime:          NewRuntimeModel(),
		mode:             ModeIdle,
		showBanner:       showBanner,
		deps:             deps,
		history:          history,
		wire:             w,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
	}
}

// Init 实现 tea.Model 接口。
// 返回初始命令，包括 spinner 动画和事件监听。
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
	)
}

// Update 实现 tea.Model 接口。
// 处理所有传入的消息并更新模型状态。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle setup mode separately
		if m.mode == ModeSetup {
			return m.handleSetupKeyPress(msg)
		}
		// 如果在 session 选择模式，特殊处理键盘输入
		if m.mode == ModeSessionSelect {
			return m.handleSessionSelectKeyPress(msg)
		}
		if m.mode == ModeCheckpointSelect {
			return m.handleCheckpointSelectKeyPress(msg)
		}
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
		m = m.finishRuntime(msg)
		return m, nil

	case ClearMsg:
		m.output = m.output.Clear()
		return m, nil

	case ErrorMsg:
		m.err = msg.Err
		return m, nil

	case ResumeListMsg:
		return m.handleResumeListResult(msg)

	case ResumeSwitchMsg:
		return m.handleResumeSwitchResult(msg)

	case SessionDeleteMsg:
		return m.handleSessionDeleteResult(msg)

	case CheckpointListMsg:
		return m.handleCheckpointListResult(msg)

	case wireErrorMsg:
		m.err = msg.Err
		return m, nil

	case approvalRequestMsg:
		if msg.Request != nil {
			m.pendingApprovals[msg.Request.ID] = msg.Request
			promptText := fmt.Sprintf("⏺ %s (pending approval)\n  %s\n  [y] Approve  [s] For session  [n] Reject",
				msg.Request.Action, msg.Request.Description)
			m.output = m.output.AppendLine(TranscriptLine{
				Type:    LineTypeApproval,
				Content: promptText,
			})
		}
		m.mode = ModeApprovalPrompt
		return m, nil

	case approvalResolveMsg:
		return m.resolveApproval(msg.ID, msg.Response)
	}

	return m, tea.Batch(cmds...)
}

// View 实现 tea.Model 接口。
// 渲染整个 UI。
func (m Model) View() string {
	// 如果在 session 选择模式，渲染选择界面
	if m.mode == ModeSessionSelect {
		return m.renderSessionSelectView()
	}
	if m.mode == ModeCheckpointSelect {
		return m.renderCheckpointSelectView()
	}
	if m.mode == ModeSetup {
		return m.renderSetupView()
	}

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

	// 3.5 命令建议（内联下拉框）
	if m.showCommandSuggestions {
		suggestions := m.renderCommandSuggestions()
		if suggestions != "" {
			sections = append(sections, suggestions)
		}
	}

	// 4. 状态栏
	statusBar := m.renderStatusBar()
	if statusBar != "" {
		sections = append(sections, statusBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderBanner 渲染启动横幅。
func (m Model) renderBanner() string {
	return components.RenderBanner(components.BannerInfo{
		SessionID:      m.deps.StartupInfo.SessionID,
		SessionReused:  m.deps.StartupInfo.SessionReused,
		ModelName:      m.deps.StartupInfo.ModelName,
		AppVersion:     m.deps.StartupInfo.AppVersion,
		ConversationDB: m.deps.StartupInfo.ConversationDB,
		LastRole:       m.deps.StartupInfo.LastRole,
		LastSummary:    m.deps.StartupInfo.LastSummary,
		WorkDir:        m.deps.WorkDir,
	})
}

// handleKeyPress 处理键盘输入。
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Approval mode key handling
	if m.mode == ModeApprovalPrompt {
		switch msg.String() {
		case "y":
			return m.resolveFirstPending(wire.ApprovalApprove)
		case "s":
			return m.resolveFirstPending(wire.ApprovalApproveForSession)
		case "n":
			return m.resolveFirstPending(wire.ApprovalReject)
		}
		return m, nil
	}

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

	// 如果显示命令建议，处理导航和选择
	if m.showCommandSuggestions {
		filtered := m.filteredCommands()
		if len(filtered) == 0 {
			m.showCommandSuggestions = false
			m.selectedSuggestion = 0
		} else {
			switch msg.String() {
			case "up", "ctrl+p":
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(filtered) - 1
				}
				return m, nil
			case "down", "ctrl+n":
				m.selectedSuggestion++
				if m.selectedSuggestion >= len(filtered) {
					m.selectedSuggestion = 0
				}
				return m, nil
			case "enter", "tab":
				// 接受选中的建议
				if m.selectedSuggestion < len(filtered) {
					cmd := filtered[m.selectedSuggestion]
					m.input = m.input.SetValue(cmd.Name + " ")
					m.showCommandSuggestions = false
					m.selectedSuggestion = 0
				}
				return m, nil
			case "esc":
				// 关闭建议
				m.showCommandSuggestions = false
				m.selectedSuggestion = 0
				return m, nil
			}
		}
	}

	// 转发给输入模型处理
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg, m.width)

	// 检查是否需要显示命令建议
	input := m.input.Value()
	if strings.HasPrefix(input, "/") && !strings.Contains(input, " ") {
		filtered := m.filteredCommands()
		if len(filtered) > 0 {
			m.showCommandSuggestions = true
			if m.selectedSuggestion >= len(filtered) {
				m.selectedSuggestion = 0
			}
		} else {
			m.showCommandSuggestions = false
			m.selectedSuggestion = 0
		}
	} else {
		m.showCommandSuggestions = false
		m.selectedSuggestion = 0
	}

	return m, cmd
}

// filteredCommands 返回根据当前输入过滤的命令列表。
func (m Model) filteredCommands() []CommandInfo {
	input := m.input.Value()
	if !strings.HasPrefix(input, "/") {
		return availableCommands()
	}

	// 过滤匹配的命令
	var filtered []CommandInfo
	for _, cmd := range availableCommands() {
		if strings.HasPrefix(cmd.Name, input) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

// renderCommandSuggestions 渲染内联的命令建议下拉框。
func (m Model) renderCommandSuggestions() string {
	filtered := m.filteredCommands()
	if len(filtered) == 0 {
		return ""
	}

	// 下拉框样式
	dropdownStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorMuted).
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")). // bright cyan
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted)

	var lines []string
	maxDisplay := 5 // 最多显示5个建议
	if len(filtered) < maxDisplay {
		maxDisplay = len(filtered)
	}

	for i := 0; i < maxDisplay; i++ {
		cmd := filtered[i]
		if i == m.selectedSuggestion {
			lines = append(lines, selectedStyle.Render(fmt.Sprintf("> %s", cmd.Name)))
		} else {
			lines = append(lines, normalStyle.Render(fmt.Sprintf("  %s", cmd.Name)))
		}
	}

	if len(filtered) > maxDisplay {
		lines = append(lines, normalStyle.Render(fmt.Sprintf("  ... %d more", len(filtered)-maxDisplay)))
	}

	return dropdownStyle.Render(strings.Join(lines, "\n"))
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
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
		m.startRuntimeExecution(m.deps.Store, text),
	)
}

func (m Model) handleRuntimeEvents(msg RuntimeEventsMsg) (tea.Model, tea.Cmd) {
	for _, event := range msg.Events {
		if stepBegin, ok := event.(runtimeevents.StepBegin); ok && stepBegin.Number > 1 {
			m.output = m.output.FlushPending()
		}
		m.runtime = m.runtime.ApplyEvent(event)
		m.mode = ModeStreaming
		m.output = m.output.SetPending(m.runtime.ToLines())
	}

	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
	)
}
func (m Model) finishRuntime(msg RuntimeCompleteMsg) Model {
	m.mode = ModeIdle
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	if msg.Err != nil {
		m.err = msg.Err
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error: %v", msg.Err),
		})
		m.cleanupInitTempFile()
		m.activeShellActionCommand = ""
		return m
	}
	if m.activeShellActionCommand == "/compact" {
		m = m.finishCompactRuntime(msg)
	} else if m.activeShellActionCommand == "/init" {
		m = m.finishInitRuntime()
	}
	m.activeShellActionCommand = ""

	return m
}

func compactedRecordsFromResult(store contextstore.Context, result runtime.Result) []contextstore.TextRecord {
	records := []contextstore.TextRecord{contextstore.NewSystemTextRecord("session initialized")}
	if firstUser, ok, err := store.ReadFirstUserRecord(); err == nil && ok && strings.TrimSpace(firstUser.Content) != "" {
		records = append(records, firstUser)
	}
	for _, step := range result.Steps {
		if strings.TrimSpace(step.AssistantText) == "" {
			continue
		}
		records = append(records, contextstore.NewAssistantTextRecord(step.AssistantText))
	}
	return records
}

func compactedNoticeText() string {
	return "Conversation context compacted into a working summary."
}

func (m Model) rebuildOutputFromRecords(records []contextstore.TextRecord) Model {
	m.output = NewOutputModel()
	for _, line := range transcriptLineModelsFromRecords(records) {
		m.output = m.output.AppendLine(line)
	}
	if last, ok, err := m.deps.Store.Last(); err == nil && ok {
		m.deps.StartupInfo.LastRole = last.Role
		m.deps.StartupInfo.LastSummary = strings.TrimSpace(last.Content)
	}
	return m
}

func (m Model) finishCompactRuntime(msg RuntimeCompleteMsg) Model {
	records := compactedRecordsFromResult(m.deps.Store, msg.Result)
	if err := m.deps.Store.RewriteTextRecordsPreservingNamedBackup(records, "compact"); err != nil {
		m.err = err
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error: persist compacted history: %v", err),
		})
		return m
	}

	m = m.rebuildOutputFromRecords(records)
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: compactedNoticeText(),
	})
	return m
}

func (m Model) finishInitRuntime() Model {
	defer m.cleanupInitTempFile()

	agentsMD := readAgentsMD(m.deps.WorkDir)
	if agentsMD == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "AGENTS.md was not generated. The agent may have failed to create it.",
		})
		return m
	}

	// 注入 AGENTS.md 感知到真实会话上下文，让后续对话了解项目结构。
	systemMsg := fmt.Sprintf(
		"<system>The user just ran `/init` meta command. "+
			"The system has analyzed the codebase and generated an `AGENTS.md` file. "+
			"Latest AGENTS.md file content:\n%s</system>", agentsMD)
	if err := m.deps.Store.Append(contextstore.NewUserTextRecord(systemMsg)); err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error appending init result: %v", err),
		})
		return m
	}

	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: "AGENTS.md has been generated successfully.",
	})
	return m
}

func (m *Model) cleanupInitTempFile() {
	if m.initTempFile != "" {
		os.Remove(m.initTempFile)
		m.initTempFile = ""
	}
}

func readAgentsMD(workDir string) string {
	for _, name := range []string{"AGENTS.md", "agents.md"} {
		data, err := os.ReadFile(filepath.Join(workDir, name))
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

// handleCommand 处理 slash 命令。
func (m Model) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	switch {
	case cmd == "/exit" || cmd == "/quit":
		return m, tea.Quit
	case cmd == "/help":
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: helpText(),
		})
		return m, nil
	case cmd == "/clear":
		m.output = m.output.Clear()
		return m, nil
	case cmd == "/compact":
		return m.handleCompactCommand()
	case cmd == "/init":
		return m.handleInitCommand()
	case cmd == "/rewind":
		return m.handleRewindList()
	case cmd == "/version":
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: versionText(m.deps.StartupInfo.AppVersion),
		})
		return m, nil
	case cmd == "/release-notes":
		entries := changelog.ParseReleases(changelog.Content, 5)
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: releaseNotesText(entries),
		})
		return m, nil
	case cmd == "/resume":
		return m.handleResumeList()
	case cmd == "/setup":
		return m.enterSetupMode()
	default:
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Unknown command: %s", cmd),
		})
		return m, nil
	}
}

// enterSetupMode initializes the setup wizard.
func (m Model) enterSetupMode() (tea.Model, tea.Cmd) {
	cfg, _ := config.Load() // use defaults if no config exists
	m.mode = ModeSetup
	m.setupState = SetupState{
		phase:  setupPhaseWelcome,
		config: cfg,
	}
	m.showBanner = false
	return m, nil
}

// renderSetupView renders the setup wizard UI.
func (m Model) renderSetupView() string {
	var lines []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)

	promptStyle := lipgloss.NewStyle().
		Foreground(styles.ColorAccent)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)

	switch m.setupState.phase {
	case setupPhaseWelcome:
		lines = append(lines, titleStyle.Render("Setup Wizard"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Current configuration:"))
		lines = append(lines, fmt.Sprintf("  Default model: %s", m.setupState.config.DefaultModel))
		lines = append(lines, "")
		if len(m.setupState.config.Models) > 0 {
			lines = append(lines, "  Configured models:")
			for alias, mc := range m.setupState.config.Models {
				lines = append(lines, fmt.Sprintf("    - %s (provider: %s)", alias, mc.Provider))
			}
		} else {
			lines = append(lines, "  No models configured.")
		}
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to start setup, Esc/q to cancel"))

	case setupPhaseProviderSelect:
		lines = append(lines, titleStyle.Render("Select Provider"))
		lines = append(lines, "")
		lines = append(lines, "  [1] qwen")
		lines = append(lines, "  [2] openai")
		lines = append(lines, "  [3] anthropic")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number to select, Esc/q to cancel"))

	case setupPhaseAPIKeyInput:
		lines = append(lines, titleStyle.Render("API Key"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Enter API key for %s:", m.setupState.selectedProvider)))
		// Show asterisks for typed input
		masked := strings.Repeat("*", len(m.setupState.apiKeyInput))
		lines = append(lines, fmt.Sprintf("  %s", masked))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Type key and press Enter, Esc/q to cancel"))

	case setupPhaseModelSelect:
		lines = append(lines, titleStyle.Render("Select Model"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Choose model for %s:", m.setupState.selectedProvider)))
		// Show suggested models for provider
		suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
		for i, model := range suggested {
			lines = append(lines, fmt.Sprintf("  [%d] %s", i+1, model))
		}
		lines = append(lines, "  Or type a custom model name")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number or custom name, Esc/q to cancel"))

	case setupPhaseSave:
		lines = append(lines, titleStyle.Render("Save Configuration"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Ready to save:"))
		lines = append(lines, fmt.Sprintf("  Provider: %s", m.setupState.selectedProvider))
		lines = append(lines, fmt.Sprintf("  Model: %s", m.setupState.selectedModel))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to save, Esc/q to cancel"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// suggestedModelsForProvider returns common model names for a provider.
func suggestedModelsForProvider(provider string) []string {
	switch provider {
	case "qwen":
		return []string{"qwen-plus", "qwen-turbo", "qwen-max"}
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	case "anthropic":
		return []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"}
	default:
		return nil
	}
}

// handleSetupKeyPress processes keyboard input in setup mode.
func (m Model) handleSetupKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Global cancel
	if keyStr == "esc" || keyStr == "q" {
		m.mode = ModeIdle
		m.setupState = SetupState{}
		return m, nil
	}

	switch m.setupState.phase {
	case setupPhaseWelcome:
		if keyStr == "enter" {
			m.setupState.phase = setupPhaseProviderSelect
		}
		return m, nil

	case setupPhaseProviderSelect:
		switch keyStr {
		case "1":
			m.setupState.selectedProvider = "qwen"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "2":
			m.setupState.selectedProvider = "openai"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "3":
			m.setupState.selectedProvider = "anthropic"
			m.setupState.phase = setupPhaseAPIKeyInput
		}
		return m, nil

	case setupPhaseAPIKeyInput:
		switch keyStr {
		case "enter":
			if m.setupState.apiKeyInput != "" {
				m.setupState.phase = setupPhaseModelSelect
			}
		case "backspace":
			if len(m.setupState.apiKeyInput) > 0 {
				m.setupState.apiKeyInput = m.setupState.apiKeyInput[:len(m.setupState.apiKeyInput)-1]
			}
		default:
			// Accumulate printable characters
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.apiKeyInput += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseModelSelect:
		switch keyStr {
		case "enter":
			if m.setupState.selectedModel != "" {
				m.setupState.phase = setupPhaseSave
			}
		case "backspace":
			if len(m.setupState.selectedModel) > 0 {
				m.setupState.selectedModel = m.setupState.selectedModel[:len(m.setupState.selectedModel)-1]
			}
		default:
			// Check for number selection
			suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
			if len(msg.Runes) == 1 {
				num := int(msg.Runes[0] - '0')
				if num >= 1 && num <= len(suggested) {
					m.setupState.selectedModel = suggested[num-1]
					m.setupState.phase = setupPhaseSave
					return m, nil
				}
			}
			// Accumulate printable characters for custom model name
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.selectedModel += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseSave:
		if keyStr == "enter" {
			return m.saveSetupConfig()
		}
		return m, nil
	}

	return m, nil
}

// saveSetupConfig writes the setup configuration and returns to idle mode.
func (m Model) saveSetupConfig() (tea.Model, tea.Cmd) {
	// Build provider config
	providerAlias := m.setupState.selectedProvider
	providerConfig := config.ProviderConfig{
		Type:   providerAlias,
		APIKey: m.setupState.apiKeyInput,
	}
	// Use existing base URL for known providers
	if providerAlias == "qwen" {
		providerConfig.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	m.setupState.config.Providers[providerAlias] = providerConfig

	// Build model config
	modelAlias := m.setupState.selectedModel
	m.setupState.config.Models[modelAlias] = config.ModelConfig{
		Provider: providerAlias,
		Model:    modelAlias,
	}
	m.setupState.config.DefaultModel = modelAlias

	// Save config
	if err := config.Save(m.setupState.config); err != nil {
		m.mode = ModeIdle
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error saving config: %v", err),
		})
		m.setupState = SetupState{}
		return m, nil
	}

	m.mode = ModeIdle
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: "Configuration saved successfully.",
	})
	m.setupState = SetupState{}
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

	// 新动作开始前先把上一轮临时 UI 状态落定，并清空 live runtime 状态。
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()

	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: spec.StatusText,
	})
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeUser,
		Content: spec.CommandText,
	})

	if m.history != nil {
		_ = m.history.Append(spec.CommandText)
	}
	m.input.AppendHistory(spec.CommandText)

	m.input = m.input.Clear()
	m.mode = ModeThinking
	m.activeShellActionCommand = spec.CommandText
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

	// 创建隔离的临时上下文，避免 /init 探索过程污染当前会话历史。
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

	// UI 准备：与 startShellAction 共用相同的状态重置逻辑，
	// 但不把命令文本作为用户消息追加到 transcript。
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: spec.StatusText,
	})

	m.input = m.input.Clear()
	m.mode = ModeThinking
	m.activeShellActionCommand = spec.CommandText
	m.initTempFile = tmpPath
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
		ctx := wire.WithCurrent(context.Background(), m.wire)
		ctx = approval.WithContext(ctx, approval.New(m.deps.Yolo))

		result, err := m.deps.Runner.Run(ctx, store, runtime.Input{
			Prompt:       prompt,
			Model:        m.deps.ModelName,
			SystemPrompt: m.deps.SystemPrompt,
		})

		return RuntimeCompleteMsg{Result: result, Err: err}
	}
}

// renderLiveStatus 渲染实时状态区域。
func (m Model) renderLiveStatus() string {
	statusText := m.renderLiveStatusText()
	if statusText == "" {
		return ""
	}
	return styles.HelpStyle.Render(m.runtime.SpinnerView() + " " + statusText)
}

func (m Model) renderLiveStatusText() string {
	if m.runtime.CurrentTool != nil && m.runtime.CurrentTool.Status == ToolStatusRunning {
		return "Running " + formatToolCallLine(*m.runtime.CurrentTool) + "..."
	}
	return "Running..."
}

// handleResumeListResult 处理 session 列表查询结果。
func (m Model) handleResumeListResult(msg ResumeListMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error listing sessions: %v", msg.Err),
		})
		return m, nil
	}

	if len(msg.Sessions) == 0 {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "No sessions found for this work directory.",
		})
		return m, nil
	}

	// 进入 session 选择模式
	m.mode = ModeSessionSelect
	m.sessionList = msg.Sessions
	m.selectedSession = 0
	return m, nil
}

// handleSessionSelectKeyPress 处理 session 选择模式的键盘输入。
func (m Model) handleSessionSelectKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// 计算可见区域大小
	availableHeight := m.height - 8 // 预留标题、状态、帮助等
	if availableHeight < 6 {
		availableHeight = 6
	}
	linesPerSession := 2
	maxVisible := availableHeight / linesPerSession
	if maxVisible < 1 {
		maxVisible = 1
	}

	// 计算当前可见范围
	total := len(m.sessionList)
	oldScrollOffset := m.sessionScrollOffset

	switch keyStr {
	case "up", "k":
		if m.selectedSession > 0 {
			m.selectedSession--
			// 如果选中项在当前视口上方，向上滚动
			if m.selectedSession < m.sessionScrollOffset {
				m.sessionScrollOffset = m.selectedSession
			}
		}
	case "down", "j":
		if m.selectedSession < total-1 {
			m.selectedSession++
			// 如果选中项在当前视口下方，向下滚动
			if m.selectedSession >= m.sessionScrollOffset+maxVisible {
				m.sessionScrollOffset = m.selectedSession - maxVisible + 1
			}
		}
	case "enter":
		// 切换到选中的 session
		return m.handleResumeSwitch(m.sessionList[m.selectedSession].ID)
	case "ctrl+d":
		// 删除选中的 session
		if len(m.sessionList) == 0 {
			return m, nil
		}
		return m, m.deleteSelectedSession()
	case "esc", "q":
		// 取消选择，返回正常模式
		m.mode = ModeIdle
		m.sessionList = nil
		m.selectedSession = 0
		m.sessionScrollOffset = 0
		return m, nil
	}

	// 只有滚动位置变化时才需要清屏（翻页）
	if m.sessionScrollOffset != oldScrollOffset {
		return m, tea.ClearScreen
	}
	return m, nil
}

// renderSessionSelectView 渲染 session 选择界面。
func (m Model) renderSessionSelectView() string {
	var sections []string

	// 标题
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	sections = append(sections, titleStyle.Render("Select a session to resume"))
	sections = append(sections, "")

	// 计算可见区域
	// 每个session占用2行，预留标题(2行) + 状态(1行) + 帮助(1行) + 边距(2行) = 6行
	availableHeight := m.height - 6
	if availableHeight < 6 {
		availableHeight = 6
	}
	linesPerSession := 2
	maxVisibleSessions := availableHeight / linesPerSession
	if maxVisibleSessions < 1 {
		maxVisibleSessions = 1
	}

	totalSessions := len(m.sessionList)

	// 使用模型中的滚动偏移量
	scrollOffset := m.sessionScrollOffset
	// 确保滚动偏移量在有效范围内
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > totalSessions-maxVisibleSessions && totalSessions > maxVisibleSessions {
		scrollOffset = totalSessions - maxVisibleSessions
	}

	// 计算实际渲染的session范围
	startIdx := scrollOffset
	endIdx := scrollOffset + maxVisibleSessions
	if endIdx > totalSessions {
		endIdx = totalSessions
	}

	// 淡蓝色样式（用于选中项）
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")). // bright cyan = 淡蓝色
		Bold(true)
	selectedMetaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")) // bright cyan

	// Session 列表
	for i := startIdx; i < endIdx; i++ {
		s := m.sessionList[i]
		// 获取第一条用户消息作为预览
		preview := m.getSessionPreview(s.HistoryFile)

		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		// 格式化文件大小
		fileSize := formatFileSize(s.FileSize)
		// 格式化时间
		timeAgo := formatTime(s.LastModified)

		// 第一行：session id + preview
		idPreview := fmt.Sprintf("%s  %s", shortID, preview)
		// 第二行：时间 + 大小
		metaLine := fmt.Sprintf("    %s  %s", timeAgo, fileSize)

		var block string
		displayNum := i + 1
		if i == m.selectedSession {
			// 选中项：淡蓝色 + 粗体
			line1 := selectedStyle.Render(fmt.Sprintf("[%d] > %s", displayNum, idPreview))
			line2 := selectedMetaStyle.Render(fmt.Sprintf("      %s", metaLine))
			block = fmt.Sprintf("%s\n%s", line1, line2)
		} else {
			// 普通项
			block = fmt.Sprintf("[%d] %s\n    %s", displayNum, idPreview, metaLine)
		}
		sections = append(sections, block)
	}

	sections = append(sections, "")

	// 状态提示：显示当前位置和总数
	statusText := fmt.Sprintf("Session %d/%d", m.selectedSession+1, totalSessions)
	if totalSessions > maxVisibleSessions {
		statusText += fmt.Sprintf(" (showing %d-%d)", startIdx+1, endIdx)
	}
	statusStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, statusStyle.Render(statusText))

	// 帮助提示
	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, helpStyle.Render("↑/↓ or j/k to navigate, Enter to select, Ctrl+D to delete, Esc/q to cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// formatFileSize 返回友好的文件大小描述（kB / MB / GB）。
func formatFileSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}

// getSessionPreview 获取 session 的第一条用户消息预览（保留第一行）。
func (m Model) getSessionPreview(historyFile string) string {
	store := contextstore.New(historyFile)
	record, found, err := store.ReadFirstUserRecord()
	if err != nil || !found {
		return "..."
	}

	content := strings.TrimSpace(record.Content)
	// 只取第一行
	lines := strings.Split(content, "\n")
	firstLine := strings.TrimSpace(lines[0])
	// 使用 rune 截断以正确处理 UTF-8/中文，限制在一行内显示
	runes := []rune(firstLine)
	if len(runes) > 50 {
		return string(runes[:50]) + "..."
	}
	return firstLine
}

// handleResumeSwitchResult 处理 session 切换结果。
func (m Model) handleResumeSwitchResult(msg ResumeSwitchMsg) (tea.Model, tea.Cmd) {
	// 无论成功与否，退出 session 选择模式
	m.mode = ModeIdle
	m.sessionList = nil
	m.selectedSession = 0

	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error loading session: %v", msg.Err),
		})
		return m, nil
	}

	// 更新 Store 到新的 session
	newStore := contextstore.New(msg.Session.HistoryFile)
	m.deps.Store = newStore

	// 更新 StartupInfo
	m.deps.StartupInfo.SessionID = msg.Session.ID
	m.deps.StartupInfo.SessionReused = true
	m.deps.StartupInfo.ConversationDB = msg.Session.HistoryFile

	// 清空当前输出
	m.output = m.output.Clear()

	// 使用 transcriptLineModelsFromRecords 将历史记录转换为 TranscriptLine
	// 这样会以标准格式显示（带颜色、前缀等）
	for _, line := range transcriptLineModelsFromRecords(msg.Records) {
		m.output = m.output.AppendLine(line)
	}

	// 添加切换提示
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: fmt.Sprintf("Switched to session %s", msg.Session.ID[:8]),
	})

	return m, nil
}

// deleteSelectedSession 返回一个删除选中 session 的命令。
func (m Model) deleteSelectedSession() tea.Cmd {
	if len(m.sessionList) == 0 || m.selectedSession >= len(m.sessionList) {
		return nil
	}

	sessionID := m.sessionList[m.selectedSession].ID
	workDir := m.deps.WorkDir

	return func() tea.Msg {
		err := session.DeleteSession(workDir, sessionID)
		return SessionDeleteMsg{SessionID: sessionID, Err: err}
	}
}

// handleSessionDeleteResult 处理 session 删除结果。
func (m Model) handleSessionDeleteResult(msg SessionDeleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		// 删除失败，显示错误并保持在选择模式
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error deleting session: %v", msg.Err),
		})
		return m, nil
	}

	// 从列表中移除已删除的 session
	var newList []session.SessionInfo
	for _, s := range m.sessionList {
		if s.ID != msg.SessionID {
			newList = append(newList, s)
		}
	}

	if len(newList) == 0 {
		// 没有剩余 session，退出选择模式
		m.mode = ModeIdle
		m.sessionList = nil
		m.selectedSession = 0
		m.sessionScrollOffset = 0
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "Session deleted. No more sessions available.",
		})
		return m, nil
	}

	// 更新列表
	m.sessionList = newList

	// 调整选中位置
	if m.selectedSession >= len(newList) {
		m.selectedSession = len(newList) - 1
	}

	// 调整滚动偏移
	availableHeight := m.height - 8
	if availableHeight < 6 {
		availableHeight = 6
	}
	maxVisible := availableHeight / 2
	if maxVisible < 1 {
		maxVisible = 1
	}
	if m.sessionScrollOffset > m.selectedSession {
		m.sessionScrollOffset = m.selectedSession
	}
	if m.sessionScrollOffset > len(newList)-maxVisible && len(newList) > maxVisible {
		m.sessionScrollOffset = len(newList) - maxVisible
	}

	// 清屏并重绘
	return m, tea.ClearScreen
}

// truncateString 截断字符串到指定长度（使用 rune 正确处理 UTF-8）。
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

func (m Model) handleRewindList() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		checkpoints, err := m.deps.Store.ListCheckpoints()
		return CheckpointListMsg{Checkpoints: checkpoints, Err: err}
	}
}

func (m Model) handleCheckpointListResult(msg CheckpointListMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error listing rewind checkpoints: %v", msg.Err),
		})
		return m, nil
	}

	if len(msg.Checkpoints) == 0 {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "No rewind checkpoints found for this session.",
		})
		return m, nil
	}

	m.mode = ModeCheckpointSelect
	m.checkpointList = msg.Checkpoints
	m.selectedCheckpoint = 0
	m.checkpointScrollOffset = 0
	return m, nil
}

func (m Model) handleCheckpointSelectKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	availableHeight := m.height - 8
	if availableHeight < 6 {
		availableHeight = 6
	}
	linesPerCheckpoint := 2
	maxVisible := availableHeight / linesPerCheckpoint
	if maxVisible < 1 {
		maxVisible = 1
	}

	total := len(m.checkpointList)
	oldScrollOffset := m.checkpointScrollOffset

	switch keyStr {
	case "up", "k":
		if m.selectedCheckpoint > 0 {
			m.selectedCheckpoint--
			if m.selectedCheckpoint < m.checkpointScrollOffset {
				m.checkpointScrollOffset = m.selectedCheckpoint
			}
		}
	case "down", "j":
		if m.selectedCheckpoint < total-1 {
			m.selectedCheckpoint++
			if m.selectedCheckpoint >= m.checkpointScrollOffset+maxVisible {
				m.checkpointScrollOffset = m.selectedCheckpoint - maxVisible + 1
			}
		}
	case "enter":
		return m.rewindToSelectedCheckpoint(), nil
	case "esc", "q":
		return m.finishCheckpointSelection(), nil
	}

	if m.checkpointScrollOffset != oldScrollOffset {
		return m, tea.ClearScreen
	}
	return m, nil
}

func (m Model) renderCheckpointSelectView() string {
	var sections []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)
	sections = append(sections, titleStyle.Render("Select a checkpoint to rewind"))
	sections = append(sections, "")

	availableHeight := m.height - 6
	if availableHeight < 6 {
		availableHeight = 6
	}
	linesPerCheckpoint := 2
	maxVisibleCheckpoints := availableHeight / linesPerCheckpoint
	if maxVisibleCheckpoints < 1 {
		maxVisibleCheckpoints = 1
	}

	totalCheckpoints := len(m.checkpointList)
	scrollOffset := m.checkpointScrollOffset
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > totalCheckpoints-maxVisibleCheckpoints && totalCheckpoints > maxVisibleCheckpoints {
		scrollOffset = totalCheckpoints - maxVisibleCheckpoints
	}

	startIdx := scrollOffset
	endIdx := scrollOffset + maxVisibleCheckpoints
	if endIdx > totalCheckpoints {
		endIdx = totalCheckpoints
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)
	selectedMetaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	for i := startIdx; i < endIdx; i++ {
		checkpoint := m.checkpointList[i]
		label := strings.TrimSpace(checkpoint.PromptPreview)
		if label == "" {
			label = fmt.Sprintf("checkpoint %d", checkpoint.ID)
		}

		metaLine := fmt.Sprintf("ID %d", checkpoint.ID)
		if createdAt := strings.TrimSpace(checkpoint.CreatedAt); createdAt != "" {
			metaLine = fmt.Sprintf("ID %d  %s", checkpoint.ID, createdAt)
		}

		var block string
		displayNum := i + 1
		if i == m.selectedCheckpoint {
			line1 := selectedStyle.Render(fmt.Sprintf("[%d] > %s", displayNum, truncateString(label, 70)))
			line2 := selectedMetaStyle.Render(fmt.Sprintf("      %s", metaLine))
			block = fmt.Sprintf("%s\n%s", line1, line2)
		} else {
			block = fmt.Sprintf("[%d] %s\n    %s", displayNum, truncateString(label, 70), metaLine)
		}
		sections = append(sections, block)
	}

	sections = append(sections, "")

	statusText := fmt.Sprintf("Checkpoint %d/%d", m.selectedCheckpoint+1, totalCheckpoints)
	if totalCheckpoints > maxVisibleCheckpoints {
		statusText += fmt.Sprintf(" (showing %d-%d)", startIdx+1, endIdx)
	}
	statusStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, statusStyle.Render(statusText))

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)
	sections = append(sections, helpStyle.Render("↑/↓ or j/k to navigate, Enter to select, Esc/q to cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func rewindedNoticeText(checkpointID int) string {
	return fmt.Sprintf("Conversation rewound to checkpoint %d.", checkpointID)
}

func (m Model) finishCheckpointSelection() Model {
	m.mode = ModeIdle
	m.checkpointList = nil
	m.selectedCheckpoint = 0
	m.checkpointScrollOffset = 0
	return m
}

func (m Model) rewindToSelectedCheckpoint() Model {
	if len(m.checkpointList) == 0 || m.selectedCheckpoint >= len(m.checkpointList) {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error rewinding conversation: no checkpoint selected",
		})
		return m.finishCheckpointSelection()
	}

	checkpoint := m.checkpointList[m.selectedCheckpoint]
	if _, err := m.deps.Store.RevertToCheckpoint(checkpoint.ID); err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error rewinding conversation: %v", err),
		})
		return m.finishCheckpointSelection()
	}

	records, err := m.deps.Store.ReadAll()
	if err != nil {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("error reading rewound conversation: %v", err),
		})
		return m.finishCheckpointSelection()
	}

	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m.activeShellActionCommand = ""
	m = m.finishCheckpointSelection()
	m = m.rebuildOutputFromRecords(records)
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: rewindedNoticeText(checkpoint.ID),
	})
	return m
}

// handleResumeList 处理 /resume 命令（无参数），列出可用 session。
func (m Model) handleResumeList() (tea.Model, tea.Cmd) {
	if m.deps.WorkDir == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: work directory not set",
		})
		return m, nil
	}

	return m, func() tea.Msg {
		sessions, err := session.ListSessions(m.deps.WorkDir)
		return ResumeListMsg{Sessions: sessions, Err: err}
	}
}

// handleResumeSwitch 处理 /resume <id> 命令，切换到指定 session。
func (m Model) handleResumeSwitch(sessionID string) (tea.Model, tea.Cmd) {
	if m.deps.WorkDir == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: work directory not set",
		})
		return m, nil
	}
	if sessionID == "" {
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: "error: session ID is required",
		})
		return m, nil
	}

	return m, func() tea.Msg {
		sess, err := session.LoadSession(m.deps.WorkDir, sessionID)
		if err != nil {
			// 尝试前缀匹配
			sessions, listErr := session.ListSessions(m.deps.WorkDir)
			if listErr == nil {
				for _, s := range sessions {
					if strings.HasPrefix(s.ID, sessionID) {
						sess = session.Session{
							ID:          s.ID,
							WorkDir:     s.WorkDir,
							HistoryFile: s.HistoryFile,
						}
						err = nil
						break
					}
				}
			}
		}
		if err != nil {
			return ResumeSwitchMsg{Err: err}
		}

		// 读取新 session 的历史记录
		newStore := contextstore.New(sess.HistoryFile)
		records, _ := newStore.ReadRecentTurns(10)

		return ResumeSwitchMsg{
			Session: sess,
			Records: records,
			Err:     nil,
		}
	}
}

// currentShortcutHint returns a context-appropriate keyboard hint.
func (m Model) currentShortcutHint() string {
	switch m.mode {
	case ModeStreaming, ModeThinking:
		return ""
	}
	if m.output.HasExpandedResults() {
		return "Ctrl+O折叠"
	}
	return "Ctrl+O展开"
}

// renderStatusBar renders the status bar.
func (m Model) renderStatusBar() string {
	var leftParts []string
	var rightParts []string

	// Left: context usage, mode, model name
	if m.runtime.ContextUsage > 0 {
		pct := int(m.runtime.ContextUsage * 100)
		ctxText := styles.ContextStyle(pct).Render(fmt.Sprintf("Context: %d%%", pct))
		leftParts = append(leftParts, ctxText)
	}

	switch m.mode {
	case ModeThinking:
		leftParts = append(leftParts, styles.SystemStyle.Render("Thinking..."))
	case ModeStreaming:
		leftParts = append(leftParts, styles.SystemStyle.Render("Streaming..."))
	}

	if m.deps.ModelName != "" {
		leftParts = append(leftParts, styles.ModelStyle.Render(m.deps.ModelName))
	}

	// Right: shortcut hint + time
	shortcut := m.currentShortcutHint()
	if shortcut != "" {
		rightParts = append(rightParts, styles.HelpStyle.Render(shortcut))
	}
	rightParts = append(rightParts, styles.SystemStyle.Render(time.Now().Format("15:04:05")))

	leftContent := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)
	rightContent := lipgloss.JoinHorizontal(lipgloss.Top, rightParts...)

	gap := m.width - lipgloss.Width(leftContent) - lipgloss.Width(rightContent)
	if gap < 1 {
		gap = 1
	}

	return leftContent + strings.Repeat(" ", gap) + rightContent
}

// resolveFirstPending resolves the first pending approval request.
func (m Model) resolveFirstPending(resp wire.ApprovalResponse) (tea.Model, tea.Cmd) {
	for id := range m.pendingApprovals {
		return m.resolveApproval(id, resp)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}

// resolveApproval completes an approval request.
func (m Model) resolveApproval(id string, resp wire.ApprovalResponse) (Model, tea.Cmd) {
	if req, ok := m.pendingApprovals[id]; ok {
		req.Resolve(resp)
		delete(m.pendingApprovals, id)
	}
	m.mode = ModeThinking
	return m, m.wireReceiveLoop()
}
