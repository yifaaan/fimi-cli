# internal/tools 边界优先重构设计

## 背景

`internal/runtime` 与 `internal/acp` 的主链重构已经完成，当前主链里剩下最值得整理的一段是 `internal/tools`。现状不是功能缺失，而是执行边界与具体实现细节在阅读上还不够收口：

- `executor.go` 负责 tool-call 执行边界
- `builtin.go` 同时承载内建工具注册、bash 参数解析、bash 三模式分发、后台查询/启动逻辑
- `background.go` 负责后台任务生命周期

这些职责本身合理，但现在的阅读体验是：想看“一个工具调用怎么被允许并执行”，需要在 `executor.go`、`builtin.go` 和 `background.go` 之间来回跳转，其中 `builtin.go` 的 bash/background 逻辑又相对集中，导致主路径边界不够清晰。

本次设计采用**边界优先的中等整理**：不重做工具系统，不改 package 边界，不引入新的抽象层，只把已有职责放回更合适的位置，让 tools 链路更容易读、更容易测。

## 目标

1. 让 `executor.go` 明确只表达 tools 执行边界。
2. 让 bash/background 的三种执行路径在主题上收口，读一个文件就能看懂。
3. 保持 `background.go` 作为具体后台任务管理器，不被上层边界逻辑污染。
4. 保持 approval、temporary/refused、background task 的现有行为语义不变。
5. 通过最小必要测试守住回归边界，而不是扩大功能范围。

## 非目标

本次不做以下内容：

- 不改 `internal/tools` 的 package 边界。
- 不引入新的 manager / coordinator / dispatcher / strategy 抽象。
- 不重做工具注册机制。
- 不改变 ACP / app 层的装配职责。
- 不把 background task query 提升成独立系统能力。
- 不顺手重构其他无关 builtin handler。

## 方案选择

本次采用 **方案 B：边界优先的中等整理**。

### 放弃的方案 A：只动 executor

只整理 `executor.go` 的职责边界，风险最小，但 `builtin.go` 里的 bash/background 逻辑仍然挤在一起，收益有限。

### 放弃的方案 C：全链更深拆分

继续把 builtin handlers 和 background 生命周期拆得更细，结构会更“干净”，但改动面明显扩大，也更容易超出这轮的边界整理目标。

### 选择的方案 B：边界优先的中等整理

保留当前执行链和绝大多数具体实现，把：

- tools 执行边界
- builtin 注册入口
- bash/background 工具逻辑
- background 生命周期

分成更清楚的主题位置，但不额外引入新的跨层对象。

## 设计原则

### 1. executor 只负责边界，不负责具体工具细节

`Executor.Execute(...)` 应只表达：

- 规范化 tool name
- 校验当前 agent 是否允许调用
- 查找 handler
- 注入 approval/tool-call 上下文
- 统一包装 handler error

它不应该继续承载 bash/background 的具体分支判断，也不应该理解 builtin handler 的内部实现细节。

### 2. builtin 负责“怎么跑”，不是“能不能跑”

builtin handler 层只负责：

- 参数解析
- approval 描述生成
- 前台/后台/查询等具体执行路径选择
- 输出结果组装

也就是说，“是否允许调用”仍然留在 executor 边界，而“这个内建工具怎么执行”留在 builtin 主题文件。

### 3. background manager 保持具体、单纯

`BackgroundManager` 继续只负责后台任务生命周期：

- 启动
- 查询
- 终止
- close 时清理
- 状态快照

它不感知 approval，不感知 allowed-tools，也不感知 ACP/session。这样保持它是一个简单具体的内部组件，而不是被提升成更高层协调器。

### 4. 保留显式分支，不引入万能 helper

bash 的三种模式：

- 前台执行
- 后台启动
- 后台查询

继续保留显式分支。即使它们有少量重复，也优先保留清楚的路径，而不是为了去重再包一层 mode/flag 驱动的“大而全” helper。

## 目标结构

### `executor.go`

保留：

- `Executor`
- `NewExecutor(...)`
- `Execute(...)`
- `markRefused(...)`
- `markTemporary(...)`
- 错误语义类型（`refusedError` / `temporaryError`）

职责固定为 tools 执行边界。

### `builtin.go`

收缩为 builtin 注册入口，主要保留：

- `NewBuiltinExecutor(...)`
- `builtinHandlers(...)`
- 与 handler 注册直接相关的 option / opts 结构

它负责“接哪些 builtin handler”，不再承载 bash 的大段实现。

### `builtin_bash.go`

新增 bash 主题文件，收口：

- `bashArguments`
- `decodeBashArguments(...)`
- `bashApprovalDescription(...)`
- `newBashHandler(...)`
- `handleBashTaskQuery(...)`
- `handleBashBackground(...)`
- `handleBashForeground(...)`
- bash 测试专用固定超时 handler（如果现有测试仍需要）

这样读 bash 工具时，不需要再在 `builtin.go` 中穿过其他内建工具逻辑。

### `background.go`

继续保持单文件 concrete manager，除非实现过程中发现某个小 helper 抽出后明显更清楚，否则不继续拆文件。

## 数据流

本次整理后，tools 调用链应被读成下面的顺序：

1. runtime 产出 `ToolCall`
2. `Executor.Execute(...)` 判断：
   - name 是否有效
   - 当前 agent 是否允许该 tool
   - 是否存在对应 handler
3. executor 在边界处注入 approval/tool-call 上下文
4. builtin handler 根据参数决定具体路径
5. 如果走后台路径，由 `BackgroundManager` 处理任务生命周期
6. handler 返回 `runtime.ToolExecution`
7. executor 统一做上层 error 包装

这个数据流的关键约束是：

- approval 语义留在 executor 边界附近
- temporary/refused 错误语义继续由 tools 边界统一表达
- background task query 仍然只是 bash 工具的一个具体路径，而不是独立调度能力

## 行为约束

以下行为必须保持不变：

### executor

- 空 tool name 仍返回 refused
- disallowed tool 仍返回 refused
- 没有自定义 handler 的工具仍回退到 no-op `ToolExecution`
- handler 执行前仍注入 `approval.WithToolCallID(...)`
- handler error 仍按当前层级包装为 `execute tool %q: %w`

### bash / builtin

- approval 描述生成逻辑不变
- approval 被拒绝时，仍返回当前的“用户拒绝”结果语义
- 前台执行的 stdout / stderr / exit code 行为不变
- 后台启动仍返回 task id 文本
- 后台查询仍返回状态、命令、duration、stdout/stderr 摘要

### background manager

- `running / completed / failed / timeout / killed` 状态推进不变
- `Kill` / `Close` 的语义不变
- `Status` / `List` 的快照行为不变

## 测试策略

本次只补边界回归测试，不扩大功能范围。

### 1. executor 边界测试

重点覆盖：

- tool-call ID 注入没有丢
- disallowed tool / empty name 语义保持
- temporary / refused 包装保持

### 2. bash 路径测试

重点覆盖：

- 前台执行语义不变
- 后台启动与后台查询语义不变
- approval 路径不变

### 3. background 状态测试

重点覆盖：

- running / completed / failed / timeout / killed
- unknown task 报错
- kill / close 行为

### 4. 测试原则

- 优先复用现有测试
- 新增测试只补边界点
- 后台相关测试优先使用条件等待，不依赖固定 sleep

## 风险与控制

### 风险 1：拆 bash 主题时改变错误包装层级

控制方式：

- 保持 `Executor.Execute(...)` 作为统一 error 包装层
- builtin 内部只返回当前已有语义的错误类型

### 风险 2：前台 / 后台 / 查询三路径拆开后丢掉某个小语义

控制方式：

- 复用现有 bash 测试
- 在拆分前后跑 targeted tests 守住行为

### 风险 3：background 测试误报回归

控制方式：

- 继续使用条件等待而不是固定 sleep
- 区分“生产语义回归”和“测试时序脆弱”

## 验收标准

完成后应满足：

1. `executor.go` 一眼能看出 tools 执行边界职责。
2. `builtin.go` 不再展开 bash/background 的大段实现。
3. bash/background 三模式在同一主题文件里可顺序读完。
4. `background.go` 仍保持 concrete manager，不引入新抽象层。
5. approval、temporary/refused、background task 行为与当前实现保持一致。
6. tools 相关关键回归测试通过。

## 最终结论

这轮 `internal/tools` 应按“边界优先的中等整理”推进：

- `executor.go` 聚焦执行边界
- `builtin.go` 聚焦内建工具注册入口
- bash/background 逻辑收口到独立主题文件
- `background.go` 保持简单具体
- 用最小必要测试守住行为不变

这样可以在不引入额外架构负担的前提下，把 tools 主链整理得更清楚、更容易维护。