# PLAN

## Purpose

This file tracks the migration gap between the Python reference implementation in `temp/`
and the current Go rewrite.

Updated: 2026-03-26

The previous version of this plan was outdated in two important ways:

- it still treated shell UI as "not started"
- it still treated streaming LLM support as "missing"

Both are already present in the Go codebase. The real remaining gap is now mostly:

- richer agent spec parity
- subagent / task delegation
- web tools
- MCP integration
- wider event protocol parity
- D-Mail / ACP / `stream-json`

---

## Reference Baseline In `temp/`

The Python reference is broader than the old plan implied. The important source files are:

### Runtime / protocol

- `temp/src/kimi_cli/soul/kimisoul.py`
  - main turn loop
  - step loop with retries
  - checkpoint management
  - D-Mail rollback integration
  - MCP loading hooks
  - status updates and streamed content/tool events
- `temp/src/kimi_cli/wire/types.py`
  - actual event protocol center
  - includes `TurnBegin`, `TurnEnd`, `SteerInput`, `StepBegin`, `StepInterrupted`
  - includes `CompactionBegin`, `CompactionEnd`
  - includes MCP snapshots, subagent events, approval/question events, notifications

### UI

- `temp/src/kimi_cli/ui/print/visualize.py`
  - text output
  - `stream-json` output
- `temp/src/kimi_cli/ui/shell/`
  - interactive shell UI
  - prompt handling
  - live rendering
  - approval/question panels
  - MCP status panel
  - task browser / replay / export-import helpers
- `temp/src/kimi_cli/ui/acp/`
  - ACP-facing UI / transport mode

### Agent spec / delegation / tools

- `temp/src/kimi_cli/agent.py`
  - `extend`
  - `model`
  - `system_prompt_args` merge
  - `tools`
  - `exclude_tools`
  - `subagents`
- `temp/src/kimi_cli/subagents/`
  - subagent builder / runner / store / model
- `temp/src/kimi_cli/soul/toolset.py`
  - tool loading
  - hidden tools
  - MCP tool loading and status
- `temp/src/kimi_cli/tools/`
  - local tools
  - web tools
  - think / todo
  - plan mode
  - ask-user
  - background task tools
  - D-Mail tool
  - Agent tool
- `temp/src/kimi_cli/cli/mcp.py`
  - MCP config management CLI

---

## Current Go Snapshot

As of 2026-03-26, the Go rewrite already has more than the old plan credited it for.

### Implemented

- entry chain: `cmd/fimi -> internal/app`
- config loading with models/providers
- session create / continue flow
- JSONL history persistence
- checkpoint create / revert
- multi-step runtime loop
- tool-call execution loop
- token usage persistence
- agent spec loading with `extend`, `extend: default`, `model`, `exclude_tools`, `subagents`
- system prompt template expansion
- foreground declared subagent loading and execution
- isolated subagent history files
- LLM engine boundary
- OpenAI-compatible and Qwen-compatible providers
- streaming LLM seam
- runtime event sink boundary
- print visualizer
- shell UI with:
  - REPL loop
  - TTY live mode
  - non-TTY transcript fallback
  - `/help`, `/clear`, `/exit`
  - shell history persistence
- builtin tool runtime with 7 local handlers:
  - `bash`
  - `read_file`
  - `glob`
  - `grep`
  - `write_file`
  - `replace_file`
  - `patch_file`
- app-wired delegation tool:
  - `agent`

### Important nuance

- tool registry exposes 8 definitions total: 7 local tools + 1 delegation tool
- the default agent currently enables 7 tools and leaves `patch_file` disabled
- shell UI exists, but it is still much smaller than Python shell mode
- streaming exists for text and tool-call deltas, but protocol coverage is still narrower than Python
- foreground subagent delegation exists, but background task orchestration does not

---

## What The Old Plan Got Wrong

These statements were inaccurate and should no longer guide the roadmap:

- "Shell UI not started"
- "UI mode is print only"
- "Streaming LLM response is missing"
- "`internal/runtime/events` is missing streaming events"
- "`internal/ui/shell` is TODO"
- "Streaming LLM seam" and "Shell UI basics" should be the top next steps

More accurate replacements:

- shell UI is implemented, but lacks Python's richer interaction surface
- streaming is implemented, but only for a smaller event family
- the next priority is agent/delegation/tooling parity, not basic shell/streaming enablement
- Python event references should point to `temp/src/kimi_cli/wire/types.py`, not a simplified `soul/event.py`

---

## Gap Summary

```text
Current Go
  = app entry + config + sessions + context/history + checkpoints +
    multi-step runtime + local tool execution + streaming seam +
    print visualizer + shell UI

Python target in temp
  = current Go core +
    richer agent spec +
    subagent runtime +
    web tools +
    MCP loading/config/tool bridge +
    wider event protocol +
    D-Mail +
    ACP +
    stream-json
```

### Capability Matrix

| Area | Python reference | Current Go | Status |
| --- | --- | --- | --- |
| Entry / app wiring | yes | yes | `done` |
| Config: models/providers | yes | yes | `done` |
| Config: services / MCP | yes | no | `missing` |
| Session create/continue | yes | yes | `done` |
| Context history | yes | yes | `done` |
| Checkpoint / revert | yes | yes | `done` |
| Multi-step runtime | yes | yes | `done` |
| Step retry | provider-aware retry/backoff | simple retry loop | `partial` |
| Streaming text/tool deltas | yes | yes | `done` |
| Turn / compaction / MCP / subagent event families | yes | no | `missing` |
| Print UI | text + `stream-json` | text only | `partial` |
| Shell UI | rich shell suite | minimal shell/live shell | `partial` |
| ACP mode | yes | no | `missing` |
| Agent spec `extend` | yes | yes | `done` |
| Agent spec `extend: default` | yes | yes | `done` |
| Agent spec `model` | yes | yes | `done` |
| Agent spec `exclude_tools` / `subagents` | yes | yes | `done` |
| Local file/command tools | yes | yes | `done` |
| Web tools | yes | no | `missing` |
| Think / todo tools | yes | no | `missing` |
| Agent / subagent delegation | yes | foreground-only | `partial` |
| Background task tools/store | yes | no | `missing` |
| MCP tool bridge | yes | no | `missing` |
| D-Mail protocol | yes | no | `missing` |

---

## Reference Mapping

| Python reference | Go target | Status |
| --- | --- | --- |
| `temp/src/kimi_cli/app.py` | `cmd/fimi` + `internal/app` | `done` |
| `temp/src/kimi_cli/config.py` | `internal/config` | `partial` |
| `temp/src/kimi_cli/metadata.py` / session files | `internal/session` | `done` |
| `temp/src/kimi_cli/agent.py` | `internal/agentspec` + `internal/app` | `partial` |
| `temp/src/kimi_cli/soul/kimisoul.py` | `internal/runtime` | `partial` |
| `temp/src/kimi_cli/soul/context.py` | `internal/contextstore` | `done` |
| `temp/src/kimi_cli/wire/types.py` | `internal/runtime/events` | `partial` |
| `temp/src/kimi_cli/ui/print/` | `internal/ui/printui` | `partial` |
| `temp/src/kimi_cli/ui/shell/` | `internal/ui/shell` | `partial` |
| `temp/src/kimi_cli/ui/acp/` | - | `missing` |
| `temp/src/kimi_cli/subagents/` | `internal/app` + `internal/session` subagent history path | `partial` |
| `temp/src/kimi_cli/soul/toolset.py` MCP/tool loading parts | `internal/tools` + app wiring | `missing` |
| `temp/src/kimi_cli/tools/web/` | - | `missing` |
| `temp/src/kimi_cli/tools/think/` | - | `missing` |
| `temp/src/kimi_cli/tools/todo/` | - | `missing` |
| `temp/src/kimi_cli/tools/task/` | `internal/tools/agent.go` + `internal/app` | `partial` |
| `temp/src/kimi_cli/tools/background/` | - | `missing` |
| `temp/src/kimi_cli/tools/dmail/` | - | `missing` |
| `temp/src/kimi_cli/cli/mcp.py` | - | `missing` |

---

## Remaining Work, In Practical Order

The next roadmap should follow the actual dependency chain, not the old shell/streaming-first ordering.

### Phase 8: Agent Spec Parity

Goal: make agent definitions expressive enough to match the Python model before building delegation.

- [x] add `model` to `internal/agentspec.Spec`
- [x] add `exclude_tools`
- [x] add `subagents`
- [x] keep current inheritance rule for `system_prompt_args` merge
- [x] add overwrite semantics for `tools`, `exclude_tools`, `subagents`
- [x] apply `exclude_tools` during tool resolution in app wiring
- [x] add `extend: default` compatibility

Why now:

- subagent delegation depends on `subagents`
- tool exposure control depends on `exclude_tools`
- this is the smallest missing boundary that unlocks later phases cleanly

### Phase 9: Foreground Subagent Delegation

Goal: match the Python `Agent` tool at a minimal useful level.

- [x] add an `agent` tool contract
- [x] load subagent specs by declared name
- [x] create isolated subagent history/context
- [x] run a subagent with its own tools/system prompt/model
- [x] return final assistant summary text to the parent run

Keep out of this phase unless necessary:

- background tasks
- resume existing subagent instances
- ACP projection

### Phase 10: Background Task Model

Goal: catch up with Python's separation between foreground subagent calls and background task management.

- [ ] define background task model/store
- [ ] add task listing/output/stop tools
- [ ] persist task metadata separately from foreground transcript

### Phase 11: Services Config And Missing Utility Tools

Goal: add external information tools without mixing provider config and service config.

- [ ] extend `internal/config` with service configuration
- [ ] add `search_web`
- [ ] add `fetch_url`
- [ ] add `think`
- [ ] add `todo` / `set_todo_list`
- [ ] decide whether fetch is pure HTTP or content-extraction aware

### Phase 12: MCP Integration

Goal: bridge external MCP tools into the Go tool runtime.

- [ ] add MCP config surface
- [ ] add MCP tool loading lifecycle
- [ ] expose MCP status snapshots to runtime/UI
- [ ] decide CLI management shape for MCP config

### Phase 13: Event Protocol Expansion

Goal: widen the Go event model toward Python wire parity.

- [ ] add turn-level events
- [ ] add compaction-related events if compaction is implemented
- [ ] add MCP loading/status events
- [ ] add subagent events
- [ ] add approval/question/notification events only when the runtime can emit them
- [ ] add `stream-json` print output mode

### Phase 14: D-Mail

Goal: add the time-travel protocol on top of the existing checkpoint/revert foundation.

- [ ] add D-Mail state holder
- [ ] add D-Mail tool
- [ ] integrate fetch/send behavior into the runtime loop
- [ ] define rollback semantics and history replay behavior

### Phase 15: ACP

Goal: add machine-facing transport parity after the runtime/event model is wide enough.

- [ ] add ACP server mode
- [ ] project runtime events onto transport messages
- [ ] support cancellation/interruption across the transport boundary

---

## Immediate Next Steps

These are the real next steps now:

1. services config + web tools
2. think / todo tools
3. MCP integration
4. event protocol widening + `stream-json`
5. background task model on top of the current foreground delegation path

Not immediate anymore:

- basic shell UI
- basic streaming LLM support

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
  |     +-- web tools
  |     +-- MCP bridge
  |     +-- delegation tool
  |
  +-- subagents             minimal foreground path exists in app; dedicated module planned
  +-- dmail                 planned
```

---

## Design Notes

Good migration discipline:

- keep runtime unaware of shell rendering details
- keep tool execution behind explicit boundaries
- keep MCP as an adapter layer, not runtime-specific code
- keep subagent execution isolated from parent context storage
- treat `stream-json` as a projection of runtime events, not a separate runtime

Bad shortcuts to avoid:

- building background task orchestration before the foreground subagent boundary is stable
- mixing service config into model provider config without a clear boundary
- bolting MCP directly into runtime branches
- implementing D-Mail before the event/runtime protocol has stable extension points
- turning shell/UI concerns into special cases inside runtime
