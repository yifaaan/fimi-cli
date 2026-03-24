# PLAN

## Step Goal

This plan refresh does one coherent thing:

1. record the actual implementation status of the Go rewrite today
2. compare it against the `temp/` Python reference target
3. turn that gap into an ordered implementation roadmap

We do this now because the old plan was written when the repository was almost empty. That is no longer true. The Go version already has a runnable vertical slice, so continuing to plan as if only `main -> app.Run` existed would mislead every next implementation step.

## Design Explanation

The current Go repository is no longer at the "initial skeleton" stage.

What already exists:

- CLI entry and app wiring
- config loading and validation
- session selection and history file routing
- JSONL history persistence
- minimal LLM engine and provider wiring
- one-shot runtime execution: prompt in, assistant reply out

What is still missing compared with `temp/`:

- true multi-step agent loop
- tool calling and tool result feedback
- agent spec loading
- event stream and UI layers
- subagent delegation
- MCP integration

So the roadmap should stop being "create the first package skeleton" and become:

- preserve the finished foundation
- deepen the runtime into a real agent loop
- add tools and prompt composition
- then add UI, subagents, and ecosystem integrations

## Current Status Snapshot

### Foundation already implemented

- [x] Thin CLI entry in [`cmd/fimi/main.go`](/home/smooth/code/fimi-cli/cmd/fimi/main.go)
- [x] Application wiring in [`internal/app/app.go`](/home/smooth/code/fimi-cli/internal/app/app.go)
- [x] CLI parsing for `--help`, `--new-session`, `--model`, `--` in [`internal/app/app.go`](/home/smooth/code/fimi-cli/internal/app/app.go)
- [x] Config defaults and validation in [`internal/config/config.go`](/home/smooth/code/fimi-cli/internal/config/config.go)
- [x] Session routing and latest-session reuse in [`internal/session/session.go`](/home/smooth/code/fimi-cli/internal/session/session.go)
- [x] History JSONL persistence and bootstrap in [`internal/contextstore/context.go`](/home/smooth/code/fimi-cli/internal/contextstore/context.go)
- [x] Minimal runtime runner in [`internal/runtime/runtime.go`](/home/smooth/code/fimi-cli/internal/runtime/runtime.go)
- [x] LLM engine boundary and history-to-message mapping in [`internal/llm/engine.go`](/home/smooth/code/fimi-cli/internal/llm/engine.go)
- [x] Placeholder and QWEN client wiring in [`internal/app/llm_builder.go`](/home/smooth/code/fimi-cli/internal/app/llm_builder.go)
- [x] Test coverage for current app/config/context/runtime/session/llm slices under [`internal/`](/home/smooth/code/fimi-cli/internal)

### Partially implemented

- [~] `LoopControl` exists in config, but is not yet consumed by a real step loop in [`internal/config/config.go`](/home/smooth/code/fimi-cli/internal/config/config.go)
- [~] Runtime supports one prompt/one reply, but not iterative agent execution in [`internal/runtime/runtime.go`](/home/smooth/code/fimi-cli/internal/runtime/runtime.go)
- [~] History storage is solid, but still stores only simple text records instead of richer step/checkpoint/usage events in [`internal/contextstore/context.go`](/home/smooth/code/fimi-cli/internal/contextstore/context.go)

### Not implemented yet

- [ ] Agent spec loading comparable to [`temp/src/kimi_cli/agent.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agent.py)
- [ ] Multi-step runtime loop comparable to [`temp/src/kimi_cli/soul/kimisoul.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/kimisoul.py)
- [ ] Checkpoint / revert / usage persistence comparable to [`temp/src/kimi_cli/soul/context.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/context.py)
- [ ] Event stream comparable to [`temp/src/kimi_cli/soul/event.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/event.py)
- [ ] Tool system comparable to [`temp/src/kimi_cli/tools/`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/tools)
- [ ] Shell / print / ACP UI comparable to [`temp/src/kimi_cli/ui/`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/ui)
- [ ] Subagent delegation comparable to [`temp/src/kimi_cli/tools/task/__init__.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/tools/task/__init__.py)
- [ ] MCP integration comparable to [`temp/src/kimi_cli/tools/mcp.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/tools/mcp.py)

## Python Reference Target

The target is not "a CLI that can ask one question to a model".

The target in `temp/` is an engineered agent with these layers:

- CLI / transport:
  [`temp/src/kimi_cli/__init__.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/__init__.py)
- agent spec loading and tool composition:
  [`temp/src/kimi_cli/agent.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agent.py)
- core multi-step runtime loop:
  [`temp/src/kimi_cli/soul/kimisoul.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/kimisoul.py)
- context persistence, checkpoint, revert:
  [`temp/src/kimi_cli/soul/context.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/context.py)
- event bus:
  [`temp/src/kimi_cli/soul/event.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/soul/event.py)
- default agent spec and subagent spec:
  [`temp/src/kimi_cli/agents/koder/agent.yaml`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agents/koder/agent.yaml)
  [`temp/src/kimi_cli/agents/koder/sub.yaml`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agents/koder/sub.yaml)
- tool adapters:
  [`temp/src/kimi_cli/tools/`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/tools)
- UI adapters:
  [`temp/src/kimi_cli/ui/`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/ui)

## Gap Analysis

### What the Go version already proves

The current Go code proves these boundaries are good and do not need a redesign:

- `cmd/fimi` as process entry
- `internal/app` as application composition root
- `internal/config` as file-backed config boundary
- `internal/session` as session locator
- `internal/contextstore` as history persistence boundary
- `internal/llm` as provider-neutral LLM boundary
- `internal/runtime` as core execution boundary

That means the next work should deepen existing seams, not replace them.

### What is still missing to reach the `temp` target

#### 1. Runtime depth gap

Today:

- `runtime.Run(...)` performs one engine call and appends one assistant reply

Target:

- runtime owns a step loop
- each step can emit partial output, tool calls, tool results, and termination status
- loop control, retry, and finish conditions are explicit

#### 2. Tooling gap

Today:

- there is no tool contract, no tool registry, no tool execution policy

Target:

- structured tool interface
- built-in tools for bash, file reads/writes/search, web fetch/search
- task/subagent tool
- MCP-backed external tools

#### 3. Prompt composition gap

Today:

- system prompt is a plain string from config

Target:

- agent YAML/spec
- prompt template expansion
- tool descriptions injected into prompt/model request
- subagent-specific prompt variants

#### 4. State model gap

Today:

- history records are simple text JSONL entries

Target:

- checkpoint markers
- usage markers
- richer runtime records and recovery support

#### 5. Interface gap

Today:

- single CLI path with plain process output

Target:

- shell UI
- print UI with machine-readable stream modes
- ACP server mode
- event-driven rendering instead of direct print coupling

## Ordered Roadmap

### Phase 0: Preserve and tidy the current base

Goal:

- keep the existing vertical slice stable while preparing for deeper runtime work

Tasks:

- [x] Stabilize CLI parsing and help text
- [x] Stabilize config / session / context / llm / runtime seams
- [ ] Add a short "current architecture" note to explain the already-built vertical slice

Why this phase exists:

- the base is already good enough to build on
- rewriting it now would waste the progress already made

### Phase 1: Define the core runtime protocol

Goal:

- make `internal/runtime` capable of describing a multi-step agent loop before adding real tools

Tasks:

- [ ] Introduce runtime step result types: continue / finish / interrupt
- [ ] Define runtime event types for step begin, model output, tool call, tool result, status update
- [ ] Thread `LoopControl` into runtime execution
- [ ] Add retry policy at the runtime boundary
- [ ] Keep the first implementation tool-free if needed, but shape the API for future tool calls

Primary packages:

- `internal/runtime`
- new `internal/runtime/events` if needed

### Phase 2: Define the tool contract and minimal built-in tools

Goal:

- add the smallest viable tool system that the runtime can drive

Tasks:

- [ ] Introduce a tool interface plus tool call / tool result types
- [ ] Add tool registry / composition at the app layer
- [ ] Implement one minimal executable tool first: bash
- [ ] Add one read-only file tool first: read file
- [ ] Add tests for runtime <-> tool round trips

Primary packages:

- new `internal/tools`
- new `internal/tools/bash`
- new `internal/tools/fileops` or split file packages

### Phase 3: Add agent spec loading and prompt composition

Goal:

- stop treating the system prompt as a single config string and move toward the `temp` agent model

Tasks:

- [ ] Introduce `internal/agentspec`
- [ ] Load default agent spec from disk
- [ ] Support prompt template variables from workdir / time / AGENTS.md
- [ ] Map spec tool declarations to Go tool registrations
- [ ] Support subagent declarations in the spec model, even if execution is deferred

Primary packages:

- new `internal/agentspec`
- `internal/app`

### Phase 4: Deepen history into recoverable runtime state

Goal:

- evolve `contextstore` from simple text history into an agent state log

Tasks:

- [ ] Add checkpoint records
- [ ] Add usage records
- [ ] Add restore-from-log behavior richer than today's bootstrap
- [ ] Add revert-to-checkpoint behavior
- [ ] Keep append-only persistence as the main design

Primary packages:

- `internal/contextstore`
- `internal/runtime`

### Phase 5: Build the default agent toolset

Goal:

- cover the main operational tools used by the reference `koder` agent

Tasks:

- [ ] Bash
- [ ] ReadFile
- [ ] Glob
- [ ] Grep
- [ ] WriteFile
- [ ] StrReplaceFile / patch-style editing helper
- [ ] SearchWeb
- [ ] FetchURL
- [ ] Think
- [ ] Todo list

Reference target:

- [`temp/src/kimi_cli/agents/koder/agent.yaml`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agents/koder/agent.yaml)

### Phase 6: Add subagent delegation

Goal:

- implement the most important agent-multiplication feature after the base toolset exists

Tasks:

- [ ] Introduce a `Task`-style tool
- [ ] Load subagent specs
- [ ] Create isolated subagent session/history
- [ ] Return only summary output to the parent agent
- [ ] Prevent recursive tool explosion by limiting subagent toolsets

Reference target:

- [`temp/src/kimi_cli/tools/task/__init__.py`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/tools/task/__init__.py)
- [`temp/src/kimi_cli/agents/koder/sub.yaml`](/home/smooth/code/fimi-cli/temp/src/kimi_cli/agents/koder/sub.yaml)

### Phase 7: Add UI and transport layers

Goal:

- separate runtime events from presentation and support both human and machine-facing modes

Tasks:

- [ ] Build shell UI on top of runtime events
- [ ] Build print UI with deterministic text output
- [ ] Add stream-json style machine output
- [ ] Add ACP mode only after the event model is stable

Primary packages:

- new `internal/ui`
- new `internal/ui/shell`
- new `internal/ui/printui`
- new `internal/ui/acp`

### Phase 8: Add MCP and ecosystem integrations

Goal:

- let external tool ecosystems plug into the same runtime/tool contract

Tasks:

- [ ] Add MCP tool adapter
- [ ] Add config and wiring for external MCP configs
- [ ] Reuse the same tool contract as built-in tools

Primary packages:

- new `internal/tools/mcp`
- `internal/app`
- `internal/config`

## Suggested Immediate Next Teaching Units

The next reasonable micro-steps are now:

1. define the first runtime event types
2. define the first runtime step result type
3. thread `LoopControl` into runtime even before tools exist
4. only then add the first concrete tool

This order is intentional:

- first stabilize the core loop contract
- then attach tools to that contract
- then attach UI to runtime events

## Local Module Diagram

```text
Current Go rewrite

cmd/fimi
  -> internal/app
       -> internal/config
       -> internal/session
       -> internal/contextstore
       -> internal/llm
       -> internal/runtime

Missing next layers

internal/runtime
  -> runtime/events
  -> tools/*
  -> agentspec
  -> ui/*
```

## Global Architecture Relation

```text
Python reference target (`temp`)

CLI/UI
  -> app wiring
     -> agent spec
     -> soul runtime loop
     -> context with checkpoint/revert
     -> tools
     -> subagents
     -> MCP

Current Go rewrite

CLI entry
  -> app wiring
     -> config/session/context/llm/runtime
     -> one-shot reply flow

Roadmap direction

Current Go base
  -> deepen runtime
  -> add tools
  -> add agentspec
  -> add UI
  -> add subagents and MCP
```

## Design Rationale

This plan intentionally does **not** recommend jumping straight into UI or MCP.

That would be a common bad design move:

- adding shell rendering before a runtime event model exists
- adding tools before the runtime step protocol exists
- adding subagents before the parent runtime can already manage normal tool calls

The right order is to deepen the core first:

1. runtime protocol
2. tool contract
3. agent spec composition
4. durable runtime state
5. richer tools
6. subagents
7. UI and protocol adapters

That order preserves clean boundaries:

- `runtime` stays core agent logic
- `tools`, `ui`, `mcp`, provider adapters stay replaceable
- `app` remains the composition root instead of becoming a god package

## Next Tiny Step

The next smallest high-value implementation step is:

1. introduce a minimal runtime event model
2. emit a `StepBegin` event from runtime
3. keep the rest of the current one-shot flow unchanged

That step is small, local, and starts moving the Go runtime toward the `temp` architecture without prematurely adding tool complexity.
