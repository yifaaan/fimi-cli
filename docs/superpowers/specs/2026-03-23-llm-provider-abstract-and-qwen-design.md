# LLM Provider 抽象层 + QWEN 实现

## 目标

实现真实 LLM Client，采用 OpenAI 兼容 API 格式作为基础抽象，首先接入阿里云 DashScope (QWEN)。

## 架构

```
+------------------+
|       app        |  (装配层)
+--------+---------+
         |
         v
+--------+---------+
|     runtime      |  (协调层)
+--------+---------+
         |
         v
+--------+---------+
|   llm.Engine     |  (history 组装 + 重试)
+--------+---------+
         |
         v
+--------+---------+     +------------------+
|   llm.Client     |<----|  openai.Client   |  (抽象接口)
|   (interface)    |     |  (OpenAI 兼容)   |
+------------------+     +--------+---------+
                                  |
                                  v
                         +--------+---------+
                         |   qwen.Adapter   |  (DashScope 适配)
                         +------------------+
```

## 核心组件

### 1. `llm.Client` 接口（已存在，保持不变）

```go
type Client interface {
    Reply(request Request) (Response, error)
}
```

### 2. 新增 `internal/llm/openai` 子包

实现 OpenAI Chat Completions API 的通用 client：
- 支持 base_url 覆盖（适配各种兼容服务）
- 支持 API Key 认证
- 支持 streaming（后续扩展）

### 3. 新增 `internal/llm/qwen` 子包

- 配置 DashScope 的 base_url: `https://dashscope.aliyuncs.com/compatible-mode/v1`
- 从配置文件读取 API Key

## 配置结构扩展

```json
{
  "default_model": "qwen-plus",
  "engine_mode": "qwen",
  "providers": {
    "qwen": {
      "api_key": "sk-xxx",
      "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1"
    }
  }
}
```

## 目录结构

```
internal/llm/
├── client.go              # Client 接口定义（已有）
├── engine.go              # Engine 实现（已有）
├── types.go               # Request/Response（已有）
├── placeholder_client.go  # 占位实现（已有）
├── openai/
│   └── client.go          # OpenAI 兼容 client
└── qwen/
    └── adapter.go         # QWEN 配置适配
```

## 实现步骤

| 步骤 | 内容 | 预估代码量 |
|------|------|-----------|
| 1 | 扩展 config 结构，添加 `providers` 字段 | ~30 行 |
| 2 | 实现 `openai.Client`（通用 OpenAI 兼容 client） | ~80 行 |
| 3 | 实现 `qwen.Adapter`（QWEN 配置 + client 构建） | ~40 行 |
| 4 | 更新 `llm.BuildClient` 支持 `qwen` mode | ~20 行 |
| 5 | 添加测试 | ~100 行 |

## 设计决策

### 为什么采用 OpenAI 兼容格式？

1. QWEN DashScope 官方支持 OpenAI 兼容模式
2. 后续接入其他兼容 provider（如 DeepSeek、Moonshot）成本低
3. 社区生态成熟，调试工具丰富

### 为什么配置放在配置文件而非环境变量？

1. 用户明确选择配置文件方式
2. 便于管理多个 provider 的 key
3. 与 Python 原版 kimi-cli 的配置风格一致

## 接口边界

### `openai.Client` 对外暴露

```go
package openai

type Config struct {
    BaseURL string
    APIKey  string
    Model   string
}

func NewClient(cfg Config) llm.Client
```

### `qwen` 包对外暴露

```go
package qwen

const (
    DefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    DefaultModel   = "qwen-plus"
)

type Config struct {
    APIKey  string
    BaseURL string  // 可选，默认使用 DefaultBaseURL
    Model   string  // 可选，默认使用 DefaultModel
}

func NewClient(cfg Config) llm.Client
```

## 错误处理

- API Key 缺失：返回明确错误提示用户配置
- 网络错误：包装原始错误，保留上下文
- API 错误响应：解析 error message 并返回

## 后续扩展

- streaming 支持
- 重试机制（在 Engine 层实现）
- 其他 provider（OpenAI、DeepSeek 等）