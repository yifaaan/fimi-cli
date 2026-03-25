// Package shell provides an interactive bubbletea-based shell UI for fimi-cli.
package shell

import (
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	_ tea.Msg = promptInputMsg{}
	// 这里用一个“编译期引用”把 lipgloss 固定在模块依赖里：否则 `go mod tidy` 会移除它。
	_ = lipgloss.NewStyle
)

// promptInputMsg is sent when user submits a prompt.
type promptInputMsg struct {
	text string
}

// runtimeEventMsg wraps a runtime event for bubbletea consumption.
type runtimeEventMsg struct {
	event runtimeevents.Event
}

// runtimeDoneMsg is sent when runtime.Run completes.
type runtimeDoneMsg struct {
	result runtime.Result
	err    error
}

// interruptMsg is sent on Ctrl+C or SIGINT.
type interruptMsg struct{}

// metaCommandMsg is sent when a meta command is parsed.
type metaCommandMsg struct {
	name string
}
