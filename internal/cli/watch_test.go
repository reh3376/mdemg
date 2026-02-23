package cli

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestFileDebouncer_SingleFlush(t *testing.T) {
	var mu sync.Mutex
	var flushed []string

	deb := newFileDebouncer(100*time.Millisecond, func(files []string) {
		mu.Lock()
		flushed = append(flushed, files...)
		mu.Unlock()
	})

	deb.add("/tmp/test.go")

	// Wait for debounce
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Errorf("expected 1 flushed file, got %d", len(flushed))
	}
	if len(flushed) > 0 && flushed[0] != "/tmp/test.go" {
		t.Errorf("expected /tmp/test.go, got %s", flushed[0])
	}
}

func TestFileDebouncer_MultipleFiles(t *testing.T) {
	var mu sync.Mutex
	var flushCount int
	var totalFiles int

	deb := newFileDebouncer(200*time.Millisecond, func(files []string) {
		mu.Lock()
		flushCount++
		totalFiles += len(files)
		mu.Unlock()
	})

	// Add multiple files in rapid succession
	deb.add("/tmp/a.go")
	deb.add("/tmp/b.go")
	deb.add("/tmp/c.go")
	// Add duplicate - should be deduplicated
	deb.add("/tmp/a.go")

	// Wait for debounce
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if flushCount != 1 {
		t.Errorf("expected 1 flush, got %d", flushCount)
	}
	if totalFiles != 3 {
		t.Errorf("expected 3 unique files, got %d", totalFiles)
	}
}

func TestShouldWatch_ExtensionFilter(t *testing.T) {
	extSet := map[string]bool{
		".go":   true,
		".py":   true,
		".ts":   true,
		".json": true,
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"src/main.go", true},
		{"lib/utils.py", true},
		{"app/index.ts", true},
		{"config.json", true},
		{"image.png", false},
		{"binary", false},
		{".gitignore", false},
		{"Makefile", false},
		{"data.csv", false},
	}

	for _, tt := range tests {
		got := shouldWatch(tt.path, extSet)
		if got != tt.expected {
			t.Errorf("shouldWatch(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestWatchRecursive_ExcludesDirs(t *testing.T) {
	// Create a temp directory structure
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "node_modules", "pkg"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, ".git", "objects"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "vendor", "lib"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "src", "internal"), 0o755)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	excludeSet := map[string]bool{
		"node_modules": true,
		".git":         true,
		"vendor":       true,
	}

	if err := watchRecursive(watcher, tmpDir, excludeSet); err != nil {
		t.Fatalf("watchRecursive failed: %v", err)
	}

	// Check watched paths via watcher.WatchList()
	watched := watcher.WatchList()
	watchedSet := make(map[string]bool)
	for _, w := range watched {
		watchedSet[w] = true
	}

	// Should include root, src, src/internal
	if !watchedSet[tmpDir] {
		t.Errorf("expected root dir to be watched")
	}
	if !watchedSet[filepath.Join(tmpDir, "src")] {
		t.Errorf("expected src dir to be watched")
	}
	if !watchedSet[filepath.Join(tmpDir, "src", "internal")] {
		t.Errorf("expected src/internal dir to be watched")
	}

	// Should NOT include excluded dirs
	if watchedSet[filepath.Join(tmpDir, "node_modules")] {
		t.Errorf("node_modules should be excluded")
	}
	if watchedSet[filepath.Join(tmpDir, ".git")] {
		t.Errorf(".git should be excluded")
	}
	if watchedSet[filepath.Join(tmpDir, "vendor")] {
		t.Errorf("vendor should be excluded")
	}
}

func TestParseCSV(t *testing.T) {
	result := parseCSV(".go,.py,.ts")
	if !result[".go"] || !result[".py"] || !result[".ts"] {
		t.Errorf("expected all extensions to be in set: %v", result)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}

	// Test with spaces
	result2 := parseCSV(" .go , .py , .ts ")
	if !result2[".go"] || !result2[".py"] || !result2[".ts"] {
		t.Errorf("expected trimmed extensions: %v", result2)
	}

	// Test empty
	result3 := parseCSV("")
	if len(result3) != 0 {
		t.Errorf("expected empty set for empty input, got %v", result3)
	}
}
