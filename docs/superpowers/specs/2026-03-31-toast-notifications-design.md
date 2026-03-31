# Toast Notifications Design

Date: 2026-03-31

## Overview

Add a transient, stackable toast notification system to the Bubble Tea shell UI. Toasts originate from the runtime via the Wire channel and display above the input area with auto-dismiss and optional interaction.

## Decisions

| Decision | Choice |
|---|---|
| Trigger source | Runtime via Wire (not UI-local) |
| Types | Info, Warning, Error, Success |
| Behavior | Auto-dismiss (5s TTL) + interactive (click dismiss / action button) |
| Stack limit | 5 toasts max; oldest dropped when exceeded |
| Position | Inline row above input area |
| Wire channel | Reuse `EventMessage` with new `EventToast` event type |
| Architecture | `ToastModel` sub-model (parallel to InputModel/OutputModel/RuntimeModel) |

## Data Types

File: `internal/ui/shell/toast.go`

```go
type ToastLevel int

const (
    ToastInfo    ToastLevel = iota
    ToastWarning
    ToastError
    ToastSuccess
)

type Toast struct {
    ID        int64
    Level     ToastLevel
    Message   string
    Detail    string        // optional subtitle
    Action    string        // optional action label (e.g. "Restart")
    CreatedAt time.Time
    TTL       time.Duration // default 5s
}

// Bubble Tea messages
type ToastAddMsg    struct { Toast Toast }
type ToastDismissMsg struct { ID int64 }
type ToastTickMsg   struct { time.Time }
```

## ToastModel

```go
type ToastModel struct {
    toasts   []Toast
    maxStack int       // default 5
    nextID   int64
    width    int
}
```

### Update

- **ToastAddMsg**: assign monotonic ID, set default TTL (5s), append to slice, trim to `maxStack`, start tick timer.
- **ToastDismissMsg**: remove toast by ID.
- **ToastTickMsg** (every 500ms): remove toasts where `now - CreatedAt >= TTL`. If any remain, reschedule tick; otherwise stop.

```go
func (m *ToastModel) scheduleTick() tea.Cmd {
    return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
        return ToastTickMsg{t}
    })
}
```

### View

Renders each toast as a styled line with level-specific icon and color. No output when empty.

| Level | Icon | Border color |
|---|---|---|
| Info | `ℹ` | Blue |
| Warning | `⚠` | Yellow |
| Error | `✗` | Red |
| Success | `✓` | Green |

Line format: ` <icon> <Message>  <Detail>  [<Action>]`

Each toast line is rendered via lipgloss with the level's border color, padded to full width.

## Wire Integration

### New Event Type

File: `internal/runtime/events.go`

```go
type EventToast struct {
    Level   string `json:"level"`            // "info"|"warning"|"error"|"success"
    Message string `json:"message"`
    Detail  string `json:"detail,omitempty"`
    Action  string `json:"action,omitempty"`
}
```

### Wire Receive Loop

In `model_runtime.go` `wireReceiveLoop()`, add a case to the event type switch:

```go
case runtimeevents.EventToast:
    return ToastAddMsg{Toast: toastFromEvent(e)}
```

Where `toastFromEvent` converts the wire event to a `Toast` with `time.Now()` and default TTL.

## Main Model Integration

Three changes to `model.go`:

1. **Struct**: add `toasts ToastModel` field to `Model`.
2. **Update**: add cases for `ToastAddMsg`, `ToastDismissMsg`, `ToastTickMsg` that delegate to `m.toasts.Update(msg)`.
3. **View**: insert `m.toasts.View()` between `m.output.View()` and `m.input.View()` in the vertical layout.

## Files to Change

| File | Change |
|---|---|
| `internal/ui/shell/toast.go` | New file: ToastModel, Toast type, messages |
| `internal/runtime/events.go` | Add EventToast event type |
| `internal/ui/shell/model.go` | Add toasts field, Update cases, View insertion |
| `internal/ui/shell/model_runtime.go` | Add EventToast case in wireReceiveLoop |
| `internal/ui/shell/styles/lipgloss.go` | Add toast level styles |

## Runtime Usage (Future)

Runtime sends toasts through the wire:

```go
wire.Send(wire.EventMessage{Event: runtimeevents.EventToast{
    Level:   "success",
    Message: "Update downloaded",
    Detail:  "v0.36",
    Action:  "Restart",
}})
```

This pattern will be used by auto-update (Phase 15) and other runtime status notifications.
