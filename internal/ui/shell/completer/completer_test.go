package completer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		query, candidate string
		wantMatch        bool
	}{
		{"mod", "go.mod", true},
		{"mod", "model.go", true},
		{"main", "cmd/main.go", true},
		{"xyz", "go.mod", false},
		{"", "anything", true},
		{"go", "go.mod", true},
		{"Go", "go.mod", true}, // case-insensitive
	}

	for _, tt := range tests {
		matched, score := FuzzyMatch(tt.query, tt.candidate)
		if matched != tt.wantMatch {
			t.Errorf("FuzzyMatch(%q, %q) = %v, want %v", tt.query, tt.candidate, matched, tt.wantMatch)
		}
		if matched && score < 0 {
			t.Errorf("FuzzyMatch(%q, %q) matched but score=%d", tt.query, tt.candidate, score)
		}
	}
}

func TestFuzzyMatchPrefixBonus(t *testing.T) {
	_, scorePrefix := FuzzyMatch("go", "go.mod")
	_, scoreMid := FuzzyMatch("go", "algo.go")
	if scorePrefix <= scoreMid {
		t.Errorf("prefix match score (%d) should be > mid-match score (%d)", scorePrefix, scoreMid)
	}
}

func TestFilterAndRank(t *testing.T) {
	candidates := []string{
		"go.mod",
		"go.sum",
		"cmd/main.go",
		"internal/app/app.go",
		"model.go",
	}

	tests := []struct {
		query string
		limit int
		want  int // expected number of results
	}{
		{"go", 10, 5},      // go.mod, go.sum, cmd/main.go, internal/app/app.go, model.go
		{"mod", 10, 2},     // go.mod, model.go (but model has "mod" as subsequence? "model" -> m-o-d -> yes)
		{"xyz", 10, 0},
		{"", 3, 3},         // empty query returns first 3
	}

	for _, tt := range tests {
		results := FilterAndRank(tt.query, candidates, tt.limit)
		if len(results) != tt.want {
			t.Errorf("FilterAndRank(%q, ..., %d) returned %d results, want %d: %v",
				tt.query, tt.limit, len(results), tt.want, results)
		}
	}
}

func TestFilterAndRankOrdering(t *testing.T) {
	candidates := []string{
		"internal/app/app.go",
		"go.mod",
		"cmd/main.go",
	}
	results := FilterAndRank("go", candidates, 10)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// go.mod should score higher (prefix match) than cmd/main.go (mid match)
	if results[0] != "go.mod" {
		t.Errorf("expected go.mod as top result, got %s", results[0])
	}
}

func TestExtractFragment(t *testing.T) {
	tests := []struct {
		text      string
		cursorPos int
		wantFrag  string
		wantPos   int
		wantOK    bool
	}{
		// Basic trigger
		{"read @", 6, "", 5, true},
		{"read @go", 8, "go", 5, true},
		{"@mod", 4, "mod", 0, true},
		// No @ found
		{"read file", 9, "", -1, false},
		// Guard: alphanumeric before @
		{"foo@bar", 7, "", -1, false},
		// Guard: trigger guard before @
		{"path.@mod", 9, "", -1, false},
		// Guard: whitespace in fragment
		{"@go mod", 7, "", -1, false},
		// Multiple @: takes last one
		{"@foo @bar", 9, "bar", 5, true},
		// Cursor in middle, @ before cursor
		{"@mod test", 4, "mod", 0, true},
		// Cursor before @
		{"@mod", 0, "", -1, false},
	}

	for _, tt := range tests {
		frag, pos, ok := ExtractFragment(tt.text, tt.cursorPos)
		if ok != tt.wantOK {
			t.Errorf("extractFragment(%q, %d) ok=%v, want %v", tt.text, tt.cursorPos, ok, tt.wantOK)
			continue
		}
		if ok && (frag != tt.wantFrag || pos != tt.wantPos) {
			t.Errorf("extractFragment(%q, %d) = (%q, %d), want (%q, %d)",
				tt.text, tt.cursorPos, frag, pos, tt.wantFrag, tt.wantPos)
		}
	}
}

func TestIsIgnoredName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".git", true},
		{"node_modules", true},
		{"__pycache__", true},
		{"vendor", true},
		{"dist", true},
		{"build", true},
		{".vscode", true},
		{"foo_cache", true},
		{"my-cache", true},
		{"pkg.egg-info", true},
		{"foo.pyc", true},
		{"Foo.class", true},
		{"test.swp", true},
		{"file~", true},
		{"temp.tmp", true},
		{"old.bak", true},
		{"main.go", false},
		{"README.md", false},
		{"src", false},
		{"", true},
	}

	for _, tt := range tests {
		got := isIgnoredName(tt.name)
		if got != tt.want {
			t.Errorf("isIgnoredName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFileIndexerTopLevel(t *testing.T) {
	// Create temp dir structure
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "foo.pyc"), []byte(""), 0o644)

	idx := NewFileIndexer(dir)
	idx.ttl = 0 // force refresh every call

	paths := idx.Paths("") // empty fragment, < 3 chars -> top level

	wantCount := 3 // src/, go.mod, main.go (.git and .pyc ignored)
	if len(paths) != wantCount {
		t.Errorf("top-level paths: got %d, want %d: %v", len(paths), wantCount, paths)
	}

	// Check that .git and .pyc are excluded
	for _, p := range paths {
		if p == ".git/" || p == "foo.pyc" {
			t.Errorf("ignored path included: %s", p)
		}
	}
}

func TestFileIndexerDeepWalk(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)
	os.MkdirAll(filepath.Join(dir, "internal/app"), 0o755)
	os.MkdirAll(filepath.Join(dir, "node_modules/pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "cmd/main.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "internal/app/app.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "node_modules/pkg/index.js"), []byte(""), 0o644)

	idx := NewFileIndexer(dir)
	idx.ttl = 0

	paths := idx.Paths("cmd/") // has '/' -> deep walk

	// node_modules should be entirely skipped
	for _, p := range paths {
		if len(p) >= 13 && p[:13] == "node_modules" {
			t.Errorf("node_modules path included: %s", p)
		}
	}

	// Should include cmd/ and internal/ paths
	found := false
	for _, p := range paths {
		if p == "cmd/main.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("cmd/main.go not found in deep paths: %v", paths)
	}
}

func TestFileIndexerTTL(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte(""), 0o644)

	idx := NewFileIndexer(dir)
	idx.ttl = 10 * time.Second

	// First call populates cache
	paths1 := idx.Paths("")
	// Create new file
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte(""), 0o644)
	// Second call should return cached result (within TTL)
	paths2 := idx.Paths("")
	if len(paths1) != len(paths2) {
		t.Errorf("TTL cache should prevent refresh: %d vs %d", len(paths1), len(paths2))
	}
}
