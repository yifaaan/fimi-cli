package completer

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// FileIndexer indexes workspace files with a 2-tier TTL cache.
// Top-tier scans only immediate children of root; deep-tier walks recursively.
type FileIndexer struct {
	root string

	mu           sync.Mutex
	topPaths     []string
	topCachedAt  time.Time
	deepPaths    []string
	deepCachedAt time.Time

	ttl   time.Duration
	limit int
}

// NewFileIndexer creates a file indexer rooted at root.
// Defaults: TTL=2s, limit=1000.
func NewFileIndexer(root string) *FileIndexer {
	return &FileIndexer{
		root:  root,
		ttl:   2 * time.Second,
		limit: 1000,
	}
}

// Paths returns cached file paths appropriate for the given fragment.
// Short fragments (< 3 chars, no '/') use top-level scan; others use deep walk.
func (f *FileIndexer) Paths(fragment string) []string {
	if !stringsContains(fragment, '/') && len(fragment) < 3 {
		return f.scanTopLevel()
	}
	return f.scanDeep()
}

// IsIgnored reports whether a file/dir name should be excluded from indexing.
func (f *FileIndexer) IsIgnored(name string) bool {
	return isIgnoredName(name)
}

func (f *FileIndexer) scanTopLevel() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if time.Since(f.topCachedAt) <= f.ttl && f.topPaths != nil {
		return f.topPaths
	}

	entries, err := os.ReadDir(f.root)
	if err != nil {
		return f.topPaths
	}

	paths := make([]string, 0, min(len(entries), f.limit))
	for _, e := range entries {
		if isIgnoredName(e.Name()) {
			continue
		}
		if e.IsDir() {
			paths = append(paths, e.Name()+"/")
		} else {
			paths = append(paths, e.Name())
		}
		if len(paths) >= f.limit {
			break
		}
	}

	f.topPaths = paths
	f.topCachedAt = time.Now()
	return paths
}

func (f *FileIndexer) scanDeep() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if time.Since(f.deepCachedAt) <= f.ttl && f.deepPaths != nil {
		return f.deepPaths
	}

	paths := make([]string, 0, f.limit)
	filepath.WalkDir(f.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(f.root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}

		// Convert to forward slashes for consistency
		rel = filepath.ToSlash(rel)

		name := d.Name()
		if isIgnoredName(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Also check parent path components
		if hasIgnoredComponent(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			paths = append(paths, rel+"/")
		} else {
			paths = append(paths, rel)
		}

		if len(paths) >= f.limit {
			return filepath.SkipDir
		}
		return nil
	})

	sort.Strings(paths)

	f.deepPaths = paths
	f.deepCachedAt = time.Now()
	return paths
}

func hasIgnoredComponent(rel string) bool {
	parts := splitPath(rel)
	for i := 0; i < len(parts)-1; i++ {
		if isIgnoredName(parts[i]) {
			return true
		}
	}
	return false
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	var parts []string
	for {
		i := len(p) - 1
		for i >= 0 && p[i] != '/' {
			i--
		}
		parts = append(parts, p[i+1:])
		if i <= 0 {
			break
		}
		p = p[:i]
	}
	// Reverse to get top-down order
	for l, r := 0, len(parts)-1; l < r; l, r = l+1, r-1 {
		parts[l], parts[r] = parts[r], parts[l]
	}
	return parts
}

func stringsContains(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// --- Ignore rules (ported from Python FileMentionCompleter) ---

var ignoredNames = map[string]bool{
	// VCS metadata
	".DS_Store": true, ".bzr": true, ".git": true, ".hg": true, ".svn": true,
	// Tooling caches
	".build": true, ".cache": true, ".coverage": true, ".fleet": true,
	".gradle": true, ".idea": true, ".ipynb_checkpoints": true,
	".pnpm-store": true, ".pytest_cache": true, ".pub-cache": true,
	".ruff_cache": true, ".swiftpm": true, ".tox": true, ".venv": true,
	".vs": true, ".vscode": true, ".yarn": true, ".yarn-cache": true,
	// JS/frontend
	".next": true, ".nuxt": true, ".parcel-cache": true,
	".svelte-kit": true, ".turbo": true, ".vercel": true,
	"node_modules": true,
	// Python packaging
	"__pycache__": true, "build": true, "coverage": true,
	"dist": true, "htmlcov": true, "pip-wheel-metadata": true, "venv": true,
	// Java/JVM
	".mvn": true, "out": true, "target": true,
	// .NET / native
	"bin": true, "cmake-build-debug": true, "cmake-build-release": true, "obj": true,
	// Bazel/Buck
	"bazel-bin": true, "bazel-out": true, "bazel-testlogs": true, "buck-out": true,
	// Misc artifacts
	".dart_tool": true, ".serverless": true, ".stack-work": true,
	".terraform": true, ".terragrunt-cache": true,
	"DerivedData": true, "Pods": true, "deps": true, "tmp": true, "vendor": true,
}

var ignoredPatterns = regexp.MustCompile(`(?i)(?:` +
	`.*_cache$|` +
	`.*-cache$|` +
	`.*\.egg-info$|` +
	`.*\.dist-info$|` +
	`.*\.py[co]$|` +
	`.*\.class$|` +
	`.*\.sw[po]$|` +
	`.*~$|` +
	`.*\.(?:tmp|bak)$` +
	`)`)

func isIgnoredName(name string) bool {
	if name == "" {
		return true
	}
	if ignoredNames[name] {
		return true
	}
	return ignoredPatterns.MatchString(name)
}
