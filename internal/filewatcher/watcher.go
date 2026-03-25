// Package filewatcher provides a reusable file watching mechanism that monitors
// directories for changes and triggers callbacks with debouncing.
//
// This package extracts the core file watching logic from cmd/watch for use
// as an in-process file watcher integrated with the MDEMG API server.
package filewatcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultExtensions are the file extensions watched by default.
var DefaultExtensions = []string{
	".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java",
	".md", ".yaml", ".yml", ".json", ".toml", ".sql",
}

// DefaultExcludes are directories excluded from watching by default.
var DefaultExcludes = []string{
	"node_modules", ".git", "vendor", "__pycache__", ".venv",
	"dist", "build", ".next", ".turbo", "coverage",
}

// Config holds configuration for a file watcher.
type Config struct {
	SpaceID      string        // Target space for ingested files
	Path         string        // Directory to watch
	Extensions   []string      // File extensions to watch (empty = DefaultExtensions)
	Excludes     []string      // Directories to exclude (empty = DefaultExcludes)
	DebounceMs   int           // Debounce window in milliseconds (default: 500)
	OnChange     ChangeHandler // Callback when files change
}

// ChangeHandler is called when files change, with the list of changed file paths.
type ChangeHandler func(ctx context.Context, spaceID string, files []string)

// Watcher watches a directory tree for file changes.
type Watcher struct {
	config    Config
	watcher   *fsnotify.Watcher
	debouncer *fileDebouncer
	extSet    map[string]bool
	excludeSet map[string]bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewWatcher creates a new file watcher with the given configuration.
func NewWatcher(cfg Config) (*Watcher, error) {
	// Apply defaults
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = DefaultExtensions
	}
	if len(cfg.Excludes) == 0 {
		cfg.Excludes = DefaultExcludes
	}
	if cfg.DebounceMs <= 0 {
		cfg.DebounceMs = 500
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, err
	}
	cfg.Path = absPath

	// Create fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Build extension set
	extSet := make(map[string]bool)
	for _, ext := range cfg.Extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[ext] = true
	}

	// Build exclude set
	excludeSet := make(map[string]bool)
	for _, dir := range cfg.Excludes {
		excludeSet[dir] = true
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		config:     cfg,
		watcher:    fsWatcher,
		extSet:     extSet,
		excludeSet: excludeSet,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create debouncer
	w.debouncer = newFileDebouncer(
		time.Duration(cfg.DebounceMs)*time.Millisecond,
		func(files []string) {
			if cfg.OnChange != nil {
				cfg.OnChange(ctx, cfg.SpaceID, files)
			}
		},
	)

	return w, nil
}

// Start begins watching the configured directory.
func (w *Watcher) Start() error {
	// Watch directory tree
	if err := w.watchRecursive(w.config.Path); err != nil {
		return err
	}

	// Start event loop
	w.wg.Add(1)
	go w.eventLoop()

	slog.Info("filewatcher: watching directory", "path", w.config.Path, "space", w.config.SpaceID, "debounce_ms", w.config.DebounceMs)

	return nil
}

// Stop stops the watcher and releases resources.
func (w *Watcher) Stop() {
	w.cancel()
	_ = w.watcher.Close()
	w.debouncer.flush()
	w.wg.Wait()
	slog.Info("filewatcher: stopped watching", "path", w.config.Path)
}

// eventLoop processes file system events.
func (w *Watcher) eventLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Handle new directories: add them to the watcher
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					dirName := filepath.Base(event.Name)
					if !w.excludeSet[dirName] {
						if addErr := w.watcher.Add(event.Name); addErr == nil {
							slog.Info("filewatcher: watching new dir", "path", event.Name)
						}
					}
					continue
				}
			}

			// Filter: only watch matching extensions
			if !w.shouldWatch(event.Name) {
				continue
			}

			// Only care about write/create/rename events
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				w.debouncer.add(event.Name)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("filewatcher: error", "error", err)
		}
	}
}

// watchRecursive walks the directory tree and adds all non-excluded dirs to the watcher.
func (w *Watcher) watchRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if !info.IsDir() {
			return nil
		}

		dirName := info.Name()
		if w.excludeSet[dirName] && path != root {
			return filepath.SkipDir
		}

		if err := w.watcher.Add(path); err != nil {
			slog.Warn("filewatcher: failed to watch directory", "path", path, "error", err)
			return nil // continue watching other dirs
		}
		return nil
	})
}

// shouldWatch checks if a file's extension matches the filter set.
func (w *Watcher) shouldWatch(path string) bool {
	ext := filepath.Ext(path)
	if ext == "" {
		return false
	}
	return w.extSet[ext]
}

// fileDebouncer accumulates file paths and flushes them after a quiet period.
type fileDebouncer struct {
	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	window  time.Duration
	flushFn func(files []string)
}

func newFileDebouncer(window time.Duration, flushFn func(files []string)) *fileDebouncer {
	return &fileDebouncer{
		pending: make(map[string]struct{}),
		window:  window,
		flushFn: flushFn,
	}
}

func (d *fileDebouncer) add(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pending[path] = struct{}{}

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.window, func() {
		d.flush()
	})
}

func (d *fileDebouncer) flush() {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		return
	}

	files := make([]string, 0, len(d.pending))
	for f := range d.pending {
		files = append(files, f)
	}
	d.pending = make(map[string]struct{})
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()

	d.flushFn(files)
}

// Manager manages multiple file watchers.
type Manager struct {
	mu       sync.RWMutex
	watchers map[string]*Watcher // spaceID -> watcher
}

// NewManager creates a new file watcher manager.
func NewManager() *Manager {
	return &Manager{
		watchers: make(map[string]*Watcher),
	}
}

// AddWatcher adds and starts a watcher for the given configuration.
func (m *Manager) AddWatcher(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing watcher for this space if present
	if existing, ok := m.watchers[cfg.SpaceID]; ok {
		existing.Stop()
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		return err
	}

	if err := watcher.Start(); err != nil {
		return err
	}

	m.watchers[cfg.SpaceID] = watcher
	return nil
}

// RemoveWatcher stops and removes the watcher for the given space.
func (m *Manager) RemoveWatcher(spaceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if watcher, ok := m.watchers[spaceID]; ok {
		watcher.Stop()
		delete(m.watchers, spaceID)
	}
}

// StopAll stops all watchers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for spaceID, watcher := range m.watchers {
		watcher.Stop()
		delete(m.watchers, spaceID)
	}
}

// GetStatus returns the status of all watchers.
func (m *Manager) GetStatus() map[string]map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]map[string]any)
	for spaceID, watcher := range m.watchers {
		status[spaceID] = map[string]any{
			"path":        watcher.config.Path,
			"debounce_ms": watcher.config.DebounceMs,
			"extensions":  watcher.config.Extensions,
		}
	}
	return status
}

// ParseConfigs parses file watcher configurations from a config string.
// Format: "space_id:/path:extensions:debounce_ms,..."
// Extensions can be comma-separated within quotes or use | separator.
func ParseConfigs(configStr string) []Config {
	if configStr == "" {
		return nil
	}

	var configs []Config
	for _, item := range strings.Split(configStr, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		parts := strings.SplitN(item, ":", 4)
		if len(parts) < 2 {
			continue
		}

		cfg := Config{
			SpaceID: parts[0],
			Path:    parts[1],
		}

		if len(parts) >= 3 && parts[2] != "" {
			// Parse extensions (| separated within this part)
			exts := strings.Split(parts[2], "|")
			for _, ext := range exts {
				ext = strings.TrimSpace(ext)
				if ext != "" {
					if !strings.HasPrefix(ext, ".") {
						ext = "." + ext
					}
					cfg.Extensions = append(cfg.Extensions, ext)
				}
			}
		}

		if len(parts) >= 4 {
			if ms, err := parseIntSafe(parts[3]); err == nil {
				cfg.DebounceMs = ms
			}
		}

		configs = append(configs, cfg)
	}

	return configs
}

func parseIntSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n, nil
}
