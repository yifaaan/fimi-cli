package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui"
)

// RunFunc 是 ACP server 用来执行一次 agent prompt 的函数签名。
// 调用方提供 store（持久化）和 visualize（事件流）。
type RunFunc func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error)

// Server 是 ACP JSON-RPC 服务器。
// 它在 stdio 上监听 JSON-RPC 请求，并分发到注册的 handler。
type Server struct {
	conn     *FramedConn
	cfg      config.Config
	runFn    RunFunc
	sessions map[string]*Session
	mu       sync.Mutex
}

// NewServer 创建一个新的 ACP 服务器。
func NewServer(conn *FramedConn, cfg config.Config, runFn RunFunc) *Server {
	s := &Server{
		conn:     conn,
		cfg:      cfg,
		runFn:    runFn,
		sessions: make(map[string]*Session),
	}
	s.registerHandlers()
	return s
}

// Serve 启动 JSON-RPC 读循环。
func (s *Server) Serve(ctx context.Context) error {
	return s.conn.Serve(ctx)
}

func (s *Server) registerHandlers() {
	s.conn.Register("initialize", s.handleInitialize)
	s.conn.Register("authenticate", s.handleAuthenticate)
	s.conn.Register("new_session", s.handleNewSession)
	s.conn.Register("list_sessions", s.handleListSessions)
	s.conn.Register("resume_session", s.handleResumeSession)
	s.conn.Register("load_session", s.handleLoadSession)
	s.conn.Register("set_session_mode", s.handleSetSessionMode)
	s.conn.Register("set_session_model", s.handleSetSessionModel)
	s.conn.Register("prompt", s.handlePrompt)
	s.conn.Register("cancel", s.handleCancel)
}

func (s *Server) handleInitialize(id any, params json.RawMessage) (any, error) {
	var p InitializeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse initialize params: %w", err)
	}

	version := NegotiateVersion(p.ProtocolVersion)

	result := InitializeResult{
		ProtocolVersion: version.ProtocolVersion,
		AgentCapabilities: AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: PromptCapabilities{
				EmbeddedContext: true,
				Image:          true,
				Audio:          false,
			},
			SessionCapabilities: SessionCapabilities{
				List:   &SessionListCapabilities{},
				Resume: &SessionResumeCapabilities{},
			},
		},
		AgentInfo: Implementation{
			Name:    "fimi-cli",
			Version: "0.1.0",
		},
	}

	return result, nil
}

func (s *Server) handleAuthenticate(id any, params json.RawMessage) (any, error) {
	// Go CLI 暂不需要认证，直接返回成功
	return AuthenticateResult{}, nil
}

func (s *Server) handleNewSession(id any, params json.RawMessage) (any, error) {
	var p NewSessionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse new_session params: %w", err)
	}

	cwd := p.CWD
	if cwd == "" {
		cwd = "."
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}

	sess, err := session.New(absCWD)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	acpSess := NewSession(sess.ID, s.conn, absCWD)

	s.mu.Lock()
	s.sessions[sess.ID] = acpSess
	s.mu.Unlock()

	models := s.buildSessionModels()

	// 发送 available_commands 通知
	_ = s.conn.SendNotification("session/update", SessionUpdateNotification{
		SessionID: sess.ID,
		Update: AvailableCommandsUpdate{
			SessionUpdate:     "available_commands_update",
			AvailableCommands: []AvailableCommand{},
		},
	})

	return NewSessionResult{
		SessionID: sess.ID,
		Modes: SessionModeState{
			AvailableModes: []SessionMode{
				{ID: "default", Name: "Default", Description: "The default mode."},
			},
			CurrentModeID: "default",
		},
		Models: models,
	}, nil
}

func (s *Server) handleListSessions(id any, params json.RawMessage) (any, error) {
	var p ListSessionsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse list_sessions params: %w", err)
	}

	cwd := p.CWD
	if cwd == "" {
		cwd = "."
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}

	sessionInfos, err := session.ListSessions(absCWD)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	result := ListSessionsResult{
		Sessions: make([]SessionInfo, 0, len(sessionInfos)),
	}
	for _, si := range sessionInfos {
		result.Sessions = append(result.Sessions, SessionInfo{
			SessionID: si.ID,
			CWD:       absCWD,
		})
	}

	return result, nil
}

func (s *Server) handleResumeSession(id any, params json.RawMessage) (any, error) {
	var p ResumeSessionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse resume_session params: %w", err)
	}

	cwd := p.CWD
	if cwd == "" {
		cwd = "."
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}

	sess, err := session.LoadSession(absCWD, p.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	acpSess := NewSession(sess.ID, s.conn, absCWD)
	s.mu.Lock()
	s.sessions[sess.ID] = acpSess
	s.mu.Unlock()

	return ResumeSessionResult{
		Modes: SessionModeState{
			AvailableModes: []SessionMode{
				{ID: "default", Name: "Default", Description: "The default mode."},
			},
			CurrentModeID: "default",
		},
		Models: s.buildSessionModels(),
	}, nil
}

func (s *Server) handleLoadSession(id any, params json.RawMessage) (any, error) {
	// load_session 与 resume_session 逻辑相同
	return s.handleResumeSession(id, params)
}

func (s *Server) handleSetSessionMode(id any, params json.RawMessage) (any, error) {
	var p SetSessionModeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse set_session_mode params: %w", err)
	}
	if p.ModeID != "default" {
		return nil, fmt.Errorf("unsupported mode: %s", p.ModeID)
	}
	return nil, nil
}

func (s *Server) handleSetSessionModel(id any, params json.RawMessage) (any, error) {
	var p SetSessionModelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse set_session_model params: %w", err)
	}

	if _, ok := s.cfg.Models[p.ModelID]; !ok {
		return nil, fmt.Errorf("unknown model: %s", p.ModelID)
	}

	s.mu.Lock()
	s.cfg.DefaultModel = p.ModelID
	s.mu.Unlock()

	return nil, nil
}

func (s *Server) handlePrompt(id any, params json.RawMessage) (any, error) {
	var p PromptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse prompt params: %w", err)
	}

	s.mu.Lock()
	acpSess, ok := s.sessions[p.SessionID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", p.SessionID)
	}

	// 从 ACP 内容块中提取文本
	var promptText string
	for _, block := range p.Prompt {
		text, err := ContentBlockToText(block.Raw)
		if err != nil {
			log.Printf("[ACP] parse content block: %v", err)
			continue
		}
		promptText += text
	}

	if promptText == "" {
		return nil, fmt.Errorf("prompt is empty")
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	acpSess.SetCancel(cancel)

	// 异步执行 runtime，handler 返回 PendingResult
	go func() {
		defer acpSess.SetCancel(nil)

		store := contextstore.New(
			filepath.Join(acpSess.WorkDir(), ".fimi", acpSess.ID()+".jsonl"),
		)

		input := runtime.Input{
			Prompt:       promptText,
			Model:        s.cfg.DefaultModel,
			SystemPrompt: "",
		}

		result, err := s.runFn(ctx, store, input, acpSess.Visualize())

		if err != nil {
			_ = s.conn.SendError(id, CodeInternalError, err.Error())
			return
		}

		stopReason := mapStopReason(result.Status)
		_ = s.conn.SendResponse(id, PromptResult{StopReason: stopReason})
	}()

	return PendingResult{}, nil
}

func (s *Server) handleCancel(id any, params json.RawMessage) (any, error) {
	var p CancelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse cancel params: %w", err)
	}

	s.mu.Lock()
	acpSess, ok := s.sessions[p.SessionID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", p.SessionID)
	}

	acpSess.Cancel()
	return nil, nil
}

// buildSessionModels 从配置中构建可用模型列表。
func (s *Server) buildSessionModels() SessionModelState {
	available := make([]ModelInfo, 0, len(s.cfg.Models))
	for alias, mc := range s.cfg.Models {
		available = append(available, ModelInfo{
			ID:          alias,
			Name:        mc.Model,
			Description: fmt.Sprintf("Provider: %s", mc.Provider),
		})
	}

	return SessionModelState{
		AvailableModels: available,
		CurrentModelID:  s.cfg.DefaultModel,
	}
}

func mapStopReason(status runtime.RunStatus) string {
	switch status {
	case runtime.RunStatusFinished:
		return "end_turn"
	case runtime.RunStatusMaxSteps:
		return "max_turn_requests"
	case runtime.RunStatusFailed:
		return "end_turn"
	case runtime.RunStatusInterrupted:
		return "cancelled"
	default:
		return "end_turn"
	}
}

// Ensure FramedConn satisfies nothing extra -- it's a standalone type.
// Session's Visualize method returns a function matching ui.VisualizeFunc.
// We bridge here via RunFunc.
var _ RunFunc = func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
	// This is a default no-op; actual implementation is injected.
	return runtime.Result{}, nil
}

// AdaptRunFunc wraps ui.Run and a runner factory into a RunFunc suitable for the ACP server.
func AdaptRunFunc(uiRun func(ctx context.Context, runFunc ui.RunFunc, visualize ui.VisualizeFunc) (runtime.Result, error), newRunner func() runtime.Runner) RunFunc {
	return func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
		runner := newRunner()
		return uiRun(ctx, func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			return runner.WithEventSink(sink).Run(ctx, store, input)
		}, visualize)
	}
}
