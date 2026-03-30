# PLAN

## Purpose

This file tracks the migration gap between the Python reference snapshot in `temp/`
and the current Go rewrite.

Updated: 2026-03-30 (comprehensive update after parallel agent exploration)

---

## Reference Baseline In `temp/`

### Runtime Core

| File | Description |
| --- | --- |
| `soul/kimisoul.py` | Main runtime loop: `run() -> _turn() -> _agent_loop() -> _step()` with D-Mail rollback via `BackToTheFuture` exception |
| `soul/context.py` | JSONL history persistence, checkpoint records (incremental ID), revert-to-checkpoint with atomic file rotation |
| `soul/event.py` | Event queue: `StepBegin`, `SteerInput`, `TurnEnd`, `StepBegin`, `StepInterrupted`, `CompactionBegin/End`, `MCPLoadingBegin/End`, `StatusUpdate`, `Notification`, `ContentPart`, `ToolCall`, `ToolCallPart`, `ToolResult`, `ApprovalRequest`, `SubagentEvent`, `QuestionRequest` |
| `soul/denwarenji.py` | D-Mail state machine (`DenwaRenji`): `_pending_dmail`, `_n_checkpoints`, `send_dmail()` / `fetch_pending_dmail()` |
| `soul/message.py` | Message utilities, tool result conversion (`tool_result_to_messages`, `tool_ok_to_message_content`, `system()` helper for `<system>` tags) |
| `wire/types.py` | All wire event types, `ContextVar`-based `wire_send()` |
| `acp/session.py` | ACP session with event-to-ACP projection layer |
| `acp/server.py` | Multi-session ACP server with JSON-RPC: `initialize`, `new_session`, `load_session`, `resume_session`, `list_sessions`, `prompt`, `cancel`, `set_session_model`, `authenticate` |
| `acp/convert.py` | `acp_blocks_to_content_parts`: `TextContentBlock` -> `TextPart`, `ImageContentBlock` -> `ImageURLPart`, etc. |
| `acp/tools.py` | Terminal tool with cancellation via `ACPProcess.kill()` |
| `ui/__init__.py` | Run coordinator, cancellation boundary (`run_soul()`) |

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
| `Task` | `tools/task/__init__.py` | Foreground subagent with continuation prompt (< 200 chars -> re-run with continuation), fresh Context per subagent, `_MockEventQueue` for invisible execution |
| `SendDMail` | `tools/dmail/__init__.py` | Time-travel message: calls `denwa_renji.send_dmail()`, returns inverted success signal |
| `SearchWeb` | `tools/web/search.py` | Web search via Moonshot API, 5-20 results, optional content crawling |
| `FetchURL` | `tools/web/fetch.py` | URL fetch with trafilatura text extraction, `ToolResultBuilder` |
| `MCPTool` | `tools/mcp.py` | MCP tool adapter via `fastmcp`: `TextContent`->`TextPart`, `ImageContent`->`ImageURLPart`, `AudioContent`->`AudioURLPart` |

### Python UI

| Component | File | Features |
| --- | --- | --- |
| Print UI | `ui/print/__init__.py` | `text` mode, `stream-json` mode, stdin input |
| Shell UI | `ui/shell/__init__.py` | Interactive REPL (prompt_toolkit + Rich), dual-mode (agent/shell), `resume_prompt` asyncio.Event for steer-while-streaming, wire hub for ApprovalRequest |
| Live View | `ui/shell/liveview.py` | Rich.Live with 4fps refresh, `_ToolCallDisplay` with streaming args, key-argument subtitle extraction, status text |
| Meta Commands | `ui/shell/metacmd.py` | Registry: `/help`, `/exit`, `/release-notes`, `/init`, `/clear`, `/compact` |
| Prompt | `ui/shell/prompt.py` | `FileMentionCompleter` (fuzzy, 2-tier lazy index, 2s TTL cache, 11-category ignore list), `_render_bottom_toolbar` (time + context usage), `_HistoryEntry` JSONL persistence, key binding for Enter-accepts-completion |
| ACP Server | `ui/acp/__init__.py` | Multi-session JSON-RPC over stdio, `_ToolCallState` with streaming lexer, event projection (`_stream_events`) |

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

Updated: 2026-03-30

### Implemented Core

- Entry chain: `cmd/fimi -> internal/app`
- Config loading with models/providers/web/MCP settings
- Session create / continue / resume / list / delete flow
- JSONL history persistence with sliding window reads
- Checkpoint create / revert / list / backup rotation in `internal/contextstore`
- Multi-step runtime loop with event sink and streaming
- Tool-call execution loop with retry logic (retryable errors: 429/5xx)
- Token usage persistence
- LLM engine boundary with streaming support
- OpenAI-compatible (dual wire: Chat Completions + Responses) and Qwen-compatible providers
- MCP integration via go-sdk (multi-server, tool discovery, tool calling)
- ACP server (JSON-RPC over stdio, initialize/authenticate/session/prompt/cancel)
- Tool subtitle extraction (`internal/runtime/toolsubtitle.go`)
- Output shaping (50K chars total, 2K per line)

### Go Tools (12 registered)

| Tool | Kind | Handler | Description |
| --- | --- | --- | --- |
| `agent` | agent | yes | Subagent delegation with continuation prompt (auto-re-prompts if response < 200 chars), max-steps detection |
| `think` | utility | yes | Private reasoning note |
| `set_todo_list` | utility | yes | Todo list management |
| `bash` | command | yes | Shell commands with 120s default/300s max timeout, background tasks via `BackgroundManager`, streaming output |
| `search_web` | utility | yes | DuckDuckGo HTML scraper |
| `fetch_url` | utility | yes | HTTP fetch with heuristic content extraction + scoring |
| `read_file` | file | yes | Read file with offset/limit |
| `glob` | file | yes | Glob matching (supports `**`), path sandboxing |
| `grep` | file | yes | Regex grep with line numbers |
| `write_file` | file | yes | Write file with parent dir creation, path sandboxing |
| `replace_file` | file | yes | String replace with `replace_all` support, batch edits |
| `patch_file` | file | yes | Unified diff patch application, path sandboxing |

### Go UI

| Component | Location | Features |
| --- | --- | --- |
| Print UI | `internal/ui/printui` | Text mode, stream-json mode (via `--output`) |
| Shell UI | `internal/ui/shell` | Bubble Tea interactive UI, session resume, checkpoint/rewind, `/compact`, `/help`, `/clear`, `/exit`, `/resume`, `/rewind` |
| ACP Server | `internal/acp` | Multi-session JSON-RPC over stdio, event projection, cancel propagation |

#### Go Shell Features

- Session resume UI with delete (Ctrl+D)
- Command autocomplete popup on `/`
- Inline slash command suggestions
- Scrollable transcript
- Tool result folding with Ctrl+O toggle
- Context usage display in status bar
- Markdown rendering via glamour
- Tool subtitle extraction via `toolsubtitle.go`
- `/rewind` command for checkpoint selection and revert
- `/resume` command for session switching
- `/compact` command for context compaction with backup

#### Go Shell -- Missing Features

- **Live rendering** -- no Rich.Live equivalent, only static transcript rebuild on View()
- **Tool result cards** -- `ToolCardView()` is dead code, not wired into main View
- **`@` file mention completer** -- no fuzzy file completion system
- **Bottom toolbar** -- no time display, only context usage in status bar
- **Approval panel** -- not implemented
- **`/release-notes` / `/changelog`** -- not implemented
- **`/init` (AGENTS.md generation)** -- not implemented
- **`/model`, `/login`, `/logout`, `/mcp`, `/usage`, `/task`, `/web`** -- not implemented
- **Mode toggle** (agent vs shell) -- no dual-mode concept
- **Prompt history from disk** -- readline history exists but no fuzzy search UI
- **External editor (Ctrl-O)** -- not implemented
- **Clipboard paste** -- not implemented
- **Keyboard handler during streaming** -- ignores input while `mode != ModeIdle`

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
| **D-Mail / rollback** | BackToTheFuture + DenwaRenji | yes (`internal/dmail` + runtime integration) | `done` |
| Runtime: steer input (mid-turn injection) | `_steer_queue` | no | `missing` |
| Runtime: dynamic injection providers | plan/yolo reminders | no | `missing` |
| Runtime: shield for context writes | asyncio.shield | no | `missing` |
| Runtime: connection recovery (provider-level) | `on_retryable_error()` callback | no | `missing` |
| Runtime: RALPH loop (decision nodes) | FlowRunner | no | `missing` |
| Runtime: notification delivery per-step | yes | no | `missing` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | tenacity + jitter + connection recovery | retryable error classification | `partial` |
| Streaming text/tool deltas | yes | yes | `done` |
| Runtime events | 15+ types | 7 types | `partial` |
| Tool subtitle extraction | yes (per-tool logic) | yes (`toolsubtitle.go`) | `done` |
| Output shaping | 50K chars, 2K/line | 50K chars, 2K/line | `done` |
| Print UI: text | yes | yes | `done` |
| Print UI: stream-json | yes | yes (via --output stream-json) | `done` |
| Shell UI: basic REPL | yes | yes | `done` |
| Shell UI: live rendering | Rich.Live 4fps | Bubble Tea View() rebuild | `partial` |
| Shell: `/compact` | yes | yes | `done` |
| Shell: `/rewind` | `/clear` (revert to checkpoint 0) | `/rewind` (select any checkpoint) | `done+extra` |
| Shell: `/release-notes` | yes | no | `missing` |
| Shell: `/init` (AGENTS.md) | yes | no | `missing` |
| Shell: `@` file completer | yes (fuzzy, 2-tier, cached) | no | `missing` |
| Shell: bottom toolbar | yes (time + context) | partial (status bar only) | `partial` |
| Shell: tool result cards | yes | defined but dead | `partial` |
| Shell: approval panel | yes | no | `missing` |
| Shell: mode toggle (agent/shell) | yes | no | `missing` |
| Shell: prompt history | JSONL + UI integration | readline file, no fuzzy search | `partial` |
| ACP server mode | yes (multi-session) | yes (multi-session) | `done` |
| ACP: event projection | yes | yes | `done` |
| ACP: content block conversion | Text/Image/Audio/Resource | text only | `partial` |
| ACP: tool result conversion | full schema | truncated at 10K chars | `partial` |
| ACP: authentication | stubbed | stubbed | `same` |
| Agent spec | 7 fields | 8 fields (has `model`) | `done+extra` |
| Subagent model override | passes parent | passes parent | `same` |
| **Subagent: continuation prompt** | yes (< 200 chars re-run) | yes | `done` |
| **Subagent: background tasks** | yes | no | `missing` |
| **Subagent: full runner** | ForegroundSubagentRunner | yes | `done` |
| **Subagent: resume logic** | SubagentStore | no | `missing` |
| Local file tools | 6 tools | 6 tools (same) | `done` |
| `replace_file` replace-all + batch | yes | yes | `done` |
| `bash` background tasks | yes | `BackgroundManager` (not wired in DI) | `partial` |
| `bash` approval gate | yes | no | `missing` |
| `bash` streaming output | yes (line-by-line) | yes | `done` |
| `bash` timeout default | 60s, max 300s | 120s default, max 300s | `done` (different default) |
| `search_web` | Moonshot API | DuckDuckGo | `diverged` |
| `fetch_url` | trafilatura | heuristic extraction + scoring | `diverged` |
| **MCP tool bridge** | fastmcp adapter | go-sdk adapter | `done` |
| **SendDMail tool** | yes | yes | `done` |

### Tool Parity Detail

| Python Tool | Go Equivalent | Status |
| --- | --- | --- |
| `Bash` | `bash` | `partial` (no approval gate; BackgroundManager not wired in DI) |
| `ReadFile` | `read_file` | `done` |
| `WriteFile` | `write_file` | `done` |
| `Glob` | `glob` | `done` |
| `Grep` | `grep` | `done` |
| `StrReplaceFile` | `replace_file` | `done` (both support `replace_all` + batch edits) |
| `PatchFile` | `patch_file` | `done` |
| `Think` | `think` | `done` |
| `SetTodoList` | `set_todo_list` | `done` |
| `Task` | `agent` | `done` (both have continuation prompt; Python has background tasks too) |
| `SendDMail` | `send_dmail` | `done` |
| `SearchWeb` | `search_web` | `diverged` (Moonshot vs DuckDuckGo) |
| `FetchURL` | `fetch_url` | `diverged` (trafilatura vs heuristic) |
| `MCPTool` | MCP handler in `tools/mcp.go` | `done` |
| Test tools | - | `out of scope` |

---

## Remaining Work

### Phase 8: Web Tools (Done)

- [x] `search_web` with DuckDuckGo backend
- [x] Add `fetch_url` tool
- [x] Add readable-content extraction for `fetch_url`
- [x] Add minimal `fetch_url` metadata (`Title`, `URL`)
- [ ] Decide later whether to keep heuristic extraction or swap to trafilatura

### Phase 9: MCP Integration (Done)

- [x] Add MCP config surface in `internal/config`
- [x] Add MCP client lifecycle management (stdio transport)
- [x] Wrap MCP tools into Go tool definitions/handlers
- [ ] Surface MCP failures cleanly to UI

### Phase 10: ACP Server Mode (Mostly Done)

- [x] ACP server entry point (`fimi acp` subcommand)
- [x] Multi-session ACP server with JSON-RPC over stdio
- [x] ACP event projection: runtime events -> ACP update messages
- [x] ACP session lifecycle: `new_session`, `load_session`, `resume_session`, `list_sessions`
- [x] ACP `prompt` RPC delegation (async with streaming)
- [x] ACP `cancel` RPC with turn state cancellation
- [x] ACP `set_session_model` RPC
- [ ] ACP content block conversion (Image, Audio, Resource) -- text only for now
- [ ] ACP tool result conversion back to full ACP schema
- [ ] ACP authentication flow -- stubbed, no auth required

### Phase 11: D-Mail / Rollback Integration (Done)

- [x] Add `DenwaRenji`-style state holder (`internal/dmail/dmail.go`)
- [x] Add `SendDMail` tool
- [x] Integrate `BackToTheFuture` pattern into runtime loop
- [x] Implement checkpoint-based context revert (via `contextstore.RevertToCheckpoint`)
- [x] Synthetic message replay after rollback

### Phase 12: Shell Parity

- [ ] Wire `ToolCardView()` into main View (or redesign as live tool blocks)
- [ ] Add `@` file mention completer (fuzzy, 2-tier lazy index, ignore list)
- [ ] Add bottom toolbar (time display, keyboard tips)
- [ ] Add approval panel / question panel
- [ ] Add `/release-notes` command (changelog parser)
- [ ] Add `/init` command (AGENTS.md generation)
- [ ] Add prompt history UI integration (fuzzy search)
- [ ] Add mode toggle (agent/shell) -- lower priority
- [ ] Add external editor (Ctrl-O) -- lower priority
- [ ] Add clipboard paste -- lower priority
- [ ] Add background task browser (`/task`) -- lower priority

### Phase 13: Tool Polish (Done)

- [x] Implement full `agent` tool: subagent runner, context restore, resume logic
- [x] Add subagent continuation prompt (when response < 200 chars)
- [x] Add `replace_file` with replace-all support and batch edits
- [x] Increase bash timeout to 300s and add streaming output
- [x] Add bash background task support (BackgroundManager exists, needs DI wiring)

### Phase 14: Runtime Parity

- [ ] Add steer input queue (mid-turn user message injection)
- [ ] Add dynamic injection providers (plan/yolo reminders)
- [ ] Add shield equivalent for context writes (prevent cancellation corruption)
- [ ] Add provider-level connection recovery callback
- [ ] Expand runtime event types to match Python (steer, compaction, MCP loading, notifications)
- [ ] Add subagent event wrapping
- [ ] Add notification delivery per-step

### Phase 15: Go-Specific Cleanup

- [ ] Wire `BackgroundManager` into main DI path (currently nil in app.go)
- [ ] Close per-subagent model override (both Python and Go pass parent model)
- [ ] Decide on `patch_file` default enablement

---

## Immediate Next Steps

The highest-impact items in order:

1. **Shell parity** (Phase 12) -- tool result cards, file completer, `/release-notes`, `/init`
2. **Runtime parity** (Phase 14) -- fill remaining runtime gaps
3. **Runtime parity** (Phase 14) -- fill remaining runtime gaps
4. **Go-specific cleanup** (Phase 15) -- wire BackgroundManager, minor fixes

---

## Architecture Diagrams

### Current Go Architecture

```
cmd/fimi
  |
  v
internal/app
  |
  +-- internal/config      (models, providers, web, MCP)
  +-- internal/session     (session metadata)
  +-- internal/agentspec   (YAML agent definitions)
  +-- internal/contextstore (JSONL history, checkpoints)
  +-- internal/tools       (12 builtin tools + MCP bridge)
  +-- internal/llm         (OpenAI/Qwen providers, dual wire API)
  +-- internal/mcp         (MCP client lifecycle, tool discovery)
  +-- internal/runtime     (step loop, events, retry, subtitle)
  +-- internal/acp         (JSON-RPC server, event projection)
  +-- internal/ui
  |     +-- printui        (text output, stream-json)
  |     +-- shell          (Bubble Tea interactive)
  +-- internal/websearch   (DuckDuckGo scraper)
  +-- internal/webfetch    (HTTP fetch, content extraction)
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
  +-- dmail                rollback / time-travel (NEW)
  +-- ui/*
  |     +-- printui (text, stream-json)
  |     +-- shell (richer meta commands, tool cards, @ completer, toolbar)
  |     +-- acp (full content block conversion)
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
  |     +-- MCP bridge
  |     +-- delegation (with continuation)
  |     +-- dmail tool (NEW)
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
- Approval/question event families
- Moonshot-specific search API (using DuckDuckGo instead)
- RALPH loop (FlowRunner decision node system)
- Dual-mode shell (agent vs raw shell toggle)
