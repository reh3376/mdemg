package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileBackend writes alerts to a JSON file on disk.
type FileBackend struct {
	mu        sync.Mutex
	filePath  string
	maxAlerts int
}

// NewFileBackend creates a file backend that writes to the given path.
func NewFileBackend(filePath string, maxAlerts int) *FileBackend {
	if maxAlerts <= 0 {
		maxAlerts = 50
	}
	return &FileBackend{
		filePath:  filePath,
		maxAlerts: maxAlerts,
	}
}

func (b *FileBackend) Send(_ context.Context, a Alert) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure directory exists.
	dir := filepath.Dir(b.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("alert: mkdir %s: %w", dir, err)
	}

	// Read existing file.
	af := b.readFile()

	// Prepend new alert (newest first).
	af.Alerts = append([]Alert{a}, af.Alerts...)

	// FIFO eviction if over cap.
	if len(af.Alerts) > b.maxAlerts {
		af.Alerts = af.Alerts[:b.maxAlerts]
	}

	af.UpdatedAt = time.Now().UTC()

	return b.writeFile(af)
}

// ReadAlerts returns the current alert file contents. Safe for concurrent use.
func (b *FileBackend) ReadAlerts() AlertFile {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readFile()
}

func (b *FileBackend) readFile() AlertFile {
	data, err := os.ReadFile(b.filePath)
	if err != nil {
		return AlertFile{}
	}
	var af AlertFile
	if err := json.Unmarshal(data, &af); err != nil {
		return AlertFile{}
	}
	return af
}

func (b *FileBackend) writeFile(af AlertFile) error {
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("alert: marshal: %w", err)
	}

	// Atomic write: write to tmp, then rename.
	tmpPath := b.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("alert: write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, b.filePath); err != nil {
		return fmt.Errorf("alert: rename: %w", err)
	}
	return nil
}
