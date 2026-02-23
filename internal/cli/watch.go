// Package cli implements the file watcher command for the unified MDEMG CLI.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

var (
	defaultExtensions = ".go,.py,.ts,.tsx,.js,.jsx,.rs,.java,.md,.yaml,.yml,.json,.toml,.sql"
	defaultExcludes   = "node_modules,.git,vendor,__pycache__,.venv,dist,build,.next"
)

// watchConfig holds all configuration for the watch command.
type watchConfig struct {
	spaceID    string
	path       string
	endpoint   string
	extensions string
	exclude    string
	debounceMs int
}

// newWatchCmd creates the watch command that monitors a directory for changes
// and triggers MDEMG file ingestion via the API.
func newWatchCmd() *cobra.Command {
	cfg := &watchConfig{}

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Monitor a directory for changes and trigger MDEMG file ingestion",
		Long: `Watch a directory tree for file changes and automatically ingest modified files
into the MDEMG memory graph. Supports filtering by file extension and directory exclusion.

Example:
  mdemg watch --space-id myproject --path ./src --debounce 500`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}
			if cfg.spaceID == "" {
				return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
			}
			return runWatch(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "MDEMG space ID (or set MDEMG_SPACE_ID)")
	cmd.Flags().StringVar(&cfg.path, "path", ".", "Directory to watch")
	cmd.Flags().StringVar(&cfg.endpoint, "endpoint", "", "MDEMG API endpoint (auto-resolved if empty)")
	cmd.Flags().StringVar(&cfg.extensions, "extensions", defaultExtensions, "Comma-separated file extensions to watch")
	cmd.Flags().StringVar(&cfg.exclude, "exclude", defaultExcludes, "Comma-separated directories to exclude")
	cmd.Flags().IntVar(&cfg.debounceMs, "debounce", 500, "Debounce window in milliseconds")

	return cmd
}

// runWatch executes the watch command logic.
func runWatch(cfg *watchConfig) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(cfg.path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Resolve endpoint
	ep := cfg.endpoint
	if ep == "" {
		ep = config.ResolveEndpoint("http://localhost:9999")
	}

	// Parse extension filter
	extSet := parseCSV(cfg.extensions)
	excludeSet := parseCSV(cfg.exclude)

	log.Printf("mdemg-watch: space=%s path=%s endpoint=%s debounce=%dms",
		cfg.spaceID, absPath, ep, cfg.debounceMs)
	log.Printf("mdemg-watch: extensions=%v exclude=%v", extSet, excludeSet)

	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	// Create debouncer
	deb := newFileDebouncer(
		time.Duration(cfg.debounceMs)*time.Millisecond,
		func(files []string) {
			ingestFiles(ep, cfg.spaceID, files)
		},
	)

	// Watch directory tree
	if err := watchRecursive(watcher, absPath, excludeSet); err != nil {
		return fmt.Errorf("failed to set up watchers: %w", err)
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("mdemg-watch: watching for changes (Ctrl+C to stop)")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Handle new directories: add them to the watcher
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					dirName := filepath.Base(event.Name)
					if !excludeSet[dirName] {
						if addErr := watcher.Add(event.Name); addErr == nil {
							log.Printf("mdemg-watch: watching new dir: %s", event.Name)
						}
					}
					continue
				}
			}

			// Filter: only watch matching extensions
			if !shouldWatch(event.Name, extSet) {
				continue
			}

			// Only care about write/create/rename events
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				deb.add(event.Name)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("mdemg-watch: watcher error: %v", err)

		case <-sigCh:
			log.Printf("mdemg-watch: shutting down")
			deb.flush()
			return nil
		}
	}
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

// watchRecursive walks the directory tree and adds all non-excluded dirs to the watcher.
func watchRecursive(watcher *fsnotify.Watcher, root string, excludeSet map[string]bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if !info.IsDir() {
			return nil
		}

		dirName := info.Name()
		if excludeSet[dirName] && path != root {
			return filepath.SkipDir
		}

		if err := watcher.Add(path); err != nil {
			log.Printf("mdemg-watch: failed to watch %s: %v", path, err)
			return nil // continue watching other dirs
		}
		return nil
	})
}

// shouldWatch checks if a file's extension matches the filter set.
func shouldWatch(path string, extSet map[string]bool) bool {
	ext := filepath.Ext(path)
	if ext == "" {
		return false
	}
	return extSet[ext]
}

// ingestFiles sends changed files to the MDEMG API for ingestion with retry on transient errors.
func ingestFiles(endpoint, spaceID string, files []string) {
	log.Printf("mdemg-watch: ingesting %d file(s)", len(files))

	payload := map[string]any{
		"space_id": spaceID,
		"files":    files,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("mdemg-watch: marshal error: %v", err)
		return
	}

	url := strings.TrimRight(endpoint, "/") + "/v1/memory/ingest/files"

	const maxRetries = 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, reqErr := http.Post(url, "application/json", bytes.NewReader(body))
		if reqErr != nil {
			log.Printf("mdemg-watch: ingest request failed (attempt %d/%d): %v", attempt, maxRetries, reqErr)
			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("mdemg-watch: ingest successful (%d)", resp.StatusCode)
			return
		}

		// 4xx errors are not retryable
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			log.Printf("mdemg-watch: ingest rejected (%d): %s", resp.StatusCode, string(respBody))
			return
		}

		// 5xx errors are retryable
		log.Printf("mdemg-watch: ingest server error (%d, attempt %d/%d): %s",
			resp.StatusCode, attempt, maxRetries, string(respBody))
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	log.Printf("mdemg-watch: ingest failed after %d attempts", maxRetries)
}

// parseCSV splits a comma-separated string into a set of trimmed values.
func parseCSV(s string) map[string]bool {
	set := make(map[string]bool)
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			set[item] = true
		}
	}
	return set
}
