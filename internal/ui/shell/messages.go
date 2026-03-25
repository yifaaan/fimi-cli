package shell

import (
	"time"

	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"github.com/charmbracelet/bubbletea"
)

// RuntimeEventMsg 包装一个 runtime 事件，使其成为 Bubble Tea 消息。
// 这允许将现有的 channel-based 事件系统桥接到 Bubble Tea 的消息循环。
type RuntimeEventMsg struct {
	Event runtimeevents.Event
}

// InputSubmitMsg 表示用户提交了一个 prompt。
type InputSubmitMsg struct {
	Text string
}

// RuntimeCompleteMsg 表示 runtime 完成了处理。
type RuntimeCompleteMsg struct {
	Result runtime.Result
	Err    error
}

// TickMsg 用于 spinner 动画的定时消息。
type TickMsg struct {
	Time time.Time
}

// ResizeMsg 包装终端尺寸变化。
type ResizeMsg struct {
	Width  int
	Height int
}

// ErrorMsg 表示发生了一个错误。
type ErrorMsg struct {
	Err error
}

// ClearMsg 表示用户请求清屏。
type ClearMsg struct{}

// QuitMsg 表示用户请求退出。
type QuitMsg struct{}

// waitForRuntimeEvents 返回一个 Bubble Tea 命令，
// 该命令会阻塞直到从 channel 收到一个事件。
// 这是连接 runtime 事件系统和 Bubble Tea 消息系统的关键桥梁。
func waitForRuntimeEvents(ch <-chan runtimeevents.Event) tea.Cmd {
	return func() tea.Msg {
		if event, ok := <-ch; ok {
			return RuntimeEventMsg{Event: event}
		}
		// Channel 已关闭，返回 nil 表示无更多消息
		return nil
	}
}
