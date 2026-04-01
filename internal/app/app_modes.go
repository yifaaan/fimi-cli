package app

import (
	"context"
	"fmt"
	"os"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/session"
	"fimi-cli/internal/ui"
	"fimi-cli/internal/ui/shell"
)

func resolvePrintPrompt(input runInput) (string, error) {
	prompt := input.prompt
	if prompt == "" {
		var stdinPrompt string
		_, err := fmt.Fscanln(os.Stdin, &stdinPrompt)
		if err != nil && err.Error() != "expected newline" {
			return "", fmt.Errorf("read prompt from stdin: %w", err)
		}
		prompt = stdinPrompt
	}
	if prompt == "" {
		return "", fmt.Errorf("no prompt provided; pass as argument or via stdin")
	}

	return prompt, nil
}

func (d dependencies) preparePrintStore(workDir string) (contextstore.Context, error) {
	createSession := d.createSession
	if createSession == nil {
		createSession = session.New
	}

	sess, err := createSession(workDir)
	if err != nil {
		return contextstore.Context{}, fmt.Errorf("create print session: %w", err)
	}

	return contextstore.New(sess.HistoryFile), nil
}

func (d dependencies) resolveShellUIRunner() shellUIRunner {
	runShellUI := d.runShellUI
	if runShellUI == nil {
		runShellUI = shell.Run
	}

	return runShellUI
}

func (d dependencies) runShell(
	ctx context.Context,
	cfg config.Config,
	agent loadedAgent,
	workDir string,
	input runInput,
) error {
	cfg = resolveModelOverride(cfg, agent)

	sess, sessionReused, err := d.openRunSession(workDir, input)
	if err != nil {
		return err
	}

	store := contextstore.New(sess.HistoryFile)
	state, err := bootstrapStartupState(store)
	if err != nil {
		return err
	}
	initialRecords, err := loadShellInitialRecords(cfg, store)
	if err != nil {
		return err
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}
	defer closeRuntimeRunner(runner)

	historyFile, err := session.ShellHistoryFileForWorkDir(sess.WorkDir)
	if err != nil {
		return fmt.Errorf("resolve shell history file: %w", err)
	}
	modelName := resolveRuntimeModelName(cfg)

	return d.resolveShellUIRunner()(ctx, buildShellDependencies(
		ctx,
		runner,
		store,
		agent,
		sess,
		input,
		modelName,
		historyFile,
		initialRecords,
		buildShellStartupInfo(sess, state, sessionReused, modelName),
	))
}

// runPrint 处理 text / stream-json 模式的单次执行。
// prompt 从参数读取，如果没有则尝试从 stdin 读取一行。
func (d dependencies) runPrint(
	ctx context.Context,
	cfg config.Config,
	agent loadedAgent,
	workDir string,
	input runInput,
) error {
	prompt, err := resolvePrintPrompt(input)
	if err != nil {
		return err
	}

	cfg = resolveModelOverride(cfg, agent)

	store, err := d.preparePrintStore(workDir)
	if err != nil {
		return err
	}

	runner, err := d.buildRunnerForAgent(cfg, agent, workDir)
	if err != nil {
		return err
	}
	defer closeRuntimeRunner(runner)

	runtimeInput := buildRuntimePromptInput(cfg, agent, prompt)
	printCtx := approval.WithContext(ctx, approval.New(true))
	_, err = ui.Run(printCtx, runner.Run, store, runtimeInput, d.resolveVisualizer(input.outputMode))

	return err
}
