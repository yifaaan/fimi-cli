# PLAN

## Purpose

This file tracks the migration gap between the Python reference snapshot in `temp/`
and the current Go rewrite.

Updated: 2026-03-26

This version is based on the files that actually exist in this checkout.
The previous version mixed in capabilities and paths from a broader/upstream reference
that are not present in the checked `temp/` tree.

The biggest corrections are:

- shell UI is already implemented in Go
- streaming LLM support is already implemented in Go
- `think` and `set_todo_list` are already implemented in Go
- foreground declared-subagent delegation is already implemented in Go
- the checked Python snapshot does **not** contain:
  - `temp/src/kimi_cli/wire/types.py`
  - `temp/src/kimi_cli/subagents/`
  - `temp/src/kimi_cli/soul/toolset.py`
  - `temp/src/kimi_cli/cli/mcp.py`
  - `temp/src/kimi_cli/tools/background/`

That means the real remaining gap in this repo is now mostly:

- web tools
- MCP integration
- `stream-json` print mode
- ACP transport mode
- shell parity (`/compact`, richer meta-command surface)
- D-Mail runtime integration
- a few Go-side wiring/correctness gaps, especially around declared subagent model override

---

## Reference Baseline In `temp/`

This section lists the Python reference files that are actually present in this checkout.
Anything not listed here should not be treated as a parity requirement unless a newer
reference snapshot is imported later.

### Runtime / protocol

- `temp/src/kimi_cli/soul/kimisoul.py`
  - main turn loop
  - per-step retry loop with retryable provider errors
  - checkpointing before run / steps
  - D-Mail rollback integration
  - streamed content and tool-call events
- `temp/src/kimi_cli/soul/context.py`
  - JSONL history persistence
  - checkpoint records
  - revert-to-checkpoint support
  - usage persistence
- `temp/src/kimi_cli/soul/event.py`
  - actual checked event surface in this snapshot
  - `StepBegin`
  - `StepInterrupted`
  - `StatusUpdate`
  - content parts / tool call / tool call part / tool result flow through the queue
- `temp/src/kimi_cli/ui/__init__.py`
  - run coordinator
  - cancellation boundary between runtime and visualization

### UI

- `temp/src/kimi_cli/ui/print/__init__.py`
  - text output mode
  - `stream-json` output mode
- `temp/src/kimi_cli/ui/shell/__init__.py`
  - interactive prompt loop
  - live rendering for text / tool / status events
  - cancellation handling
- `temp/src/kimi_cli/ui/shell/liveview.py`
  - live step renderer with tool subtitles and context usage
- `temp/src/kimi_cli/ui/shell/metacmd.py`
  - `/help`
  - `/clear`
  - `/compact`
  - `/init`
  - `/release-notes`
- `temp/src/kimi_cli/ui/acp/__init__.py`
  - ACP server mode
  - prompt / cancel / session transport
  - tool-call streaming projection

### Agent spec / delegation / tools

- `temp/src/kimi_cli/agent.py`
  - `extend`
  - `name`
  - `system_prompt_path`
  - `system_prompt_args` merge
  - `tools`
  - `exclude_tools`
  - `subagents`
  - MCP tool loading via `load_agent_with_mcp`
- `temp/src/kimi_cli/tools/task/__init__.py`
  - foreground subagent delegation tool (`Task`)
- `temp/src/kimi_cli/tools/web/`
  - `SearchWeb`
  - `FetchURL`
- `temp/src/kimi_cli/tools/think/`
  - `Think`
- `temp/src/kimi_cli/tools/todo/`
  - `SetTodoList`
- `temp/src/kimi_cli/tools/mcp.py`
  - MCP tool adapter
- `temp/src/kimi_cli/tools/dmail/`
  - `SendDMail`
- `temp/src/kimi_cli/tools/file/`
  - file tools including patch support in the reference implementation

### Important snapshot caveats

The following paths cited by the previous version are not present in the checked `temp/`
snapshot and should not drive parity claims in this repo:

- `temp/src/kimi_cli/wire/types.py`
- `temp/src/kimi_cli/subagents/`
- `temp/src/kimi_cli/soul/toolset.py`
- `temp/src/kimi_cli/cli/mcp.py`
- `temp/src/kimi_cli/tools/background/`

Also note:

- the checked Python snapshot uses `temp/src/kimi_cli/soul/event.py` as its event center,
  not `wire/types.py`
- the checked Python snapshot uses a `Task` tool for delegation, not an `Agent` tool
- broad claims about approval/question panels, MCP status panel, task browser,
  replay/export helpers, or background-task orchestration are **not** grounded in the
  checked `temp/` tree

---

## Current Go Snapshot

As of 2026-03-26, the Go rewrite is further along than the previous version of this
plan credited it for.

### Implemented

- entry chain: `cmd/fimi -> internal/app`
- config loading with models/providers
- session create / continue flow
- JSONL history persistence
- checkpoint create / revert in `internal/contextstore`
- multi-step runtime loop
- tool-call execution loop
- token usage persistence
- agent spec loading with:
  - `extend`
  - `extend: default`
  - `model`
  - `exclude_tools`
  - `subagents`
- system prompt template expansion
- foreground declared subagent loading and execution
- isolated subagent history files
- LLM engine boundary
- OpenAI-compatible and Qwen-compatible providers
- streaming LLM seam
- runtime event sink boundary
- print visualizer (text mode)
- shell UI with:
  - REPL loop
  - TTY live mode
  - non-TTY transcript fallback
  - `/help`, `/clear`, `/exit`, `/quit`
  - shell history persistence
- builtin tool runtime with 10 registered tool definitions:
  - `agent`
  - `think`
  - `set_todo_list`
  - `bash`
  - `read_file`
  - `glob`
  - `grep`
  - `write_file`
  - `replace_file`
  - `patch_file`

### Important nuance

- the default agent currently enables 9 tools and leaves `patch_file` disabled
- `think` and `set_todo_list` are not just registered; they also have Go execution handlers
- print UI exists, but it is text-only; there is no `stream-json` mode yet
- shell UI exists and is usable, but it is still smaller than the Python shell path
- current runtime event coverage is still narrow:
  - `step_begin`
  - `step_interrupted`
  - `status_update`
  - `text_part`
  - `tool_call`
  - `tool_call_part`
  - `tool_result`
- checkpoint/revert exists at storage level, but runtime-integrated rollback / D-Mail semantics do not
- Go has gone beyond the checked Python snapshot by adding agent-spec `model`
- top-level agent model override is wired through app/runtime selection
- declared subagent runs still pass the parent-resolved runtime model into `runtime.Input`,
  so per-subagent model override parity is not fully closed yet

---

## What The Previous Plan Got Wrong

These statements were inaccurate and should no longer guide the roadmap:

- "Shell UI not started"
- "UI mode is print only"
- "Streaming LLM response is missing"
- "Agent spec `exclude_tools` / `subagents` are missing in Go"
- "Foreground subagent delegation is missing in Go"
- "Think / todo tools are missing in Go"
- "The Python event reference should point to `temp/src/kimi_cli/wire/types.py`"
- "Background task tooling is part of the checked Python snapshot and should be a near-term parity target"
- Python-shell claims about approvals/questions panels, MCP status panel, task browser,
  replay/export helpers, and similar richer surfaces

More accurate replacements:

- shell UI is implemented in Go, but still smaller than the Python shell path
- streaming is implemented in Go for text and tool-call deltas
- Go already has `exclude_tools`, `subagents`, foreground delegation, `think`, and `set_todo_list`
- the checked Python event reference is `temp/src/kimi_cli/soul/event.py`
- the real missing parity target is now mostly web/MCP/ACP/`stream-json`/D-Mail/shell-compaction
- background task orchestration should be treated as a separate future feature unless a newer
  reference snapshot is imported

---

## Gap Summary

```text
Current Go
  = app entry + config + sessions + context/history + checkpoint store +
    multi-step runtime + local tools + think/todo + foreground subagents +
    streaming seam + print text UI + shell UI

Python target in temp
  = current Go core +
    stream-json print mode +
    ACP transport mode +
    web tools +
    MCP tool loading +
    shell /compact and a richer meta-command surface +
    D-Mail runtime rollback
```

### Capability Matrix

Rows marked `extra` mean Go already has a capability that is not part of the checked
Python snapshot.

| Area | Python reference in `temp/` | Current Go | Status |
| --- | --- | --- | --- |
| Entry / app wiring | yes | yes | `done` |
| Config: models/providers | yes | yes | `done` |
| Session create/continue | yes | yes | `done` |
| Context history | yes | yes | `done` |
| Checkpoint / revert storage | yes | yes | `done` |
| Runtime-managed rollback / D-Mail | yes | no | `missing` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | provider-aware retry loop | simple retryable loop | `partial` |
| Streaming text/tool deltas | yes | yes | `done` |
| Core runtime event family (`step/status/text/tool`) | yes | yes | `done` |
| Print UI: text | yes | yes | `done` |
| Print UI: `stream-json` | yes | no | `missing` |
| Shell UI: live step/tool/status rendering | yes | yes | `partial` |
| Shell meta commands | richer set incl. `/compact` | basic set only | `partial` |
| ACP mode | yes | no | `missing` |
| Agent spec `extend` | yes | yes | `done` |
| Agent spec `exclude_tools` / `subagents` | yes | yes | `done` |
| Agent spec `model` | no | yes | `extra` |
| Think tool | yes | yes | `done` |
| Todo tool | yes | yes | `done` |
| Local file/command tools | yes | yes | `done` |
| Web tools | yes | no | `missing` |
| Foreground subagent delegation | yes | yes, but smaller surface | `partial` |
| MCP tool bridge | yes | no | `missing` |
| Patch tool implementation | yes | yes | `done` |

### Things intentionally removed from the parity target

The following are **not** treated as current migration gaps, because they are not backed
by the checked `temp/` snapshot:

- background task tools/store
- approval/question event families
- task browser / replay / export UI
- broad MCP status panel UI
- `wire/types.py`-style wider protocol families

If we later import a newer Python reference snapshot that contains those features,
add a second plan or reopen this one with that newer snapshot as the baseline.

---

## Reference Mapping

| Python reference | Go target | Status |
| --- | --- | --- |
| `temp/src/kimi_cli/__init__.py` | `cmd/fimi` + `internal/app` | `done` |
| `temp/src/kimi_cli/config.py` | `internal/config` | `partial` |
| `temp/src/kimi_cli/metadata.py` | `internal/session` | `done` |
| `temp/src/kimi_cli/agent.py` | `internal/agentspec` + `internal/app` | `partial` |
| `temp/src/kimi_cli/soul/kimisoul.py` | `internal/runtime` | `partial` |
| `temp/src/kimi_cli/soul/context.py` | `internal/contextstore` | `partial` |
| `temp/src/kimi_cli/soul/event.py` | `internal/runtime/events` | `done` |
| `temp/src/kimi_cli/ui/print/__init__.py` | `internal/ui/printui` | `partial` |
| `temp/src/kimi_cli/ui/shell/__init__.py` + `liveview.py` + `metacmd.py` | `internal/ui/shell` | `partial` |
| `temp/src/kimi_cli/ui/acp/__init__.py` | - | `missing` |
| `temp/src/kimi_cli/tools/task/__init__.py` | `internal/tools/agent.go` + `internal/app` | `partial` |
| `temp/src/kimi_cli/tools/web/` | - | `missing` |
| `temp/src/kimi_cli/tools/think/` | `internal/tools/builtin.go` + registry/app wiring | `done` |
| `temp/src/kimi_cli/tools/todo/` | `internal/tools/builtin.go` + registry/app wiring | `done` |
| `temp/src/kimi_cli/tools/mcp.py` | - | `missing` |
| `temp/src/kimi_cli/tools/dmail/` | - | `missing` |
| `temp/src/kimi_cli/tools/file/` | `internal/tools/builtin.go` | `done` |

---

## Remaining Work, In Practical Order

The next roadmap should follow the actual checked `temp/` snapshot, not the older,
broader plan.

### Phase 8: Web Tools And External Service Wiring

Goal: close the largest missing tool capability that is actually present in `temp/`.

- [ ] extend Go config/app wiring with a clean external-service boundary
- [ ] add `search_web`
- [ ] implement `search_web` with a DuckDuckGo backend first
- [ ] add `fetch_url`
- [ ] decide whether `fetch_url` is raw HTTP only or readability/extraction aware

Why now:

- these are real gaps against the checked Python snapshot
- this is a clean adapter-layer addition
- it does not require changing runtime semantics first

### Phase 9: MCP Integration

Goal: match the Python MCP adapter path that is actually present in `temp/`.

- [ ] add MCP config surface
- [ ] add MCP client lifecycle and tool discovery
- [ ] wrap MCP tools into Go tool definitions / handlers
- [ ] surface MCP failures cleanly to app/UI

Keep out of this phase unless necessary:

- broad MCP status UI
- speculative MCP management CLI that is not present in the checked Python snapshot

### Phase 10: Print / Transport Parity

Goal: close the machine-facing output gap.

- [ ] add `stream-json` print output mode
- [ ] add ACP server mode
- [ ] project the existing runtime event family onto ACP transport messages
- [ ] support cancellation across the ACP boundary

### Phase 11: Shell Parity

Goal: match the checked Python shell features that clearly matter.

- [ ] add `/compact`
- [ ] decide whether `/init` belongs in Go or should remain out of scope
- [ ] decide whether `/release-notes` belongs in Go or should remain out of scope
- [ ] improve live shell presentation only where it closes a clear Python gap

### Phase 12: D-Mail / Rollback Integration

Goal: build on the checkpoint store that already exists in Go.

- [ ] add D-Mail state holder
- [ ] add `SendDMail` tool
- [ ] checkpoint before run and per-step boundaries
- [ ] integrate rollback / synthetic message replay into the runtime loop

### Phase 13: Go-Specific Cleanup / Divergence

Goal: stabilize features that Go already has or almost has.

- [ ] close declared-subagent `model` override path so subagents actually run with their own resolved model
- [ ] decide whether `patch_file` should stay disabled in the default agent
- [ ] decide whether background task orchestration belongs in the Go rewrite as a separate future feature

---

## Immediate Next Steps

These are the real next steps now:

1. web tools + external service wiring
2. MCP integration
3. `stream-json` print mode
4. ACP transport mode
5. shell `/compact`
6. D-Mail / runtime rollback

Not immediate anymore:

- basic shell UI
- basic streaming LLM support
- think / todo tools
- foreground subagent delegation
- background task model as a parity requirement

---

## Local Architecture Diagram

```text
cmd/fimi
  |
  v
internal/app
  |
  +-- internal/config
  +-- internal/session
  +-- internal/agentspec
  +-- internal/contextstore
  +-- internal/tools
  +-- internal/llm
  +-- internal/ui
  |     +-- printui
  |     +-- shell
  |
  v
internal/runtime
  |
  +-- drives step loop
  +-- reads/writes contextstore
  +-- calls llm engine
  +-- executes tools
  +-- emits runtime events
```

## Target Architecture Diagram

```text
CLI / Shell / Print / ACP
  |
  v
internal/app
  |
  +-- config                infrastructure
  +-- session               infrastructure
  +-- agentspec             adapter/integration
  +-- ui/*                  adapter/integration
  |
  v
internal/runtime            core agent logic
  |
  +-- contextstore          core logic + persistence
  +-- runtime/events        core boundary
  +-- llm                   replaceable adapter boundary
  +-- tools                 replaceable tool boundary
  |     +-- builtin local tools
  |     +-- think / todo
  |     +-- web tools       planned
  |     +-- MCP bridge      planned
  |     +-- delegation tool
  |
  +-- dmail                 planned on top of checkpoints
```

---

## Design Notes

Good migration discipline:

- treat the checked `temp/` tree as the ground truth for parity
- keep runtime unaware of shell / print / ACP rendering details
- keep tool execution behind explicit boundaries
- keep MCP as an adapter layer, not runtime-specific branching
- keep D-Mail on top of the existing checkpoint store
- treat `stream-json` as a projection, not a second runtime
- distinguish clearly between:
  - true temp parity gaps
  - Go-only extra features
  - speculative future work

Bad shortcuts to avoid:

- planning around Python files that are not present in the checked `temp/` snapshot
- calling background tasks a parity gap without reference code in `temp/`
- assuming subagent model override is fully correct just because `agentspec.Spec` already has `model`
- bolting MCP directly into runtime branches
- letting shell/UI concerns leak into runtime control flow
