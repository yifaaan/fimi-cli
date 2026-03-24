# PLAN

## Purpose

This file tracks the migration gap between the current Go rewrite and the Python reference in `temp/`.

The old plan stopped at "build the initial CLI skeleton". That is now outdated. The Go codebase already has a working entry chain, config loading, session selection, JSONL history storage, a minimal LLM engine boundary, and a single-turn runtime. The plan now needs to answer a different question:

`What is still missing before fimi-cli reaches the real kimi-cli target?`

## Current Status Snapshot

### Current Execution Focus

Date: 2026-03-24

Active phase: Phase 7, Event Stream And Print UI (next active)

Completed in this session:

- [x] add session metadata store with per-workdir `last_session_id`
- [x] make "new session" the default path and add an explicit continue path
- [x] stop inferring the active session from history file mtime
- [x] return a clear user-facing error when `--continue` has no previous session
- [x] define `internal/runtime/events` and wire a no-op event sink into runtime
- [x] add explicit unfinished step state
- [x] let runtime loop represent "continue" without using errors
- [x] keep real tool execution deferred until the runtime tool boundary exists
- [x] route `max_retries_per_step` from app config into runtime
- [x] retry transient step-construction failures before history append
- [x] classify pre-execution tool guardrail failures as refusal
- [x] classify command timeout / runner failure as temporary execution failure
- [x] stop the run on temporary tool execution failure and preserve failing call context
- [x] record tool execution failures as structured failed runtime steps
- [x] **tool-loop closure**: persist tool call + tool result messages to history

Next priority items:

- [x] decide whether later phases should surface temporary tool failures as model-visible tool results
  - **决策：是的，临时失败已经是 model-visible 的 tool result**
  - 当前 `advanceRun()` 在返回错误前会先写入 tool records 到 history
  - 失败内容通过 `formatToolFailureContent()` 格式化，包含 `failure_kind` 分类
  - 与 Python 的差异：Go 停止 run，Python 继续循环；但两者都让模型能看到失败
  - 停止 run 是更安全的选择，防止 runaway 行为；用户可以继续 session 让模型看到失败后决定
- [x] Runtime 输入语义对齐：用户 prompt 只追加一次，后续 step 基于增长的 history 推进
  - **已实现**：`Run()` 只在开始时追加 user record，后续 step 通过 `store.ReadRecentTurns()` 读取增长的历史

### Already Implemented In Go

- [x] `cmd/fimi -> internal/app.Run` entry chain
- [x] CLI argument parsing with `--help`, `--new-session`, `--model`, and `--`
- [x] Config loading, defaults, validation, model/provider mapping
- [x] Explicit session creation/continue semantics with metadata-backed `last_session_id`
- [x] JSONL history persistence with bootstrap and recent-turn reads
- [x] Minimal runtime: one prompt in, one assistant reply out
- [x] LLM request/message construction boundary
- [x] Placeholder client and QWEN/OpenAI-compatible provider groundwork
- [x] Basic module-level tests across `app`, `config`, `contextstore`, `llm`, `runtime`, `session`

### Still Missing Versus `temp/kimi-cli`

- [x] Agent spec loading from YAML
- [x] System prompt template expansion
- [x] Agent inheritance / extension (基础 extend)
- [ ] Agent spec parity: `exclude_tools` / `subagents`（Python 有，Go 还没有）
- [x] Multi-step runtime loop **闭环**：tool calls -> execute -> tool results 写回 history -> 下一步 LLM 能继续
- [x] Runtime 输入语义对齐：用户 prompt 只追加一次，后续 step 基于增长的 history 推进（避免每步重复注入同一个 prompt）
- [x] Structured tool-call protocol between model and runtime（tool-call + tool-result 消息形状、失败分类、是否对模型可见）
  - 消息形状：`StepResult.BuildToolStepRecords()` 构造 assistant(tool_calls) + tool result 记录
  - 失败分类：`formatToolFailureContent()` 包含 `failure_kind: temporary/refused/error`
  - 模型可见：失败也写入 history，后续 session 模型能看到
- [x] Tool registry and tool execution layer（已具雏形，`internal/tools/executor.go` + `BuiltinRegistry`）
- [x] Tool output shaping：`message/brief` 元信息、长度/行数/字节上限、截断提示（Python `ToolResultBuilder`）
  - **已实现**：`OutputShaper` + 集成到 bash/read_file/glob/grep handlers
- [ ] Core tools parity:
  - [x] bash/read_file/glob/grep/write_file/replace_file（最小版）
  - [x] patch_file
  - [ ] web/search + web/fetch
  - [ ] think/todo/task（子 agent / 任务派发）
- [ ] Task/subagent delegation（runtime + tool 层协议未完成）
- [x] Checkpoint/revert 与 token-usage 持久化（Python `Context` 的 `_checkpoint` / `_usage` 语义）
  - [x] Token usage 持久化（`_usage` 记录）
  - [x] Checkpoint 创建（`_checkpoint` 记录）
  - [x] Revert 到指定 checkpoint
  - [x] History 文件轮转备份
  - [x] `Snapshot` / recent-turn helpers 暴露给 runtime 和 app
- [ ] Runtime event bus for UI/streaming（StepBegin/StatusUpdate/Interrupted + message parts 流式输出）
- [ ] UI modes: shell, print, ACP
- [ ] MCP integration
- [x] Explicit session metadata：显式 `last_session_id`（Python `metadata.py`），而不是仅用 history mtime 推断
  - **已实现**：`session.New()` 写入 metadata，`session.Continue()` 只读取 `last_session_id`
  - 这让 `continue` 成为显式契约，而不是依赖 history mtime 的启发式猜测
- [ ] Service config beyond model providers（例如 Python 的 search service 配置）

> 注：`runtime tool-loop` 闭环和显式 session 语义都已经完成。当前最阻塞的是 **runtime observability**（event bus + print UI）和少量 **parity gaps**（`exclude_tools`、web/subagent/MCP 能力）。

## Gap Summary

The current Go rewrite is no longer "empty", and it is also no longer just a shell around a one-shot model call.

What is already in place:

- application entry and dependency wiring
- explicit session semantics backed by metadata
- agent spec loading and prompt expansion
- multi-step runtime loop with tool-call closure
- local repo tool execution with guardrails
- model/provider configuration
- history snapshot, usage, checkpoint, and revert primitives

What is still missing:

- runtime event streaming / observability
- user-facing UI modes and transports
- parity extras in agent spec
- advanced tools and delegation (`web/*`, task/subagent, MCP)
- service config beyond model providers

In short:

```text
Current Go
  = CLI + explicit session semantics + agentspec + multi-step runtime + local tools + context history/checkpoint

Target from temp
  = current core + explicit session semantics + event bus + UI/transport + delegation + advanced integrations
```

## Reference Mapping

| Python Reference | Go Rewrite Target | Status |
| --- | --- | --- |
| `temp/src/kimi_cli/__init__.py` | `cmd/fimi` + `internal/app` | partially done |
| `temp/src/kimi_cli/config.py` | `internal/config` | mostly done for model/provider basics |
| `temp/src/kimi_cli/metadata.py` | `internal/session` | partially done |
| `temp/src/kimi_cli/agent.py` | `internal/agentspec` + app wiring | mostly done; `exclude_tools` / `subagents` still missing |
| `temp/src/kimi_cli/soul/kimisoul.py` | `internal/runtime` | core multi-step loop done; events/UI integration still missing |
| `temp/src/kimi_cli/soul/context.py` | `internal/contextstore` | mostly done for history + usage + checkpoint/revert |
| `temp/src/kimi_cli/soul/event.py` | `internal/runtime/events` | not started |
| `temp/src/kimi_cli/soul/message.py` | `internal/runtime/messages` | partial via history/tool message shaping, dedicated package not started |
| `temp/src/kimi_cli/tools/*` | `internal/tools/*` | local repo tools done; web/task/MCP tools missing |
| `temp/src/kimi_cli/ui/*` | `internal/ui/*` | not started |

## Recommended Build Order

The missing work should not be implemented in the same order as the Python files appear on disk.

The safest order for the Go rewrite is:

1. keep session semantics explicit before more history producers appear
2. add a shared runtime/UI event boundary
3. build the smallest text print mode on top of that boundary
4. add the interactive shell on the same event stream
5. close agent-spec parity needed for delegation (`exclude_tools`, `subagents`)
6. add isolated subagent/task execution
7. add advanced tools together with the config they require
8. add machine-facing transport parity last

This order keeps the system runnable at each stage and avoids building shell/ACP frontends before there is a stable runtime to drive them.

## Phased Roadmap

### Phase 0: Completed Foundation

- [x] `internal/app`
- [x] `internal/config`
- [x] `internal/session`
- [x] `internal/contextstore` basic JSONL history
- [x] `internal/llm` minimal request/reply abstraction
- [x] `internal/runtime` single-turn execution

This phase gave us a working CLI prototype and stable package seams.

### Phase 1: Agent Composition Layer

Status: completed

Goal: introduce the missing layer between `app` and `runtime` that knows how to load an agent definition.

- [x] Create `internal/agentspec`
- [x] Parse agent YAML from disk
- [x] Support agent name, system prompt path, tool list
- [x] Support `system_prompt_args` substitution
- [x] Load system prompt text from file
- [x] Resolve agent file path from CLI/app defaults
- [x] Keep tool loading explicit in Go instead of Python-style reflection

This phase now gives the Go rewrite a real local composition seam:

- default agent assets live under `agents/default`
- `app` loads agent definitions from disk
- system prompts support explicit argument substitution
- child agent specs can extend a base agent file

Why now:

- `temp` relies on `agent.py` as the composition seam
- without this layer, runtime will either hardcode prompts/tools or app will become too large

### Phase 2: Runtime Loop Kernel

Goal: upgrade `internal/runtime` from "single-turn runner" to "multi-step agent loop".

- [x] Add step result model: assistant output, tool calls, finish state
- [x] Consume `config.LoopControl` inside runtime
- [x] Implement max-step loop
- [x] Implement retry boundaries for retryable model/tool failures
- [x] Add clear run result states: finished, failed, max-steps
- [x] Add interrupted run state（需要 context.Context / cancellation 机制）

Why before tools:

- tools need a runtime contract to plug into
- otherwise tool code will force ad hoc control flow into runtime

### Phase 3: Tool Runtime Boundary

Goal: define how runtime sees tools, without tying runtime to bash/file/web specifics.

- [x] Create `internal/tools`
- [x] Define tool interface, schema, call, and result types
- [x] Create tool registry used by app/agentspec
- [x] Define runtime <-> tools adapter boundary
- [x] Add message conversion for tool calls and tool results

Classification:

- `internal/runtime`: core agent logic, stable
- `internal/tools/*`: adapter/integration logic, replaceable

### Phase 4: Minimum Useful Tool Set

Status: completed

Goal: reach the first meaningfully autonomous version.

- [x] `bash` tool handler
- [x] `read_file` tool handler
- [x] `glob` tool handler
- [x] `grep` tool handler
- [x] `write_file` tool handler
- [x] `replace_file` tool handler
- [x] `patch_file` tool handler

Guardrails required in the same phase:

- [x] work-dir confinement（通过 `resolveWorkspacePath` / `normalizeWorkspacePattern` 实现）
- [x] command timeout / cancellation boundary（bash 通过 `context.WithTimeout` 实现）
- [x] clear stdout/stderr/result shaping
- [x] refusal path for unsupported or dangerous operations（越界路径、无效参数等标记为 refused）

Why this phase is the first major milestone:

- after it, the agent can inspect and modify a local repo instead of only chatting about it

### Phase 5: Richer Context Store

Status: completed

Goal: close the biggest persistence gap with `temp/soul/context.py`.

- [x] persist token usage records
- [x] persist checkpoints
- [x] implement revert-to-checkpoint
- [x] keep append-only rotation strategy for rollback history
- [x] expose snapshot helpers needed by runtime and app

Why after a basic tool loop exists:

- checkpoint/revert is most valuable when multi-step execution can actually go wrong

### Phase 6: Session Metadata And Continue Semantics

Status: completed

Goal: replace mtime-based session reuse with the explicit contract used by Python before subagent histories and multiple transports make that heuristic unsafe.

- [x] add session metadata store with per-workdir `last_session_id`
- [x] make "new session" the default path and add an explicit continue path
- [x] return an error when continue is requested but no previous session exists
- [x] stop inferring the active session from history file mtime
- [x] cover future sibling histories (for example subagent runs) in session tests

Why now:

- Python's `Task` tool creates sibling history files for subagents
- once multiple histories coexist, "latest mtime wins" is the wrong contract

### Phase 7: Event Stream And Print UI

Status: next active phase

Goal: make runtime observable before building the full interactive shell.

- [x] create `internal/runtime/events`
- [x] define an event sink boundary with a no-op default
- [x] add a `run_soul`-style coordinator between app/runtime and UI consumers
- [ ] widen the runtime/LLM seam enough to emit step begin / text parts / tool-call deltas / tool results / status / interruption
  - 当前已接入非流式事件：`step begin`、`text`、`tool_call`、`tool_result`、`status`、`interrupted`
  - `tool-call delta` 仍依赖后续 streaming LLM seam
- [ ] create a minimal `internal/ui/printui`
- [ ] support plain text output first
- [ ] defer `stream-json` until the event boundary is stable

Why print UI before shell UI:

- it exercises the runtime/event contract with less presentation complexity
- Python's `stream-json` mode is partly history-tail based, so it is not the cleanest first slice

### Phase 8: Shell UI

Goal: add the main interactive interface only after runtime events are stable.

- [ ] create `internal/ui/shell`
- [ ] interactive prompt loop
- [ ] render step/tool progress
- [ ] add minimal meta commands only after the shell loop is stable

Keep out of this phase unless required:

- ACP
- MCP
- subagents

### Phase 9: Agent Spec Parity And Delegation

Goal: close the larger capability gaps with the Python reference.

- [ ] extend `internal/agentspec.Spec` with `ExcludeTools` and `Subagents`
- [ ] add `SubagentSpec { Path, Description }` and resolve subagent paths relative to the declaring YAML
- [ ] match Python inheritance semantics exactly: merge only `system_prompt_args`; overwrite `tools`, `exclude_tools`, and `subagents`
- [ ] apply `exclude_tools` during app tool resolution, preserving declared tool order
- [ ] add tests for child override behavior, path resolution, and exclusion after inheritance
- [ ] add a minimal `task` tool contract with `description`, `subagent_name`, and `prompt`
- [ ] add isolated subagent execution with fresh history/context and final-summary return semantics

Why after shell basics:

- task/subagent is not "just another tool"; it depends on agent-spec parity and explicit session/history behavior
- getting this wrong early creates recursive runtime coupling that is harder to unwind later

### Phase 10: Advanced Tools And Service Config

Goal: add external-network and adapter-heavy capabilities only when their config and runtime seams can land together.

- [ ] add `services` config surface only together with the tools that consume it
- [ ] web/search/fetch tools
- [ ] richer service config
- [ ] MCP tool adapter / CLI MCP config loading

Why here:

- Python's extra config beyond model providers is mainly the search service
- config-only placeholders are low value until the matching tools exist

### Phase 11: Transport Parity

Goal: support machine-facing execution modes after the local CLI is solid.

- [ ] ACP server mode
- [ ] richer print mode input/output formats
- [ ] cancellation and interruption plumbing across transport boundaries

## Immediate Next Teaching Units

The next several implementation steps should follow the real dependency chain from `temp`:

1. Define `internal/runtime/events` plus a transport-neutral run coordinator.
2. Widen the runtime/LLM seam only as far as needed to emit step/tool/status events.
3. Add a minimal plain-text `printui` consumer on that event stream.
4. After that seam is stable, extend `internal/agentspec` with `exclude_tools` and `subagents` so `task` can be designed on a real contract.

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
  +-- internal/tools
  +-- internal/contextstore
  +-- internal/llm
  |
  v
internal/runtime
  |
  +-- reads/writes internal/contextstore
  +-- calls internal/llm
  +-- executes internal/tools
  |
  +-- future: internal/runtime/events
  +-- future: internal/ui/*
```

## Target Architecture Diagram

```text
CLI / UI / Transport
  |
  v
internal/app
  |
  +-- internal/agentspec        adapter/integration, replaceable
  +-- internal/config           infrastructure
  +-- internal/session          infrastructure
  +-- internal/ui/*             adapter/integration, replaceable
  |
  v
internal/runtime                core agent logic, stable
  |
  +-- internal/runtime/events   core boundary, stable
  +-- internal/runtime/messages core support, stable
  +-- internal/contextstore     core logic + persistence
  +-- internal/llm              adapter boundary, replaceable
  +-- internal/tools/*          adapter/integration, replaceable
```

## Design Notes

Good migration discipline:

- keep runtime unaware of shell rendering
- keep tools behind explicit interfaces
- keep provider-specific HTTP code out of runtime
- keep history persistence append-first and testable

Bad migration shortcuts to avoid:

- turning `internal/app` into a giant god package
- hardcoding tools directly inside runtime branches
- implementing shell UI before runtime events exist
- adding subagents before the main agent loop is stable
