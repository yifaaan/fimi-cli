# PLAN

## Purpose

This file tracks the Go rewrite against the real target in `temp/`.

The old plan stopped at the "bootstrap the skeleton" stage. That is no longer accurate:

- `cmd/fimi`
- `internal/app`
- `internal/config`
- `internal/session`
- `internal/contextstore`
- `internal/llm`
- `internal/runtime`

already form a working vertical slice.

From now on, the plan should answer two questions clearly:

1. What is already implemented in Go?
2. What is still missing to reach `temp/src/kimi_cli` parity?

## Current Status

### Done

- [x] Thin executable entrypoint: `cmd/fimi/main.go`
- [x] Application wiring entry: `internal/app.Run`
- [x] CLI parsing for:
  - `--help` / `-h`
  - `--new-session`
  - `--model <alias>`
  - `--` terminator
- [x] CLI help text with structured sections and examples
- [x] Config loading, defaults, and validation in `internal/config`
- [x] Logical model alias -> provider/model resolution
- [x] Session selection and forced new session in `internal/session`
- [x] JSONL history persistence in `internal/contextstore`
- [x] History bootstrap and recent-turn window reads
- [x] LLM message construction in `internal/llm`
- [x] Placeholder client for local tests
- [x] QWEN adapter through OpenAI-compatible transport
- [x] Single-run runtime path:
  - read history
  - call engine once
  - append user record
  - append assistant record
- [x] Solid unit-test coverage for the current vertical slice

### Partially Done

- [~] `config.LoopControl` exists, but runtime does not yet execute a true step loop
- [~] OpenAI-compatible client exists, but app-level provider selection is still focused on `placeholder` and `qwen`
- [~] Context persistence exists, but checkpoint / revert / usage metadata parity with `temp` is still missing

### Not Started Yet

- [ ] Agent spec loading from YAML
- [ ] System prompt templating with builtin args
- [ ] Tool runtime boundary and tool registry
- [ ] Multi-step agent loop
- [ ] Structured tool call / tool result model
- [ ] Runtime event bus
- [ ] Interactive shell UI
- [ ] Print / stream-json UI
- [ ] ACP server mode
- [ ] Subagent delegation
- [ ] MCP tool integration
- [ ] Web tools and service config parity
- [ ] Checkpoint rollback flow
- [ ] Usage/token accounting persistence parity

## Current Vs Target

### Current Go Shape

```text
CLI
  -> internal/app
      -> config
      -> session
      -> contextstore
      -> runtime (single turn)
          -> llm.Engine
              -> provider adapter
```

### Target Shape From `temp`

```text
CLI / UI / ACP
  -> app composition
      -> config
      -> session metadata
      -> agent spec loader
      -> context restore/checkpoints
      -> runtime loop
           -> llm step
           -> tools
           -> events
           -> retry / max-step / finish
      -> shell / print / acp adapters
```

## Gap Analysis Against `temp`

| Target Area in `temp` | Python Reference | Go Rewrite Today | Gap |
| --- | --- | --- | --- |
| CLI modes | `shell`, `print`, `acp` | single CLI path | missing multiple frontends |
| Agent composition | `agent.py` loads YAML, prompts, tools, MCP | no `internal/agentspec` yet | missing agent assembly layer |
| Runtime loop | `soul/kimisoul.py` supports multi-step execution, retries, interruptions | `internal/runtime` is single-turn only | largest core-logic gap |
| Context model | `soul/context.py` stores checkpoints and usage and supports revert | JSONL text history only | missing rollback and richer metadata |
| Tool system | bash, file, web, todo, think, task, MCP | no `internal/tools/*` | missing main agent capability surface |
| Event stream | `soul/event.py` feeds UI | no runtime event boundary | missing UI/runtime seam |
| UI layer | shell / print / ACP | no `internal/ui/*` | missing operator experience and automation surface |
| Subagents | `tools/task` + subagent specs | no delegation path | missing parallel/distributed task handling |
| Services / web | search + fetch + service config | no web/service layer | missing online research capability |

## Migration Strategy

The main rule for the next phase is:

- do not jump straight into all tools
- first upgrade the runtime boundary so tools can plug into a stable loop

That keeps core logic separate from adapters, matching both `AGENTS.md` and the Python reference.

### Phase 1: Upgrade Runtime From Single Turn To Real Step Loop

Goal:

- turn `internal/runtime` from "one request, one reply" into "loop until finished / max-steps / error"

Deliverables:

- [ ] introduce step-level runtime state and completion semantics
- [ ] consume `config.LoopControl`
- [ ] add retry policy at runtime boundary
- [ ] define minimal event interface for step begin / step finish / interruption
- [ ] keep current history append path working while loop stays text-only

Exit criteria:

- runtime can execute more than one model step
- runtime can stop because of completion or max-steps
- runtime has a stable seam for future tool integration

### Phase 2: Add Structured Action Model

Goal:

- stop treating model output as only free-form assistant text

Deliverables:

- [ ] define runtime-facing action/result types
- [ ] extend `internal/llm` so engine can return either final text or tool-call intent
- [ ] keep provider adapters behind `internal/llm`
- [ ] add tests for action decoding and loop transitions

Exit criteria:

- runtime can distinguish:
  - final assistant answer
  - tool invocation request
  - loop continuation

### Phase 3: Introduce Tool Runtime Boundary

Goal:

- create the stable adapter seam that Python keeps in `tools/*`

Deliverables:

- [ ] add `internal/tools` base interfaces
- [ ] add tool registry / lookup
- [ ] add tool input/output envelope
- [ ] wire runtime to execute requested tools and append observations back into context

Exit criteria:

- runtime does not know concrete tool implementations
- tools can be added as adapters without changing loop core

### Phase 4: Implement Local Core Tools First

Goal:

- cover the highest-value local coding workflow before web/MCP/subagents

Priority order:

- [ ] bash
- [ ] read file
- [ ] glob
- [ ] grep
- [ ] write file
- [ ] string replace / patch-like edit

Why this order:

- it unlocks local repo reasoning and code modification first
- it matches the minimum useful coding-agent path before advanced integrations

Exit criteria:

- the agent can inspect and modify a local repository through tools, not just chat

### Phase 5: Add Agent Spec And Prompt Composition

Goal:

- move agent identity and toolset selection out of hardcoded app wiring

Deliverables:

- [ ] create `internal/agentspec`
- [ ] load YAML agent definitions
- [ ] support prompt file loading
- [ ] support builtin prompt args such as work dir and `AGENTS.md`
- [ ] connect selected tools to the loaded agent spec

Exit criteria:

- app wiring chooses an agent spec
- runtime receives system prompt and toolset from composition layer

### Phase 6: Add Runtime Events And First UI Adapter

Goal:

- make agent execution observable before building every UI mode

Deliverables:

- [ ] define event stream boundary
- [ ] emit step begin / content / tool / status events
- [ ] implement first terminal-oriented UI adapter

Recommended order:

1. print-style event consumer
2. shell UI
3. ACP mode

Reason:

- print mode is the simplest way to prove the event contract
- shell UI should be built after the event model is stable

### Phase 7: Add Advanced Parity Features

Goal:

- close the highest-value gaps with `temp` after the core loop and tool seam are stable

Deliverables:

- [ ] checkpoint markers in context history
- [ ] revert / rollback flow
- [ ] token usage persistence
- [ ] subagent tool
- [ ] MCP integration
- [ ] web search / fetch tools
- [ ] richer service config

## Recommended Teaching Order

The next teaching units should stay small, but they should follow dependency order.

### Immediate Next Units

1. `internal/runtime`: introduce a loop result model that can represent `continue`, `finish`, and `max steps reached`
2. `internal/runtime`: start consuming `config.LoopControl`
3. `internal/runtime`: define a minimal event sink interface
4. `internal/llm` + `internal/runtime`: add structured action output instead of text-only reply
5. `internal/tools`: introduce the first generic tool interface

### Important Constraint

Do not start with:

- shell UI
- ACP
- MCP
- subagents

before the runtime loop and tool boundary exist.

Those are adapter-heavy features. Building them before stabilizing the core will create churn.

## Module Classification

### Core Agent Logic

- `internal/runtime`
- future runtime event boundary
- future structured action model
- future tool execution orchestration

These should be treated as relatively stable.

### Replaceable Adapters

- `internal/llm/*`
- future `internal/tools/*`
- future `internal/ui/*`
- future MCP integration

These should remain behind explicit interfaces.

### Supporting Infrastructure

- `internal/app`
- `internal/config`
- `internal/session`
- `internal/contextstore`
- future `internal/agentspec`

These wire the system together but should not absorb loop-specific behavior.

## Definition Of "On Track"

The rewrite is on track if, in order:

1. Go runtime becomes multi-step before tools proliferate
2. tools are introduced behind a registry/interface boundary
3. agent spec chooses prompts and tools instead of hardcoded app logic
4. UI modes consume runtime events instead of poking into runtime internals
5. advanced features like MCP and subagents come last

If we keep that order, the Go rewrite should converge toward `temp` without a major architecture reset.
