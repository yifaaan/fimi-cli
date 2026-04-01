package shell

import (
	"os"
	"path/filepath"
	"strings"

	"fimi-cli/internal/ui/shell/completer"
	tea "github.com/charmbracelet/bubbletea"
)

// handleFileIndexResult handles asynchronously-indexed file paths.
func (m Model) handleFileIndexResult(msg FileIndexResultMsg) (tea.Model, tea.Cmd) {
	if !m.showFileCompletion || m.fileIndexer == nil {
		return m, nil
	}

	input := m.input.Value()
	fragment, atPos, ok := completer.ExtractFragment(input, m.input.CursorPos())
	if !ok {
		m.showFileCompletion = false
		m.fileCompletionItems = nil
		return m, nil
	}

	m.fileCompletionItems = completer.FilterAndRank(fragment, msg.Paths, 20)
	m.fileCompletionAtPos = atPos
	m.showFileCompletion = len(m.fileCompletionItems) > 0
	m.selectedFileCompletion = 0
	return m, nil
}

// refreshFileIndex kicks off an async file index refresh.
func (m Model) refreshFileIndex() tea.Cmd {
	if m.fileIndexer == nil {
		return nil
	}
	return func() tea.Msg {
		paths := m.fileIndexer.Paths("x/y") // force deep walk
		return FileIndexResultMsg{Paths: paths}
	}
}

// isCompletedFile checks if the fragment resolves to an existing file.
func isCompletedFile(root, fragment string) bool {
	candidate := strings.TrimRight(fragment, "/")
	if candidate == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, candidate))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// renderFileCompletion renders the file completion dropdown.
func (m Model) renderFileCompletion() string {
	if !m.showFileCompletion || len(m.fileCompletionItems) == 0 {
		return ""
	}

	var items []string
	maxDisplay := 5
	if len(m.fileCompletionItems) < maxDisplay {
		maxDisplay = len(m.fileCompletionItems)
	}

	for i := 0; i < maxDisplay; i++ {
		items = append(items, m.fileCompletionItems[i])
	}

	return m.renderDropdown("Files", items, m.selectedFileCompletion, len(m.fileCompletionItems)-maxDisplay)
}

// resetFileCompletion clears file completion state.
func (m *Model) resetFileCompletion() {
	m.showFileCompletion = false
	m.fileCompletionItems = nil
	m.selectedFileCompletion = 0
	m.fileCompletionAtPos = 0
}

// Suppress unused import (tea is used in handleFileIndexResult return signature)
var _ tea.Msg = FileIndexResultMsg{}
