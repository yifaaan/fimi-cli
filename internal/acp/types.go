package acp

import "encoding/json"

// --- JSON-RPC 2.0 消息类型 ---

// Request 是 JSON-RPC 2.0 请求。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response 是 JSON-RPC 2.0 响应。
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// Notification 是 JSON-RPC 2.0 通知（没有 id）。
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// RPCError 是 JSON-RPC 2.0 错误对象。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC 标准错误码
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// ACP 自定义错误码
	CodeAuthRequired = -32001
)

// --- ACP 协议类型 ---

// Implementation 描述 agent 或 client 的基本信息。
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeParams 是 initialize 方法的参数。
type InitializeParams struct {
	ProtocolVersion   int                `json:"protocol_version"`
	ClientCapabilities *ClientCapabilities `json:"client_capabilities,omitempty"`
	ClientInfo        *Implementation     `json:"client_info,omitempty"`
}

// InitializeResult 是 initialize 方法的返回值。
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocol_version"`
	AgentCapabilities AgentCapabilities `json:"agent_capabilities"`
	AuthMethods       []AuthMethod      `json:"auth_methods,omitempty"`
	AgentInfo         Implementation    `json:"agent_info"`
}

// ClientCapabilities 描述 client 支持的能力。
type ClientCapabilities struct {
	Fs        *FsCapabilities        `json:"fs,omitempty"`
	Terminal  *TerminalCapabilities  `json:"terminal,omitempty"`
}

// FsCapabilities 描述 client 的文件系统能力。
type FsCapabilities struct {
	ReadTextFile  bool `json:"read_text_file,omitempty"`
	WriteTextFile bool `json:"write_text_file,omitempty"`
}

// TerminalCapabilities 描述 client 的终端能力。
type TerminalCapabilities struct {
	Create     bool `json:"create,omitempty"`
	Output     bool `json:"output,omitempty"`
	WaitForExit bool `json:"wait_for_exit,omitempty"`
	Kill       bool `json:"kill,omitempty"`
}

// AgentCapabilities 描述 agent 支持的能力。
type AgentCapabilities struct {
	LoadSession        bool               `json:"load_session"`
	PromptCapabilities PromptCapabilities `json:"prompt_capabilities"`
	MCPCapabilities    MCPCapabilities    `json:"mcp_capabilities,omitempty"`
	SessionCapabilities SessionCapabilities `json:"session_capabilities,omitempty"`
}

// PromptCapabilities 描述 prompt 的能力。
type PromptCapabilities struct {
	EmbeddedContext bool `json:"embedded_context"`
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
}

// MCPCapabilities 描述 MCP 的能力。
type MCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

// SessionCapabilities 描述 session 的能力。
type SessionCapabilities struct {
	List   *SessionListCapabilities   `json:"list,omitempty"`
	Resume *SessionResumeCapabilities `json:"resume,omitempty"`
}

// SessionListCapabilities 描述 list session 能力。
type SessionListCapabilities struct{}

// SessionResumeCapabilities 描述 resume session 能力。
type SessionResumeCapabilities struct{}

// AuthMethod 描述一种认证方式。
type AuthMethod struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	FieldMeta   map[string]any `json:"field_meta,omitempty"`
}

// --- Session 相关类型 ---

// NewSessionParams 是 new_session 方法的参数。
type NewSessionParams struct {
	CWD        string     `json:"cwd"`
	MCPServers []MCPServer `json:"mcp_servers,omitempty"`
}

// MCPServer 描述一个 MCP 服务器配置。
type MCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"` // "stdio", "http", "sse"
	Headers map[string]string `json:"headers,omitempty"`
}

// NewSessionResult 是 new_session 方法的返回值。
type NewSessionResult struct {
	SessionID string            `json:"session_id"`
	Modes     SessionModeState  `json:"modes"`
	Models    SessionModelState `json:"models"`
}

// SessionModeState 描述 session 的 mode 状态。
type SessionModeState struct {
	AvailableModes []SessionMode `json:"available_modes"`
	CurrentModeID  string        `json:"current_mode_id"`
}

// SessionMode 描述一个可用的 mode。
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionModelState 描述 session 的 model 状态。
type SessionModelState struct {
	AvailableModels []ModelInfo    `json:"available_models"`
	CurrentModelID  string         `json:"current_model_id"`
}

// ModelInfo 描述一个可用的模型。
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ListSessionsParams 是 list_sessions 方法的参数。
type ListSessionsParams struct {
	CWD string `json:"cwd,omitempty"`
}

// ListSessionsResult 是 list_sessions 方法的返回值。
type ListSessionsResult struct {
	Sessions []SessionInfo `json:"sessions"`
}

// SessionInfo 描述一个 session 的摘要信息。
type SessionInfo struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd,omitempty"`
}

// ResumeSessionParams 是 resume_session 方法的参数。
type ResumeSessionParams struct {
	CWD       string `json:"cwd"`
	SessionID string `json:"session_id"`
}

// ResumeSessionResult 是 resume_session 方法的返回值。
type ResumeSessionResult struct {
	Modes  SessionModeState  `json:"modes"`
	Models SessionModelState `json:"models"`
}

// SetSessionModeParams 是 set_session_mode 方法的参数。
type SetSessionModeParams struct {
	ModeID    string `json:"mode_id"`
	SessionID string `json:"session_id"`
}

// SetSessionModelParams 是 set_session_model 方法的参数。
type SetSessionModelParams struct {
	ModelID   string `json:"model_id"`
	SessionID string `json:"session_id"`
}

// AuthenticateParams 是 authenticate 方法的参数。
type AuthenticateParams struct {
	MethodID string `json:"method_id"`
}

// AuthenticateResult 是 authenticate 方法的返回值。
type AuthenticateResult struct{}

// CancelParams 是 cancel 方法的参数。
type CancelParams struct {
	SessionID string `json:"session_id"`
}

// --- Prompt 相关类型 ---

// PromptParams 是 prompt 方法的参数。
type PromptParams struct {
	Prompt    []ContentBlock `json:"prompt"`
	SessionID string         `json:"session_id"`
}

// ContentBlock 是 prompt 内容的多态边界。
type ContentBlock struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	Raw  json.RawMessage `json:"-"` // 保留原始 JSON
}

// PromptResult 是 prompt 方法的返回值。
type PromptResult struct {
	StopReason string `json:"stop_reason"`
}

// --- Session Update 通知类型 ---

// SessionUpdateNotification 是 session_update 通知的参数。
type SessionUpdateNotification struct {
	SessionID string `json:"session_id"`
	Update    any    `json:"update"`
}

// AgentMessageChunk 是 agent 文本流通知。
type AgentMessageChunk struct {
	SessionUpdate string          `json:"session_update"`
	Content       TextContentBlock `json:"content"`
}

// AgentThoughtChunk 是 agent 思考流通知。
type AgentThoughtChunk struct {
	SessionUpdate string          `json:"session_update"`
	Content       TextContentBlock `json:"content"`
}

// ToolCallStart 是工具调用开始通知。
type ToolCallStart struct {
	SessionUpdate string               `json:"session_update"`
	ToolCallID    string               `json:"tool_call_id"`
	Title         string               `json:"title"`
	Status        string               `json:"status"` // "in_progress"
	Content       []ToolCallContentItem `json:"content,omitempty"`
}

// ToolCallProgress 是工具调用进度/结果通知。
type ToolCallProgress struct {
	SessionUpdate string               `json:"session_update"`
	ToolCallID    string               `json:"tool_call_id"`
	Title         string               `json:"title,omitempty"`
	Status        string               `json:"status"` // "completed", "failed"
	Content       []ToolCallContentItem `json:"content,omitempty"`
}

// ToolCallContentItem 是工具调用内容项。
type ToolCallContentItem struct {
	Type    string          `json:"type"`
	Content TextContentBlock `json:"content"`
}

// TextContentBlock 是文本内容块。
type TextContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// AvailableCommandsUpdate 是可用命令更新通知。
type AvailableCommandsUpdate struct {
	SessionUpdate      string              `json:"session_update"`
	AvailableCommands  []AvailableCommand  `json:"available_commands"`
}

// AvailableCommand 描述一个可用命令。
type AvailableCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// --- Sentinel 类型 ---

// PendingResult 是 prompt handler 返回的哨兵值。
// Serve 循环检测到它时跳过自动响应，由 handler 自己发送响应。
type PendingResult struct{}

// IsPending 检查 handler 返回值是否是 PendingResult。
func IsPending(result any) bool {
	_, ok := result.(PendingResult)
	return ok
}
