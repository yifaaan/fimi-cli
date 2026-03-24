package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/llm"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/session"
)

func TestBuildLLMConfig(t *testing.T) {
	cfg := config.Config{
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 5,
		},
	}

	got := buildLLMConfig(cfg)
	if got.HistoryTurnLimit != 5 {
		t.Fatalf("buildLLMConfig().HistoryTurnLimit = %d, want %d", got.HistoryTurnLimit, 5)
	}
}

func TestBuildRuntimeConfig(t *testing.T) {
	cfg := config.Config{
		LoopControl: config.LoopControl{
			MaxStepsPerRun: 9,
		},
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 7,
		},
	}

	got := buildRuntimeConfig(cfg)
	if got.ReplyHistoryTurnLimit != 7 {
		t.Fatalf("buildRuntimeConfig().ReplyHistoryTurnLimit = %d, want %d", got.ReplyHistoryTurnLimit, 7)
	}
	if got.MaxStepsPerRun != 9 {
		t.Fatalf("buildRuntimeConfig().MaxStepsPerRun = %d, want %d", got.MaxStepsPerRun, 9)
	}
}

func TestBuildRuntimeInput(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
		SystemPrompt: "You are the configured agent.",
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input)
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "custom-model",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestApplyRunInputToConfig(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "default-model",
			},
			"fast-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "fast-model",
			},
		},
	}

	got, err := applyRunInputToConfig(cfg, runInput{modelAlias: "fast-model"})
	if err != nil {
		t.Fatalf("applyRunInputToConfig() error = %v", err)
	}
	if got.DefaultModel != "fast-model" {
		t.Fatalf("applyRunInputToConfig().DefaultModel = %q, want %q", got.DefaultModel, "fast-model")
	}
	if cfg.DefaultModel != "default-model" {
		t.Fatalf("original cfg.DefaultModel = %q, want %q", cfg.DefaultModel, "default-model")
	}
}

func TestApplyRunInputToConfigReturnsErrorForUnknownModelAlias(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "default-model",
		Models: map[string]config.ModelConfig{
			"default-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "default-model",
			},
		},
	}

	_, err := applyRunInputToConfig(cfg, runInput{modelAlias: "missing-model"})
	if err == nil {
		t.Fatalf("applyRunInputToConfig() error = nil, want non-nil")
	}
	if err.Error() != `model "missing-model" not found in config.models` {
		t.Fatalf("applyRunInputToConfig() error = %q, want %q", err.Error(), `model "missing-model" not found in config.models`)
	}
}

func TestBuildRuntimeInputUsesConfiguredModelName(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "primary",
		SystemPrompt: "You are the configured agent.",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider: config.ProviderTypeQWEN,
				Model:    "qwen-plus",
			},
		},
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input)
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "qwen-plus",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestBuildRuntimeInputFallsBackToModelAliasWhenModelNameEmpty(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "primary",
		SystemPrompt: "You are the configured agent.",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider: config.ProviderTypeQWEN,
			},
		},
	}
	input := runInput{
		prompt: "fix the test",
	}

	got := buildRuntimeInput(cfg, input)
	want := runtime.Input{
		Prompt:       "fix the test",
		Model:        "primary",
		SystemPrompt: "You are the configured agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRuntimeInput() = %#v, want %#v", got, want)
	}
}

func TestHelpText(t *testing.T) {
	got := helpText()
	want := "" +
		"Usage:\n" +
		"  fimi [--new-session] [--model <alias>] [--help] [prompt...]\n" +
		"  fimi [options] -- [prompt text starting with flags]\n" +
		"\n" +
		"Flags:\n" +
		"  --new-session    Start a fresh session for this run\n" +
		"  --model <alias>  Override the configured model for this run\n" +
		"  -h, --help       Show this help message\n" +
		"\n" +
		"Prompt Rules:\n" +
		"  --                Stop parsing flags; everything after it is prompt text\n" +
		"  prompt...         Remaining args are joined into one prompt string\n" +
		"\n" +
		"Examples:\n" +
		"  fimi --new-session fix the flaky test\n" +
		"  fimi --new-session --model fast-model refactor the session loader\n" +
		"  fimi -- --help should be treated as prompt text\n"

	if got != want {
		t.Fatalf("helpText() = %q, want %q", got, want)
	}
}

func TestPrintHelpWritesHelpText(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer reader.Close()

	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	printHelp()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if string(data) != helpText() {
		t.Fatalf("printHelp() output = %q, want %q", string(data), helpText())
	}
}

func TestParseRunInput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runInput
		wantErr error
	}{
		{
			name: "prompt only",
			args: []string{"fix", "tests"},
			want: runInput{
				prompt: "fix tests",
			},
		},
		{
			name: "force new session flag removed from prompt",
			args: []string{"--new-session", "fix", "tests"},
			want: runInput{
				prompt:          "fix tests",
				forceNewSession: true,
			},
		},
		{
			name: "force new session without prompt",
			args: []string{"--new-session"},
			want: runInput{
				forceNewSession: true,
			},
		},
		{
			name: "model override",
			args: []string{"--model", "fast-model", "fix", "tests"},
			want: runInput{
				prompt:     "fix tests",
				modelAlias: "fast-model",
			},
		},
		{
			name: "model override and new session",
			args: []string{"--new-session", "--model", "fast-model", "fix"},
			want: runInput{
				prompt:          "fix",
				forceNewSession: true,
				modelAlias:      "fast-model",
			},
		},
		{
			name: "help long flag",
			args: []string{"--help"},
			want: runInput{
				showHelp: true,
			},
		},
		{
			name: "help short flag",
			args: []string{"-h"},
			want: runInput{
				showHelp: true,
			},
		},
		{
			name: "flag terminator keeps literal flag in prompt",
			args: []string{"--new-session", "--", "--new-session", "fix"},
			want: runInput{
				prompt:          "--new-session fix",
				forceNewSession: true,
			},
		},
		{
			name: "flag terminator keeps literal help flag in prompt",
			args: []string{"--", "--help", "fix"},
			want: runInput{
				prompt: "--help fix",
			},
		},
		{
			name: "flag terminator keeps literal model flag in prompt",
			args: []string{"--", "--model", "fast-model", "fix"},
			want: runInput{
				prompt: "--model fast-model fix",
			},
		},
		{
			name:    "unknown flag",
			args:    []string{"--bad-flag", "fix"},
			wantErr: ErrUnknownCLIFlag,
		},
		{
			name:    "model flag requires value",
			args:    []string{"--model"},
			wantErr: ErrCLIFlagValueRequired,
		},
		{
			name:    "model flag rejects another flag as value",
			args:    []string{"--model", "--new-session", "fix"},
			wantErr: ErrCLIFlagValueRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunInput(tt.args)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseRunInput() error = %v, want wrapped %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseRunInput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDependenciesRunUsesInjectedProcessDependencies(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var gotWorkDir string
	var gotModelAlias string

	type printedState struct {
		session       session.Session
		historyPath   string
		state         startupState
		sessionReused bool
		model         string
		called        bool
	}

	printed := printedState{}
	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				SystemPrompt: "You are the configured agent.",
				Models: map[string]config.ModelConfig{
					"custom-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "custom-model",
					},
				},
				HistoryWindow: config.HistoryWindow{
					RuntimeTurns: 2,
					LLMTurns:     1,
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		openSession: func(workDir string) (session.Session, bool, error) {
			gotWorkDir = workDir
			return session.Session{
				ID:          "session-123",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, true, nil
		},
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotModelAlias = cfg.DefaultModel
			return llm.NewPlaceholderClient(), nil
		},
		printStartupState: func(
			sess session.Session,
			ctx contextstore.Context,
			state startupState,
			sessionReused bool,
			model string,
		) {
			printed.session = sess
			printed.historyPath = ctx.Path()
			printed.state = state
			printed.sessionReused = sessionReused
			printed.model = model
			printed.called = true
		},
	}

	err := deps.run([]string{"fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotWorkDir != "/tmp/fimi-project" {
		t.Fatalf("openSession() got workDir = %q, want %q", gotWorkDir, "/tmp/fimi-project")
	}
	if gotModelAlias != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotModelAlias, "custom-model")
	}
	if !printed.called {
		t.Fatalf("printStartupState() called = false, want true")
	}
	if printed.session.ID != "session-123" {
		t.Fatalf("printed session ID = %q, want %q", printed.session.ID, "session-123")
	}
	if printed.historyPath != historyFile {
		t.Fatalf("printed history path = %q, want %q", printed.historyPath, historyFile)
	}
	if !printed.sessionReused {
		t.Fatalf("printed sessionReused = false, want true")
	}
	if printed.model != "custom-model" {
		t.Fatalf("printed model = %q, want %q", printed.model, "custom-model")
	}

	wantLastRecord := contextstore.NewAssistantTextRecord("assistant placeholder reply: fix tests")
	if !printed.state.historyExists {
		t.Fatalf("printed historyExists = false, want true")
	}
	if !printed.state.historySeeded {
		t.Fatalf("printed historySeeded = false, want true")
	}
	if printed.state.historyCount != 3 {
		t.Fatalf("printed historyCount = %d, want %d", printed.state.historyCount, 3)
	}
	if !printed.state.hasLastRecord {
		t.Fatalf("printed hasLastRecord = false, want true")
	}
	if printed.state.lastRecord != wantLastRecord {
		t.Fatalf("printed lastRecord = %#v, want %#v", printed.state.lastRecord, wantLastRecord)
	}

	ctx := contextstore.New(historyFile)
	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	wantRecords := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord(initialRecordContent),
		contextstore.NewUserTextRecord("fix tests"),
		wantLastRecord,
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("history records = %#v, want %#v", records, wantRecords)
	}
}

func TestDependenciesRunPrintsHelpBeforeLoadingConfig(t *testing.T) {
	var loadConfigCalled bool
	var resolveWorkDirCalled bool
	var openSessionCalled bool
	var buildRunnerCalled bool
	var helpCalled bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			loadConfigCalled = true
			return config.Config{}, nil
		},
		resolveWorkDir: func() (string, error) {
			resolveWorkDirCalled = true
			return "", nil
		},
		openSession: func(workDir string) (session.Session, bool, error) {
			openSessionCalled = true
			return session.Session{}, false, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			buildRunnerCalled = true
			return &stubRunner{}, nil
		},
		printHelp: func() {
			helpCalled = true
		},
	}

	err := deps.run([]string{"--help"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !helpCalled {
		t.Fatalf("printHelp() called = false, want true")
	}
	if loadConfigCalled {
		t.Fatalf("loadConfig() called = true, want false")
	}
	if resolveWorkDirCalled {
		t.Fatalf("resolveWorkDir() called = true, want false")
	}
	if openSessionCalled {
		t.Fatalf("openSession() called = true, want false")
	}
	if buildRunnerCalled {
		t.Fatalf("buildRuntimeRunner() called = true, want false")
	}
}

func TestDependenciesRunAppliesModelOverrideToRunnerAndPrinter(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var gotRunnerCfg config.Config
	var printedModel string

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "default-model",
				SystemPrompt: "You are the configured agent.",
				Models: map[string]config.ModelConfig{
					"default-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "default-model",
					},
					"fast-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "actual-fast-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		openSession: func(workDir string) (session.Session, bool, error) {
			return session.Session{
				ID:          "session-123",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, true, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			gotRunnerCfg = cfg
			return &stubRunner{
				result: runtime.Result{
					Steps: []runtime.StepResult{
						{
							Kind: runtime.StepKindFinished,
							AppendedRecords: []contextstore.TextRecord{
								contextstore.NewUserTextRecord("fix tests"),
								contextstore.NewAssistantTextRecord("runner reply"),
							},
						},
					},
				},
				appendToContext: true,
			}, nil
		},
		printStartupState: func(
			sess session.Session,
			ctx contextstore.Context,
			state startupState,
			sessionReused bool,
			model string,
		) {
			printedModel = model
		},
	}

	err := deps.run([]string{"--model", "fast-model", "fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotRunnerCfg.DefaultModel != "fast-model" {
		t.Fatalf("runner cfg.DefaultModel = %q, want %q", gotRunnerCfg.DefaultModel, "fast-model")
	}
	if printedModel != "actual-fast-model" {
		t.Fatalf("printed model = %q, want %q", printedModel, "actual-fast-model")
	}
}

func TestDependenciesRunCreatesNewSessionWhenRequested(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	var openCalled bool
	var createCalled bool
	var gotCreateWorkDir string
	var printedSession session.Session
	var printedReused bool

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				SystemPrompt: "You are the configured agent.",
				Models: map[string]config.ModelConfig{
					"custom-model": {
						Provider: config.ProviderTypePlaceholder,
						Model:    "custom-model",
					},
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		openSession: func(workDir string) (session.Session, bool, error) {
			openCalled = true
			return session.Session{}, true, nil
		},
		createSession: func(workDir string) (session.Session, error) {
			createCalled = true
			gotCreateWorkDir = workDir
			return session.Session{
				ID:          "session-new",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			return &stubRunner{
				result: runtime.Result{
					Steps: []runtime.StepResult{
						{
							Kind: runtime.StepKindFinished,
							AppendedRecords: []contextstore.TextRecord{
								contextstore.NewUserTextRecord("fix tests"),
								contextstore.NewAssistantTextRecord("runner reply"),
							},
						},
					},
				},
				appendToContext: true,
			}, nil
		},
		printStartupState: func(
			sess session.Session,
			ctx contextstore.Context,
			state startupState,
			sessionReused bool,
			model string,
		) {
			printedSession = sess
			printedReused = sessionReused
		},
	}

	err := deps.run([]string{"--new-session", "fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if openCalled {
		t.Fatalf("openSession() called = true, want false")
	}
	if !createCalled {
		t.Fatalf("createSession() called = false, want true")
	}
	if gotCreateWorkDir != "/tmp/fimi-project" {
		t.Fatalf("createSession() got workDir = %q, want %q", gotCreateWorkDir, "/tmp/fimi-project")
	}
	if printedSession.ID != "session-new" {
		t.Fatalf("printed session ID = %q, want %q", printedSession.ID, "session-new")
	}
	if printedReused {
		t.Fatalf("printed sessionReused = true, want false")
	}
}

func TestDependenciesRunUsesInjectedRunnerBuilder(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	runner := &stubRunner{
		result: runtime.Result{
			Steps: []runtime.StepResult{
				{
					Kind: runtime.StepKindFinished,
					AppendedRecords: []contextstore.TextRecord{
						contextstore.NewUserTextRecord("fix tests"),
						contextstore.NewAssistantTextRecord("runner reply"),
					},
				},
			},
		},
		appendToContext: true,
	}
	var gotCfg config.Config
	var printed startupState

	deps := dependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "custom-model",
				SystemPrompt: "You are the configured agent.",
				HistoryWindow: config.HistoryWindow{
					RuntimeTurns: 4,
				},
			}, nil
		},
		resolveWorkDir: func() (string, error) {
			return "/tmp/fimi-project", nil
		},
		openSession: func(workDir string) (session.Session, bool, error) {
			return session.Session{
				ID:          "session-456",
				WorkDir:     workDir,
				HistoryFile: historyFile,
			}, false, nil
		},
		buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
			gotCfg = cfg
			return runner, nil
		},
		printStartupState: func(
			sess session.Session,
			ctx contextstore.Context,
			state startupState,
			sessionReused bool,
			model string,
		) {
			printed = state
		},
	}

	err := deps.run([]string{"fix", "tests"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("runner builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if !reflect.DeepEqual(runner.gotInput, runtime.Input{
		Prompt:       "fix tests",
		Model:        "custom-model",
		SystemPrompt: "You are the configured agent.",
	}) {
		t.Fatalf("runner got Input = %#v, want %#v", runner.gotInput, runtime.Input{
			Prompt:       "fix tests",
			Model:        "custom-model",
			SystemPrompt: "You are the configured agent.",
		})
	}
	if runner.gotCtx.Path() != historyFile {
		t.Fatalf("runner got history path = %q, want %q", runner.gotCtx.Path(), historyFile)
	}
	if printed.historyCount != 3 {
		t.Fatalf("printed historyCount = %d, want %d", printed.historyCount, 3)
	}
	if printed.lastRecord != contextstore.NewAssistantTextRecord("runner reply") {
		t.Fatalf("printed lastRecord = %#v, want %#v", printed.lastRecord, contextstore.NewAssistantTextRecord("runner reply"))
	}
}

func TestDependenciesRunWrapsBoundaryErrors(t *testing.T) {
	errConfigFailed := errors.New("config failed")
	errGetWDFailed := errors.New("getwd failed")
	errParseInputFailed := ErrUnknownCLIFlag
	errFlagValueRequired := ErrCLIFlagValueRequired
	errOpenSessionFailed := errors.New("open session failed")
	errCreateSessionFailed := errors.New("create session failed")
	errBuildRunnerFailed := errors.New("build runner failed")
	errRunnerFailed := errors.New("runner failed")

	tests := []struct {
		name        string
		setup       func(t *testing.T) dependencies
		wantErr     error
		wantErrText string
	}{
		{
			name: "load config",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Config{}, errConfigFailed
					},
				}
			},
			wantErr: errConfigFailed,
		},
		{
			name: "parse input",
			setup: func(t *testing.T) dependencies {
				return dependencies{}
			},
			wantErr: errParseInputFailed,
		},
		{
			name: "flag value required",
			setup: func(t *testing.T) dependencies {
				return dependencies{}
			},
			wantErr: errFlagValueRequired,
		},
		{
			name: "apply model override",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Config{
							DefaultModel: "default-model",
							Models: map[string]config.ModelConfig{
								"default-model": {
									Provider: config.ProviderTypePlaceholder,
									Model:    "default-model",
								},
							},
						}, nil
					},
				}
			},
			wantErrText: `model "missing-model" not found in config.models`,
		},
		{
			name: "resolve work dir",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "", errGetWDFailed
					},
				}
			},
			wantErr: errGetWDFailed,
		},
		{
			name: "open session",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					openSession: func(workDir string) (session.Session, bool, error) {
						return session.Session{}, false, errOpenSessionFailed
					},
				}
			},
			wantErr: errOpenSessionFailed,
		},
		{
			name: "create session",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					createSession: func(workDir string) (session.Session, error) {
						return session.Session{}, errCreateSessionFailed
					},
				}
			},
			wantErr: errCreateSessionFailed,
		},
		{
			name: "build runner",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					openSession: func(workDir string) (session.Session, bool, error) {
						return session.Session{
							ID:          "session-123",
							WorkDir:     workDir,
							HistoryFile: filepath.Join(t.TempDir(), "history.jsonl"),
						}, true, nil
					},
					buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
						return nil, errBuildRunnerFailed
					},
				}
			},
			wantErr: errBuildRunnerFailed,
		},
		{
			name: "run runtime",
			setup: func(t *testing.T) dependencies {
				return dependencies{
					loadConfig: func() (config.Config, error) {
						return config.Default(), nil
					},
					resolveWorkDir: func() (string, error) {
						return "/tmp/fimi-project", nil
					},
					openSession: func(workDir string) (session.Session, bool, error) {
						return session.Session{
							ID:          "session-123",
							WorkDir:     workDir,
							HistoryFile: filepath.Join(t.TempDir(), "history.jsonl"),
						}, true, nil
					},
					buildRuntimeRunner: func(cfg config.Config) (runtimeRunner, error) {
						return &stubRunner{err: errRunnerFailed}, nil
					},
				}
			},
			wantErr: errRunnerFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := tt.setup(t)

			args := []string{"fix", "tests"}
			if tt.name == "parse input" {
				args = []string{"--bad-flag", "fix", "tests"}
			}
			if tt.name == "flag value required" {
				args = []string{"--model"}
			}
			if tt.name == "apply model override" {
				args = []string{"--model", "missing-model", "fix", "tests"}
			}
			if tt.name == "create session" {
				args = []string{"--new-session", "fix", "tests"}
			}

			err := deps.run(args)
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("run() error = nil, want %q", tt.wantErrText)
				}
				if err.Error() != tt.wantErrText {
					t.Fatalf("run() error = %q, want %q", err.Error(), tt.wantErrText)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want wrapped %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildRunnerRunsWithWiredPlaceholderEngine(t *testing.T) {
	cfg := config.Config{
		DefaultModel: "custom-model",
		SystemPrompt: "You are the configured agent.",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 2,
			LLMTurns:     1,
		},
	}
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))
	runner, err := buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner() error = %v", err)
	}

	result, err := runner.Run(ctx, runtime.Input{
		Prompt:       "hello",
		Model:        cfg.DefaultModel,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantResult := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
		contextstore.NewAssistantTextRecord("assistant placeholder reply: hello"),
	}
	if len(result.Steps) != 1 {
		t.Fatalf("len(Run().Steps) = %d, want 1", len(result.Steps))
	}
	if !reflect.DeepEqual(result.Steps[0].AppendedRecords, wantResult) {
		t.Fatalf("Run().Steps[0].AppendedRecords = %#v, want %#v", result.Steps[0].AppendedRecords, wantResult)
	}

	records, err := ctx.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(records, wantResult) {
		t.Fatalf("history records = %#v, want %#v", records, wantResult)
	}
}

func TestDependenciesBuildRunnerUsesInjectedClientBuilder(t *testing.T) {
	var gotCfg config.Config
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotCfg = cfg
			return llm.NewPlaceholderClient(), nil
		},
	}
	cfg := config.Config{
		DefaultModel: "custom-model",
		SystemPrompt: "You are the configured agent.",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
	}
	ctx := contextstore.New(filepath.Join(t.TempDir(), "history.jsonl"))

	runner, err := deps.buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner() error = %v", err)
	}

	_, err = runner.Run(ctx, runtime.Input{
		Prompt:       "hello",
		Model:        cfg.DefaultModel,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if gotCfg.Models["custom-model"].Provider != config.ProviderTypePlaceholder {
		t.Fatalf("builder got provider = %q, want %q", gotCfg.Models["custom-model"].Provider, config.ProviderTypePlaceholder)
	}
}

func TestBuildRunnerReturnsErrorForUnsupportedMode(t *testing.T) {
	_, err := buildRunner(config.Config{
		DefaultModel: "broken",
		Models: map[string]config.ModelConfig{
			"broken": {
				Provider: "custom-provider",
				Model:    "broken-model",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"custom-provider": {
				Type: "unsupported",
			},
		},
	})
	if !errors.Is(err, ErrUnsupportedProviderType) {
		t.Fatalf("buildRunner() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

type stubRunner struct {
	gotCtx          contextstore.Context
	gotInput        runtime.Input
	result          runtime.Result
	err             error
	appendToContext bool
}

func (r *stubRunner) Run(ctx contextstore.Context, input runtime.Input) (runtime.Result, error) {
	r.gotCtx = ctx
	r.gotInput = input
	if r.err != nil {
		return runtime.Result{}, r.err
	}

	if r.appendToContext {
		for _, step := range r.result.Steps {
			for _, record := range step.AppendedRecords {
				if err := ctx.Append(record); err != nil {
					return runtime.Result{}, err
				}
			}
		}
	}

	return r.result, nil
}
