# PLAN

## Purpose

This file tracks the migration gap between the current Go rewrite and the Python reference in `temp/`.

The old plan stopped at "build the initial CLI skeleton". That is now outdated. The Go codebase already has a working entry chain, config loading, session selection, JSONL history storage, a minimal LLM engine boundary, and a single-turn runtime. The plan now needs to answer a different question:

`What is still missing before fimi-cli reaches the real kimi-cli target?`

---

## Python Reference Architecture (from `temp/`)

Based on detailed analysis of the Python implementation:

### Core Loop (`soul/kimisoul.py`)

```
run() → _agent_loop() → _step() (循环直到 finished 或达到最大步数)

┌─────────────────────────────────────────────────────────┐
│ run(user_input, event_queue)                           │
│   1. _checkpoint()     ← 创建初始检查点                 │
│   2. append_message(user)                               │
│   3. _agent_loop()                                     │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│ _agent_loop()                                          │
│   while True:                                          │
│     StepBegin(step_no) → event_queue                  │
│     try:                                               │
│       _checkpoint()                                   │
│       set_n_checkpoints()                             │
│       finished = _step()                              │
│     except BackToTheFuture → revert + continue       │
│     except ChatProviderError/CancelledError → raise  │
│     if finished → return                             │
│     step_no++                                         │
│     if step_no > max_steps → MaxStepsReached         │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│ _step() → _step_impl() (带重试机制)                     │
│   kosong.step() → LLM 调用                            │
│   result.tool_results() → 等待工具结果                │
│   _grow_context() → 更新上下文                        │
│   fetch_pending_dmail() → 检查 D-Mail                 │
│   return not result.tool_calls (True=完成)           │
└─────────────────────────────────────────────────────────┘
```

**关键设计特点:**
- **重试机制**: 使用 `tenacity` 库，对特定 API 错误（429, 500, 502, 503）自动重试
- **中断屏蔽**: `_grow_context` 使用 `asyncio.shield` 防止上下文操作被中断
- **BackToTheFuture**: 异常机制实现时间旅行 - 当收到 D-Mail 时回滚到指定检查点

### Event System (`soul/event.py`)

```python
type ControlFlowEvent = StepBegin | StepInterrupted | StatusUpdate
type Event = ControlFlowEvent | ContentPart | ToolCall | ToolCallPart | ToolResult

class EventQueue:
    _queue: asyncio.Queue()
    put_nowait(event)      # 生产者（KimiSoul）调用
    async get() → Event    # 消费者（UI）调用
    shutdown()
```

### Tools Architecture (`tools/`)

| 工具 | 主要参数 | 特点 |
|------|----------|------|
| **Bash** | `command`, `timeout` (默认60s, 最大300s) | 异步子进程，流式输出收集 |
| **ReadFile** | `path`, `line_offset`, `n_lines` | 最多1000行, 每行最多2000字符 |
| **WriteFile** | `path`, `content`, `mode` (overwrite/append) | 路径安全检查 |
| **Glob** | `pattern`, `directory`, `include_dirs` | 禁止 `**` 开头 |
| **Grep** | `pattern`, `path`, `output_mode`, ... | 底层用 ripgrepy |
| **PatchFile** | `path`, `diff` | 使用 patch_ng 库 |
| **StrReplaceFile** | `path`, `edit` (old/new/replace_all) | 支持批量编辑 |
| **SearchWeb** | `query`, `limit`, `include_content` | moonshot 搜索 API |
| **FetchURL** | `url` | aiohttp + trafilatura |
| **Task** | `description`, `subagent_name`, `prompt` | 子代理独立 Context |
| **Think** | `thought` | 仅记录，返回空 |
| **SetTodoList** | `todos` | 渲染为纯文本 |
| **SendDMail** | `message`, `checkpoint_id` | 时间旅行消息 |

**共享工具 (`utils.py`):**
- `ToolResultBuilder` - 带字符/行限制的输出构建器
- `load_desc()` - 从 .md 文件加载工具描述
- `truncate_line()` - 行截断工具

### UI Architecture (`ui/`)

```
KimiSoul (Soul)
    |
    | async events via EventQueue
    v
run_soul() [ui/__init__.py]
    - spawns visualization task + run task concurrently
    - coordinates cancellation via asyncio.wait()
    |
    v
EventQueue [soul/event.py]
    |
    v
Visualization Loop
    ├── ShellApp._visualize (rich.Live)
    │     ├── StepLiveView: 实时 step 渲染
    │     ├── CustomPromptSession: prompt_toolkit
    │     └── MetaCommandRegistry: /help, /clear, /exit
    │
    └── PrintApp._visualize_text / _visualize_stream_json
```

### D-Mail 时间旅行 (`soul/denwarenji.py`)

```python
class DMail(BaseModel):
    message: str
    checkpoint_id: int  # 要发送到的检查点

class DenwaRenji:
    send_dmail(dmail)              # agent 调用
    fetch_pending_dmail() → DMail  # KimiSoul 调用
```

**工作流程:**
1. agent 调用 `send_dmail()` 发送消息到过去的检查点
2. `_step()` 结束后 `fetch_pending_dmail()` 检查待处理邮件
3. 如果有 → 抛出 `BackToTheFuture` 异常
4. `_agent_loop()` 捕获，调用 `revert_to()` 回滚
5. 添加 D-Mail 内容到历史，继续循环

---

## Current Status Snapshot

### Current Execution Focus

Date: 2026-03-25

Active phase: Phase 7, Event Stream And Print UI (mostly complete)

**Phase 7 已完成:**
- [x] `internal/runtime/events` 包创建
- [x] 事件 sink 边界定义（`Sink` 接口 + no-op 默认）
- [x] app/runtime 与 UI 消费者之间的协调器
- [x] `internal/ui/printui` 最小实现
- [x] 纯文本输出支持
- [x] 非流式事件：`step_begin`, `text_part`, `tool_call`, `tool_result`, `status_update`, `interrupted`
- [x] 流式 LLM seam：Text delta / ToolCall delta（SSE → events.Sink）
- [x] print UI：TextPart 流式输出（不对每个 delta 自动换行），避免重复打印

**Phase 7 待完成:**
- [ ] `stream-json` 输出模式（保留到后续 phase，一次性定 shape）

### Already Implemented In Go

- [x] `cmd/fimi -> internal/app.Run` entry chain
- [x] CLI argument parsing with `--help`, `--new-session`, `--model`, `--continue`
- [x] Config loading, defaults, validation, model/provider mapping
- [x] Explicit session creation/continue semantics with metadata-backed `last_session_id`
- [x] JSONL history persistence with bootstrap and recent-turn reads
- [x] Multi-step runtime loop with tool-call closure
- [x] LLM request/message construction boundary
- [x] OpenAI/Qwen-compatible provider
- [x] Tool registry and execution layer (7 tools: bash, read_file, glob, grep, write_file, replace_file, patch_file)
- [x] Tool output shaping (`OutputShaper`)
- [x] Workspace path guardrails
- [x] Token usage persistence
- [x] Checkpoint creation and revert
- [x] History file rotation backup
- [x] Agent spec loading from YAML with `extend` inheritance
- [x] System prompt template expansion
- [x] Basic event streaming to print UI

### Still Missing Versus `temp/kimi-cli`

**高优先级 (阻塞核心功能):**
- [ ] Agent spec parity: `exclude_tools` / `subagents`
- [ ] Shell UI (interactive prompt loop, liveview)
- [ ] Streaming LLM response (text/tool-call deltas)

**中优先级 (扩展能力):**
- [ ] Task/subagent delegation
- [ ] Web tools (search, fetch)
- [ ] Think/todo tools
- [ ] MCP integration
- [ ] Services config (beyond model providers)

**低优先级 (高级特性):**
- [ ] D-Mail 时间旅行机制
- [ ] ACP server mode
- [ ] Stream-json output format

---

## Gap Summary

```text
Current Go
  = CLI + explicit session + agentspec + multi-step runtime + local tools +
    context history/checkpoint + basic event stream + print UI

Target from temp
  = current core + streaming LLM + shell UI + web tools +
    task delegation + MCP + D-Mail + ACP
```

**核心差异:**

| 方面 | Go 实现 | Python 实现 |
|------|---------|-------------|
| **架构风格** | 依赖注入 + 接口边界清晰 | monolithic 模块 |
| **工具执行** | 同步 HandlerFunc | 类继承 + asyncio |
| **事件系统** | 基础事件定义 | 完整 EventQueue + 事件类型层次 |
| **UI 模式** | print 仅 | print + shell + ACP |
| **工具集** | 7 个本地工具 | 13+ 工具（含 web/task/dmail） |
| **时间旅行** | checkpoint/revert 仅 | D-Mail 消息机制 |
| **流式输出** | 非流式 | 完整 streaming |

---

## Reference Mapping

| Python Reference | Go Rewrite Target | Status |
| --- | --- | --- |
| `__init__.py` (entry) | `cmd/fimi` + `internal/app` | **done** |
| `config.py` | `internal/config` | **mostly done** (缺少 services) |
| `metadata.py` | `internal/session` | **done** |
| `agent.py` | `internal/agentspec` | **mostly done** (缺少 exclude_tools/subagents) |
| `soul/kimisoul.py` | `internal/runtime` | **core done** (缺少 streaming) |
| `soul/context.py` | `internal/contextstore` | **done** |
| `soul/event.py` | `internal/runtime/events` | **partial** (缺少流式事件) |
| `soul/message.py` | `internal/llm/tool_messages.go` | **done** |
| `soul/denwarenji.py` | - | **not started** |
| `tools/bash/` | `internal/tools/builtin.go` | **done** |
| `tools/file/read.py` | `internal/tools/builtin.go` | **done** |
| `tools/file/write.py` | `internal/tools/builtin.go` | **done** |
| `tools/file/glob.py` | `internal/tools/builtin.go` | **done** |
| `tools/file/grep.py` | `internal/tools/builtin.go` | **done** |
| `tools/file/patch.py` | `internal/tools/builtin.go` | **done** |
| `tools/file/replace.py` | `internal/tools/builtin.go` | **done** |
| `tools/web/` | - | **not started** |
| `tools/think/` | - | **not started** |
| `tools/todo/` | - | **not started** |
| `tools/task/` | - | **not started** |
| `tools/mcp.py` | - | **not started** |
| `ui/shell/` | - | **not started** |
| `ui/print/` | `internal/ui/printui` | **partial** (缺少 stream-json) |
| `ui/acp/` | - | **not started** |

---

## Recommended Build Order

1. **Streaming LLM seam** - 让事件流支持真正的流式输出
2. **Shell UI basics** - 交互式 prompt loop + liveview
3. **Agent spec parity** - exclude_tools + subagents
4. **Task tool** - 子代理派发
5. **Web tools** - search/fetch
6. **MCP integration** - 外部工具协议
7. **Advanced features** - D-Mail, ACP, stream-json

---

## Phased Roadmap

### Phase 0-6: Completed Foundation

- [x] Phase 0: `internal/app`, `config`, `session`, `contextstore`, `llm`, `runtime` basic
- [x] Phase 1: Agent Composition Layer (`agentspec`)
- [x] Phase 2: Runtime Loop Kernel (multi-step)
- [x] Phase 3: Tool Runtime Boundary
- [x] Phase 4: Minimum Useful Tool Set (7 tools)
- [x] Phase 5: Richer Context Store (checkpoint/revert)
- [x] Phase 6: Session Metadata And Continue Semantics

### Phase 7: Event Stream And Print UI

Status: **in progress**

Goal: make runtime observable before building the full interactive shell.

- [x] create `internal/runtime/events`
- [x] define an event sink boundary with a no-op default
- [x] add coordinator between app/runtime and UI consumers
- [x] create minimal `internal/ui/printui`
- [x] support plain text output
- [x] emit non-streaming events: step_begin, text, tool_call, tool_result, status
- [ ] widen runtime/LLM seam for streaming (text deltas, tool-call deltas)
- [ ] defer `stream-json` until the event boundary is stable

**Python 对比:**
```
Python EventQueue:
  - asyncio.Queue based
  - Event types: StepBegin, StepInterrupted, StatusUpdate,
    TextPart, ToolCall, ToolCallPart, ToolResult

Go events.Sink:
  - interface-based
  - Events: StepBegin, TextPart, ToolCall, ToolResult, Status, Interrupted
  - Missing: ToolCallPart (streaming)
```

### Phase 8: Shell UI

Goal: add the main interactive interface.

- [ ] create `internal/ui/shell`
- [ ] interactive prompt loop (prompt_toolkit equivalent)
- [ ] render step/tool progress (rich.Live equivalent)
- [ ] input history persistence
- [ ] meta commands (`/help`, `/clear`, `/exit`)

**Python 参考 (`ui/shell/`):**
```
shell/
├── __init__.py    # ShellApp class
├── console.py     # rich.Console wrapper
├── prompt.py      # CustomPromptSession (prompt_toolkit)
├── liveview.py    # StepLiveView (rich.Live)
└── metacmd.py     # @meta_command decorator
```

Keep out of this phase unless required:
- ACP
- MCP
- subagents

### Phase 9: Agent Spec Parity And Delegation

Goal: close capability gaps for delegation.

- [ ] extend `internal/agentspec.Spec` with `ExcludeTools` and `Subagents`
- [ ] add `SubagentSpec { Path, Description }` and resolve paths relative to declaring YAML
- [ ] match Python inheritance semantics: merge `system_prompt_args`; overwrite `tools`, `exclude_tools`, `subagents`
- [ ] apply `exclude_tools` during app tool resolution
- [ ] add `task` tool contract with `description`, `subagent_name`, `prompt`
- [ ] add isolated subagent execution with fresh history/context

**Python 参考 (`tools/task/__init__.py`):**
```python
class Task(CallableTool2[Params]):
    async def __call__(params):
        # 1. Load subagent spec by name
        # 2. Create fresh Context (独立 history 文件)
        # 3. Run subagent loop
        # 4. Return final summary
```

### Phase 10: Advanced Tools And Service Config

Goal: add external-network capabilities.

- [ ] add `services` config surface (search service config)
- [ ] `web/search` tool (moonshot search API)
- [ ] `web/fetch` tool (aiohttp + trafilatura equivalent)
- [ ] `think` tool (simple thought logging)
- [ ] `todo` tool (todo list management)
- [ ] MCP tool adapter / CLI MCP config loading

**Python 参考 (`tools/web/`):**
```python
class SearchWeb:
    params: query, limit (default 5, max 20), include_content
    uses: moonshot search API via aiohttp

class FetchURL:
    params: url
    uses: aiohttp + trafilatura for content extraction
```

### Phase 11: D-Mail Time Travel (Optional)

Goal: implement the unique time-travel messaging feature.

- [ ] `DenwaRenji` equivalent in Go
- [ ] `BackToTheFuture` error type
- [ ] `dmail` tool
- [ ] integrate with checkpoint/revert flow

**Python 参考 (`soul/denwarenji.py`):**
```python
class DMail(BaseModel):
    message: str
    checkpoint_id: int

class DenwaRenji:
    def send_dmail(self, dmail): ...
    def fetch_pending_dmail(self) -> DMail | None: ...
```

### Phase 12: Transport Parity

Goal: support machine-facing execution modes.

- [ ] ACP server mode
- [ ] `stream-json` output format
- [ ] cancellation and interruption across transport boundaries

---

## Immediate Next Steps

Based on the analysis, the recommended next teaching units are:

1. **Streaming LLM Seam** - 让 runtime 能够接收流式 LLM 响应并发出 text/tool-call delta 事件
2. **Shell UI Basics** - 交互式 prompt + liveview
3. **Agent Spec Extensions** - exclude_tools + subagents 字段
4. **Task Tool** - 子代理派发协议

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
  +-- internal/tools
  +-- internal/contextstore
  +-- internal/llm
  +-- internal/ui/printui
  |
  v
internal/runtime
  |
  +-- reads/writes internal/contextstore
  +-- calls internal/llm
  +-- executes internal/tools
  +-- emits to internal/runtime/events
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
  |     +-- printui             done
  |     +-- shell               TODO
  |     +-- acp                 TODO
  |
  v
internal/runtime                core agent logic, stable
  |
  +-- internal/runtime/events   core boundary, stable (partial)
  +-- internal/contextstore     core logic + persistence
  +-- internal/llm              adapter boundary, replaceable
  +-- internal/tools/*          adapter/integration, replaceable
  |     +-- builtin.go          done (7 tools)
  |     +-- web.go              TODO
  |     +-- task.go             TODO
  |     +-- mcp.go              TODO
```

---

## Design Notes

Good migration discipline:

- keep runtime unaware of shell rendering
- keep tools behind explicit interfaces
- keep provider-specific HTTP code out of runtime
- keep history persistence append-first and testable
- emit events, don't call UI directly

Bad migration shortcuts to avoid:

- turning `internal/app` into a giant god package
- hardcoding tools directly inside runtime branches
- implementing shell UI before runtime events exist
- adding subagents before the main agent loop is stable
- bypassing event sink for direct UI calls
