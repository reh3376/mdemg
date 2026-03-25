package retrieval

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mdemg/internal/models"
)

const (
	// defaultMaxFileSize is 50MB — triggers rotation when exceeded.
	defaultMaxFileSize int64 = 50 * 1024 * 1024

	// defaultPruneMaxAge is 90 days — files older than this are pruned.
	defaultPruneMaxAge = 90 * 24 * time.Hour
)

// trainingCandidate is the per-candidate payload written to JSONL.
type trainingCandidate struct {
	NodeID string  `json:"node_id"`
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
}

// trainingRecord is a single JSONL line capturing one rerank event.
type trainingRecord struct {
	Timestamp    string              `json:"timestamp"`
	Query        string              `json:"query"`
	Candidates   []trainingCandidate `json:"candidates"`
	RerankScores []float64           `json:"rerank_scores"`
	LatencyMs    float64             `json:"latency_ms"`
}

// DataCollector captures rerank training data to JSONL files on disk.
// It is safe for concurrent use.
type DataCollector struct {
	dir         string
	enabled     bool
	mu          sync.Mutex
	currentFile *os.File
	currentSize int64
	maxFileSize int64
	fileSeq     int // monotonic counter to disambiguate filenames within the same second
}

// NewDataCollector creates a new training data collector.
// If enabled is false, all Collect calls are no-ops.
// The dir is created (with parents) if it does not exist.
func NewDataCollector(enabled bool, dir string) *DataCollector {
	dc := &DataCollector{
		dir:         dir,
		enabled:     enabled,
		maxFileSize: defaultMaxFileSize,
	}
	if !enabled {
		return dc
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Error("DataCollector: failed to create dir", "dir", dir, "error", err)
	}
	return dc
}

// Collect asynchronously writes a training record to the current JSONL file.
// It is a no-op when the collector is disabled.
func (dc *DataCollector) Collect(query string, candidates []models.RetrieveResult, scores []float64, latencyMs float64) {
	if !dc.enabled {
		return
	}

	// Build record outside the goroutine to capture current state.
	rec := trainingRecord{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Query:        query,
		Candidates:   make([]trainingCandidate, len(candidates)),
		RerankScores: scores,
		LatencyMs:    latencyMs,
	}
	for i, c := range candidates {
		rec.Candidates[i] = trainingCandidate{
			NodeID: c.NodeID,
			Name:   c.Name,
			Score:  c.Score,
		}
	}

	go dc.writeRecord(rec)
}

// writeRecord serialises a single record to the current file, rotating if needed.
func (dc *DataCollector) writeRecord(rec trainingRecord) {
	data, err := json.Marshal(rec)
	if err != nil {
		slog.Error("DataCollector: marshal error", "error", err)
		return
	}
	data = append(data, '\n')

	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Rotate if current file exceeds max size.
	if dc.currentFile != nil && dc.currentSize >= dc.maxFileSize {
		dc.rotateFileLocked()
	}

	// Open a new file if none is open.
	if dc.currentFile == nil {
		if err := dc.openNewFileLocked(); err != nil {
			slog.Error("DataCollector: open file error", "error", err)
			return
		}
	}

	n, err := dc.currentFile.Write(data)
	if err != nil {
		slog.Error("DataCollector: write error", "error", err)
		return
	}
	dc.currentSize += int64(n)
}

// rotateFileLocked closes the current file. A new file will be opened on the next write.
// Caller must hold dc.mu.
func (dc *DataCollector) rotateFileLocked() {
	if dc.currentFile != nil {
		_ = dc.currentFile.Close()
		dc.currentFile = nil
		dc.currentSize = 0
	}
}

// openNewFileLocked opens a new timestamped JSONL file.
// Caller must hold dc.mu.
func (dc *DataCollector) openNewFileLocked() error {
	dc.fileSeq++
	name := fmt.Sprintf("training_%s_%04d.jsonl", time.Now().UTC().Format("20060102_150405"), dc.fileSeq)
	path := filepath.Join(dc.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) //nolint:gosec // training data is non-sensitive
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat %s: %w", path, err)
	}
	dc.currentFile = f
	dc.currentSize = info.Size()
	return nil
}

// PruneOldFiles removes training JSONL files older than maxAge.
// Pass 0 to use the default of 90 days.
func (dc *DataCollector) PruneOldFiles(maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = defaultPruneMaxAge
	}
	cutoff := time.Now().Add(-maxAge)

	entries, err := os.ReadDir(dc.dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dc.dir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dc.dir, e.Name())
			if err := os.Remove(path); err != nil {
				slog.Error("DataCollector: prune failed", "path", path, "error", err)
			}
		}
	}
	return nil
}

// Close flushes and closes the current file.
func (dc *DataCollector) Close() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.currentFile != nil {
		err := dc.currentFile.Close()
		dc.currentFile = nil
		dc.currentSize = 0
		return err
	}
	return nil
}
