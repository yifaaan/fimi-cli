package app

import (
	"fmt"
	"os"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/session"
)

// Run 是当前应用装配层的最小入口。
// 现在它还不执行 agent，只负责把 CLI 入口稳定下来。
func Run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(args) > 0 {
		return fmt.Errorf("arguments are not supported yet: %v", args)
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current work dir: %w", err)
	}

	sess, err := session.New(workDir)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	ctx := contextstore.New(sess.HistoryFile)
	if err := ctx.Append(contextstore.TextRecord{
		Role:    "system",
		Content: "session initialized",
	}); err != nil {
		return fmt.Errorf("append initial history record: %w", err)
	}

	fmt.Printf("session: %s\n", sess.ID)
	fmt.Printf("history: %s\n", ctx.Path())

	_ = cfg

	return nil
}
