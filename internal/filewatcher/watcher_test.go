package filewatcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewWatcher_Defaults(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(Config{
		SpaceID: "test",
		Path:    dir,
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	if len(w.extSet) != len(DefaultExtensions) {
		t.Errorf("extSet size = %d, want %d", len(w.extSet), len(DefaultExtensions))
	}
	if len(w.excludeSet) != len(DefaultExcludes) {
		t.Errorf("excludeSet size = %d, want %d", len(w.excludeSet), len(DefaultExcludes))
	}
	if w.config.DebounceMs != 500 {
		t.Errorf("DebounceMs = %d, want 500", w.config.DebounceMs)
	}
}

func TestNewWatcher_CustomConfig(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		Extensions: []string{".go", "py"}, // "py" without dot
		Excludes:   []string{"vendor"},
		DebounceMs: 100,
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	if !w.extSet[".go"] {
		t.Error(".go should be in extSet")
	}
	if !w.extSet[".py"] {
		t.Error(".py should be in extSet (auto-prefixed with dot)")
	}
	if !w.excludeSet["vendor"] {
		t.Error("vendor should be in excludeSet")
	}
	if w.config.DebounceMs != 100 {
		t.Errorf("DebounceMs = %d, want 100", w.config.DebounceMs)
	}
}

func TestNewWatcher_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	rel, _ := filepath.Rel("/", dir)
	// Use the temp dir directly since it's already absolute
	w, err := NewWatcher(Config{
		SpaceID: "test",
		Path:    dir,
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	if !filepath.IsAbs(w.config.Path) {
		t.Errorf("path should be absolute, got %q", w.config.Path)
	}
	_ = rel
}

func TestWatcher_ShouldWatch(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		Extensions: []string{".go", ".py"},
	})
	defer w.Stop()

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"script.py", true},
		{"readme.md", false},
		{"Makefile", false},
		{"", false},
	}

	for _, tt := range tests {
		got := w.shouldWatch(tt.path)
		if got != tt.want {
			t.Errorf("shouldWatch(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestWatcher_StartAndStop(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		DebounceMs: 50,
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not complete within 5 seconds")
	}
}

func TestWatcher_DetectsFileChange(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var changedFiles []string

	w, err := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		Extensions: []string{".go"},
		DebounceMs: 50,
		OnChange: func(_ context.Context, spaceID string, files []string) {
			mu.Lock()
			changedFiles = append(changedFiles, files...)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Give watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Create a .go file
	testFile := filepath.Join(dir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for debounce + processing
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(changedFiles) == 0 {
		t.Error("expected OnChange to be called with changed files")
	}
}

func TestWatcher_IgnoresExcludedExtension(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var changedFiles []string

	w, err := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		Extensions: []string{".go"},
		DebounceMs: 50,
		OnChange: func(_ context.Context, _ string, files []string) {
			mu.Lock()
			changedFiles = append(changedFiles, files...)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create a .txt file (not in extensions)
	testFile := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(changedFiles) > 0 {
		t.Errorf("OnChange should not be called for .txt files, got %v", changedFiles)
	}
}

func TestWatcher_ExcludesDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create excluded directory before starting watcher
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var mu sync.Mutex
	var changedFiles []string

	w, err := NewWatcher(Config{
		SpaceID:    "test",
		Path:       dir,
		Extensions: []string{".go"},
		Excludes:   []string{"vendor"},
		DebounceMs: 50,
		OnChange: func(_ context.Context, _ string, files []string) {
			mu.Lock()
			changedFiles = append(changedFiles, files...)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write in excluded directory
	testFile := filepath.Join(vendorDir, "dep.go")
	if err := os.WriteFile(testFile, []byte("package dep"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(changedFiles) > 0 {
		t.Errorf("OnChange should not fire for excluded dir, got %v", changedFiles)
	}
}

// --- fileDebouncer tests ---

func TestFileDebouncer_Deduplicates(t *testing.T) {
	var mu sync.Mutex
	var flushed []string

	d := newFileDebouncer(50*time.Millisecond, func(files []string) {
		mu.Lock()
		flushed = append(flushed, files...)
		mu.Unlock()
	})

	d.add("file1.go")
	d.add("file1.go") // duplicate
	d.add("file2.go")

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Errorf("expected 2 unique files, got %d: %v", len(flushed), flushed)
	}
}

func TestFileDebouncer_ResetsTimer(t *testing.T) {
	var mu sync.Mutex
	flushCount := 0

	d := newFileDebouncer(100*time.Millisecond, func(files []string) {
		mu.Lock()
		flushCount++
		mu.Unlock()
	})

	// Add files with gaps smaller than debounce window
	d.add("file1.go")
	time.Sleep(50 * time.Millisecond)
	d.add("file2.go")
	time.Sleep(50 * time.Millisecond)
	d.add("file3.go")

	// Wait for debounce to fire
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if flushCount != 1 {
		t.Errorf("expected 1 flush (debounced), got %d", flushCount)
	}
}

func TestFileDebouncer_FlushEmpty(t *testing.T) {
	d := newFileDebouncer(50*time.Millisecond, func(files []string) {
		t.Error("flush should not be called when pending is empty")
	})

	d.flush() // should be a no-op
}

// --- Manager tests ---

func TestManager_NewManager(t *testing.T) {
	m := NewManager()
	if m.watchers == nil {
		t.Error("watchers map should be initialized")
	}
}

func TestManager_AddAndRemoveWatcher(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()

	err := m.AddWatcher(Config{
		SpaceID:    "space1",
		Path:       dir,
		DebounceMs: 50,
	})
	if err != nil {
		t.Fatalf("AddWatcher: %v", err)
	}

	status := m.GetStatus()
	if _, ok := status["space1"]; !ok {
		t.Error("space1 should be in status")
	}

	m.RemoveWatcher("space1")
	status = m.GetStatus()
	if _, ok := status["space1"]; ok {
		t.Error("space1 should be removed")
	}
}

func TestManager_AddWatcher_ReplacesExisting(t *testing.T) {
	m := NewManager()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	_ = m.AddWatcher(Config{SpaceID: "space1", Path: dir1, DebounceMs: 50})
	_ = m.AddWatcher(Config{SpaceID: "space1", Path: dir2, DebounceMs: 50})

	status := m.GetStatus()
	if len(status) != 1 {
		t.Errorf("should have 1 watcher, got %d", len(status))
	}
	if status["space1"]["path"] != dir2 {
		t.Errorf("path should be updated to %q", dir2)
	}
}

func TestManager_RemoveWatcher_NonExistent(t *testing.T) {
	m := NewManager()
	m.RemoveWatcher("nonexistent") // should not panic
}

func TestManager_StopAll(t *testing.T) {
	m := NewManager()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	_ = m.AddWatcher(Config{SpaceID: "s1", Path: dir1, DebounceMs: 50})
	_ = m.AddWatcher(Config{SpaceID: "s2", Path: dir2, DebounceMs: 50})

	m.StopAll()

	status := m.GetStatus()
	if len(status) != 0 {
		t.Errorf("after StopAll, should have 0 watchers, got %d", len(status))
	}
}

func TestManager_GetStatus(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()

	_ = m.AddWatcher(Config{
		SpaceID:    "space1",
		Path:       dir,
		Extensions: []string{".go", ".py"},
		DebounceMs: 200,
	})
	defer m.StopAll()

	status := m.GetStatus()
	s := status["space1"]
	if s["debounce_ms"] != 200 {
		t.Errorf("debounce_ms = %v, want 200", s["debounce_ms"])
	}
}

// --- ParseConfigs tests ---

func TestParseConfigs_Empty(t *testing.T) {
	configs := ParseConfigs("")
	if configs != nil {
		t.Errorf("expected nil for empty string, got %v", configs)
	}
}

func TestParseConfigs_SingleEntry(t *testing.T) {
	configs := ParseConfigs("myspace:/tmp/src")
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].SpaceID != "myspace" {
		t.Errorf("SpaceID = %q, want %q", configs[0].SpaceID, "myspace")
	}
	if configs[0].Path != "/tmp/src" {
		t.Errorf("Path = %q, want %q", configs[0].Path, "/tmp/src")
	}
}

func TestParseConfigs_WithExtensions(t *testing.T) {
	configs := ParseConfigs("space:/path:.go|.py")
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if len(configs[0].Extensions) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(configs[0].Extensions))
	}
}

func TestParseConfigs_WithDebounce(t *testing.T) {
	configs := ParseConfigs("space:/path:.go:200")
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].DebounceMs != 200 {
		t.Errorf("DebounceMs = %d, want 200", configs[0].DebounceMs)
	}
}

func TestParseConfigs_MultipleEntries(t *testing.T) {
	configs := ParseConfigs("s1:/path1,s2:/path2")
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
}

func TestParseConfigs_SkipsInvalid(t *testing.T) {
	configs := ParseConfigs("invalid,,s1:/path1,")
	if len(configs) != 1 {
		t.Fatalf("expected 1 valid config, got %d", len(configs))
	}
}

func TestParseIntSafe(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"123", 123},
		{"  456  ", 456},
		{"0", 0},
		{"100ms", 100},
		{"", 0},
	}
	for _, tt := range tests {
		got, _ := parseIntSafe(tt.input)
		if got != tt.want {
			t.Errorf("parseIntSafe(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
