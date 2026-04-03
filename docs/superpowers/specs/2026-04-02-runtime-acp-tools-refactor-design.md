# Runtime / ACP / tools 核心流重构设计

## 背景

当前项目的 Runtime / ACP / tools 主链已经具备较完整能力，但核心问题不在“功能缺失”，而在于主流程阅读成本偏高、局部职责混杂、文件主题不够清晰。

本次重构不追求重设计，也不追求一次性统一整个仓库风格，而是聚焦 `internal/runtime`、`internal/acp`、`internal/tools`、`internal/app/app_acp.go` 这一条核心链路，在不明显扩大改动面的前提下，同时降低认知负担并收紧职责边界。

## 目标

1. 让 Runtime 主流程更直白，读代码时能快速看懂一次 run 的完整推进顺序。
2. 让 ACP 的 session 状态、wire 投影、approval 跟踪、tool-call 投影、content 构造各自收口。
3. 保持 tools executor 简单具体，不为重构制造额外抽象。
4. 保持 `internal/app/app_acp.go` 作为装配层，不承载更多业务逻辑。
5. 以行为不变为前提做结构整理，允许顺手修正明显设计问题和局部不一致，但不做大范围语义重写。

## 非目标

本次不做以下内容：

- 不改 package 边界。
- 不引入新的跨 package coordinator / projector / dispatcher 抽象。
- 不顺手重构 shell、print UI、config、session 其他支撑层。
- 不为了统一风格而批量重命名。
- 不把现有具体类型抽成一批新 interface。
- 不做深度 pipeline 化或框架化重设计。

## 范围

### 包级范围

- `internal/runtime`
- `internal/acp`
- `internal/tools`
- `internal/app/app_acp.go`

### 重点文件

- `internal/runtime/runtime.go`
- `internal/acp/session.go`
- `internal/tools/executor.go`
- `internal/app/app_acp.go`

## 设计原则

### 1. 保留现有 API 和 package 关系

重构重点是文件组织和私有实现边界，而不是重新设计对外接口。除非测试或调用点已经因为当前实现问题而必须调整，否则尽量保持公开 API 不变。

### 2. 用清晰私有函数拆 workflow，不用新抽象层掩盖流程

优先使用同包内、语义明确的私有函数和按主题拆分的文件，而不是新增 service、manager、coordinator 等类型。代码应更容易沿调用顺序阅读，而不是更“架构化”。

### 3. 接受少量重复，换取流程清晰

如果两个流程在行为、错误处理、状态推进上存在细微差异，应保留明确分支，而不是为了去重引入 flag 或 mode 驱动的 helper。

### 4. 重构先保行为，再整理结构

新的结构必须优先守住当前行为：history 持久化、runtime 事件发送、approval 透传、tool call 投影、D-Mail rollback、retry 状态、context usage 状态都不能在整理过程中悄悄改变。

## 方案选择

本次采用“边界优先的中等重构”。

### 放弃的方案 A：保守拆文件

仅按文件拆分而不重新收口职责，虽然风险最低，但会保留当前职责混杂的问题，收益有限。

### 放弃的方案 C：深度重设计

重新建模 Runtime / ACP / tools 为更显式的 pipeline 或 projector 体系，虽然长期演进空间更大，但改动面过大，也违背本次“在不扩大改动面的前提下整理主链”的约束。

### 选择的方案 B：边界优先的中等重构

保留当前 package、绝大多数类型和对外行为，把一个类型里塞太多细节的问题拆开，让主流程、状态、投影、格式转换分别落在更清楚的位置。

## Runtime 设计

### 现状问题

`internal/runtime/runtime.go` 当前集中承载了以下职责：

- `Runner` 和公开类型定义
- run loop
- checkpoint 持久化
- D-Mail checkpoint marker 注入
- step checkpoint
- retry/backoff
- engine 调用与 streaming
- tool-call step 推进
- tool result 持久化
- runtime event emit
- status snapshot 计算
- rollback 与 D-Mail 注入

这些职责都属于 runtime，但全部堆在一个文件和少数大函数里，导致阅读 `Runner.Run()` 时需要频繁切换上下文。

### 目标边界

`Runner` 继续表示“一次 runtime 执行的驱动器”，但 `Run()` 只保留 workflow 级顺序控制，不再把 checkpoint、retry、rollback、event emit 的细节直接展开在主流程里。

### 文件拆分

建议将 `internal/runtime` 整理为以下主题文件：

- `runtime.go`
  - 保留公开类型定义
  - `Runner`、`New`、`NewWithToolExecutor`
  - `Run()` 主流程
- `runtime_step.go`
  - `runStep`
  - `advanceRun`
  - `executeToolCalls`
  - tool-call step 相关推进逻辑
- `runtime_retry.go`
  - `runStepWithRetry`
  - backoff / jitter / sleep 逻辑
- `runtime_events.go`
  - `emitEvent`
  - `emitStepEvents`
  - status snapshot 构造
  - display output fallback 逻辑
- `runtime_dmail.go`
  - checkpoint marker 注入
  - run start checkpoint
  - step checkpoint
  - pending D-Mail rollback 与注入
- `runtime_errors.go`
  - `isInterruptedError`
  - `IsRetryable`
  - `IsRefused`
  - `IsTemporary`
  - 其他错误分类 helper

### Run 主流程

`Run()` 调整目标是保持语义不变，但让逻辑顺序一眼可见。主流程应收敛为：

1. 规范化 prompt，初始化 `Result`
2. 持久化 run 开始边界
3. 进入 step 循环
   - 检查中断
   - 发送 `StepBegin`
   - 持久化 step checkpoint（仅 D-Mail 模式）
   - 执行 `runStepWithRetry`
   - 执行 `advanceRun`
   - 检查并应用 pending D-Mail rollback
   - finished 时返回
4. 达到 `MaxStepsPerRun` 时返回 `RunStatusMaxSteps`

为此，把现在展开在 `Run()` 内部的细节收成语义明确的私有函数，例如：

- `persistRunStart(...)`
- `persistStepCheckpoint(...)`
- `applyPendingDMail(...)`
- `appendCheckpointMarker(...)`

这些函数只服务于 Runtime 内部，不额外引入新的结构体层级。

### Step 推进

`runStep` 与 `advanceRun` 的职责继续保留，但要让各自职责更单纯：

#### `runStep`

负责：

- 从 `store` 读取最近 history
- 构造 `ReplyInput`
- 调用 engine（streaming 或非 streaming）
- 持久化 usage
- 生成 `StepResult`

整理重点：

- 保留当前行为，不改变 streaming 支持方式
- 把“构建 reply input”“调用 engine”“持久化 usage”“构造 text/tool step 结果”分成清楚段落
- 保持 `StepResult` 作为 step 输出的唯一中心结构

#### `advanceRun`

负责：

- 根据 `StepKind` 推进 step 后续动作
- 统一把结果追加到 `Result.Steps`
- 统一发 step 相关事件
- 根据 `StepStatus` 决定 run 是否 finished

整理重点：

- 用更直白的内部私有函数压缩 tool-call 分支
- 推荐拆成：
  - `advanceFinishedStep(...)`
  - `advanceToolCallStep(...)`
- `advanceToolCallStep(...)` 负责：
  - 执行 tools
  - 写回 `ToolExecutions` 或 `ToolFailure`
  - 持久化 tool records

这样 `advanceRun()` 保留为 step 后处理入口，而不是大段混杂逻辑。

### D-Mail 与 checkpoint

D-Mail 本质上仍是 runtime 的一部分，不需要外提 package，也不需要新建复杂对象。关键是集中表达 checkpoint 语义，避免主流程中重复出现同一组动作。

建议统一为以下动作：

- `appendCheckpointMarker(...)`
- `persistRunStart(...)`
- `persistStepCheckpoint(...)`
- `applyPendingDMail(...)`

这些动作共同负责：

- 创建 checkpoint metadata
- 更新 D-Mail 可见 checkpoint 计数
- 在需要时追加 `<system>CHECKPOINT N</system>` 记录
- rollback 后重新建立 checkpoint
- 注入 D-Mail 指令记录

目标是把“checkpoint + marker + rollback + 注入”收成一个清楚主题，而不是在主流程里散落多段近似代码。

### 事件发送

`emitStepEvents()` 保留现有语义，但迁移到独立主题文件中，明确它是 runtime 到 wire 的投影层。

职责保持为：

- 在未流式输出时补发 `TextPart`
- 发送 `ToolCall`
- 发送 `ToolResult`
- 发送 `StatusUpdate`

相关 helper，例如 display output fallback 和 context usage snapshot 构造，也放到同一主题文件中，避免继续散落在 `runtime.go` 主文件里。

## ACP 设计

### 现状问题

`internal/acp/session.go` 当前同时承担：

- ACP session 状态持有
- cancel/model state 访问
- pending approval 跟踪
- started tool call 跟踪
- wire message 分发
- runtime event 到 ACP update 的投影
- content block 构造与 fallback 文本裁剪

这些逻辑彼此相关，但并不应该全部挤在一个文件中，否则阅读 ACP 行为时很难区分“状态管理”和“消息投影”。

### 目标边界

`Session` 继续作为 ACP 会话状态入口，但：

- 状态管理保留在 `session.go`
- wire 消息翻译单独收口
- tool call 投影单独收口
- approval 生命周期管理单独收口
- content 构造单独收口

### 文件拆分

建议将 `internal/acp` 中的 `session.go` 按主题拆为：

- `session.go`
  - `Session` 结构
  - `NewSession`
  - `HistoryFile`
  - `CurrentModelID`
  - `SetModelID`
  - `SetCancel`
  - `Cancel`
- `session_wire.go`
  - `VisualizeWire`
  - `translateAndSendMessage`
  - `translateAndSendEvent`
- `session_toolcalls.go`
  - `sendToolCallPart`
  - `sendToolCallStart`
  - `sendToolCallProgress`
- `session_approval.go`
  - `sendApprovalRequest`
  - `ResolveApproval`
  - `ClearPendingApprovals`
- `content.go`
  - `buildACPContentItems`
  - `buildTextContentItems`
  - `hasNonTextContent`
  - `firstNonEmptyToolOutput`

### Session 状态边界

`Session` 本体只保留：

- session 基础信息
- `modelID`
- 当前 prompt 的 `cancelFn`
- `pendingApprovals`
- `startedToolCalls`

也就是：`Session` 只负责“存状态并提供状态访问入口”，不再承担大段 ACP notification 组装逻辑。

### wire 投影边界

`VisualizeWire` 和消息分发逻辑迁到 `session_wire.go` 后，阅读 ACP 映射时可以只看这一层：

- `wire.EventMessage` -> runtime event 投影
- `*wire.ApprovalRequest` -> approval update

其中 `translateAndSendEvent` 继续按 runtime event 类型分派，但只做“把消息送到对应发送函数”的工作，不再夹杂 content 构造细节。

### tool call 投影

tool call 相关函数迁到 `session_toolcalls.go` 后，应保持当前行为：

- `ToolCallPart`：在首个 chunk 时补发 start，并持续发 progress
- `ToolCall`：如果此前已通过 `ToolCallPart` 建立，则发 progress；否则发 start
- `ToolResult`：按 error/normal 转成 completed/failed，并清理 started tracking

这里的重点不是改协议，而是让这组逻辑从 session 状态访问代码中脱开，成为单独主题。

### approval 生命周期

approval 跟踪逻辑迁到 `session_approval.go` 后，继续保持：

- 接收到 ACP 侧的 approval request update 时登记 pending request
- 外部 `ResolveApproval` 根据 ID 回填 response
- prompt 结束或取消时 `ClearPendingApprovals` 统一拒绝未决请求

这能让 ACP approval 路径更容易单独阅读和测试。

### content block 构造

`buildACPContentItems(...)` 及相关 fallback helper 移到 `content.go` 后，ACP rich content 和 fallback 文本裁剪逻辑将成为独立主题。

这样未来如果需要调整：

- rich content 到 ACP content block 的映射
- 多媒体内容时是否附带 fallback text
- 文本截断长度

只需要看 `content.go`，而不用在 `Session` 代码里翻找。

## tools 设计

### 现状判断

`internal/tools/executor.go` 当前规模不大，主路径已经比较接近理想状态：

1. 校验 tool name
2. 校验是否允许调用
3. 查找 handler
4. 注入 approval tool call id
5. 执行 handler
6. 包装错误

它的主要问题不是“太复杂”，而是要避免在本轮重构中被过度设计。

### 设计结论

`Executor` 保持单文件和具体实现，不刻意拆成更多结构。

只做以下约束澄清：

- `Executor` 是 policy + dispatch 适配器
- 它负责 allowed-tools 校验与 handler 调用，不承担 runtime flow
- `approval.WithToolCallID(...)` 继续在这里注入，因为这是 tool execution 边界的一部分
- `markRefused(...)` 和 `markTemporary(...)` 保持最小错误语义包装层，不扩展出新抽象

如果实施过程中发现测试组织需要轻微拆分，可只把错误标记辅助逻辑拆到相邻小文件；否则维持一个文件更直接。

## app 装配层设计

### `internal/app/app_acp.go`

该文件继续保持装配层角色，不承接更多业务流程。

具体约束：

- 配置加载、工作目录解析、tool registry 解析、agent 加载、runner 构建继续发生在 app 层
- approval 上下文注入仍可保留在 ACP run 入口附近，因为它属于会话运行装配
- 不把 runtime、ACP 投影或 tools 行为实现反向塞回 app 层

目标是让 `app_acp.go` 继续保持“接线薄层”，而不是重新成为另一个逻辑中心。

## 实施顺序

### 第 1 步：整理 Runtime 文件边界

先拆 runtime 文件和私有函数，但尽量不动行为判断顺序。优先把 `Run()` 主链、retry、events、D-Mail/checkpoint 逻辑按主题移动到更清晰的位置。

### 第 2 步：整理 ACP 文件边界

把 `session.go` 中的 wire 投影、tool call 投影、approval 管理、content helper 抽到主题文件，保持 `Session` 本体聚焦状态。

### 第 3 步：只对 tools executor 做局部清晰化

避免扩大范围。除非实施时发现明显的命名或局部错误包装问题，否则不对 `Executor` 做结构性调整。

### 第 4 步：最小调整 app 装配层

只修正因重构带来的调用组织变化，不把新逻辑加进去。

### 第 5 步：补回归测试

在结构整理完成后，只为最容易在重构中回归的边界补测试，而不是重写整套测试。

## 测试策略

### Runtime

重点验证：

1. `Run()` workflow 没变
   - prompt 持久化
   - step 推进
   - finished / interrupted / max_steps 分支
2. tool-call step 语义没变
   - 成功时 assistant + tool result records 正确写入
   - 失败时 failure tool result 仍正确写入
3. D-Mail / retry / status emit 没变
   - D-Mail 场景才有 step checkpoint
   - retry 状态仍能发出与清除
   - rollback 后仍建立新 checkpoint 并注入 D-Mail 指令

### ACP

重点验证：

1. `wire.EventMessage` 与 `ApprovalRequest` 仍被正确投影
2. `ToolCallPart` / `ToolCall` / `ToolResult` 的 start/progress/completion 行为没变
3. rich content 和 fallback text 构造没变
4. pending approval 与 started tool call 的状态清理没变

### tools

重点验证：

- 空工具名拒绝
- 未授权工具拒绝
- handler 错误包装不变
- approval tool call id 注入不丢

### 测试原则

- 优先复用现有测试风格
- 新增测试只补关键回归点
- 不为了拆文件去重写大量已有测试

## 风险与控制

### 风险 1：拆文件后调用关系变绕

控制方式：

- 只按主题拆文件，不新增跨层 struct
- 让 `Run()`、`VisualizeWire()` 继续作为入口函数
- 用明确命名的私有函数表达动作，而不是抽象化流程对象

### 风险 2：局部去重过度导致语义隐藏

控制方式：

- 接受少量重复
- 对有差异的流程保留显式分支
- 不使用 flag/mode 驱动的万能 helper

### 风险 3：结构整理过程中意外改变行为

控制方式：

- 先移动和收口，再做最小内部整理
- 用回归测试守住 runtime step 语义、ACP 投影语义、approval 生命周期

## 验收标准

重构完成后，应满足：

1. `internal/runtime/runtime.go` 不再承载 retry、events、D-Mail、checkpoint 等全部细节，`Run()` 可直接读出 workflow。
2. Runtime 的 checkpoint、retry、tool-call step、status emit、D-Mail rollback 行为与重构前保持一致。
3. `internal/acp/session.go` 聚焦 session 状态，不再同时承载 wire 投影、approval 管理、content 构造全部细节。
4. ACP 的 `TextPart`、`ToolCallPart`、`ToolCall`、`ToolResult`、approval request 投影行为与重构前保持一致。
5. `internal/tools/executor.go` 仍保持小而具体，不引入新的不必要抽象。
6. `internal/app/app_acp.go` 仍是装配层，没有吸收更多业务逻辑。
7. 新结构比当前实现更容易阅读、调试和局部修改。

## 最终结论

这次重构应以“边界优先的中等重构”推进：

- Runtime 收敛主流程，细节按主题下沉到同包私有函数和文件
- ACP 保留 `Session` 作为状态入口，但把 wire 投影、tool call 投影、approval 管理、content 构造分别收口
- tools executor 维持小而具体
- app 层继续保持薄装配
- 用最小必要测试证明行为没变

这样可以在不引入额外架构负担的情况下，让 Runtime / ACP / tools 这条主链更清晰、更容易维护。