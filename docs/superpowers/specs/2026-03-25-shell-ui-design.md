# Shell UI Design (Phase 8)

Date: 2026-03-25
Status: Approved

## Scope

Build a minimal interactive shell UI using bubbletea (Charm ecosystem).

**In scope:**
- Interactive prompt loop
- Streaming output (reuse Phase 7 streaming)
- Basic meta commands: `/exit`, `/help`
- Graceful Ctrl+C handling (clear prompt or cancel run)
- Flag-based mode switch: `--shell` or `-i`

**Out of scope (future phases):**
- Input history persistence
- Auto-completion (meta commands, file mentions)
- Rich live view with spinners
- Bottom toolbar
- Additional meta commands (`/clear`, `/status`)

## Architecture

### Mode Switch

```
cmd/fimi/main.go
    │
    ▼
internal/app
    │
    ├── no --shell ──► runSingleShot() ──► ui.Run() ──► printui
    │
    └── --shell ──► runShell() ──► shell.Run() ──► bubbletea
```

### Package Structure

```
internal/ui/shell/
├── shell.go       # Run() entry point, signal handling
├── model.go       # Model struct and tea.Model implementation
├── update.go      # Update() logic (prompt, runtime, meta commands)
├── view.go        # View() rendering
├── msgs.go        # Message types
├── metacmd.go     # Meta command registry (/exit, /help)
├── sink.go        # NewEventSink adapter
└── shell_test.go  # Unit tests
```

### Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│ shell.Run()                                                  │
│   1. Create event channel                                    │
│   2. Create SinkFunc that sends to channel                   │
│   3. Wire SinkFunc into runtime.Runner                       │
│   4. Start bubbletea program                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ bubbletea event loop                                         │
│                                                              │
│   user types ──► Update(msg) ──► View()                     │
│        │                                                     │
│        ▼                                                     │
│   promptInputMsg ──► start goroutine:                        │
│                          runner.Run(ctx, store, input)       │
│                              │                               │
│                              ▼                               │
│                          sink.Emit(event)                    │
│                              │                               │
│                              ▼                               │
│                          channel ──► runtimeEventMsg         │
│                              │                               │
│                              ▼                               │
│   Update(runtimeEventMsg) ──► append to output buffer       │
│                              ──► View() re-renders           │
└─────────────────────────────────────────────────────────────┘
```

## Components

### Model

```go
type Model struct {
    // Dependencies (injected)
    runner       runtime.Runner
    store        contextstore.Context
    model        string
    systemPrompt string

    // Prompt state
    prompt string

    // Runtime state
    running bool
    result  runtime.Result
    err     error

    // Output buffer
    output strings.Builder

    // UI state
    showHelp bool
}
```

### Messages

```go
type promptInputMsg struct{ text string }
type runtimeEventMsg struct{ event events.Event }
type runtimeDoneMsg struct {
    result runtime.Result
    err    error
}
type interruptMsg struct{}
```

### Meta Commands

| Command | Description | Action |
|---------|-------------|--------|
| `/exit` | Exit the shell | `tea.Quit` |
| `/help` | Show available commands | Set `showHelp=true` |

### Event Sink Adapter

```go
func NewEventSink(events chan<- events.Event) events.Sink {
    return events.SinkFunc(func(ctx context.Context, event events.Event) error {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case events <- event:
            return nil
        }
    })
}
```

### Update Logic

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKey(msg)
    case promptInputMsg:
        return m.handlePrompt(msg)
    case runtimeEventMsg:
        return m.handleRuntimeEvent(msg)
    case runtimeDoneMsg:
        return m.handleRuntimeDone(msg)
    case interruptMsg:
        return m.handleInterrupt()
    }
    return m, nil
}
```

### Cancellation

- **Ctrl+C during prompt**: Clear current input line
- **Ctrl+C during run**: Cancel runtime (ctx.Done), return interrupted status
- **Ctrl+D**: Exit shell immediately

### View

```
[previous output]

● > user prompt here
```

- Green dot (●): Ready for input
- Yellow dot (●): LLM is generating
- Output buffer accumulates streaming text and tool results

## Entry Point Integration

```go
// internal/app/app.go

type runInput struct {
    // ... existing fields ...
    shellMode bool  // NEW
}

func parseRunInput(args []string) (runInput, error) {
    // ... existing parsing ...
    if parseFlags && (arg == "--shell" || arg == "-i") {
        input.shellMode = true
        continue
    }
    // ...
}

func (d dependencies) run(args []string) error {
    input, err := parseRunInput(args)
    // ... setup ...

    if input.shellMode {
        return d.runShell(ctx, runner, store, cfg, agent)
    }
    return d.runSingleShot(...)
}

func (d dependencies) runShell(...) error {
    return shell.Run(ctx, runner, store, model, systemPrompt)
}
```

## Testing Strategy

1. **Unit tests for Model.Update()**
   - Prompt input triggers runtime start
   - Events append to output buffer
   - Meta commands execute correctly

2. **Mock runner**
   - Returns predictable events
   - Simulates streaming delays

3. **Integration test**
   - Full shell.Run() with mock dependencies
   - Verify prompt → run → output cycle

## Dependencies

```go
import (
    "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"  // optional styling
)
```

## Classification

- `internal/ui/shell` — **adapter/integration** (replaceable UI layer)
- `internal/runtime` — **core logic** (stable, unchanged)

## Future Extensions

- History persistence (JSONL file per workdir)
- Tab completion for meta commands
- File mention completion (@path)
- Rich live view with spinners
- Bottom toolbar (context usage, time)
- Additional meta commands
