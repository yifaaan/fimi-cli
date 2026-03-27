# PLAN

## Purpose

This file tracks the migration gap between the Python reference snapshot in `temp/`
and the current Go rewrite.

Updated: 2026-03-27

This version is based on a thorough comparison of the files that actually exist in
the `temp/` checkout against the current Go implementation in `internal/`.

---

## Reference Baseline In `temp/`

### Runtime Core

| File | Description |
| --- | --- |
| `soul/kimisoul.py` | Main runtime loop with step-based execution, D-Mail rollback via `BackToTheFuture` exception |
| `soul/context.py` | JSONL history persistence, checkpoint records, revert-to-checkpoint |
| `soul/event.py` | Event queue with `StepBegin`, `StepInterrupted`, `StatusUpdate`, content/tool events |
| `soul/denwarenji.py` | D-Mail state machine (`DenwaRenji`) for time-travel messaging |
| `soul/message.py` | Message utilities, tool result conversion |
| `ui/__init__.py` | Run coordinator, cancellation boundary |

### Python Tools (16 total)

| Tool | File | Description |
| --- | --- | --- |
| `Bash` | `tools/bash/__init__.py` | Shell commands with 300s timeout, output streaming |
| `ReadFile` | `tools/file/read.py` | Read file with offset/limit/line truncation |
| `WriteFile` | `tools/file/write.py` | Write/append to files within work directory |
| `Glob` | `tools/file/glob.py` | Glob matching (max 1000 results) |
| `Grep` | `tools/file/grep.py` | ripgrep wrapper with context/multiline/type filters |
| `StrReplaceFile` | `tools/file/replace.py` | String find/replace with replace-all support |
| `PatchFile` | `tools/file/patch.py` | Unified diff patches via `patch_ng` |
| `Think` | `tools/think/__init__.py` | Private reasoning note |
| `SetTodoList` | `tools/todo/__init__.py` | Todo list management |
| `Task` | `tools/task/__init__.py` | Foreground subagent delegation with continuation prompt |
| `SendDMail` | `tools/dmail/__init__.py` | Time-travel message to past checkpoint |
| `SearchWeb` | `tools/web/search.py` | Web search via Moonshot API |
| `FetchURL` | `tools/web/fetch.py` | URL fetch with trafilatura text extraction |
| `MCPTool` | `tools/mcp.py` | MCP tool adapter wrapping FastMCP |
| `Plus`/`Compare`/`Panic` | `tools/test.py` | Test utilities |

### Python UI

| Component | File | Features |
| --- | --- | --- |
| Print UI | `ui/print/__init__.py` | `text` mode, `stream-json` mode, stdin input |
| Shell UI | `ui/shell/__init__.py` | Interactive REPL, live rendering, Rich text |
| Live View | `ui/shell/liveview.py` | Step renderer with tool subtitles, context usage |
| Meta Commands | `ui/shell/metacmd.py` | `/help`, `/exit`, `/quit`, `/clear`, `/compact`, `/init`, `/release-notes` |
| Prompt | `ui/shell/prompt.py` | History persistence, file mention completer (`@`), bottom toolbar |
| ACP Server | `ui/acp/__init__.py` | JSON-RPC agent protocol over stdio |

### Python Agent Spec

| Field | Description |
| --- | --- |
| `extend` | Inheritance from base spec (`"default"` keyword supported) |
| `name` | Agent name |
| `system_prompt_path` | Path to system prompt template |
| `system_prompt_args` | Template variables (includes `KIMI_NOW`, `KIMI_WORK_DIR`, etc.) |
| `tools` | List of tool specs (`"module:ClassName"` format) |
| `exclude_tools` | Tools to remove from inherited set |
| `subagents` | Named subagent specs with `path`/`description` |

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
| `agent` | agent | yes | Declared subagent delegation |
| `think` | utility | yes | Private reasoning note |
| `set_todo_list` | utility | yes | Todo list management |
| `bash` | command | yes | Shell commands with 30s timeout |
| `search_web` | utility | yes | DuckDuckGo search |
| `fetch_url` | utility | yes | HTTP fetch with readable-content extraction |
| `read_file` | file | yes | Read file with offset/limit |
| `glob` | file | yes | Glob matching (supports `**`) |
| `grep` | file | yes | Regex grep with line numbers |
| `write_file` | file | yes | Write file with parent dir creation |
| `replace_file` | file | yes | Single string replacement |
| `patch_file` | file | yes | Unified diff patch application |

### Go UI

| Component | Location | Features |
| --- | --- | --- |
| Print UI | `internal/ui/printui` | Text mode only |
| Shell UI | `internal/ui/shell` | Bubble Tea interactive UI, session resume |

#### Go Shell Meta Commands

- `/help` - Show help
- `/clear` - Clear screen
- `/exit`, `/quit` - Exit shell
- `/resume` - List/switch sessions

#### Go Shell Features

- Session resume UI with delete (Ctrl+D)
- Command autocomplete popup on `/`
- Inline slash command suggestions
- Scrollable transcript
- Collapsible tool results
- Context usage display

### Go Agent Spec

| Field | Status |
| --- | --- |
| `extend` | done (supports `"default"` keyword) |
| `name` | done |
| `model` | done (Go extra - not in Python) |
| `system_prompt_path` | done |
| `system_prompt_args` | done |
| `tools` | done |
| `exclude_tools` | done |
| `subagents` | done |

---

## Gap Analysis

### Capability Matrix

| Area | Python `temp/` | Go | Status |
| --- | --- | --- | --- |
| Entry / app wiring | yes | yes | `done` |
| Config: models/providers | yes | yes | `done` |
| Config: web search | Moonshot API | DuckDuckGo | `diverged` |
| Session create/continue | yes | yes | `done` |
| Session resume UI | no | yes | `extra` |
| Context history (JSONL) | yes | yes | `done` |
| Checkpoint storage | yes | yes | `done` |
| Runtime-managed rollback / D-Mail | yes | no | `missing` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | tenacity with jitter | simple retry loop | `partial` |
| Streaming text/tool deltas | yes | yes | `done` |
| Runtime events | 7 types | 7 types | `done` |
| Print UI: text | yes | yes | `done` |
| Print UI: stream-json | yes | yes (via --output stream-json) | `done` |
| Shell UI: basic REPL | yes | yes | `done` |
| Shell UI: live rendering | Rich-based | Bubble Tea | `partial` |
| Shell: `/compact` | yes | yes | `done` |
| Shell: `/init` | yes | no | `missing` |
| Shell: `/release-notes` | yes | no | `missing` |
| Shell: file mention (`@`) | yes | no | `missing` |
| Shell: bottom toolbar | yes | no | `missing` |
| ACP server mode | yes | no | `missing` |
| Agent spec | 7 fields | 8 fields (has `model`) | `done+extra` |
| Subagent model override | passes parent | passes parent | `same` |
| Subagent continuation prompt | yes | no | `missing` |
| Local file tools | 6 tools | 5 tools | `partial` |
| `search_web` | Moonshot API | DuckDuckGo | `diverged` |
| `fetch_url` | yes (trafilatura) | yes (heuristic extraction + metadata) | `done-diverged` |
| MCP tool bridge | yes | no | `missing` |
| `SendDMail` tool | yes | no | `missing` |
| Tool subtitle extraction | yes | no | `missing` |

### Tool Parity Detail

| Python Tool | Go Equivalent | Status |
| --- | --- | --- |
| `Bash` | `bash` | `done` (different timeout: 300s vs 30s) |
| `ReadFile` | `read_file` | `done` |
| `WriteFile` | `write_file` | `done` |
| `Glob` | `glob` | `done` |
| `Grep` | `grep` | `done` |
| `StrReplaceFile` | `replace_file` | `partial` (Go: single replace only) |
| `PatchFile` | `patch_file` | `done` |
| `Think` | `think` | `done` |
| `SetTodoList` | `set_todo_list` | `done` |
| `Task` | `agent` | `partial` (Go missing continuation prompt) |
| `SendDMail` | - | `missing` |
| `SearchWeb` | `search_web` | `diverged` (different backend) |
| `FetchURL` | `fetch_url` | `done-diverged` (Go uses built-in HTML extraction, not trafilatura) |
| `MCPTool` | - | `missing` |
| Test tools | - | `out of scope` |

---

## Remaining Work

### Phase 8: Web Tools (Mostly Done)

Go has `search_web` with DuckDuckGo, and now has `fetch_url` with built-in readable extraction plus minimal metadata.

- [x] `search_web` with DuckDuckGo backend
- [x] Add `fetch_url` tool
- [x] Add readable-content extraction for `fetch_url`
- [x] Add minimal `fetch_url` metadata (`Title`, `URL`)
- [ ] Decide later whether to keep heuristic extraction or swap to a stronger readability algorithm

### Phase 9: MCP Integration

- [ ] Add MCP config surface in `internal/config`
- [ ] Add MCP client lifecycle management
- [ ] Wrap MCP tools into Go tool definitions/handlers
- [ ] Surface MCP failures cleanly to UI

### Phase 10: Print / Transport Parity

- [x] Add `stream-json` print output mode
- [x] Add `--output` CLI flag for mode selection (`text`, `stream-json`, `shell`)
- [ ] Add ACP server mode
- [ ] Project runtime events onto ACP transport messages
- [ ] Support cancellation across ACP boundary

### Phase 11: Shell Parity

- [ ] Add startup banner version line
- [x] Add Claude-Code-like startup banner/logo
- [ ] Add `/compact` meta command
- [ ] Decide on `/init` scope
- [ ] Decide on `/release-notes` scope
- [ ] Add file mention completer (`@`)
- [ ] Add tool subtitle extraction for live rendering

### Phase 12: D-Mail / Rollback Integration

- [ ] Add `DenwaRenji`-style state holder
- [ ] Add `SendDMail` tool
- [ ] Implement `BackToTheFuture` exception pattern for rollback
- [ ] Integrate synthetic message replay

### Phase 13: Tool Polish

- [ ] Add subagent continuation prompt (when response < 200 chars)
- [ ] Consider adding `replace_file` with replace-all support
- [ ] Increase bash timeout to match Python (300s vs 30s)

### Phase 14: Go-Specific Cleanup

- [ ] Close per-subagent model override (both Python and Go pass parent model)
- [ ] Decide on `patch_file` default enablement

---

## Immediate Next Steps

1. ~~**Startup banner version line**~~ - ✅ done
2. ~~**`/compact` meta command**~~ - ✅ done
3. ~~**`stream-json` print mode**~~ - ✅ done (with `--output stream-json`)
4. **MCP integration** - external tool support
5. **ACP server mode** - IDE/extension integration
6. **D-Mail / rollback** - time-travel debugging

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
  +-- internal/tools       (11 builtin tools)
  +-- internal/llm         (OpenAI/Qwen providers)
  +-- internal/ui
  |     +-- printui        (text output)
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
  +-- ui/*                 adapter/integration
  |     +-- printui (text, stream-json)
  |     +-- shell (richer meta commands)
  |     +-- acp (NEW)
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
  |
  +-- dmail (NEW)          rollback / time-travel
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
