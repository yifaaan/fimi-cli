# PLAN

## Purpose

This file tracks the migration gap between the Python reference snapshot in `temp/`
and the current Go rewrite.

Updated: 2026-03-27 (comprehensive update after full codebase exploration)

---

## Reference Baseline In `temp/`

### Runtime Core

| File | Description |
| --- | --- |
| `soul/kimisoul.py` | Main runtime loop: `run() -> _turn() -> _agent_loop() -> _step()` with D-Mail rollback via `BackToTheFuture` exception |
| `soul/context.py` | JSONL history persistence, checkpoint records (incremental ID), revert-to-checkpoint with atomic file rotation |
| `soul/event.py` | Event queue: `TurnBegin`, `SteerInput`, `TurnEnd`, `StepBegin`, `StepInterrupted`, `CompactionBegin/End`, `MCPLoadingBegin/End`, `StatusUpdate`, `Notification`, `ContentPart`, `ToolCall`, `ToolCallPart`, `ToolResult`, `ApprovalRequest`, `SubagentEvent`, `QuestionRequest` |
| `soul/denwarenji.py` | D-Mail state machine (`DenwaRenji`): `_pending_dmail`, `_n_checkpoints`, `send_dmail()` / `fetch_pending_dmail()` |
| `soul/message.py` | Message utilities, tool result conversion |
| `wire/types.py` | All wire event types, `ContextVar`-based `wire_send()` |
| `acp/session.py` | ACP session with event-to-ACP projection layer |
| `acp/server.py` | Multi-session ACP server with JSON-RPC: `initialize`, `new_session`, `load_session`, `resume_session`, `list_sessions`, `prompt`, `cancel`, `set_session_model`, `authenticate` |
| `acp/convert.py` | `acp_blocks_to_content_parts`: `TextContentBlock` -> `TextPart`, `ImageContentBlock` -> `ImageURLPart`, etc. |
| `acp/tools.py` | Terminal tool with cancellation via `ACPProcess.kill()` |
| `ui/__init__.py` | Run coordinator, cancellation boundary |

#### Runtime Loop Detail (`soul/kimisoul.py`)

```
run(user_input)
  ├── slash command? -> execute directly
  └── FlowRunner.ralph_loop() (if max_ralph_iterations != 0)
      └── _turn()
            └── _agent_loop()
                  └── while True:
                        ├── auto-compact check
                        ├── checkpoint() + denwa_renji.set_n_checkpoints()
                        ├── _step()
                        │     ├── notifications.deliver_pending()
                        │     ├── _collect_injections() [plan/yolo reminders]
                        │     ├── tenacity retry + _run_with_connection_recovery()
                        │     ├── result.tool_results()
                        │     ├── asyncio.shield(_grow_context()) [atomic context write]
                        │     ├── denwa_renji.fetch_pending_dmail() -> BackToTheFuture
                        │     └── return StepOutcome or None
                        ├── _consume_pending_steers() [mid-turn user input injection]
                        ├── handle BackToTheFuture: revert + checkpoint + append message
                        └── continue or return TurnOutcome
```

Key runtime patterns NOT in Go rewrite:
- **BackToTheFuture exception**: caught at `_agent_loop()` level, revert handled outside `except` block
- **Steer input**: queue-based mid-turn user message injection via `_steer_queue`
- **Dynamic injection providers**: `PlanModeInjectionProvider`, `YoloModeInjectionProvider` per-step
- **Connection recovery**: provider-level `on_retryable_error()` callback + tenacity
- **asyncio.shield**: `_grow_context()` shielded from cancellation
- **SubagentEvent wrapping**: nested subagent events attributed to parent
- **Notification delivery**: before each step, reconciles with background tasks

### Python Tools (16 total)

| Tool | File | Description |
| --- | --- | --- |
| `Bash` | `tools/bash/__init__.py` | Shell commands with foreground max 300s, background max 24h, asyncio-level timeout, approval gate, stdout/stderr streaming |
| `ReadFile` | `tools/file/read.py` | Read file with offset/limit/line truncation |
| `WriteFile` | `tools/file/write.py` | Write/append to files within work directory |
| `Glob` | `tools/file/glob.py` | Glob matching (max 1000 results) |
| `Grep` | `tools/file/grep.py` | ripgrep wrapper with context/multiline/type filters |
| `StrReplaceFile` | `tools/file/replace.py` | `replace_all` support, batch edits, diff display blocks, plan-mode approval gate |
| `PatchFile` | `tools/file/patch.py` | Unified diff patches via `patch_ng` |
| `Think` | `tools/think/__init__.py` | Private reasoning note |
| `SetTodoList` | `tools/todo/__init__.py` | Todo list management |
| `Task` | `tools/task/__init__.py` | Foreground subagent via `ForegroundSubagentRunner` + `run_with_summary_continuation` (if < 200 chars, re-run with continuation prompt), background tasks via `BackgroundTasks` |
| `SendDMail` | `tools/dmail/__init__.py` | Time-travel message: calls `denwa_renji.send_dmail()`, returns inverted success signal |
| `SearchWeb` | `tools/web/search.py` | Web search via Moonshot API |
| `FetchURL` | `tools/web/fetch.py` | URL fetch with trafilatura text extraction |
| `MCPTool` | `tools/mcp.py` + `acp/mcp.py` | MCP tool adapter: `TextContent`->`TextPart`, `ImageContent`->`ImageURLPart`, `AudioContent`->`AudioURLPart`, server config conversion to `fastmcp.MCPConfig` |
| `Plus`/`Compare`/`Panic` | `tools/test.py` | Test utilities (out of scope) |

### Python UI

| Component | File | Features |
| --- | --- | --- |
| Print UI | `ui/print/__init__.py` | `text` mode, `stream-json` mode, stdin input |
| Shell UI | `ui/shell/__init__.py` | Interactive REPL (prompt_toolkit + Rich), dual-mode (agent/shell), `resume_prompt` asyncio.Event for steer-while-streaming, wire hub for ApprovalRequest |
| Live View | `ui/shell/visualize.py` | Rich.Live with 10fps refresh, `_ToolCallBlock` with streaming args, key-argument subtitle extraction, subagent nesting, `_ContentBlock` markdown streaming, `_NotificationBlock` |
| Meta Commands | `ui/shell/slash.py` | Full registry: `/help`, `/version`, `/model`, `/editor`, `/changelog` (release-notes), `/clear`, `/new`, `/sessions`, `/task`, `/web`, `/mcp`, `/login`, `/logout`, `/usage`, `/debug`, `/export`, `/import`, `/reload` |
| Prompt | `ui/shell/prompt.py` | `LocalFileMentionCompleter` (fuzzy, 2-tier lazy index, 2s TTL cache, massive ignore list), `_render_bottom_toolbar` (git branch badge, cwd, rotating keyboard tips), `RunningPromptDelegate`, clipboard paste, external editor (Ctrl-O) |
| ACP Server | `acp/server.py` | Multi-session JSON-RPC over stdio, session lifecycle, cancellation, model switching, auth |

### Python Agent Spec

| Field | Description |
| --- | --- |
| `extend` | Inheritance from base spec (`"default"` keyword supported) |
| `name` | Agent name |
| `system_prompt_path` | Path to system prompt template |
| `system_prompt_args` | Template variables (includes `KIMI_NOW`, `KIMI_WORK_DIR`, etc.) |
| `tools` | List of tool specs (`"package:ClassName"` format, fully-qualified import path) |
| `exclude_tools` | Tools to remove from inherited set |
| `subagents` | Named subagent specs with `path` (relative YAML) / `description` |
| `model` | Model override (Go extra) |

---

## Current Go Snapshot

Updated: 2026-03-27

### Implemented Core

- Entry chain: `cmd/fimi -> internal/app`
- Config loading with models/providers/web settings
- Session create / continue / resume flow
- JSONL history persistence
- Checkpoint create / revert in `internal/contextstore`
- Multi-step runtime loop with event sink
- Tool-call execution loop
- Token usage persistence
- LLM engine boundary with streaming support
- OpenAI-compatible and Qwen-compatible providers

### Go Tools (12 registered)

| Tool | Kind | Handler | Description |
| --- | --- | --- | --- |
| `agent` | agent | skeleton only | Subagent delegation (no execution, no continuation) |
| `think` | utility | yes | Private reasoning note |
| `set_todo_list` | utility | yes | Todo list management |
| `bash` | command | partial | Shell commands with 30s timeout (no streaming, no background) |
| `search_web` | utility | yes | DuckDuckGo search |
| `fetch_url` | utility | yes | HTTP fetch with readable-content extraction |
| `read_file` | file | yes | Read file with offset/limit |
| `glob` | file | yes | Glob matching (supports `**`) |
| `grep` | file | yes | Regex grep with line numbers |
| `write_file` | file | yes | Write file with parent dir creation |
| `replace_file` | file | partial | Single replace only (rejects multi-occurrence) |
| `patch_file` | file | yes | Unified diff patch application |

### Go UI

| Component | Location | Features |
| --- | --- | --- |
| Print UI | `internal/ui/printui` | Text mode, stream-json mode (via `--output`) |
| Shell UI | `internal/ui/shell` | Bubble Tea interactive UI, session resume, checkpoint/rewind, `/compact`, `/help`, `/clear`, `/exit`, `/resume` |

#### Go Shell Features

- Session resume UI with delete (Ctrl+D)
- Command autocomplete popup on `/`
- Inline slash command suggestions
- Scrollable transcript
- Tool result folding with Ctrl+O toggle
- Context usage display
- Markdown rendering (defined but not fully wired)
- `ToolCardView()` defined but not wired into main View

#### Go Shell -- Missing Features

- **Tool subtitle extraction** -- no key-argument formatter per tool name
- **Live rendering** -- no Rich.Live equivalent, only static transcript rebuild on View()
- **Tool result cards** -- `ToolCardView()` is dead code, `renderLiveStatus()` only shows one line
- **Approval panel** -- not implemented
- **`@` file mention completer** -- no completion system
- **Bottom toolbar** -- no git branch badge, cwd display, or keyboard tips
- **Mode toggle** (agent vs shell) -- no dual-mode concept
- **Prompt history from disk** -- `historyStore` exists but no UI integration
- **`/changelog` / `/release-notes`** -- not implemented
- **`/model`, `/login`, `/logout`, `/mcp`, `/usage`, `/task`, `/web`** -- not implemented
- **External editor (Ctrl-O)** -- not implemented
- **Clipboard paste** -- not implemented
- **Keyboard handler during streaming** -- ignores input while `mode != ModeIdle`
- **`/init` command** -- does not exist in Python either (onboarding is in startup path)

---

## Gap Analysis

### Capability Matrix

| Area | Python `temp/` | Go | Status |
| --- | --- | --- | --- |
| Entry / app wiring | yes | yes | `done` |
| Config: models/providers | yes | yes | `done` |
| Config: web search | Moonshot API | DuckDuckGo | `diverged` |
| Session create/continue | yes | yes | `done` |
| Session resume UI | yes | yes | `done` |
| Context history (JSONL) | yes | yes | `done` |
| Checkpoint storage | yes | yes | `done` |
| **D-Mail / rollback** | BackToTheFuture + DenwaRenji | no | `missing` |
| Runtime: steer input (mid-turn injection) | `_steer_queue` | no | `missing` |
| Runtime: dynamic injection providers | plan/yolo reminders | no | `missing` |
| Runtime: asyncio.shield for context writes | yes | no | `missing` |
| Runtime: connection recovery (provider-level) | `on_retryable_error()` callback | no | `missing` |
| Runtime: RALPH loop (decision nodes) | FlowRunner | no | `missing` |
| Runtime: notification delivery per-step | yes | no | `missing` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | tenacity + jitter + connection recovery | simple retry loop | `partial` |
| Streaming text/tool deltas | yes | yes | `done` |
| Runtime events | 15 types | 7 types | `partial` |
| Print UI: text | yes | yes | `done` |
| Print UI: stream-json | yes | yes (via --output stream-json) | `done` |
| Shell UI: basic REPL | yes | yes | `done` |
| Shell UI: live rendering (Rich.Live) | Rich.Live 10fps | Bubble Tea View() rebuild | `partial` |
| Shell: `/compact` | yes | yes | `done` |
| Shell: `/changelog` (release-notes) | yes | no | `missing` |
| Shell: `@` file completer | yes | no | `missing` |
| Shell: bottom toolbar | yes | no | `missing` |
| Shell: tool subtitle extraction | yes | no | `missing` |
| Shell: tool result cards | yes | defined but dead | `partial` |
| Shell: approval panel | yes | no | `missing` |
| Shell: mode toggle (agent/shell) | yes | no | `missing` |
| Shell: prompt history | yes | store exists, no UI | `partial` |
| ACP server mode | yes (multi-session) | yes (multi-session) | `done` |
| Agent spec | 7 fields | 8 fields (has `model`) | `done+extra` |
| Subagent model override | passes parent | passes parent | `same` |
| **Subagent: continuation prompt** | yes (< 200 chars re-run) | yes | `done` |
| **Subagent: background tasks** | yes | no | `missing` |
| **Subagent: full runner** | ForegroundSubagentRunner | yes | `done` |
| **Subagent: resume logic** | SubagentStore | no | `missing` |
| Local file tools | 6 tools | 5 tools | `partial` |
| `replace_file` replace-all | yes | yes | `done` |
| `bash` background tasks | yes | yes | `done` |
| `bash` approval gate | yes | no | `missing` |
| `bash` streaming output | yes | no | `missing` |
| `bash` timeout default | 60s, max 300s | 120s default, max 300s | `done` |
| `search_web` | Moonshot API | DuckDuckGo | `diverged` |
| `fetch_url` | trafilatura | heuristic extraction + metadata | `done-diverged` |
| **MCP tool bridge** | fastmcp adapter | go-sdk adapter | `done` |
| **SendDMail tool** | yes | no | `missing` |

### Tool Parity Detail

| Python Tool | Go Equivalent | Status |
| --- | --- | --- |
| `Bash` | `bash` | `partial` (no streaming, no approval gate) |
| `ReadFile` | `read_file` | `done` |
| `WriteFile` | `write_file` | `done` |
| `Glob` | `glob` | `done` |
| `Grep` | `grep` | `done` |
| `StrReplaceFile` | `replace_file` | `done` (Go: supports `replace_all`; Python: `replace_all` + batch edits) |
| `PatchFile` | `patch_file` | `done` |
| `Think` | `think` | `done` |
| `SetTodoList` | `set_todo_list` | `done` |
| `Task` | `agent` | `done` (Go has continuation prompt, max-steps detection; Python has background tasks too) |
| `SendDMail` | - | `missing` |
| `SearchWeb` | `search_web` | `diverged` (Moonshot vs DuckDuckGo) |
| `FetchURL` | `fetch_url` | `done-diverged` (trafilatura vs heuristic) |
| `MCPTool` | - | `missing` |
| Test tools | - | `out of scope` |

---

## Remaining Work

### Phase 8: Web Tools (Mostly Done)

- [x] `search_web` with DuckDuckGo backend
- [x] Add `fetch_url` tool
- [x] Add readable-content extraction for `fetch_url`
- [x] Add minimal `fetch_url` metadata (`Title`, `URL`)
- [ ] Decide later whether to keep heuristic extraction or swap to trafilatura

### Phase 9: MCP Integration

- [x] Add MCP config surface in `internal/config`
- [x] Add MCP client lifecycle management (stdio transport)
- [x] Wrap MCP tools into Go tool definitions/handlers
- [ ] Surface MCP failures cleanly to UI

### Phase 10: ACP Server Mode

- [x] ACP server entry point (`fimi acp` subcommand)
- [x] Multi-session ACP server with JSON-RPC over stdio
- [x] ACP event projection: runtime events -> ACP update messages
- [x] ACP session lifecycle: `new_session`, `load_session`, `resume_session`, `list_sessions`
- [x] ACP `prompt` RPC delegation (async with streaming)
- [x] ACP `cancel` RPC with turn state cancellation
- [x] ACP `set_session_model` RPC
- [ ] ACP content block conversion (Image, Audio, Resource) — text only for now
- [ ] ACP tool result conversion back to full ACP schema
- [ ] ACP authentication flow — stubbed, no auth required
- [x] Support cancellation across ACP boundary

### Phase 11: D-Mail / Rollback Integration

- [ ] Add `DenwaRenji`-style state holder (`internal/dmail/dmail.go`)
- [ ] Add `SendDMail` tool
- [ ] Integrate `BackToTheFuture` pattern into runtime loop
- [ ] Implement checkpoint-based context revert
- [ ] Synthetic message replay after rollback

### Phase 12: Shell Parity

- [ ] Add tool subtitle extraction for live rendering
- [ ] Wire `ToolCardView()` into main View (or redesign as live tool blocks)
- [ ] Add `@` file mention completer (fuzzy, 2-tier lazy index, ignore list)
- [ ] Add bottom toolbar (git branch badge, cwd, keyboard tips)
- [ ] Add approval panel / question panel
- [ ] Add `/changelog` command
- [ ] Add markdown rendering into live transcript
- [ ] Add prompt history UI integration
- [ ] Add mode toggle (agent/shell) -- lower priority
- [ ] Add external editor (Ctrl-O) -- lower priority
- [ ] Add clipboard paste -- lower priority
- [ ] Add background task browser (`/task`) -- lower priority

### Phase 13: Tool Polish

- [x] Implement full `agent` tool: subagent runner, context restore, resume logic
- [x] Add subagent continuation prompt (when response < 200 chars)
- [x] Add `replace_file` with replace-all support and batch edits
- [x] Increase bash timeout to 300s and add streaming output
- [x] Add bash background task support

### Phase 14: Runtime Parity

- [ ] Add steer input queue (mid-turn user message injection)
- [ ] Add dynamic injection providers (plan/yolo reminders)
- [ ] Add asyncio.shield equivalent for context writes
- [ ] Add provider-level connection recovery callback
- [ ] Expand runtime event types to match Python (steer, compaction, MCP loading, notifications)
- [ ] Add subagent event wrapping
- [ ] Add notification delivery per-step

### Phase 15: Go-Specific Cleanup

- [ ] Close per-subagent model override (both Python and Go pass parent model)
- [ ] Decide on `patch_file` default enablement

---

## Immediate Next Steps

The highest-impact items in order:

1. **MCP integration** (Phase 9) -- external tool support ecosystem
2. **ACP server mode** (Phase 10) -- IDE/extension integration
3. **D-Mail / rollback** (Phase 11) -- time-travel debugging
4. **Tool polish** (Phase 13) -- make `agent` tool functional, fix `replace_file`
5. **Shell parity** (Phase 12) -- tool subtitles, file completer, bottom toolbar
6. **Runtime parity** (Phase 14) -- fill remaining runtime gaps

---

## Architecture Diagrams

### Current Go Architecture

```
cmd/fimi
  |
  v
internal/app
  |
  +-- internal/config      (models, providers, web)
  +-- internal/session     (session metadata)
  +-- internal/agentspec   (YAML agent definitions)
  +-- internal/contextstore (JSONL history, checkpoints)
  +-- internal/tools       (11+1 builtin tools)
  +-- internal/llm         (OpenAI/Qwen providers)
  +-- internal/ui
  |     +-- printui        (text output, stream-json)
  |     +-- shell          (Bubble Tea interactive)
  |
  v
internal/runtime
  |
  +-- Step loop with event sink
  +-- Reads/writes contextstore
  +-- Calls llm engine
  +-- Executes tools
  +-- Emits events
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
  +-- ui/*
  |     +-- printui (text, stream-json)
  |     +-- shell (richer meta commands, tool subtitles, @ completer, toolbar)
  |     +-- acp (NEW: JSON-RPC over stdio)
  |
  v
internal/runtime           core agent logic
  |
  +-- contextstore         core logic + persistence
  +-- runtime/events       core boundary
  +-- llm                  replaceable adapter
  +-- tools                replaceable boundary
  |     +-- builtin local tools
  |     +-- web tools (search_web, fetch_url)
  |     +-- MCP bridge (NEW)
  |     +-- delegation (with continuation)
```

---

## Design Notes

### Good Patterns

- Runtime unaware of UI rendering details
- Tool execution behind explicit `ToolExecutor` interface
- Event sink as UI boundary
- Config-driven model/provider selection
- Checkpoint store independent of runtime

### Things to Avoid

- Bolting MCP directly into runtime branches
- Letting shell concerns leak into runtime
- Assuming `agentspec.Spec.Model` means per-subagent override works
- Planning around Python files not in `temp/`

---

## Out of Scope

The following are present in `temp/` but not treated as migration targets:

- Test tools (`Plus`, `Compare`, `Panic`)
- Background task tools (not in checked snapshot)
- Approval/question event families
- Task browser / replay / export UI
- Moonshot-specific search API (using DuckDuckGo instead)
- RALPH loop (FlowRunner decision node system)
- Dual-mode shell (agent vs raw shell toggle)
