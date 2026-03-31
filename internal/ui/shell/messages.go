package shell

import (
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
)

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

// CheckpointListMsg 表示当前 session 的 checkpoint 列表查询结果。
type CheckpointListMsg struct {
	Checkpoints []contextstore.CheckpointRecord
	Err         error
}
// SessionDeleteMsg 表示 session 删除结果。
type SessionDeleteMsg struct {
	SessionID string
	Err       error
}

// ClearMsg 表示用户请求清屏。
type ClearMsg struct{}

// FileIndexResultMsg delivers asynchronously-indexed file paths.
type FileIndexResultMsg struct {
	Paths []string
}
