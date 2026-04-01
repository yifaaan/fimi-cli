package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fimi-cli/internal/config"
	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// setupPhase represents phases in the setup wizard.
type setupPhase int

const (
	setupPhaseWelcome setupPhase = iota
	setupPhaseProviderSelect
	setupPhaseAPIKeyInput
	setupPhaseModelSelect
	setupPhaseSave
)

// SetupState holds temporary state during setup wizard.
type SetupState struct {
	phase            setupPhase
	selectedProvider string
	selectedModel    string
	apiKeyInput      string
	config           config.Config
}

// enterSetupMode initializes the setup wizard.
func (m Model) enterSetupMode() (tea.Model, tea.Cmd) {
	cfg, _ := config.Load()
	m.mode = ModeSetup
	m.setupState = SetupState{
		phase:  setupPhaseWelcome,
		config: cfg,
	}
	m.showBanner = false
	return m, nil
}

// renderSetupView renders the setup wizard UI.
func (m Model) renderSetupView() string {
	var lines []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)

	promptStyle := lipgloss.NewStyle().
		Foreground(styles.ColorAccent)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)

	switch m.setupState.phase {
	case setupPhaseWelcome:
		lines = append(lines, titleStyle.Render("Setup Wizard"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Current configuration:"))
		lines = append(lines, fmt.Sprintf("  Default model: %s", m.setupState.config.DefaultModel))
		lines = append(lines, "")
		if len(m.setupState.config.Models) > 0 {
			lines = append(lines, "  Configured models:")
			for alias, mc := range m.setupState.config.Models {
				lines = append(lines, fmt.Sprintf("    - %s (provider: %s)", alias, mc.Provider))
			}
		} else {
			lines = append(lines, "  No models configured.")
		}
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to start setup, Esc/q to cancel"))

	case setupPhaseProviderSelect:
		lines = append(lines, titleStyle.Render("Select Provider"))
		lines = append(lines, "")
		lines = append(lines, "  [1] qwen")
		lines = append(lines, "  [2] openai")
		lines = append(lines, "  [3] anthropic")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number to select, Esc/q to cancel"))

	case setupPhaseAPIKeyInput:
		lines = append(lines, titleStyle.Render("API Key"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Enter API key for %s:", m.setupState.selectedProvider)))
		lines = append(lines, fmt.Sprintf("  %s", strings.Repeat("*", len(m.setupState.apiKeyInput))))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Type key and press Enter, Esc/q to cancel"))

	case setupPhaseModelSelect:
		lines = append(lines, titleStyle.Render("Select Model"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Choose model for %s:", m.setupState.selectedProvider)))
		suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
		for i, model := range suggested {
			lines = append(lines, fmt.Sprintf("  [%d] %s", i+1, model))
		}
		lines = append(lines, "  Or type a custom model name")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number or custom name, Esc/q to cancel"))

	case setupPhaseSave:
		lines = append(lines, titleStyle.Render("Save Configuration"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Ready to save:"))
		lines = append(lines, fmt.Sprintf("  Provider: %s", m.setupState.selectedProvider))
		lines = append(lines, fmt.Sprintf("  Model: %s", m.setupState.selectedModel))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to save, Esc/q to cancel"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// suggestedModelsForProvider returns common model names for a provider.
func suggestedModelsForProvider(provider string) []string {
	switch provider {
	case "qwen":
		return []string{"qwen-plus", "qwen-turbo", "qwen-max"}
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	case "anthropic":
		return []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"}
	default:
		return nil
	}
}

// handleSetupKeyPress processes keyboard input in setup mode.
func (m Model) handleSetupKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	if keyStr == "esc" || keyStr == "q" {
		m.mode = ModeIdle
		m.setupState = SetupState{}
		return m, nil
	}

	switch m.setupState.phase {
	case setupPhaseWelcome:
		if keyStr == "enter" {
			m.setupState.phase = setupPhaseProviderSelect
		}
		return m, nil

	case setupPhaseProviderSelect:
		switch keyStr {
		case "1":
			m.setupState.selectedProvider = "qwen"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "2":
			m.setupState.selectedProvider = "openai"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "3":
			m.setupState.selectedProvider = "anthropic"
			m.setupState.phase = setupPhaseAPIKeyInput
		}
		return m, nil

	case setupPhaseAPIKeyInput:
		switch keyStr {
		case "enter":
			if m.setupState.apiKeyInput != "" {
				m.setupState.phase = setupPhaseModelSelect
			}
		case "backspace":
			if len(m.setupState.apiKeyInput) > 0 {
				m.setupState.apiKeyInput = m.setupState.apiKeyInput[:len(m.setupState.apiKeyInput)-1]
			}
		default:
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.apiKeyInput += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseModelSelect:
		switch keyStr {
		case "enter":
			if m.setupState.selectedModel != "" {
				m.setupState.phase = setupPhaseSave
			}
		case "backspace":
			if len(m.setupState.selectedModel) > 0 {
				m.setupState.selectedModel = m.setupState.selectedModel[:len(m.setupState.selectedModel)-1]
			}
		default:
			suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
			if len(msg.Runes) == 1 {
				num := int(msg.Runes[0] - '0')
				if num >= 1 && num <= len(suggested) {
					m.setupState.selectedModel = suggested[num-1]
					m.setupState.phase = setupPhaseSave
					return m, nil
				}
			}
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.selectedModel += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseSave:
		if keyStr == "enter" {
			return m.saveSetupConfig()
		}
		return m, nil
	}

	return m, nil
}

// saveSetupConfig writes the setup configuration and returns to idle mode.
func (m Model) saveSetupConfig() (tea.Model, tea.Cmd) {
	providerAlias := m.setupState.selectedProvider
	providerConfig := config.ProviderConfig{
		Type:   providerAlias,
		APIKey: m.setupState.apiKeyInput,
	}
	if providerAlias == "qwen" {
		providerConfig.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	m.setupState.config.Providers[providerAlias] = providerConfig

	modelAlias := m.setupState.selectedModel
	m.setupState.config.Models[modelAlias] = config.ModelConfig{
		Provider: providerAlias,
		Model:    modelAlias,
	}
	m.setupState.config.DefaultModel = modelAlias

	if err := config.Save(m.setupState.config); err != nil {
		m.mode = ModeIdle
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error saving config: %v", err),
		})
		m.setupState = SetupState{}
		return m, nil
	}

	m.mode = ModeIdle
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: "Configuration saved successfully.",
	})
	m.setupState = SetupState{}
	return m, nil
}

func (m *Model) cleanupInitTempFile() {
	if m.initTempFile != "" {
		os.Remove(m.initTempFile)
		m.initTempFile = ""
	}
}

func readAgentsMD(workDir string) string {
	for _, name := range []string{"AGENTS.md", "agents.md"} {
		data, err := os.ReadFile(filepath.Join(workDir, name))
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}
