package shell

import (
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"

	"github.com/charmbracelet/bubbletea"
)

const runtimeEventBatchSize = 64

// RuntimeEventMsg 包装一个 runtime 事件，使其成为 Bubble Tea 消息。
// 这允许将现有的 channel-based 事件系统桥接到 Bubble Tea 的消息循环。
type RuntimeEventMsg struct {
	Event runtimeevents.Event
}

// RuntimeEventsMsg 批量传递 runtime 事件，并显式标记事件流是否已关闭。
// 这样可以降低高频流式输出导致的 UI 重绘压力，并确保尾部事件先于完成态被消费。
type RuntimeEventsMsg struct {
	Events []runtimeevents.Event
	Closed bool
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

// ResumeListMsg 表示 session 列表查询结果。
type ResumeListMsg struct {
	Sessions []session.SessionInfo
	Err      error
}

// ResumeSwitchMsg 表示 session 切换结果。
type ResumeSwitchMsg struct {
	Session session.Session
	Records []contextstore.TextRecord
	Err     error
}

// ClearMsg 表示用户请求清屏。
type ClearMsg struct{}

// waitForRuntimeEvents 返回一个 Bubble Tea 命令。
// 它会阻塞等待首个事件，然后尽可能多地批量提取后续已缓冲事件，
// 避免在流式输出时为每个 token 触发一次完整 UI 更新。
func waitForRuntimeEvents(ch <-chan runtimeevents.Event) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return RuntimeEventsMsg{Closed: true}
		}

		events := make([]runtimeevents.Event, 0, runtimeEventBatchSize)
		events = append(events, first)

		for len(events) < runtimeEventBatchSize {
			select {
			case event, ok := <-ch:
				if !ok {
					return RuntimeEventsMsg{
						Events: events,
						Closed: true,
					}
				}
				events = append(events, event)
			default:
				return RuntimeEventsMsg{Events: events}
			}
		}

		return RuntimeEventsMsg{Events: events}
	}
}
