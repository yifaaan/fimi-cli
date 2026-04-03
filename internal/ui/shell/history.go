package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type historyStore struct {
	path    string
	entries []string
}

func loadHistoryStore(path string) (historyStore, error) {
	store := historyStore{path: path}
	if strings.TrimSpace(path) == "" {
		return store, nil
	}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return historyStore{}, fmt.Errorf("open shell history %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		store.entries = append(store.entries, line)
	}
	if err := scanner.Err(); err != nil {
		return historyStore{}, fmt.Errorf("scan shell history %q: %w", path, err)
	}

	return store, nil
}

func (s *historyStore) Append(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}
	if strings.TrimSpace(s.path) == "" {
		s.entries = append(s.entries, entry)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create shell history dir for %q: %w", s.path, err)
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open shell history %q for append: %w", s.path, err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, entry); err != nil {
		return fmt.Errorf("append shell history %q: %w", s.path, err)
	}

	s.entries = append(s.entries, entry)
	return nil
}

func (s *historyStore) Clear() error {
	s.entries = nil
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create shell history dir for %q: %w", s.path, err)
	}
	if err := os.WriteFile(s.path, nil, 0o644); err != nil {
		return fmt.Errorf("clear shell history %q: %w", s.path, err)
	}
	return nil
}

func (s historyStore) Entries() []string {
	return append([]string(nil), s.entries...)
}
