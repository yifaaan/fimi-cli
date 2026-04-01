package shell

import (
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui/shell/completer"
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
	toasts  ToastModel

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

	// Checkpoint 选择相关状态
	checkpointList         []contextstore.CheckpointRecord
	selectedCheckpoint     int
	checkpointScrollOffset int

	// Command 建议相关状态
	showCommandSuggestions bool
	selectedSuggestion     int

	// File mention completion state
	showFileCompletion     bool
	fileCompletionItems    []string
	selectedFileCompletion int
	fileIndexer            *completer.FileIndexer
	fileCompletionAtPos    int // byte offset of '@' that triggered completion

	// Setup wizard state (active when mode == ModeSetup)
	setupState SetupState

	// Wire for bidirectional communication with runtime
	wire                    *wire.Wire
	pendingApprovals        map[string]*wire.ApprovalRequest
	commitLateRuntimeEvents bool

	// Approval selection state (for ModeApprovalPrompt)
	approvalSelection int // 0=Approve, 1=Approve for session, 2=Reject
}

// CommandInfo 表示一个可用的命令。
type CommandInfo struct {
	Name        string
	Description string
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
		{Name: "/task", Description: "Inspect background tasks"},
		{Name: "/reload", Description: "Reload configuration"},
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

	// Pre-load persistent history into input navigation.
	var historyEntries []string
	if history != nil {
		historyEntries = history.Entries()
	}

	return Model{
		input:            NewInputModelWithHistory(historyEntries),
		output:           output,
		runtime:          NewRuntimeModel(),
		toasts:           NewToastModel(),
		mode:             ModeIdle,
		showBanner:       showBanner,
		deps:             deps,
		history:          history,
		wire:             w,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
		fileIndexer:      completer.NewFileIndexer(deps.WorkDir),
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
func (m Model) Update(msg tea.Msg) (updated tea.Model, cmd tea.Cmd) {
	updated = m
	defer func() {
		model, ok := updated.(Model)
		if !ok {
			return
		}
		var printCmd tea.Cmd
		model, printCmd = model.consumeTranscriptPrintCmd()
		updated = model
		if printCmd == nil {
			return
		}
		if cmd == nil {
			cmd = printCmd
			return
		}
		cmd = tea.Sequence(printCmd, cmd)
	}()

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
		m.output, outputCmd = m.output.Update(msg, m.width, m.currentOutputViewportHeight())
		if outputCmd != nil {
			cmds = append(cmds, outputCmd)
		}
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		var outputCmd tea.Cmd
		m.output, outputCmd = m.output.Update(msg, m.width, m.currentOutputViewportHeight())
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
		m.output, outputCmd = m.output.Update(msg, m.width, m.currentOutputViewportHeight())
		m.toasts = m.toasts.SetWidth(msg.Width)
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
		// 继续运行 wireReceiveLoop 来消费残留的 wire 事件
		return m, tea.Batch(
			m.runtime.SpinnerCmd(),
			m.wireReceiveLoop(),
		)

	case ClearMsg:
		m.output = m.output.Clear()
		m.commitLateRuntimeEvents = false
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
		}
		m.mode = ModeApprovalPrompt
		m.approvalSelection = 0
		return m, nil

	case approvalResolveMsg:
		return m.resolveApproval(msg.ID, msg.Response)

	case FileIndexResultMsg:
		return m.handleFileIndexResult(msg)

	case ToastAddMsg, ToastDismissMsg, ToastTickMsg:
		var cmd tea.Cmd
		m.toasts, cmd = m.toasts.Update(msg)
		return m, cmd
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
	if m.mode == ModeApprovalPrompt {
		return m.renderApprovalView()
	}

	leadingSections, trailingSections := m.mainViewLayoutSections()

	var sections []string
	sections = append(sections, leadingSections...)
	outputView := m.renderOutputForLayout(leadingSections, trailingSections)
	if outputView != "" {
		sections = append(sections, outputView)
	}
	sections = append(sections, trailingSections...)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) mainViewLayoutSections() ([]string, []string) {
	var leadingSections []string
	var trailingSections []string

	if m.showBanner {
		leadingSections = append(leadingSections, m.renderBanner())
		leadingSections = append(leadingSections, "")
	}

	if m.mode != ModeIdle {
		if liveStatus := m.renderLiveStatus(); liveStatus != "" {
			trailingSections = append(trailingSections, liveStatus)
		}
	}

	if toastsView := m.toasts.View(); toastsView != "" {
		trailingSections = append(trailingSections, toastsView)
	}

	trailingSections = append(trailingSections, m.input.View())

	if m.showCommandSuggestions {
		if suggestions := m.renderCommandSuggestions(); suggestions != "" {
			trailingSections = append(trailingSections, suggestions)
		}
	}

	if m.showFileCompletion {
		if suggestions := m.renderFileCompletion(); suggestions != "" {
			trailingSections = append(trailingSections, suggestions)
		}
	}

	if statusBar := m.renderStatusBar(); statusBar != "" {
		trailingSections = append(trailingSections, statusBar)
	}

	return leadingSections, trailingSections
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

func (m Model) renderOutputForLayout(before, after []string) string {
	output := m.output.WithViewportHeight(m.outputHeightForLayout(before, after))
	return output.InteractiveView()
}

func (m Model) outputHeightForLayout(before, after []string) int {
	if m.height <= 0 {
		return 1
	}

	reserved := joinedSectionHeight(before) + joinedSectionHeight(after)
	if len(before) > 0 {
		reserved++
	}
	if len(after) > 0 {
		reserved++
	}

	available := m.height - reserved
	if available < 1 {
		return 1
	}
	return available
}

func (m Model) currentOutputViewportHeight() int {
	if m.mode == ModeApprovalPrompt {
		if req := m.currentApprovalRequest(); req != nil {
			return m.outputHeightForLayout(nil, m.approvalViewTrailingSections(req))
		}
		return 1
	}
	before, after := m.mainViewLayoutSections()
	return m.outputHeightForLayout(before, after)
}

func joinedSectionHeight(sections []string) int {
	if len(sections) == 0 {
		return 0
	}
	return lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

func (m Model) consumeTranscriptPrintCmd() (Model, tea.Cmd) {
	rendered := m.output.RenderUnprintedLines()
	if len(rendered) == 0 {
		return m, nil
	}
	m.output = m.output.MarkPrinted()
	return m, tea.Println(strings.Join(rendered, "\n"))
}

// handleKeyPress 处理键盘输入。
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeApprovalPrompt {
		return m.handleApprovalKeyPress(msg)
	}

	// 全局快捷键
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		// 运行时或审批模式：拒绝所有 pending 审批，然后退出
		if m.mode != ModeIdle {
			for id := range m.pendingApprovals {
				if req, ok := m.pendingApprovals[id]; ok {
					req.Resolve(wire.ApprovalReject)
				}
			}
			return m, tea.Quit
		}
		return m, tea.Quit

	case "ctrl+l":
		// 清屏
		m.output = m.output.Clear()
		m.commitLateRuntimeEvents = false
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

	// 如果显示文件补全建议，处理导航和选择
	if m.showFileCompletion {
		switch msg.String() {
		case "up", "ctrl+p":
			m.selectedFileCompletion--
			if m.selectedFileCompletion < 0 {
				m.selectedFileCompletion = len(m.fileCompletionItems) - 1
			}
			return m, nil
		case "down", "ctrl+n":
			m.selectedFileCompletion++
			if m.selectedFileCompletion >= len(m.fileCompletionItems) {
				m.selectedFileCompletion = 0
			}
			return m, nil
		case "enter", "tab":
			if m.selectedFileCompletion < len(m.fileCompletionItems) {
				selected := m.fileCompletionItems[m.selectedFileCompletion]
				// Delete @fragment and insert the selected path
				endPos := m.input.CursorPos()
				m.input = m.input.DeleteRange(m.fileCompletionAtPos, endPos)
				m.input = m.input.InsertAtCursor(selected + " ")
			}
			m.showFileCompletion = false
			m.fileCompletionItems = nil
			m.selectedFileCompletion = 0
			return m, nil
		case "esc":
			m.showFileCompletion = false
			m.fileCompletionItems = nil
			m.selectedFileCompletion = 0
			return m, nil
		}
		// Fall through for typing more characters
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

	input := m.input.Value()

	// Check for @ file mention trigger
	if m.fileIndexer != nil {
		fragment, atPos, ok := completer.ExtractFragment(input, m.input.CursorPos())
		if ok && !isCompletedFile(m.deps.WorkDir, fragment) {
			paths := m.fileIndexer.Paths(fragment)
			if len(paths) > 0 {
				m.fileCompletionItems = completer.FilterAndRank(fragment, paths, 20)
			} else {
				m.fileCompletionItems = nil
			}
			m.fileCompletionAtPos = atPos
			m.showFileCompletion = len(m.fileCompletionItems) > 0
			m.selectedFileCompletion = 0
			m.showCommandSuggestions = false
			m.selectedSuggestion = 0
			return m, cmd
		}
		m.showFileCompletion = false
		m.fileCompletionItems = nil
		m.selectedFileCompletion = 0
	}

	// Check for slash command suggestions
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

	// 在启动新一轮执行前，先把上一轮残留的 UI 状态落定，
	// 并切换到新的 wire，避免旧 run 的迟到事件污染当前 run。
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m = m.prepareRuntimeExecution()

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
	m.resetFileCompletion()
	m.mode = ModeThinking

	// 启动 runtime 执行
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
		m.startRuntimeExecution(m.deps.Store, text),
	)
}

func (m Model) handleRuntimeEvents(msg RuntimeEventsMsg) (tea.Model, tea.Cmd) {
	// 如果 runtime 已完成，仍处理事件以消费残留的 wire 消息，
	// 但不改变 mode，避免把已完成的界面重新拉回 spinning 状态。
	wasIdle := m.mode == ModeIdle

	for _, event := range msg.Events {
		if wasIdle {
			if statusUpdate, ok := event.(runtimeevents.StatusUpdate); ok {
				m.runtime.ContextUsage = statusUpdate.Status.ContextUsage
			}
			if m.commitLateRuntimeEvents {
				m.output = m.output.AppendCommittedRuntimeEvent(event)
			}
			continue
		}

		if stepBegin, ok := event.(runtimeevents.StepBegin); ok && stepBegin.Number > 1 {
			m.output = m.output.FlushPending()
		}
		m.runtime = m.runtime.ApplyEvent(event)
		m.mode = ModeStreaming
		m.output = m.output.SetPending(m.runtime.ToLines())
	}

	if wasIdle {
		// 不重开 wireReceiveLoop（runtime 已关闭）
		return m, nil
	}
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
	)
}
func (m Model) finishRuntime(msg RuntimeCompleteMsg) Model {
	commitLateEvents := m.activeShellActionCommand == ""
	if m.wire != nil {
		m.wire.Shutdown()
	}
	m.mode = ModeIdle
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	if msg.Err != nil {
		m.commitLateRuntimeEvents = commitLateEvents
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
		commitLateEvents = false
		m = m.finishCompactRuntime(msg)
	} else if m.activeShellActionCommand == "/init" {
		commitLateEvents = false
		m = m.finishInitRuntime()
	}
	m.activeShellActionCommand = ""
	m.commitLateRuntimeEvents = commitLateEvents

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
	if m.mode == ModeIdle {
		return ""
	}

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
