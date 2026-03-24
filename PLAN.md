# PLAN

## Purpose

This file tracks the migration gap between the current Go rewrite and the Python reference in `temp/`.

The old plan stopped at "build the initial CLI skeleton". That is now outdated. The Go codebase already has a working entry chain, config loading, session selection, JSONL history storage, a minimal LLM engine boundary, and a single-turn runtime. The plan now needs to answer a different question:

`What is still missing before fimi-cli reaches the real kimi-cli target?`

## Current Status Snapshot

### Current Execution Focus

Date: 2026-03-24

Active phase: Phase 2, Runtime Loop Kernel (mostly complete)

Completed in this session:

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
- [x] Session discovery and forced new session creation
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
- [ ] Checkpoint/revert 与 token-usage 持久化（Python `Context` 的 `_checkpoint` / `_usage` 语义）
- [ ] Runtime event bus for UI/streaming（StepBegin/StatusUpdate/Interrupted + message parts 流式输出）
- [ ] UI modes: shell, print, ACP
- [ ] MCP integration
- [ ] Richer session metadata：显式 `last_session_id`（Python `metadata.py`），而不是仅用 history mtime 推断
- [ ] Service config beyond model providers（例如 Python 的 search service 配置）

> 注：当前最阻塞的是 **runtime tool-loop 闭环**（tool 结果不写回 history），否则多步代理无法基于工具输出继续执行。

## Gap Summary

The current Go rewrite is no longer "empty", but it is still mostly the outer shell and infrastructure layer.

What is already in place:

- application entry and dependency wiring
- local persistence primitives
- model/provider configuration
- one-shot prompt execution

What is still missing:

- agent composition
- multi-step autonomous execution
- tool runtime
- user-facing UI modes

In short:

```text
Current Go
  = CLI shell + config + session + history + one-turn runtime

Target from temp
  = composition root + agent spec + multi-step loop + tools + events + UI + subagents + MCP
```

## Reference Mapping

| Python Reference | Go Rewrite Target | Status |
| --- | --- | --- |
| `temp/src/kimi_cli/__init__.py` | `cmd/fimi` + `internal/app` | partially done |
| `temp/src/kimi_cli/config.py` | `internal/config` | mostly done for model/provider basics |
| `temp/src/kimi_cli/metadata.py` | `internal/session` | partially done |
| `temp/src/kimi_cli/agent.py` | `internal/agentspec` + app wiring | mostly done for local agent loading |
| `temp/src/kimi_cli/soul/kimisoul.py` | `internal/runtime` | only minimal single-turn subset done |
| `temp/src/kimi_cli/soul/context.py` | `internal/contextstore` | basic history done, checkpoint/revert missing |
| `temp/src/kimi_cli/soul/event.py` | `internal/runtime/events` | not started |
| `temp/src/kimi_cli/soul/message.py` | `internal/runtime/messages` | not started |
| `temp/src/kimi_cli/tools/*` | `internal/tools/*` | partially done |
| `temp/src/kimi_cli/ui/*` | `internal/ui/*` | not started |

## Recommended Build Order

The missing work should not be implemented in the same order as the Python files appear on disk.

The safest order for the Go rewrite is:

1. finish the core runtime contracts
2. add tool execution boundaries
3. add a minimal tool-backed agent loop
4. enrich persistence with checkpoint/revert
5. add event streaming
6. add alternative UIs and advanced integrations

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

Goal: reach the first meaningfully autonomous version.

- [x] `bash` tool handler
- [x] `read_file` tool handler
- [x] `glob` tool handler
- [x] `grep` tool handler
- [x] `write_file` tool handler
- [x] `replace_file` tool handler

Guardrails required in the same phase:

- [x] work-dir confinement（通过 `resolveWorkspacePath` / `normalizeWorkspacePattern` 实现）
- [x] command timeout / cancellation boundary（bash 通过 `context.WithTimeout` 实现）
- [x] clear stdout/stderr/result shaping
- [x] refusal path for unsupported or dangerous operations（越界路径、无效参数等标记为 refused）

Why this phase is the first major milestone:

- after it, the agent can inspect and modify a local repo instead of only chatting about it

### Phase 5: Richer Context Store

Goal: close the biggest persistence gap with `temp/soul/context.py`.

- [ ] persist token usage records
- [ ] persist checkpoints
- [ ] implement revert-to-checkpoint
- [ ] keep append-only rotation strategy for rollback history
- [ ] expose snapshot helpers needed by runtime and UI

Why after a basic tool loop exists:

- checkpoint/revert is most valuable when multi-step execution can actually go wrong

### Phase 6: Event Stream And Print UI

Goal: make runtime observable before building the full interactive shell.

- [ ] create `internal/runtime/events`
- [ ] emit step begin / text part / tool call / tool result / status events
- [ ] create a minimal `internal/ui/printui`
- [ ] support plain text output first
- [ ] then add stream-json output for automation

Why print UI before shell UI:

- it exercises the runtime/event contract with less presentation complexity

### Phase 7: Shell UI

Goal: add the main interactive interface only after runtime events are stable.

- [ ] create `internal/ui/shell`
- [ ] interactive prompt loop
- [ ] render step/tool progress
- [ ] add minimal meta commands only after the shell loop is stable

Keep out of this phase unless required:

- ACP
- MCP
- subagents

### Phase 8: Agent Parity Features

Goal: close the larger capability gaps with the Python reference.

- [ ] subagent/task tool
- [ ] MCP tool adapter
- [ ] web/search/fetch tools
- [ ] richer service config
- [ ] session metadata for explicit continue semantics
- [ ] optional agent inheritance and subagent declarations

These are important, but they should sit on top of a stable local-agent core.

### Phase 9: Transport Parity

Goal: support machine-facing execution modes after the local CLI is solid.

- [ ] ACP server mode
- [ ] richer print mode input/output formats
- [ ] cancellation and interruption plumbing across transport boundaries

## Immediate Next Teaching Units

The next several implementation steps should stay close to the runtime core:

1. Add a structured runtime step output type that can represent either plain assistant completion or pending tool calls.
2. Introduce a tiny loop in `internal/runtime` that consumes `MaxStepsPerRun`, even if the first version still only supports "finish immediately".
3. Define the first tool interface in `internal/tools` and wire a no-op or fake registry through app/runtime.
4. Add one read-only tool first, preferably file read or bash echo-style execution, before implementing write-capable tools.

## Local Architecture Diagram

```text
cmd/fimi
  |
  v
internal/app
  |
  +-- internal/config
  +-- internal/session
  +-- internal/contextstore
  +-- internal/llm
  +-- internal/runtime
  |
  +-- future: internal/agentspec
  +-- future: internal/tools/*
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
