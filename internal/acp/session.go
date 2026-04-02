package acp

import (
	"context"
	"sync"

	sessionpkg "fimi-cli/internal/session"
	"fimi-cli/internal/wire"
)

// Session wraps the ACP-facing state for a single runtime session.
type Session struct {
	session sessionpkg.Session
	conn    *FramedConn
	mu      sync.Mutex
	modelID string

	// cancelFn cancels the currently-running prompt, if any.
	cancelFn context.CancelFunc

	pendingApprovals map[string]*wire.ApprovalRequest
	startedToolCalls map[string]bool
}

// NewSession creates a new ACP session wrapper.
func NewSession(sess sessionpkg.Session, conn *FramedConn, modelID string) *Session {
	return &Session{
		session:          sess,
		conn:             conn,
		modelID:          modelID,
		pendingApprovals: make(map[string]*wire.ApprovalRequest),
		startedToolCalls: make(map[string]bool),
	}
}

// HistoryFile returns the backing session history path.
func (s *Session) HistoryFile() string {
	return s.session.HistoryFile
}

// CurrentModelID returns the session's current model selection.
func (s *Session) CurrentModelID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelID
}

// SetModelID updates the session's current model selection.
func (s *Session) SetModelID(modelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelID = modelID
}

// SetCancel stores the cancel func for the currently-running prompt.
func (s *Session) SetCancel(fn context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelFn = fn
}

// Cancel cancels the currently-running prompt, if any.
func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
}
