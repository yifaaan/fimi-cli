# PLAN

## Purpose

This file tracks the migration gap between the Python reference snapshot in `temp/`
and the current Go rewrite.

Updated: 2026-04-01

---

## Reference Baseline In `temp/` (v0.35)

### Runtime Core

| File | Description |
| --- | --- |
| `soul/kimisoul.py` | Main runtime loop: `run() -> _agent_loop() -> _step()` with D-Mail rollback, Wire-based event dispatch, approval piping task |
| `soul/wire.py` | Bidirectional channel (`Wire` class) between soul and UI via `asyncio.Queue[WireMessage]`. ContextVar `current_wire` for implicit access. Carries events, `ApprovalRequest`, and control-flow types (`StepBegin`, `StepInterrupted`, `StatusUpdate`, `ControlFlowEvent`, `Event`). **Note:** No separate `soul/event.py` — all event types live in `wire.py`. |
| `soul/denwarenji.py` | D-Mail state machine (`DenwaRenji`): `_pending_dmail`, `_n_checkpoints`, `send_dmail()` / `fetch_pending_dmail()` |
| `soul/message.py` | Message utilities, tool result conversion (`tool_result_to_messages`, `tool_ok_to_message_content`, `system()` helper for `<system>` tags) |
| `soul/approval.py` | **NEW** Permission gating for tools. `Approval` class with `_yolo` mode, `_auto_approve_actions`, `request()` → `ApprovalRequest` via wire. Must be called from within tool context. |
| `soul/toolset.py` | **NEW** `CustomToolset` wraps `SimpleToolset`, sets `current_tool_call` ContextVar during tool execution. Enables approval system. |
| `acp/session.py` | ACP session with event-to-ACP projection layer |
| `acp/server.py` | Multi-session ACP server with JSON-RPC: `initialize`, `new_session`, `load_session`, `resume_session`, `list_sessions`, `prompt`, `cancel`, `set_session_model`, `authenticate` |
| `ui/__init__.py` | Run coordinator, cancellation boundary (`run_soul()`) |

#### Runtime Loop Detail (`soul/kimisoul.py` v0.35)

```
run(user_input, wire)
  ├── set wire via ContextVar (current_wire.set(wire))
  ├── checkpoint() + append user message
  └── _agent_loop(wire)
        └── while True:
              ├── wire.send(StepBegin(step_no))
              ├── spawn _pipe_approval_to_wire() task (concurrent with step)
              ├── checkpoint() + denwa_renji.set_n_checkpoints()
              ├── _step(wire) with tenacity retry
              │     ├── kosong.step() callbacks use wire.send()
              │     ├── wait for tool results
              │     ├── asyncio.shield(_grow_context())
              │     ├── check ToolRejectedError
              │     ├── check denwa_renji.fetch_pending_dmail() -> BackToTheFuture
              │     └── return not result.tool_calls
              ├── cancel approval_task
              ├── handle BackToTheFuture: revert + checkpoint + append message
              └── continue or return
```

#### Wire System Detail (`soul/wire.py`)

```python
class Wire:
    def __init__(self): self._queue = asyncio.Queue[WireMessage]()
    def send(self, msg): self._queue.put_nowait(msg)
    async def receive(self): return await self._queue.get()
    def shutdown(self): self._queue.shutdown()

current_wire = ContextVar[Wire | None]("current_wire", default=None)

type WireMessage = Event | ApprovalRequest
type Event = ControlFlowEvent | ContentPart | ToolCall | ToolCallPart | ToolResult
```

#### Approval System Detail (`soul/approval.py`)

```python
class Approval:
    _yolo: bool = False
    _auto_approve_actions: set[str]
    _request_queue: asyncio.Queue[ApprovalRequest]
    
    async def request(self, action, description) -> bool:
        if self._yolo or action in self._auto_approve_actions: return True
        request = ApprovalRequest(tool_call_id, action, description)
        await self._request_queue.put(request)
        return await request.wait()  # blocks until UI resolves

class ApprovalRequest:
    def resolve(self, response: ApprovalResponse): self._future.set_result(response)
    async def wait(self) -> bool: return await self._future

class ApprovalResponse(Enum):
    APPROVE = "approve"
    APPROVE_FOR_SESSION = "approve_for_session"  # adds to auto_approve_actions
    REJECT = "reject"
```

### Python Tools (14 tools, excluding test tools)

| Tool | File | Description |
| --- | --- | --- |
| `Bash` | `tools/bash/__init__.py` | Shell commands with 60s default/300s max timeout, line-by-line output streaming, `ToolResultBuilder` (50K chars, 2K/line), no approval gate in tool itself |
| `ReadFile` | `tools/file/read.py` | Read file with offset/limit, 1000-line cap, 100KB cap, 2K-char-per-line truncation, `cat -n` format |
| `WriteFile` | `tools/file/write.py` | Write/append to files within work directory (path sandboxing), parent must exist |
| `Glob` | `tools/file/glob.py` | Glob matching (max 1000 results), rejects `**` prefix, path sandboxing |
| `Grep` | `tools/file/grep.py` | ripgrep wrapper via `ripgrepy` with context/multiline/type filters, no path sandboxing |
| `StrReplaceFile` | `tools/file/replace.py` | `replace_all` support, batch edits (list of Edit), path sandboxing |
| `PatchFile` | `tools/file/patch.py` | Unified diff patches via `patch_ng`, path sandboxing |
| `Think` | `tools/think/__init__.py` | Private reasoning note (no-op) |
| `SetTodoList` | `tools/todo/__init__.py` | Todo list management (replace entire list), renders markdown bullets |
| `Task` | `tools/task/__init__.py` | Foreground subagent with continuation prompt (< 200 chars -> re-run with continuation), fresh Context per subagent |
| `SendDMail` | `tools/dmail/__init__.py` | Time-travel message: calls `denwa_renji.send_dmail()`, returns inverted success signal |
| `SearchWeb` | `tools/web/search.py` | Web search via Moonshot API, 5-20 results, optional content crawling |
| `FetchURL` | `tools/web/fetch.py` | URL fetch with trafilatura text extraction, `ToolResultBuilder` |
| `MCPTool` | `tools/mcp.py` | MCP tool adapter via `fastmcp`: `TextContent`->`TextPart`, `ImageContent`->`ImageURLPart`, `AudioContent`->`AudioURLPart` |

### Python UI

| Component | File | Features |
| --- | --- | --- |
| Print UI | `ui/print/__init__.py` | `text` mode, `stream-json` mode, stdin input, sets `yolo=True` on approval |
| Shell UI | `ui/shell/__init__.py` | Interactive REPL (prompt_toolkit + Rich), dual-mode (agent/shell), Wire-based event consumption, background tasks, toast notifications |
| Live View | `ui/shell/liveview.py` | Rich.Live with 4fps refresh, `_ToolCallDisplay` with streaming args, key-argument subtitle extraction, status text |
| Meta Commands | `ui/shell/metacmd.py` | Registry: `/help`, `/exit`, `/version`, `/release-notes`, `/feedback`, `/init`, `/clear`, `/compact`, `/setup`, `/reload` |
| Prompt | `ui/shell/prompt.py` | `FileMentionCompleter` (fuzzy, 2-tier lazy index, 2s TTL cache, 11-category ignore list), `_render_bottom_toolbar` (time + mode + context usage), prompt history JSONL |
| ACP Server | `ui/acp/__init__.py` | Multi-session JSON-RPC over stdio, `_ToolCallState` with streaming lexer, event projection, approval request handling |
| Setup | `ui/shell/setup.py` | **NEW** First-run setup wizard: select platform → enter API key → select model → save config → reload |
| Update | `ui/shell/update.py` | **NEW** Auto-update: fetch latest version from CDN, download tar.gz, extract to `~/.local/bin/kimi`, background check every 60s |

### Python Config (v0.35)

| Field | Type | Default | Notes |
| --- | --- | --- | --- |
| `default_model` | `str` | `""` | Empty string on first run |
| `models` | `dict[str, LLMModel]` | `{}` | Empty on first run |
| `providers` | `dict[str, LLMProvider]` | `{}` | Empty on first run |
| `loop_control` | `LoopControl` | `max_steps=100, max_retries=3` | |
| `services.moonshot_search` | `MoonshotSearchConfig \| None` | `None` | Optional |

**Key change from v0.32:** Config is empty on first run, requires `/setup` to populate.

### Python Agent Spec

| Field | Description |
| --- | --- |
| `extend` | Inheritance from base spec (`"default"` keyword supported) |
| `name` | Agent name |
| `system_prompt_path` | Path to system prompt template |
| `system_prompt_args` | Template variables (includes `KIMI_NOW`, `KIMI_WORK_DIR`, etc.) |
| `tools` | List of tool specs (`"package:ClassName"` format) |
| `exclude_tools` | Tools to remove from inherited set |
| `subagents` | Named subagent specs with `path` / `description` |

### Python AgentGlobals (v0.35)

```python
class AgentGlobals(NamedTuple):
    config: Config
    llm: LLM | None  # Optional in v0.35
    builtin_args: BuiltinSystemPromptArgs
    denwa_renji: DenwaRenji
    session: Session
    approval: Approval  # NEW in v0.35
```

---

## Current Go Snapshot

Updated: 2026-04-01

### Implemented Core

- Entry chain: `cmd/fimi -> internal/app`
- Config loading with models/providers/web/MCP settings
- Session create / continue / resume / list / delete flow
- JSONL history persistence with sliding window reads
- Checkpoint create / revert / list / backup rotation in `internal/contextstore`
- Multi-step runtime loop with event sink and streaming
- Tool-call execution loop with retry logic, backoff/jitter, and retry status updates
- Token usage persistence
- Cancellation-safe shielding for runtime-owned contextstore writes
- LLM engine boundary with streaming support
- OpenAI-compatible (dual wire: Chat Completions + Responses) and Qwen-compatible providers
- MCP integration via go-sdk (multi-server, tool discovery, tool calling)
- ACP server (JSON-RPC over stdio, initialize/authenticate/session/prompt/cancel)
- Tool subtitle extraction (`internal/runtime/toolsubtitle.go`)
- Output shaping (50K chars total, 2K per line)
- D-Mail integration (`internal/dmail`) with runtime rollback
- Wire system (`internal/wire`) — bidirectional buffered channel between runtime and UI
- Approval system (`internal/approval`) — yolo mode, auto-approve actions, approve-for-session
- Tool approval gates on bash, write_file, replace_file

### Go Tools (13 registered)

| Tool | Kind | Handler | Description |
| --- | --- | --- | --- |
| `agent` | agent | yes | Subagent delegation with continuation prompt (auto-re-prompts if response < 200 chars), max-steps detection |
| `think` | utility | yes | Private reasoning note |
| `set_todo_list` | utility | yes | Todo list management |
| `bash` | command | yes | Shell commands with 120s default/300s max timeout, streaming output, **background mode** (`background:true`, `task_id`) via `BackgroundManager` (24h timeout) |
| `search_web` | utility | yes | DuckDuckGo HTML scraper |
| `fetch_url` | utility | yes | HTTP fetch with heuristic content extraction + scoring |
| `read_file` | file | yes | Read file with offset/limit |
| `glob` | file | yes | Glob matching (supports `**`), path sandboxing |
| `grep` | file | yes | Regex grep with line numbers |
| `write_file` | file | yes | Write file with parent dir creation, path sandboxing |
| `replace_file` | file | yes | String replace with `replace_all` support, batch edits |
| `patch_file` | file | yes | Unified diff patch application, path sandboxing |
| `send_dmail` | utility | yes | D-Mail time-travel message, triggers runtime rollback |

### Go UI

| Component | Location | Features |
| --- | --- | --- |
| Print UI | `internal/ui/printui` | Text mode, stream-json mode (via `--output`) |
| Shell UI | `internal/ui/shell` | Bubble Tea interactive UI, session resume, checkpoint/rewind, toast notifications, `@` file completer, tool cards, history persistence |
| ACP Server | `internal/acp` | Multi-session JSON-RPC over stdio, event projection, cancel propagation, `set_session_mode` handler |

#### Go Shell Features

- Session resume UI with delete (Ctrl+D)
- Command autocomplete popup on `/`
- Inline slash command suggestions
- Scrollable transcript
- Tool result cards with status icon, args box, output box (`components/tool_card.go`)
- Tool result folding with Ctrl+O toggle
- Approval panel with arrow-key selection (approve / approve-for-session / reject)
- Ctrl+C resolves pending approvals on exit
- Context usage display in status bar
- Markdown rendering via glamour (`renderers/markdown.go`)
- Tool subtitle extraction via `toolsubtitle.go`
- Toast notifications (`toast.go`): 4 levels (Info/Warning/Error/Success), TTL auto-dismiss, max 5 stack
- Prompt history persistence (`history.go`): line-delimited file, up/down arrow navigation
- `@` file mention completer (`completer/`): fuzzy matching, 2-tier TTL cache, 11-category ignore list
- Cursor positioning in InputModel: left/right arrows, mid-string insert/delete
- `/rewind` command for checkpoint selection and revert
- `/resume` command for session switching
- `/compact` command for context compaction with backup
- `/init` command for AGENTS.md generation
- `/version` command for version display
- `/release-notes` command for changelog display
- `/setup` command for interactive config wizard
- `/reload` command for config hot-reload + file index refresh

#### Go Shell Subpackages

| Subpackage | Files | Description |
| --- | --- | --- |
| `completer/` | `fileindex.go`, `fuzzy.go` | File indexer (2-tier TTL cache, ignore list) + fuzzy matcher (prefix/consecutive/position scoring) |
| `components/` | `banner.go`, `status_bar.go`, `tool_card.go` | Reusable UI widgets |
| `renderers/` | `markdown.go` | Glamour-based markdown rendering |
| `styles/` | `colors.go`, `lipgloss.go` | Lipgloss color/style definitions |

---

## Gap Analysis

### Capability Matrix

| Area | Python v0.35 | Go | Status |
| --- | --- | --- | --- |
| Entry / app wiring | yes | yes | `done` |
| Config: models/providers | yes | yes | `done` |
| Config: web search | Moonshot API | DuckDuckGo | `diverged` |
| Session create/continue | yes | yes | `done` |
| Session resume UI | yes | yes | `done` |
| Context history (JSONL) | yes | yes | `done` |
| Checkpoint storage | yes | yes | `done` |
| D-Mail / rollback | BackToTheFuture + DenwaRenji | yes (`internal/dmail` + runtime integration) | `done` |
| Wire system | `Wire` class with ContextVar | `internal/wire` (buffered chan 64, context key, Send/Receive/Shutdown) | `done` |
| Approval system | `Approval` class with yolo/auto-approve/reject | `internal/approval` (yolo, auto-approve, approve-for-session, reject) | `done` |
| Tool context tracking | `CustomToolset` + ContextVar | approval context propagation via `approval.WithContext` | `done` |
| Runtime: steer input | no explicit mechanism | no | `same` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | tenacity + jitter + connection recovery | retryable error classification + backoff/jitter + status updates | `done` |
| Streaming text/tool deltas | yes | yes | `done` |
| Runtime events | 7 types + ApprovalRequest | 7 runtime event types + ApprovalRequest + ToastMessage via wire | `done+extra` |
| Tool subtitle extraction | yes (per-tool logic) | yes (`toolsubtitle.go`) | `done` |
| Output shaping | 50K chars, 2K/line | 50K chars, 2K/line | `done` |
| Print UI: text | yes | yes | `done` |
| Print UI: stream-json | yes | yes (via --output stream-json) | `done` |
| Shell UI: basic REPL | yes | yes | `done` |
| Shell UI: live rendering | Rich.Live 4fps | Bubble Tea View() rebuild | `partial` |
| Shell: `/compact` | yes | yes | `done` |
| Shell: `/rewind` | `/clear` (revert to checkpoint 0) | `/rewind` (select any checkpoint) | `done+extra` |
| Shell: `/release-notes` | yes | yes (embedded changelog) | `done` |
| Shell: `/version` | yes | yes | `done` |
| Shell: `/setup` | yes (interactive wizard) | yes (ModeSetup with 5 phases) | `done` |
| Shell: `/reload` | yes (config hot-reload) | yes (config.Load + file index refresh) | `done` |
| Shell: `/init` (AGENTS.md) | yes | yes | `done` |
| Shell: `@` file completer | yes (fuzzy, 2-tier, cached) | yes (`completer/` subpackage, fuzzy matching, 2-tier TTL cache) | `done` |
| Shell: bottom toolbar | yes (time + mode + context) | yes (status bar with time + keyboard hint) | `done` |
| Shell: tool result cards | yes | yes (`components/tool_card.go`, wired into runtime event pipeline) | `done` |
| Shell: approval panel | yes | yes (ModeApprovalPrompt, arrow-key selection) | `done` |
| Shell: mode toggle (agent/shell) | yes (Ctrl-K) | no | `missing` |
| Shell: toast notifications | yes | yes (`toast.go`, 4 levels, TTL auto-dismiss, max 5 stack, via wire `ToastMessage`) | `done` |
| Shell: prompt history | yes (per-directory JSONL) | yes (line-delimited file, up/down arrow) | `done` |
| Shell: background tasks | yes (auto-update) | no | `missing` |
| Auto-update | yes (background check + install) | no | `missing` |
| First-run setup wizard | yes (`/setup`) | yes (`/setup` interactive wizard) | `done` |
| ACP server mode | yes (multi-session) | yes (multi-session) | `done` |
| ACP: RPC handlers | `initialize`, `new_session`, `load_session`, `resume_session`, `list_sessions`, `prompt`, `cancel`, `set_session_model`, `authenticate` | same + `set_session_mode` | `done+extra` |
| ACP: event projection | yes | yes | `done` |
| ACP: content block conversion | Text/Image/Audio/Resource | text only | `partial` |
| ACP: tool result conversion | full schema | truncated at 10K chars | `partial` |
| ACP: authentication | stubbed | stubbed | `same` |
| Agent spec | 7 fields | 8 fields (has `model`) | `done+extra` |
| Subagent: continuation prompt | yes (< 200 chars re-run) | yes | `done` |
| MCP tool bridge | fastmcp adapter | go-sdk adapter | `done` |
| SendDMail tool | yes | yes | `done` |

### Tool Parity Detail

| Python Tool | Go Equivalent | Status |
| --- | --- | --- |
| `Bash` | `bash` | `done` (approval gate added; timeout 120s vs Python 60s; Go adds background mode) |
| `ReadFile` | `read_file` | `done` |
| `WriteFile` | `write_file` | `done` |
| `Glob` | `glob` | `done` (Go supports `**`, Python rejects prefix) |
| `Grep` | `grep` | `done` |
| `StrReplaceFile` | `replace_file` | `done` (both support `replace_all` + batch edits) |
| `PatchFile` | `patch_file` | `done` |
| `Think` | `think` | `done` |
| `SetTodoList` | `set_todo_list` | `done` |
| `Task` | `agent` | `done` (both have continuation prompt) |
| `SendDMail` | `send_dmail` | `done` |
| `SearchWeb` | `search_web` | `diverged` (Moonshot vs DuckDuckGo) |
| `FetchURL` | `fetch_url` | `diverged` (trafilatura vs heuristic) |
| `MCPTool` | MCP handler in `tools/mcp.go` | `done` |

### Snapshot Corrections / Notes

- Python reference has **no separate `soul/event.py`**; event types are defined in `soul/wire.py`.
- Python shell currently auto-approves `ApprovalRequest` in `ui/shell/__init__.py`; approval UI there is still a TODO stub.
- Go shell parity has moved ahead in several places: toast notifications, prompt history, `@` completer, `/reload`, and live tool cards are implemented.
- Main remaining Go parity gaps are shell mode toggle, background task management in ShellApp, and the Python auto-update system.

---

## Remaining Work

### Phase 12: Shell Parity

- [x] Add `/version` command
- [x] Add `/release-notes` command (changelog parser, embedded CHANGELOG.md)
- [x] Add keyboard shortcut hint to status bar (Ctrl+O展开/折叠)
- [x] Add `/setup` command (interactive config wizard with ModeSetup)
- [x] Add config.Save() and SaveFile() for atomic config writes
- [x] Add approval panel (ModeApprovalPrompt, arrow-key selection, approve/approve-for-session/reject)
- [x] Add `@` file mention completer (fuzzy, 2-tier lazy index, ignore list, cursor positioning)
- [x] Add `/reload` command (config hot-reload + file index refresh)
- [x] Add cursor positioning to InputModel (left/right arrows, mid-string insert/delete)
- [x] Add toast notifications system (`toast.go` + wire `ToastMessage`)
- [x] Add prompt history persistence (`history.go`, line-delimited local file)
- [ ] Add mode toggle (agent/shell) -- lower priority
- [ ] Add external editor (Ctrl-O) -- lower priority
- [ ] Add clipboard paste -- lower priority
- [ ] Add background task browser (`/task`) -- lower priority

### Phase 13: Wire/Approval System -- DONE

- [x] Implement bidirectional Wire-like channel (events + approval requests)
- [x] Add context key pattern for implicit wire access (`wire.WithCurrent` / `wire.Current`)
- [x] Implement Approval system with yolo mode and auto-approve actions
- [x] Add `ApprovalRequest` message type via wire
- [x] Add tool approval gates (bash, write_file, replace_file)
- [x] Add approval panel UI (ModeApprovalPrompt, arrow keys, 3-way resolve)
- [x] Race condition fix: `wasIdle` pattern for late wire events after completion

### Phase 14: Runtime Parity

- [x] Add exponential backoff with jitter for step retry
- [x] Add shield equivalent for context writes (prevent cancellation corruption)
- [ ] Add background task management in ShellApp

### Phase 15: Auto-Update System

Python reference has this system (`ui/shell/update.py` + background check in `ShellApp`), but Go does not yet.

- [ ] Add version check against CDN
- [ ] Add tar.gz download and extraction
- [ ] Add background update check task
- [ ] Add toast notification for available updates

### Phase 16: Go-Specific Cleanup

- [ ] Decide on bash timeout default (Python: 60s, Go: 120s)
- [ ] Align grep sandboxing (Python: no sandbox, Go: should it have?)
- [ ] Decide on Glob `**` handling (Python rejects prefix, Go allows)

---

## Immediate Next Steps

Highest-impact remaining items in order:

1. **Runtime parity** (Phase 14) -- background task management in `ShellApp`
2. **Auto-update** (Phase 15) -- background version check + install flow
3. **Shell remaining polish** (Phase 12) -- mode toggle, external editor, clipboard paste, background task browser
4. **Go-specific cleanup** (Phase 16) -- align tool defaults with Python

---

## Architecture Diagrams

### Current Go Architecture

```
cmd/fimi
  |
  v
internal/app
  |
  +-- internal/config       (models, providers, web, MCP, history window)
  +-- internal/session      (session metadata)
  +-- internal/agentspec    (YAML agent definitions)
  +-- internal/contextstore (JSONL history, checkpoints)
  +-- internal/tools        (13 builtin tools + MCP bridge + background manager)
  +-- internal/llm          (OpenAI/Qwen providers, dual wire API)
  +-- internal/mcp          (MCP client lifecycle, tool discovery)
  +-- internal/runtime      (step loop, events, retry, subtitle)
  +-- internal/dmail        (D-Mail state machine, rollback trigger)
  +-- internal/wire         (bidirectional channel, events + approval + toast)
  +-- internal/approval     (permission gating, yolo, auto-approve)
  +-- internal/acp          (JSON-RPC server, event projection)
  +-- internal/ui
  |     +-- printui         (text output, stream-json)
  |     +-- shell           (Bubble Tea interactive)
  |           +-- completer (file index + fuzzy match)
  |           +-- components (banner, status bar, tool card)
  |           +-- renderers (markdown)
  |           +-- styles    (lipgloss styles)
  +-- internal/websearch    (DuckDuckGo scraper)
  +-- internal/webfetch     (HTTP fetch, content extraction)
```

### Target Architecture (After Gaps Closed)

```
CLI / Shell / Print / ACP
  |
  v
internal/app
  |
  +-- config               infrastructure
  +-- session              infrastructure
  +-- agentspec            adapter/integration
  +-- dmail                rollback / time-travel
  +-- approval             permission gating (NEW)
  +-- wire                 bidirectional channel (NEW)
  +-- ui/*
  |     +-- printui (text, stream-json)
  |     +-- shell (richer meta commands, tool cards, @ completer, toolbar, toasts)
  |     +-- acp (full content block conversion, approval requests)
  |
  v
internal/runtime           core agent logic
  |
  +-- contextstore         core logic + persistence
  +-- runtime/events       core boundary (events + ApprovalRequest)
  +-- llm                  replaceable adapter
  +-- tools                replaceable boundary
  |     +-- builtin local tools
  |     +-- web tools (search_web, fetch_url)
  |     +-- MCP bridge
  |     +-- delegation (with continuation)
  |     +-- dmail tool
```

---

## Design Notes

### Good Patterns

- Runtime unaware of UI rendering details
- Tool execution behind explicit `ToolExecutor` interface
- Event sink as UI boundary
- Config-driven model/provider selection
- Checkpoint store independent of runtime
- Tool subtitle extraction lives in runtime, not UI

### Things to Avoid

- Bolting MCP directly into runtime branches
- Letting shell concerns leak into runtime
- Assuming `agentspec.Spec.Model` means per-subagent override works
- Planning around Python files not in `temp/`

---

## Out of Scope

The following are present in `temp/` but not treated as migration targets:

- Test tools (`Plus`, `Compare`, `Panic`)
- Moonshot-specific search API (using DuckDuckGo instead)
- RALPH loop (FlowRunner decision node system) -- noted as not present in v0.35
- Feedback submission command (`/feedback`) -- external service integration
