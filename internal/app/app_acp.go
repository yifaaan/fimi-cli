package app

import (
	"context"
	"fmt"
	"os"

	"fimi-cli/internal/acp"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/ui"
)

// runACP 启动 ACP JSON-RPC 服务器。
func (d dependencies) runACP(ctx context.Context) error {
	loadConfig := d.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	conn := acp.NewFramedConn(os.Stdin, os.Stdout)

	resolveWorkDir := d.resolveWorkDir
	if resolveWorkDir == nil {
		resolveWorkDir = os.Getwd
	}

	workDir, err := resolveWorkDir()
	if err != nil {
		return fmt.Errorf("get work dir: %w", err)
	}

	registry := d.resolveToolRegistry()
	agent, err := d.loadRunAgent(workDir, registry)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	runFn := func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
		runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
		if err != nil {
			return runtime.Result{}, fmt.Errorf("build ACP runner: %w", err)
		}
		defer closeRuntimeRunner(runner)

		return ui.Run(ctx, runner.Run, store, input, visualize)
	}

	server := acp.NewServer(conn, cfg, runFn)
	return server.Serve(ctx)
}
