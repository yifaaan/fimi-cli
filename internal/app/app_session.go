package app

import (
	"errors"
	"fmt"

	"fimi-cli/internal/session"
)

// openRunSession 根据当前应用输入决定 session 获取策略。
// 是否复用旧 session 属于 app 层决策，而不是 session 包内部规则。
func (d dependencies) openRunSession(workDir string, input runInput) (session.Session, bool, error) {
	if input.continueSession {
		continueSession := d.continueSession
		if continueSession == nil {
			continueSession = session.Continue
		}

		sess, err := continueSession(workDir)
		if err != nil {
			return session.Session{}, false, renderContinueSessionError(workDir, err)
		}

		return sess, true, nil
	}

	createSession := d.createSession
	if createSession == nil {
		createSession = session.New
	}

	sess, err := createSession(workDir)
	if err != nil {
		return session.Session{}, false, fmt.Errorf("create session: %w", err)
	}

	return sess, false, nil
}

func renderContinueSessionError(workDir string, err error) error {
	if errors.Is(err, session.ErrNoPreviousSession) {
		return fmt.Errorf(
			"no previous session found for work dir %q; rerun without --continue to start a new session: %w",
			workDir,
			session.ErrNoPreviousSession,
		)
	}

	return fmt.Errorf("continue session: %w", err)
}
