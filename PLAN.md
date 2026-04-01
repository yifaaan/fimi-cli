# PLAN

## 2026-04-01 Shell UI Screenshot Parity Plan

### Goal

Make `fimi` shell UI match the provided screenshot as closely as possible in the transcript area:

- user input is rendered as a clearly distinct muted bubble/block
- assistant output is rendered as narrative bullet paragraphs rather than plain role-tagged lines
- visible "think" / analysis flow and tool activity are rendered inline in the same conversation stream
- tool details, command previews, search/read summaries, and edit diffs are shown in the screenshot's grouped style

The screenshot should be treated as the visual source of truth for spacing, separators, prefixes, grouping rhythm, and preview density.

### Current State

- `internal/ui/shell/model_output.go` still uses a flat `[]TranscriptLine` model and renders one line type at a time.
- `internal/ui/shell/model_runtime.go` converts runtime events into `stepLines`, but the mapping is still mostly `one event -> one line`.
- `internal/ui/shell/styles/colors.go` and `internal/ui/shell/styles/lipgloss.go` define generic shell styles, not screenshot-specific transcript blocks.
- `internal/runtime/events/events.go` already has `ToolResult.DisplayOutput`, and `internal/runtime/runtime.go` already forwards it.
- `internal/tools/builtin_patch.go` already generates a richer `DisplayOutput` for patch diffs, but most other tools still only provide coarse plain-text output.
- Current shell UI still exposes implementation-oriented affordances such as `Step N`, raw tool-card rows, and status bar text that do not match the screenshot's transcript-first presentation.

### Gap Analysis

The main gap is not only styling. The current transcript data model is too shallow for the target UI.

To reproduce the screenshot, shell needs concepts like:

- user message blocks
- assistant narrative blocks
- grouped activity sections such as `Explored`, `Ran ...`, `Edited ...`
- nested detail rows such as `Read foo.go`, `Search ...`
- inline preview bodies for command output and file diffs
- elapsed-time dividers such as `Worked for 1m 11s`

Those concepts do not map cleanly onto the current `LineTypeUser / LineTypeAssistant / LineTypeToolCall / LineTypeToolResult` model.

### Design Decisions

#### 1. Move From Line Model To Block Model

Refactor the shell transcript renderer from a flat line list to richer transcript blocks. A minimal shape can be:

- `UserPromptBlock`
- `AssistantNoteBlock`
- `ActivityGroupBlock`
- `DividerBlock`
- `ElapsedBlock`
- `SystemNoticeBlock`
- `ErrorBlock`

Each activity group should hold:

- semantic title, for example `Explored`, `Ran pwd && rg --files`, `Edited PLAN.md`
- ordered activity items, for example `Read`, `Search`, `Write`, `Patch`
- optional preview body, for example command output snippet or inline diff
- render metadata such as accent color and truncation mode

#### 2. Separate Narrative From Tool Activity

Visible assistant prose and visible tool activity should no longer be flattened into the same primitive.

- `runtimeevents.TextPart` should accumulate into assistant narrative bullet paragraphs.
- Consecutive tool calls/results should aggregate into activity groups until a new assistant paragraph or step boundary appears.
- `think` should not render as the current raw tool-call text. It should be mapped to a screenshot-style visible analysis/note block.

Important constraint:

- the current provider/streaming layer does not expose hidden chain-of-thought
- the visible "think process" can therefore only come from assistant text plus explicit `think` tool content
- if future providers expose reasoning summaries, add a dedicated runtime event kind instead of overloading `TextPart`

#### 3. Define Activity Grouping Rules

Initial grouping rules:

- `read_file`, `glob`, `grep`, `search_web`, `fetch_url` -> group as `Explored`
- `bash` -> group as `Ran <command>`
- `write_file`, `replace_file`, `patch_file` -> group as `Edited` or `Updated`
- `set_todo_list` -> group as `Planned`
- `agent` -> group as `Delegated`

Nested detail rows should use screenshot-style tree/list rendering such as:

- `Read internal/ui/shell/model_output.go`
- `Search renderLiveStatus|renderStatusBar in shell`
- `Read patch_test.go`

#### 4. Use `DisplayOutput` As The UI Preview Payload

Keep `Output` as the model-facing/full-fidelity result, and treat `DisplayOutput` as shell UI preview payload.

Plan:

- continue preserving full tool result text in `Output`
- populate `DisplayOutput` for all important tools
- render inline previews from `DisplayOutput`
- never require shell to reverse-engineer structure from coarse human text if the tool can provide a richer preview directly

This is the fastest path to screenshot parity without redesigning history persistence.

#### 5. Screenshot-Oriented Visual Language

Update shell transcript styling to match the screenshot:

- user message: full-width muted background block, padded, no explicit role label
- assistant note: plain white/soft foreground with leading bullet `•`
- activity group title: stronger title line, subtle accent on verbs or command text
- nested detail rows: tree glyphs and compact indented previews
- divider: full-width rule between logical chunks
- elapsed row: `Worked for 1m 11s`

The current rounded tool card presentation should be removed from the shell transcript path.

### File-Level Modification Plan

#### `internal/ui/shell/model_output.go`

- replace the current line-centric render path with a block-centric render path
- keep viewport, scrollback, and "latest turn stays interactive" behavior
- add rendering for user bubble, assistant bullets, grouped activities, inline previews, diff bodies, dividers, and elapsed rows
- update tests that currently assert the old compact line layout

#### `internal/ui/shell/model_runtime.go`

- replace `stepLines []TranscriptLine` with richer pending turn state
- maintain grouping state for the current assistant note and current activity group
- consume `ToolResult.DisplayOutput`
- map tool names to semantic activity titles and nested item labels
- add elapsed/progress support for screenshot-style `Worked for ...` separators

#### `internal/ui/shell/model.go`

- keep the existing runtime/wire lifecycle
- reduce reliance on `renderLiveStatus()` and `renderStatusBar()` for user-visible progress
- move more progress signaling into transcript blocks
- ensure redraw cadence supports elapsed divider updates without breaking input responsiveness

#### `internal/ui/shell/styles/colors.go`

- add dedicated palette tokens for transcript foreground, muted foreground, divider, user bubble background, activity accent, command accent, diff add, and diff remove

#### `internal/ui/shell/styles/lipgloss.go`

- replace generic transcript styles with dedicated block styles such as:
  - `UserBubbleStyle`
  - `AssistantBulletStyle`
  - `ActivityTitleStyle`
  - `ActivityVerbStyle`
  - `ActivityDetailStyle`
  - `DividerStyle`
  - `ElapsedStyle`
  - `DiffAddStyle`
  - `DiffRemoveStyle`

#### `internal/runtime/events/events.go`

- keep `ToolResult.DisplayOutput`
- only add a new event kind if the screenshot parity work proves that assistant text + tool events still cannot represent visible reasoning cleanly

#### `internal/runtime/runtime.go`

- keep forwarding `DisplayOutput`
- if a new event kind is introduced later, emit it here from streaming/tool-step assembly

#### `internal/tools/builtin.go`

- add `DisplayOutput` for foreground `bash`
- command preview should show compact stdout/stderr snippets, line counts, and truncation markers in screenshot-friendly form
- background task query / foreground command output should remain distinguishable

#### `internal/tools/builtin_readonly.go`

- add `DisplayOutput` for `read_file`, `glob`, `grep`, `search_web`, and `fetch_url`
- previews should be compact and line-count aware
- file-path listings should support `... +N lines` truncation like the screenshot

#### `internal/tools/builtin_write.go`

- add `DisplayOutput` for `write_file` and `replace_file`
- `replace_file` should expose a small inline diff or line-level replacement preview instead of only `"replaced N occurrence(s)"`

#### `internal/tools/builtin_patch.go`

- keep the existing patch diff preview path
- normalize its preview formatting so it fits the new transcript block renderer

#### `internal/ui/printui/printui.go`

- keep plain-text print mode stable
- only update it if any new runtime event kind requires a textual fallback

### Implementation Order

1. Freeze the target transcript spec from the screenshot

- document exact prefixes, spacing, divider behavior, truncation rules, and activity titles
- create golden examples for one representative tool-heavy turn

2. Build the new transcript primitives and renderer

- introduce block types and renderers in shell UI
- preserve scrolling and interactive-tail behavior while changing only the presentation layer

3. Enrich tool preview payloads

- populate `DisplayOutput` across read/search/bash/write/replace/patch tools
- centralize preview helpers so truncation and formatting are consistent

4. Add grouping and elapsed behavior

- aggregate consecutive tool events into semantic groups
- insert dividers and elapsed markers at the same rhythm as the screenshot

5. Polish styles against real terminal output

- tune colors, padding, wrap width, and preview density
- remove remaining rounded tool-card assumptions from shell transcript

6. Add regression coverage

- snapshot/golden tests for `InteractiveView()`
- unit tests for grouping rules, preview truncation, diff rendering, and elapsed formatting
- one end-to-end shell test for a multi-step turn with assistant note + explored block + command block + edit diff

### Acceptance Criteria

- submitted user messages are visually distinct and rendered as muted blocks like the screenshot
- assistant narrative is rendered as bullet-style prose instead of plain assistant lines
- visible "think" / analysis flow appears inline in the transcript rather than as raw tool-card text
- tool activity is grouped into sections such as `Explored`, `Ran ...`, and `Edited ...`
- command output previews and file-edit diffs are shown inline, with compact truncation behavior
- the shell still supports scrolling, late-event commit, and older-turn scrollback
- `printui` and persisted history stay compatible with the richer shell presentation

### Risks And Constraints

- exact screenshot parity depends on richer per-tool preview payloads; styling alone will not be enough
- the current provider stack does not expose hidden chain-of-thought, so visible reasoning can only be built from assistant text and explicit `think` activity
- replacing the transcript presentation model will touch multiple tests because current assertions assume one-line-per-event rendering


# kimi-cli 实现参考（基于 `temp/` 快照）

更新时间：2026-04-01

本文档记录 `temp/` 里的 Python 版 `kimi-cli` 关键实现，并同步标注它和当前 Go 版 `fimi-cli` 的真实差异。用途有两个：

- 看 Python 参考实现到底是怎么工作的
- 判断 Go 版接下来最值得补哪一块 parity

---

## 1. 参考实现入口

Python 参考实现主要分布在这些目录：

- `temp/src/kimi_cli/soul/`：runtime 核心，含 agent loop、wire、approval、context、D-Mail
- `temp/src/kimi_cli/tools/`：内置工具和 MCP 适配
- `temp/src/kimi_cli/ui/`：print、shell、acp 三套 UI
- `temp/src/kimi_cli/config.py`：配置模型
- `temp/src/kimi_cli/agent.py`：agent spec、system prompt、工具装载

主调用链：

```text
CLI/App
  -> Agent / AgentGlobals
  -> KimiSoul.run()
  -> _agent_loop()
  -> _step()
  -> kosong.step(...)
  -> tool calls / wire events / approval requests
  -> UI 消费 wire 并渲染
```

---

## 2. Runtime 核心

### 2.1 `soul/kimisoul.py`

核心类：`KimiSoul`

职责：

- 管理一次用户输入对应的 agent loop
- 在 run 开始前和每个 step 前做 checkpoint
- 调 LLM step
- 等待工具结果并扩展 context
- 处理 D-Mail 回滚
- 通过 `Wire` 把实时事件发给 UI
- 把 approval queue 转发到 wire

关键流程：

```text
run(user_input, wire)
  1. checkpoint()
  2. append user message
  3. current_wire.set(wire)
  4. _agent_loop(wire)

_agent_loop(wire)
  while True:
    - wire.send(StepBegin(step_no))
    - spawn _pipe_approval_to_wire()
    - checkpoint()
    - denwa_renji.set_n_checkpoints(...)
    - _step(wire)
    - 若收到 BackToTheFuture:
        revert_to(checkpoint)
        checkpoint()
        append injected future message
        continue
    - finished => return
```

### 2.2 `_step()` 的行为

`_step()` 是单步推理的核心：

1. 用 tenacity 包装 `kosong.step()`
2. 把文本流、tool call、tool result 都直接发到 wire
3. 等待所有 tool results
4. `asyncio.shield(_grow_context(...))`，尽量避免 context 写一半被取消
5. 检查是否有工具被拒绝
6. 检查是否有待处理 D-Mail
7. 若无后续 tool call，则结束本次 run

### 2.3 重试机制

Python 参考实现已经有：

- exponential backoff
- jitter
- 仅对连接类 / 超时 / 429 / 5xx 做重试

对应代码在 `temp/src/kimi_cli/soul/kimisoul.py`。

---

## 3. Wire 与 Approval

### 3.1 `soul/wire.py`

Python 没有单独的 `event.py`；事件类型直接定义在 `wire.py`。

核心对象：

```python
class Wire:
    def __init__(self): self._queue = asyncio.Queue[WireMessage]()
    def send(self, msg): ...
    async def receive(self): ...
    def shutdown(self): ...
```

还有：

```python
current_wire = ContextVar[Wire | None]("current_wire", default=None)
```

`WireMessage = Event | ApprovalRequest`

其中 `Event` 包括：

- `StepBegin`
- `StepInterrupted`
- `StatusUpdate`
- `ContentPart`
- `ToolCall`
- `ToolCallPart`
- `ToolResult`

也就是说，Python 的 wire 同时承载：

- runtime 事件
- tool call 增量
- approval 请求

### 3.2 `soul/approval.py`

核心类：`Approval`

能力：

- `yolo`
- session 级 auto-approve
- `ApprovalRequest` 排队
- UI resolve 后继续执行

语义：

```python
await approval.request(action, description) -> bool
```

Python 版 approval 依赖 `soul/toolset.py` 里的 `current_tool_call`，保证请求里带上正确的 `tool_call_id`。

---

## 4. Context / Checkpoint / D-Mail

### 4.1 `soul/context.py`

`Context` 负责：

- JSONL 历史持久化
- `_checkpoint` 记录
- `_usage` 记录
- `checkpoint()`
- `revert_to(checkpoint_id)`

checkpoint 行为：

- 追加 `_checkpoint`
- 可选再写一个用户侧可见的 `CHECKPOINT N`

`revert_to()` 会：

- 轮转原 history 文件
- 重新写入新文件
- 只保留目标 checkpoint 之前的内容

### 4.2 `soul/denwarenji.py`

`DenwaRenji` 很小，但很关键。

内部状态：

- `_pending_dmail`
- `_n_checkpoints`

接口：

- `send_dmail(...)`
- `set_n_checkpoints(...)`
- `fetch_pending_dmail()`

工作方式：

- 工具先塞入一个待处理 D-Mail
- runtime 在 step 后检查 pending D-Mail
- 命中后抛 `BackToTheFuture`
- 主循环负责回滚 checkpoint，并注入“来自未来”的 system 指令

---

## 5. Agent / Tool 装载

### 5.1 `agent.py`

`temp/src/kimi_cli/agent.py` 负责：

- 解析 agent spec
- 处理 `extend`
- 读取 system prompt 模板
- 注入 builtin args
- 装载工具
- 装载 MCP 工具

关键数据结构：

- `AgentSpec`
- `SubagentSpec`
- `Agent`
- `AgentGlobals`
- `BuiltinSystemPromptArgs`

### 5.2 system prompt 内置变量

Python 版内置变量包括：

- `KIMI_NOW`
- `KIMI_WORK_DIR`
- `KIMI_WORK_DIR_LS`
- `KIMI_AGENTS_MD`

### 5.3 工具装载机制

Python 工具不是写死注册表，而是按 agent spec 里的 `"package:ClassName"` 动态装载。

这和当前 Go 的静态 registry + handler 装配是明显不同的设计。

---

## 6. Python 工具能力

主要工具如下：

| 工具 | 文件 | 说明 |
| --- | --- | --- |
| `Bash` | `tools/bash/__init__.py` | shell 命令，60s 默认超时，流式 stdout/stderr，审批 |
| `ReadFile` | `tools/file/read.py` | 文件读取，offset/limit，1000 行和 100KB 级别上限 |
| `WriteFile` | `tools/file/write.py` | 写/追加文件 |
| `Glob` | `tools/file/glob.py` | glob 匹配，路径沙箱 |
| `Grep` | `tools/file/grep.py` | ripgrep 包装，支持更多过滤参数 |
| `StrReplaceFile` | `tools/file/replace.py` | replace / replace_all / batch edits |
| `PatchFile` | `tools/file/patch.py` | unified diff patch |
| `Think` | `tools/think/__init__.py` | 私有思考 |
| `SetTodoList` | `tools/todo/__init__.py` | 替换整个 todo 列表 |
| `Task` | `tools/task/__init__.py` | 子 agent 调度，短回复 continuation |
| `SendDMail` | `tools/dmail/__init__.py` | 发送 D-Mail |
| `SearchWeb` | `tools/web/search.py` | Moonshot Search API |
| `FetchURL` | `tools/web/fetch.py` | `trafilatura` 文本抽取 |
| `MCPTool` | `tools/mcp.py` | MCP 结果转 Text/Image/Audio content block |

几个要点：

- `Task` 会给 subagent 建独立 context，并把 approval request 转回主 wire
- `SearchWeb` 依赖 `services.moonshot_search`
- `FetchURL` 用 `trafilatura`
- `MCPTool` 会保留 richer content，而不是简单转字符串

---

## 7. Python UI

### 7.1 Print UI

位置：`temp/src/kimi_cli/ui/print/__init__.py`

支持：

- `text`
- `stream-json`
- 从 stdin 读输入
- print 模式直接把 approval 设成 `yolo`

### 7.2 Shell UI

位置：`temp/src/kimi_cli/ui/shell/`

已实现特征：

- REPL
- wire 事件消费
- live tool call 展示
- prompt history
- `@` 文件补全
- `/help` `/version` `/release-notes` `/init` `/clear` `/compact` `/setup` `/reload`
- raw shell mode / agent mode 双模式切换
- 后台 update 检查

### 7.3 Setup Wizard

位置：`temp/src/kimi_cli/ui/shell/setup.py`

流程不是静态提示，而是：

1. 选平台
2. 输 API key
3. 请求 `GET /models`
4. 从远端模型列表中选 model
5. 保存 `provider` / `model` / `max_context_size`
6. 如果平台支持，还会自动配置 `services.moonshot_search`

### 7.4 Auto Update

位置：`temp/src/kimi_cli/ui/shell/update.py`

能力：

- 启动后后台检查最新版本
- 下载 tar.gz
- 安装到 `~/.local/bin/kimi`
- 通过 toast 提示升级

### 7.5 ACP

位置：`temp/src/kimi_cli/ui/acp/__init__.py`

特点：

- 多 session
- 事件投影到 ACP 协议结构
- `ToolCallPart` 流式拼接
- 审批请求可透传
- richer content block 映射更完整

---

## 8. Python 配置模型

位置：`temp/src/kimi_cli/config.py`

关键字段：

- `default_model: ""`
- `models: {}`
- `providers: {}`
- `loop_control.max_steps_per_run = 100`
- `loop_control.max_retries_per_step = 3`
- `services.moonshot_search`

关键语义：

- 首次运行默认配置是空的
- 需要靠 `/setup` 完成初始化
- provider 类型是 Kimi CLI 自己的语义：`kimi` / `openai_legacy` / `_chaos`

---

## 9. 与当前 Go 实现对照后的修正结论

下面这部分是本次更新最重要的地方：它修正了旧文档里已经过时的判断。

### 9.1 Go 已经补齐的 parity

当前 Go 版已经做完，或者已经明显不再落后于 Python 的部分：

- runtime 多 step loop
- step retry 的 backoff + jitter
- context 写入 shield 等价保护
- D-Mail / rollback
- wire + approval
- shell approval panel
- subagent continuation prompt
- `/setup`
- `/reload`
- `/compact`
- `/rewind`
- `/resume`
- `/task`
- toast
- prompt history
- `@` file completer
- ACP 多 session 基本框架

旧版 `kimi.md` / `PLAN.md` 里把下面几项写成 “Go 还没有”，现在都不准确了：

- exponential backoff + jitter
- shield-like context protection
- approval panel
- prompt history
- `@` completer
- `/reload`

### 9.2 当前真正还没对齐的点

和 `temp/` 对照后，Go 现在最明显的缺口是：

1. temp 风格的 provider / setup / config 流程
2. ACP 事件/审批/content block 完整投影
3. raw shell mode / agent-shell 双模式切换
4. auto-update

### 9.3 最大的用户侧差距：provider / setup / config 链路

这是现在最值得优先做的一块。

Python 参考实现里：

- `/setup` 是平台导向的
- 会真实请求 `/models`
- 会落盘 `max_context_size`
- 会在支持的平台下自动写入 `services.moonshot_search`
- 默认配置是空的，强依赖 setup 完成初始化

而当前 Go 版：

- `config.Default()` 仍带占位默认模型和 placeholder 路径
- `/setup` 只是本地静态向导，不会请求 provider 的模型列表
- 不会写入 `ContextWindowTokens`
- 没有 `services.moonshot_search`
- `/setup` 甚至提供了 `anthropic` 选项，但 runtime 并不支持该 provider

如果目标是继续对齐 `temp/` 的 kimi-cli，这里是当前最核心的缺口。

### 9.4 ACP 仍是 partial parity

当前 Go ACP 已经有：

- `initialize`
- `new_session`
- `list_sessions`
- `resume_session`
- `load_session`
- `prompt`
- `cancel`
- `set_session_model`
- `set_session_mode`

但和 Python 参考实现相比仍缺：

- `ApprovalRequest` 在 ACP 路径上的处理
- `ToolCallPart` 流式参数拼接
- richer content block 映射
- MCP 图片/音频内容的结构化透传

### 9.5 一些明确的主动分歧

这些不一定要改，但需要文档里明确：

- `search_web`：Go 用 DuckDuckGo；Python 用 Moonshot Search
- `fetch_url`：Go 用启发式抽取；Python 用 `trafilatura`
- `bash`：Go 默认 120s，Python 默认 60s
- `bash`：Go 多了后台任务模式和 `/task`
- `write_file` / `replace_file`：Go 有 approval gate，Python 当前主要是 `bash` 走 approval
- MCP：Go 目前把结果压成字符串；Python 会保留 text/image/audio content

---

## 10. 对 Go 版最有价值的参考文件

如果后面继续补 parity，最该反复对照的是：

### Runtime

- `temp/src/kimi_cli/soul/kimisoul.py`
- `temp/src/kimi_cli/soul/wire.py`
- `temp/src/kimi_cli/soul/approval.py`
- `temp/src/kimi_cli/soul/context.py`
- `temp/src/kimi_cli/soul/denwarenji.py`

### 配置与启动

- `temp/src/kimi_cli/config.py`
- `temp/src/kimi_cli/ui/shell/setup.py`
- `temp/src/kimi_cli/tools/web/search.py`

### ACP

- `temp/src/kimi_cli/ui/acp/__init__.py`
- `temp/src/kimi_cli/tools/mcp.py`

### Shell 交互

- `temp/src/kimi_cli/ui/shell/metacmd.py`
- `temp/src/kimi_cli/ui/shell/prompt.py`
- `temp/src/kimi_cli/ui/shell/update.py`

---

## 11. 当前结论

“下一步先做什么”其实有两个答案，取决于你优先看哪种差距。

### 11.1 如果按代码正确性 / parity 完整度排

应该先做：

- ACP parity

因为当前 ACP 路径仍然有真正的行为缺口：

- approval 没有完整接通
- `ToolCallPart` 没有投影
- richer content block 没保真

这比一般的 shell polish 更像一个还没收口的 subsystem。

### 11.2 如果按终端用户第一感受排

应该先做：

- temp 风格的 provider + `/setup` + model discovery + search config

因为这是 `temp/` 参考实现里最有辨识度的产品能力，也是当前最大的用户侧落差。

### 11.3 我的建议

综合当前代码状态，我会建议：

1. 先补 ACP parity
2. 紧接着补 provider / setup / config parity

这样顺序更稳：

- 先把还存在行为缺口的 transport 收口
- 再把最显著的配置与初始化体验差距补上


Updated: 2026-04-01

Purpose: 跟踪 `temp/` 中 Python `kimi-cli` 参考实现和当前 Go 版 `fimi-cli` 的真实差距，并给出下一步优先级。

---

## 1. 当前状态结论

Go 版现在已经不是“runtime 还没搭起来”的阶段了。

更准确的判断是：

- runtime 主链已经基本成型
- shell 主链也已经基本成型
- 当前剩下的缺口主要集中在 ACP 深水区、provider/setup/config parity，以及少量 temp 语义对齐项

---

## 2. 已完成或基本完成的 parity

下面这些能力，Go 版已经做完，或者已经不再是主要差距：

### Runtime / Core

- [x] 多 step runtime loop
- [x] 用户 prompt 边界 checkpoint
- [x] step 前 checkpoint
- [x] D-Mail / rollback
- [x] streaming text / tool call / tool result
- [x] step retry + backoff + jitter
- [x] context write shielding
- [x] context usage / retry status 回传

关键实现：

- `internal/runtime/runtime.go`
- `internal/dmail/dmail.go`
- `internal/contextstore/`

### Wire / Approval

- [x] bidirectional wire
- [x] `ApprovalRequest.Wait/Resolve`
- [x] yolo / auto-approve / approve-for-session
- [x] shell 审批面板

关键实现：

- `internal/wire/wire.go`
- `internal/wire/message.go`
- `internal/approval/approval.go`
- `internal/ui/shell/model.go`

### Shell

- [x] `/compact`
- [x] `/rewind`
- [x] `/resume`
- [x] `/task`
- [x] `/init`
- [x] `/version`
- [x] `/release-notes`
- [x] `/setup`
- [x] `/reload`
- [x] prompt history
- [x] `@` 文件补全
- [x] toast
- [x] tool cards
- [x] approval UI

关键实现：

- `internal/ui/shell/model.go`
- `internal/ui/shell/shell.go`
- `internal/ui/shell/history.go`
- `internal/ui/shell/completer/`
- `internal/ui/shell/toast.go`

### Tools / Integration

- [x] `agent`
- [x] `think`
- [x] `set_todo_list`
- [x] `bash`
- [x] `read_file`
- [x] `glob`
- [x] `grep`
- [x] `write_file`
- [x] `replace_file`
- [x] `patch_file`
- [x] `send_dmail`
- [x] `search_web`
- [x] `fetch_url`
- [x] MCP bridge

关键实现：

- `internal/tools/registry.go`
- `internal/tools/builtin.go`
- `internal/tools/agent.go`
- `internal/tools/mcp.go`
- `internal/mcp/`

### ACP 基础框架

- [x] `initialize`
- [x] `new_session`
- [x] `list_sessions`
- [x] `resume_session`
- [x] `load_session`
- [x] `prompt`
- [x] `cancel`
- [x] `set_session_model`
- [x] `set_session_mode`

关键实现：

- `internal/acp/server.go`
- `internal/acp/session.go`
- `internal/acp/types.go`

---

## 3. 旧计划里已过时的判断

这几项不该再放在 “待完成” 里：

- `background task management in ShellApp`
- `/task` background task browser

因为它们已经存在：

- `bash` 支持 `background:true`
- `bash` 支持 `task_id`
- shell 已有 `/task` list / status / kill
- session 级 `BackgroundManager` 已接入 runner

相关实现：

- `internal/tools/background.go`
- `internal/tools/builtin.go`
- `internal/app/app.go`
- `internal/ui/shell/model.go`

---

## 4. 当前真正还没补齐的差距

### A. ACP parity 仍然明显不完整

这是当前最实质的缺口。

Python `temp` 里 ACP 路径具备：

- approval request 处理
- `ToolCallPart` 流式参数拼接
- 更完整的 tool-call 状态投影
- richer content block 映射

当前 Go ACP 仍缺：

- [ ] ACP 路径注入 approval 上下文
- [ ] ACP 侧消费 `ApprovalRequest`
- [ ] ACP 投影 `ToolCallPart`
- [ ] richer MCP content 映射，而不是简单压成字符串

当前症状：

- ACP 只投影 `TextPart` / `ToolCall` / `ToolResult`
- `ApprovalRequest` 在 ACP 路径上没有真正接通
- `ToolCallPart` 被忽略
- 非文本 content block 没有完整保真

关键文件：

- `internal/app/app.go`
- `internal/acp/session.go`
- `internal/ui/run.go`
- `internal/tools/mcp.go`

### B. Provider / Setup / Config 还没对齐 temp

这是最大的用户侧产品差距。

Python `temp` 里：

- 默认配置为空
- `/setup` 是必经路径
- `/setup` 会请求远端 `/models`
- 会保存模型的 context window
- 会自动配置 `services.moonshot_search`
- provider 语义是 temp 自己的那套平台导向语义

当前 Go 版：

- [ ] 默认配置仍是 placeholder/内建默认模型语义
- [ ] `/setup` 还是静态本地向导
- [ ] 不会拉取远端模型列表
- [ ] 不会写 `ContextWindowTokens`
- [ ] 没有 `services.moonshot_search`
- [ ] `/setup` 暴露了 `anthropic` 选项，但 runtime 并未支持该 provider

关键文件：

- `internal/config/config.go`
- `internal/ui/shell/model.go`
- `internal/app/llm_builder.go`
- `temp/src/kimi_cli/config.py`
- `temp/src/kimi_cli/ui/shell/setup.py`

### C. Shell raw mode / dual-mode 还没做

Python `temp` 里有 agent mode / shell mode 切换。

当前 Go 版：

- [ ] 没有 `Ctrl-K` 模式切换
- [ ] 没有 temp 那种 raw shell mode

这不是阻塞主链的缺口，但确实仍未对齐。

### D. Auto-update 还没做

Python `temp` 里有：

- 后台版本检查
- 下载 tar.gz
- 安装到本地
- toast 提示更新

当前 Go 版：

- [ ] 没有等价 auto-update 流程

注意不要和已经存在的 background bash task 混淆。

### E. 少量工具语义仍偏薄

这些工具已经“有同名能力”，但还没完全对齐 temp 的语义：

- [ ] `read_file`：还没有 temp 那套 offset/limit/cat -n 风格输出
- [ ] `grep`：参数能力比 temp 少很多
- [ ] `search_web.include_content`：Go 的 DuckDuckGo backend 当前实际忽略
- [ ] `fetch_url`：Go 是 heuristic extractor，不是 `trafilatura`
- [ ] MCP 结果：Go 目前多媒体内容被压成字符串说明

这块建议放在 ACP 和配置链路之后做。

---

## 5. 明确的主动分歧

这些不一定要改，但计划里应该明确它们是“主动分歧”而不是“漏实现”：

- `search_web`：Go 用 DuckDuckGo；Python 用 Moonshot Search
- `fetch_url`：Go 用自写启发式抽取；Python 用 `trafilatura`
- `bash`：Go 默认 120s；Python 默认 60s
- `bash`：Go 多了后台任务模式和 `/task`
- `write_file` / `replace_file`：Go 增加了 approval gate
- MCP：Go 当前做字符串降级；Python 保留 richer content

---

## 6. 下一步优先级

### Priority 1: ACP parity

如果按“当前代码最需要补的 correctness gap”排，ACP 应该放第一。

原因：

- 它是当前最明显的 subsystem-level 缺口
- 差距不是 UI polish，而是行为语义缺失
- 范围相对收敛，能独立成一个完整阶段

最小闭环建议：

1. 在 ACP run 路径注入 `approval.WithContext(...)`
2. 让 ACP transport 能处理 `ApprovalRequest`
3. 把 `ToolCallPart` 投影到 ACP 更新流
4. 给 MCP richer content 找到最小可接受映射

### Priority 2: Provider / Setup / Config parity

如果按“和 temp 的产品体验差距”排，这块应该排第一或第二。

建议最小闭环：

1. 决定是否引入 temp 对应的 provider 语义
2. 重写 `/setup` 为平台选择 + 远端模型发现
3. 保存 `ContextWindowTokens`
4. 增加 `services.moonshot_search`
5. 删除或真正实现 `anthropic` 选项
6. 重新决定 first-run config 是否保留 placeholder 默认模型

### Priority 3: Shell raw mode toggle

建议做成一个小而干净的 parity phase：

- `Ctrl-K`
- agent/shell mode 状态
- 前台 raw shell command 执行路径

### Priority 4: Auto-update

建议独立成单独 phase，不和 ACP 或 setup 混做。

### Priority 5: Tool semantic parity

建议顺序：

1. `read_file`
2. `grep`
3. `search_web.include_content`
4. MCP richer result handling

---

## 7. 建议的执行顺序

如果你想按“代码正确性和架构收口”推进：

1. ACP parity
2. provider/setup/config parity
3. shell raw mode toggle
4. auto-update
5. tool semantic parity

如果你想按“终端用户第一感受”推进：

1. provider/setup/config parity
2. ACP parity
3. auto-update
4. shell raw mode toggle
5. tool semantic parity

我更推荐第一种顺序，因为 ACP 现在是更明显的行为缺口。

---

## 8. 本轮结论

一句话总结：

- `PLAN.md` 不该再把 runtime / shell 主链写成“还没补齐”
- 真正该做的下一步，是先把 ACP parity 补到不再绕开 approval、不再丢 tool delta

紧随其后的第二步，是把 provider/setup/config 这条链路做成真正对齐 `temp/` 的实现。
