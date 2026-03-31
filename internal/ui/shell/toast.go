package shell

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"fimi-cli/internal/ui/shell/styles"
)

// ToastLevel represents the severity/type of a toast notification.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastWarning
	ToastError
	ToastSuccess
)

// Toast is a single notification item.
type Toast struct {
	ID        int64
	Level     ToastLevel
	Message   string
	Detail    string
	Action    string
	CreatedAt time.Time
	TTL       time.Duration
}

// Bubble Tea messages.
type ToastAddMsg struct{ Toast Toast }
type ToastDismissMsg struct{ ID int64 }
type ToastTickMsg struct{ Time time.Time }

// ToastModel manages a stack of transient toast notifications.
type ToastModel struct {
	toasts   []Toast
	maxStack int
	nextID   int64
	width    int
}

func NewToastModel() ToastModel {
	return ToastModel{
		maxStack: 5,
	}
}

func (m ToastModel) SetWidth(w int) ToastModel {
	m.width = w
	return m
}

func (m ToastModel) Update(msg tea.Msg) (ToastModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ToastAddMsg:
		t := msg.Toast
		t.ID = m.nextID
		m.nextID++
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		if t.TTL == 0 {
			t.TTL = 5 * time.Second
		}
		m.toasts = append(m.toasts, t)
		if len(m.toasts) > m.maxStack {
			m.toasts = m.toasts[len(m.toasts)-m.maxStack:]
		}
		return m, scheduleToastTick()

	case ToastDismissMsg:
		for i, t := range m.toasts {
			if t.ID == msg.ID {
				m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
				break
			}
		}
		if len(m.toasts) > 0 {
			return m, scheduleToastTick()
		}
		return m, nil

	case ToastTickMsg:
		now := time.Now()
		remaining := m.toasts[:0]
		for _, t := range m.toasts {
			if now.Sub(t.CreatedAt) < t.TTL {
				remaining = append(remaining, t)
			}
		}
		m.toasts = remaining
		if len(m.toasts) > 0 {
			return m, scheduleToastTick()
		}
		return m, nil
	}
	return m, nil
}

func (m ToastModel) View() string {
	if len(m.toasts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range m.toasts {
		style := toastStyle(t.Level)
		icon := toastIcon(t.Level)
		line := fmt.Sprintf(" %s %s", icon, t.Message)
		if t.Detail != "" {
			line += "  " + t.Detail
		}
		if t.Action != "" {
			line += fmt.Sprintf("  [%s]", t.Action)
		}
		if m.width > 0 {
			b.WriteString(style.Width(m.width).Render(line))
		} else {
			b.WriteString(style.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func scheduleToastTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return ToastTickMsg{Time: t}
	})
}

func toastStyle(level ToastLevel) lipgloss.Style {
	switch level {
	case ToastInfo:
		return styles.ToastInfoStyle
	case ToastWarning:
		return styles.ToastWarningStyle
	case ToastError:
		return styles.ToastErrorStyle
	case ToastSuccess:
		return styles.ToastSuccessStyle
	default:
		return styles.ToastInfoStyle
	}
}

func toastIcon(level ToastLevel) string {
	switch level {
	case ToastInfo:
		return "ℹ"
	case ToastWarning:
		return "⚠"
	case ToastError:
		return "✗"
	case ToastSuccess:
		return "✓"
	default:
		return "ℹ"
	}
}

func parseToastLevel(s string) ToastLevel {
	switch s {
	case "warning":
		return ToastWarning
	case "error":
		return ToastError
	case "success":
		return ToastSuccess
	default:
		return ToastInfo
	}
}
