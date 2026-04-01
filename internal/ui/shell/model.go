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

// Mode defines the current UI mode.
type Mode int

const (
	// ModeIdle waits for user input.
	ModeIdle Mode = iota
	// ModeThinking shows the working spinner while the runtime is busy.
	ModeThinking
	// ModeStreaming shows the streaming response state.
	ModeStreaming
	// ModeSessionSelect shows the session picker.
	ModeSessionSelect
	// ModeCheckpointSelect shows the checkpoint picker.
	ModeCheckpointSelect
	// ModeSetup runs the interactive setup wizard.
	ModeSetup
	// ModeApprovalPrompt waits for an approval decision.
	ModeApprovalPrompt
)

// Model is the root Bubble Tea model.
// It composes the input, output, and runtime submodels.
type Model struct {
	// Submodels.
	input   InputModel
	output  OutputModel
	runtime RuntimeModel
	toasts  ToastModel

	// Shared state.
	width  int
	height int
	mode   Mode
	err    error

	// Startup banner visibility.
	showBanner bool

	// Dependencies.
	deps    Dependencies
	history *historyStore

	// Active shell action command, used for post-run handling.
	activeShellActionCommand string

	// Temporary history file used by /init.
	initTempFile string

	// Session selection state.
	sessionList         []session.SessionInfo
	selectedSession     int
	sessionScrollOffset int // 濠电姷鏁告慨鎾晝閵夆晜鍤岄柣鎰靛墯閸欏繘鏌嶉崫鍕櫣缁炬儳顭烽悡顐﹀炊閵婏箓鏆遍梺鍐叉惈閹冲繘鎮?
	// Checkpoint selection state.
	checkpointList         []contextstore.CheckpointRecord
	selectedCheckpoint     int
	checkpointScrollOffset int

	// Command suggestion state.
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
	runtimeStartedAt  time.Time
}

// CommandInfo describes an available command.
type CommandInfo struct {
	Name        string
	Description string
}

// availableCommands returns the supported shell commands.
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

// NewModel creates a new Bubble Tea model.
func NewModel(deps Dependencies, history *historyStore) Model {
	// 婵犵數濮烽。浠嬪焵椤掆偓閸熷潡鍩€椤掆偓缂嶅﹪骞冨Ο璇茬窞闁归偊鍓欓悵妯荤節閵忥絾纭鹃柨鏇樺劦瀹曟洖鈻庨幘瀵稿幈闂佸搫鍊介褎淇婇崫鍕勫酣宕惰闊剚顨ラ悙鎵獢妞ゃ垺鐩俊鍫曞炊閳哄﹥啸闂傚倷绀侀幖顐も偓姘煎墯閺呰埖绂掔€ｎ€附鎱ㄥ鍡楀姦闁绘梻鈷堥弫鍌炴煕閺囥劌骞橀柡?
	showBanner := deps.StartupInfo != (StartupInfo{})

	// Create wire for runtime communication
	w := wire.New(0)

	output := NewOutputModel()
	for _, block := range transcriptLineModelsFromRecords(deps.InitialRecords) {
		output = output.AppendBlock(block)
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

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
	)
}

// Update implements tea.Model.
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
		// Handle session selection mode separately.
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
		m.runtime = m.runtime.Advance(time.Now())
		m.output = m.output.SetPending(m.mergeInitialPendingBlocks(m.runtime.ToBlocks()))
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// 闂傚倷绀侀幖顐⒚洪妶澶嬪仱闁靛ň鏅涢拑鐔封攽閻樻彃顏ら柛瀣尭閳藉鈻庤箛鎿冩綌闂佺厧鐏堥弲鐘诲蓟濞戙垹惟闁靛鍠栭崜鍐测攽閻愯尙澧涢柛銊潐缁傚秹骞栨担闀愮炊闂佸憡娲熷褍鈻?
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
		// 缂傚倸鍊搁崐椋庣礊閳ь剟鏌涘☉鍗炵仭闁哄棔鍗抽弻鈩冨緞閸℃ɑ鐝曢梺鍛婃⒐閿曘垽濡?wireReceiveLoop 闂傚倷绀侀幖顐λ囬銏犵？闁肩⒈鍓涢惌鎾愁渻鐎ｎ亜顒㈤悗姘煼閹嘲鈻庤箛鎿冧紑闂佸搫妫欓悷鈺呭蓟閿濆鏅查柛鈩冪懄閸ｎ喖鈹?wire 婵犵數鍋涢悺銊у垝瀹€鍕垫晞闁告洦鍋€閺?
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
			m.runtime = m.runtime.ApplyApprovalRequest(msg.Request, 0)
			m.output = m.output.SetPending(m.mergeInitialPendingBlocks(m.runtime.ToBlocks()))
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

// View renders the full UI.
func (m Model) View() string {
	// 婵犵數濮烽。浠嬪焵椤掆偓閸熷潡鍩€椤掆偓缂嶅﹪骞冨Ο璇茬窞闁归偊鍓氬畵?session 闂傚倸鍊风欢锟犲磻閸曨垁鍥箥椤旂懓浜炬慨妯稿劚婵″ジ鎽堕敐澶嬬厓闁靛鍎抽敍宥囩棯椤撴稑浜鹃梻鍌欑劍鐎笛呯矙閹达箑瀚夋い鎺戝€归崯娲煕閳╁啰鈽夐柦鍐枛閺岀喖宕滆閸旓箓鏌涚€ｎ偅灏扮€垫澘瀚埀顒婄秵娴滆埖绂掗幘顔解拺濞村吋鐟ч崚浼存煟椤撶偛鈧悂鈥?
	if m.mode == ModeSessionSelect {
		return m.renderSessionSelectView()
	}
	if m.mode == ModeCheckpointSelect {
		return m.renderCheckpointSelectView()
	}
	if m.mode == ModeSetup {
		return m.renderSetupView()
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

	if toastsView := m.toasts.View(); toastsView != "" {
		trailingSections = append(trailingSections, toastsView)
	}

	if liveStatus := m.renderLiveStatus(); liveStatus != "" {
		trailingSections = append(trailingSections, liveStatus)
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

	return leadingSections, trailingSections
}

// renderBanner renders the startup banner.
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
	return output.View()
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
	return m, nil
}

func joinTranscriptForTeaPrint(rendered []string) string {
	if len(rendered) == 0 {
		return ""
	}
	return strings.Join(rendered, "\n")
}

// handleKeyPress processes keyboard input.
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeApprovalPrompt {
		return m.handleApprovalKeyPress(msg)
	}

	// Global shortcuts.
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		// When busy, reject all pending approvals before quitting.
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
		// 濠电姷鏁搁崑鐐哄箰閹间礁绠犻幖娣妽閸?
		m.output = m.output.Clear()
		m.commitLateRuntimeEvents = false
		return m, nil

	case "ctrl+o":
		// Toggle the last collapsible tool result.
		var toggled bool
		m.output, toggled = m.output.ToggleExpand()
		_ = toggled // Refresh even if nothing collapsible changed.
		return m, nil
	}

	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m, nil
	}

	// 婵犵數濮烽。浠嬪焵椤掆偓閸熷潡鍩€椤掆偓缂嶅﹪骞冨Ο璇茬窞濠电姴瀛╁銊╂⒑閹稿海绠撻柟鍐茬箻閻擃剟顢楅埀顒勨€︾捄銊﹀磯闁告繂瀚锋导鈧梻浣筋嚃閸犳牠鏁冮鍫濇瀬鐎广儱娲ｅ▽顏堟煠閹帒鍔氭い蹇撶埣濮婄儤瀵煎▎鎴犮偡闂佺閰ｆ禍鑸典繆閻㈢鐐婃い鎺嶈兌閸旀悂姊洪棃娑辩劸闁稿酣浜堕幃妤佺節濮橆剛楠囬梺鍓插亞閸犳劗浜搁銏＄厱?
	if m.mode != ModeIdle {
		return m, nil
	}

	// 婵犵數濮烽。浠嬪焵椤掆偓閸熷潡鍩€椤掆偓缂嶅﹪骞冨Ο璇茬窞闁归偊鍓欏鐑芥⒑閸涘﹥瀵欏ù锝夘棑妞规娊姊绘担鍛婃儓闁稿﹦顭堢叅闁挎繂鎳夐弸搴ㄦ煏韫囧鐏柛瀣耿閺屾洘寰勯崼婵嗗閻庢鍠涘▔鏇㈠焵椤掑倸浠柛濠冪箘缁辨挸顫濇０婵囨櫓闂佸憡绋戦悺銊╁疾椤掑嫭鍊堕柣鎰暩閹藉倹淇婇弻銉ゆ喚闁哄矉缍侀幃娆撳矗婢舵ɑ顥ｆ俊鐐€栧ú鐔哥閸洖绠犳繝濠傜墱閺佸倿鏌涢弴銊ュ妞ゅ浚鍨跺缁樼瑹閸パ傜盎濡炪倧闄勬竟鍡涘焵?
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

	// 婵犵數濮烽。浠嬪焵椤掆偓閸熷潡鍩€椤掆偓缂嶅﹪骞冨Ο璇茬窞闁归偊鍓欏鐑芥⒑閸涘﹥瀵欏ù锝夘棑妞规娊姊绘担鍛婂暈缂佸甯″畷鐟扳攽鐎ｎ亞顔呴梺闈涚墕閹峰宕甸崼婢棃鏁傜粵瀣妼闂佸摜鍋為幐鎶藉蓟閵娿儮妲堟俊顖氱仢椤忣厼顪冮妶蹇氬悅闁哄懐濞€楠炲啴宕奸弴鐐茶€垮┑掳鍊曢敃銈呪枔婵傚憡鈷戦柣鎾抽椤ュ繐霉濠婂嫮鐭掓鐐茬箲鐎佃偐鈧稒顭囬崝鍨節閵忥絾纭炬い鎴濇嚇椤?
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
				// Accept the selected suggestion.
				if m.selectedSuggestion < len(filtered) {
					cmd := filtered[m.selectedSuggestion]
					m.input = m.input.SetValue(cmd.Name + " ")
					m.showCommandSuggestions = false
					m.selectedSuggestion = 0
				}
				return m, nil
			case "esc":
				// 闂傚倷鑳堕…鍫㈡崲閹寸偟绠惧┑鐘蹭迹濞戙垹绾ч柟鎼幖濞堟ɑ淇婇妶蹇涙妞ゆ垶鐟╁畷?
				m.showCommandSuggestions = false
				m.selectedSuggestion = 0
				return m, nil
			}
		}
	}

	// Forward input to the input model.
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

// filteredCommands returns commands filtered by the current input.
func (m Model) filteredCommands() []CommandInfo {
	input := m.input.Value()
	if !strings.HasPrefix(input, "/") {
		return availableCommands()
	}

	// Filter matching commands.
	var filtered []CommandInfo
	for _, cmd := range availableCommands() {
		if strings.HasPrefix(cmd.Name, input) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

// renderCommandSuggestions renders the inline command suggestion dropdown.
func (m Model) renderCommandSuggestions() string {
	filtered := m.filteredCommands()
	if len(filtered) == 0 {
		return ""
	}

	// Dropdown styles.
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
	maxDisplay := 5 // Show up to five suggestions.
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

// handleSubmit processes a submitted prompt.
func (m Model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}

	// 婵犵數濮伴崹鐓庘枖濞戞埃鍋撳鐓庢珝妤?slash 闂傚倷绀侀幉锛勭矙閹烘鍨傛繝闈涱儏缁?
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}

	// 闂傚倷绶氬鑽ゆ嫻閻旂厧绀夌€光偓閸曨偆鐣鹃柟鍏肩暘閸斿瞼绮堟径鎰厪濠电姴绻樺顕€鏌ｉ幘顖氫壕婵犵數鍋為崹鍫曞箰閹绢喖纾婚柟鎹愵嚙缁狙囨煟閹存繃顥滈柡瀣枎椤儻顦撮柡鍜佸亰楠炲啴骞嶉缁樺媰闂佽姤锚椤︿粙寮搁埀顒勬⒒娴ｇ懓顕滅紒瀣灴閹崇喖顢涢悙鎻掑亶濠电偞鍨崹鍦不椤斿皷鏀介柣妯跨簿閸忓矂鏌ｉ悢绋垮闁宠棄顦甸獮姗€顢涘顐㈩棜闂備礁鎼ˇ閬嶅磿閹版澘绐楁俊銈呮噺閸嬧晜銇勯幇鍫曟闁稿鏅滅换娑㈠幢濡櫣浠搁梺?UI 闂傚倷鑳剁划顖炩€﹂崼銉ユ槬闁哄稁鍘奸悞鍨亜閹寸偛顕滅紒浣峰嵆閺岀喖顢涘Ο琛″亾濡ゅ懎鐒垫い鎺戝枤濞兼劙鏌ｉ弮鎴濆婵?
	// Flush prior pending UI state before starting a new run.
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m = m.prepareRuntimeExecution()

	// 婵犵數鍎戠徊钘壝洪敂鐐床闁稿瞼鍋為崑銈夋煏婵炵偓娅呴柛灞诲姂閺屾盯鍩勯崘鐐暥闂?
	if m.history != nil {
		_ = m.history.Append(text)
	}
	m.input.AppendHistory(text)

	// 濠电姷鏁搁崑鐐哄箰閹间礁绠犻柟鐗堟緲閻撴﹢鏌″鍐ㄥ缂傚秴娲弻鐔煎箚瑜嶉弳杈┾偓娈垮枦濞呮洟銆冮妷鈺傚€烽柡澶嬪灥婵摜绱撴担铏瑰笡闁挎洏鍨归悾?thinking 濠电姷顣藉Σ鍛村垂椤忓牆鐒垫い鎺嗗亾缁剧虎鍘惧☉鐢稿焵?
	m.input = m.input.Clear()
	m.resetFileCompletion()
	m.mode = ModeThinking
	m.runtimeStartedAt = time.Now()

	// 闂?user prompt 濠电姷鏁搁崕鎴犵礊閳ь剚銇勯弴鍡楀閸欏繘鏌ｉ幇顒佹儓缂佲偓?pending blocks闂傚倷鐒︾€笛呯矙閹达附鍎楀ù锝囧劋瀹曟煡鏌″搴″箹闁告劏鍋撻柣搴ｆ嚀鐎氼厽绔熼崱娑樻辈濠电姴浼呰ぐ鎺戠闁艰婢橀ˉ婵嗏攽閻橆偄浜?runtime events 婵犵數鍋為崹鍫曞蓟閵娾晩鏁勯柛娑卞枟濞呯娀鏌ｅΟ娆惧殭闁告瑥锕弻娑㈠箻濡炵偓顦风紒?
	m.output = m.output.SetPending([]TranscriptBlock{
		{
			Kind:     BlockKindUserPrompt,
			UserText: text,
		},
	})

	// 闂傚倷绀侀幉锟犲礄瑜版帒鍨傞柣妤€鐗婇崣?runtime 闂傚倷绀佸﹢閬嶆偡閹惰棄骞㈤柍鍝勫€归弶?
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
		m.startRuntimeExecution(m.deps.Store, text),
	)
}

func (m Model) handleRuntimeEvents(msg RuntimeEventsMsg) (tea.Model, tea.Cmd) {
	// Continue consuming wire events after runtime completion without re-entering streaming mode.
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

		// Keep initial blocks from pending output.
		runtimeBlocks := m.runtime.ToBlocks()
		initialBlocks := m.collectInitialPendingBlocks()
		if len(initialBlocks) > 0 {
			// 闂備浇顕х换鎰崲閹邦儵娑樜旈崨顓炵€梺鍓茬厛閸嬪懘宕?blocks 闂傚倷娴囬妴鈧柛瀣崌閺岀喖顢涢崱妤€顏柛姘煎亰濮婃椽宕崟顓烆暤闂佺顑嗛幑鍥蓟濞戙垹绠荤€规洖娉﹁缁?
			allBlocks := make([]TranscriptBlock, 0, len(initialBlocks)+len(runtimeBlocks))
			allBlocks = append(allBlocks, initialBlocks...)
			allBlocks = append(allBlocks, runtimeBlocks...)
			m.output = m.output.SetPending(allBlocks)
		} else {
			m.output = m.output.SetPending(runtimeBlocks)
		}
	}

	if wasIdle {
		// 婵犵數鍋為崹鍫曞箰閸濄儳鐭撶憸鐗堝笒闂傤垳绱掔€ｎ偒鍎ラ柛銈嗘礋閺?wireReceiveLoop闂傚倷鐒︾€笛呯矙閹存繍鐔嗗☉鏃傚崟time 闂佽娴烽幊鎾诲箟闄囬妵鎰板礃椤旇偐鐓戦悷婊勬煥椤繑绻濆鍏兼櫍闂侀潧臎閸滀焦孝
		return m, nil
	}
	return m, tea.Batch(
		m.runtime.SpinnerCmd(),
		m.wireReceiveLoop(),
	)
}

// collectInitialPendingBlocks collects initial pending blocks.
func (m Model) collectInitialPendingBlocks() []TranscriptBlock {
	var initialBlocks []TranscriptBlock
	for _, block := range m.output.pending {
		if block.Kind == BlockKindSystemNotice || block.Kind == BlockKindUserPrompt {
			initialBlocks = append(initialBlocks, block)
		} else {
			// Stop once pending blocks move beyond the initial system/user prefix.
			break
		}
	}
	return initialBlocks
}

func (m Model) mergeInitialPendingBlocks(runtimeBlocks []TranscriptBlock) []TranscriptBlock {
	initialBlocks := m.collectInitialPendingBlocks()
	if len(initialBlocks) == 0 {
		return runtimeBlocks
	}
	allBlocks := make([]TranscriptBlock, 0, len(initialBlocks)+len(runtimeBlocks))
	allBlocks = append(allBlocks, initialBlocks...)
	allBlocks = append(allBlocks, runtimeBlocks...)
	return allBlocks
}

func (m Model) finishRuntime(msg RuntimeCompleteMsg) Model {
	commitLateEvents := m.activeShellActionCommand == ""
	if m.wire != nil {
		m.wire.Shutdown()
	}
	m.mode = ModeIdle
	m.output = m.output.FlushPending()
	m.runtime = m.runtime.Reset()
	m.runtimeStartedAt = time.Time{}
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
	for _, block := range transcriptLineModelsFromRecords(records) {
		m.output = m.output.AppendBlock(block)
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

	// Inject the generated AGENTS.md content into the conversation context.
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
		return "Ctrl+O collapse"
	}
	return "Ctrl+O expand"
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
