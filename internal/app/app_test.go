package app

import (
	"errors"
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
		HistoryWindow: config.HistoryWindow{
			RuntimeTurns: 7,
		},
	}

	got := buildRuntimeConfig(cfg)
	if got.ReplyHistoryTurnLimit != 7 {
		t.Fatalf("buildRuntimeConfig().ReplyHistoryTurnLimit = %d, want %d", got.ReplyHistoryTurnLimit, 7)
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

func TestDependenciesRunUsesInjectedRunnerBuilder(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "history.jsonl")
	runner := &stubRunner{
		result: runtime.Result{
			AppendedRecords: []contextstore.TextRecord{
				contextstore.NewUserTextRecord("fix tests"),
				contextstore.NewAssistantTextRecord("runner reply"),
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
	errOpenSessionFailed := errors.New("open session failed")
	errBuildRunnerFailed := errors.New("build runner failed")
	errRunnerFailed := errors.New("runner failed")

	tests := []struct {
		name    string
		setup   func(t *testing.T) dependencies
		wantErr error
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

			err := deps.run([]string{"fix", "tests"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want wrapped %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildEngineUsesPlaceholderByDefault(t *testing.T) {
	cfg := config.Config{
		DefaultModel: config.DefaultModelName,
		Models: map[string]config.ModelConfig{
			config.DefaultModelName: {
				Provider: config.ProviderTypePlaceholder,
				Model:    config.DefaultModelName,
			},
		},
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 3,
		},
	}

	engine, err := buildEngine(cfg)
	if err != nil {
		t.Fatalf("buildEngine() error = %v", err)
	}

	reply, err := engine.Reply(runtime.ReplyInput{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
}

func TestDependenciesBuildEngineUsesInjectedClientBuilder(t *testing.T) {
	var gotCfg config.Config
	deps := dependencies{
		buildLLMClient: func(cfg config.Config) (llm.Client, error) {
			gotCfg = cfg
			return llm.NewPlaceholderClient(), nil
		},
	}

	engine, err := deps.buildEngine(config.Config{
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: config.ProviderTypePlaceholder,
				Model:    "custom-model",
			},
		},
		HistoryWindow: config.HistoryWindow{
			LLMTurns: 3,
		},
	})
	if err != nil {
		t.Fatalf("buildEngine() error = %v", err)
	}

	if gotCfg.DefaultModel != "custom-model" {
		t.Fatalf("builder got DefaultModel = %q, want %q", gotCfg.DefaultModel, "custom-model")
	}
	if gotCfg.Models["custom-model"].Provider != config.ProviderTypePlaceholder {
		t.Fatalf("builder got provider = %q, want %q", gotCfg.Models["custom-model"].Provider, config.ProviderTypePlaceholder)
	}

	reply, err := engine.Reply(runtime.ReplyInput{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply != "assistant placeholder reply: hello" {
		t.Fatalf("Reply() = %q, want %q", reply, "assistant placeholder reply: hello")
	}
}

func TestBuildEngineReturnsErrorForUnsupportedMode(t *testing.T) {
	_, err := buildEngine(config.Config{
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
		t.Fatalf("buildEngine() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

func TestBuildLLMClientForProviderReturnsErrorForUnsupportedType(t *testing.T) {
	_, err := buildLLMClientForProvider(
		"custom-provider",
		config.ProviderConfig{Type: "unsupported"},
		config.ModelConfig{Model: "broken-model"},
	)
	if !errors.Is(err, ErrUnsupportedProviderType) {
		t.Fatalf("buildLLMClientForProvider() error = %v, want wrapped %v", err, ErrUnsupportedProviderType)
	}
}

func TestBuildLLMClientForProviderUsesPlaceholderBuilder(t *testing.T) {
	client, err := buildLLMClientForProvider(
		config.DefaultProviderName,
		config.ProviderConfig{Type: config.ProviderTypePlaceholder},
		config.ModelConfig{Model: "unused"},
	)
	if err != nil {
		t.Fatalf("buildLLMClientForProvider() error = %v", err)
	}

	reply, err := client.Reply(llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("client.Reply() error = %v", err)
	}
	if reply.Text != "assistant placeholder reply: hello" {
		t.Fatalf("client.Reply().Text = %q, want %q", reply.Text, "assistant placeholder reply: hello")
	}
}

func TestBuildLLMClientForProviderUsesQWENBuilder(t *testing.T) {
	_, err := buildLLMClientForProvider(
		"aliyun-prod",
		config.ProviderConfig{
			Type:    config.ProviderTypeQWEN,
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		config.ModelConfig{
			Model: "qwen-plus",
		},
	)
	if err == nil {
		t.Fatalf("buildLLMClientForProvider() error = nil, want non-nil")
	}
	want := "qwen api_key is required; set providers.aliyun-prod.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)"
	if err.Error() != want {
		t.Fatalf("buildLLMClientForProvider() error = %q, want %q", err.Error(), want)
	}
}

func TestBuildEngineResolvesProviderByTypeNotProviderName(t *testing.T) {
	_, err := buildEngine(config.Config{
		DefaultModel: "qwen-workhorse",
		Models: map[string]config.ModelConfig{
			"qwen-workhorse": {
				Provider: "aliyun-prod",
				Model:    "qwen-plus",
			},
		},
		Providers: map[string]config.ProviderConfig{
			"aliyun-prod": {
				Type:    config.ProviderTypeQWEN,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			},
		},
	})
	if err == nil {
		t.Fatalf("buildEngine() error = nil, want non-nil")
	}
	want := "qwen api_key is required; set providers.aliyun-prod.api_key in your ~/.config/fimi/config.json (get your key from https://dashscope.console.aliyun.com/apiKey)"
	if err.Error() != "build llm client: "+want {
		t.Fatalf("buildEngine() error = %q, want %q", err.Error(), "build llm client: "+want)
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
	if !reflect.DeepEqual(result.AppendedRecords, wantResult) {
		t.Fatalf("Run().AppendedRecords = %#v, want %#v", result.AppendedRecords, wantResult)
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
		for _, record := range r.result.AppendedRecords {
			if err := ctx.Append(record); err != nil {
				return runtime.Result{}, err
			}
		}
	}

	return r.result, nil
}
