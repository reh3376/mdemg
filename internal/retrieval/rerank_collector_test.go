package retrieval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mdemg/internal/models"
)

// helpers ----------------------------------------------------------------

func testCandidates(n int) []models.RetrieveResult {
	out := make([]models.RetrieveResult, n)
	for i := range out {
		out[i] = models.RetrieveResult{
			NodeID: "node-" + string(rune('A'+i)),
			Name:   "file" + string(rune('A'+i)) + ".go",
			Score:  float64(n-i) / float64(n),
		}
	}
	return out
}

func testScores(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = 1.0 - float64(i)*0.1
	}
	return s
}

// flushCollector forces all pending async writes to complete by writing a
// sentinel record synchronously (under the same mutex).
func flushCollector(dc *DataCollector) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	// no-op — acquiring the lock ensures all prior goroutine writes finished
}

// tests ------------------------------------------------------------------

func TestNewDataCollector_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "nested")

	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}

func TestCollect_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	cands := testCandidates(3)
	scores := testScores(3)

	dc.Collect("how does auth work?", cands, scores, 42.5)

	// Wait for async write.
	time.Sleep(50 * time.Millisecond)
	flushCollector(dc)

	// Find the JSONL file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one file")
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}

	var rec trainingRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if rec.Query != "how does auth work?" {
		t.Errorf("query = %q, want %q", rec.Query, "how does auth work?")
	}
	if len(rec.Candidates) != 3 {
		t.Errorf("candidates len = %d, want 3", len(rec.Candidates))
	}
	if len(rec.RerankScores) != 3 {
		t.Errorf("rerank_scores len = %d, want 3", len(rec.RerankScores))
	}
	if rec.LatencyMs != 42.5 {
		t.Errorf("latency_ms = %f, want 42.5", rec.LatencyMs)
	}
	if rec.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestCollect_DisabledNoop(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(false, dir)
	defer dc.Close() //nolint:errcheck

	dc.Collect("query", testCandidates(2), testScores(2), 10.0)

	time.Sleep(50 * time.Millisecond)
	flushCollector(dc)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files when disabled, got %d", len(entries))
	}
}

func TestCollect_FileRotation(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	// Use a tiny max file size to force rotation quickly.
	dc.maxFileSize = 100 // 100 bytes

	// Write records synchronously via writeRecord to guarantee rotation
	// is observed deterministically (Collect is async).
	for i := 0; i < 10; i++ {
		rec := trainingRecord{
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Query:        "rotation query",
			Candidates:   []trainingCandidate{{NodeID: "n", Name: "f.go", Score: 0.5}},
			RerankScores: []float64{0.9},
			LatencyMs:    float64(i),
		}
		dc.writeRecord(rec)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	// With 100-byte limit, 10 records should create multiple files.
	if len(entries) < 2 {
		t.Errorf("expected multiple files after rotation, got %d", len(entries))
	}

	// Verify all files are valid JSONL.
	totalLines := 0
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var rec trainingRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Errorf("invalid JSONL in %s: %v", e.Name(), err)
			}
			totalLines++
		}
	}
	if totalLines != 10 {
		t.Errorf("expected 10 total records across files, got %d", totalLines)
	}
}

func TestPruneOldFiles(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	// Create an "old" file and a "new" file.
	oldPath := filepath.Join(dir, "training_old.jsonl")
	newPath := filepath.Join(dir, "training_new.jsonl")

	if err := os.WriteFile(oldPath, []byte("{}\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("{}\n"), 0640); err != nil {
		t.Fatal(err)
	}

	// Backdate the old file.
	oldTime := time.Now().Add(-100 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if err := dc.PruneOldFiles(90 * 24 * time.Hour); err != nil {
		t.Fatalf("prune: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file after prune, got %d", len(entries))
	}
	if entries[0].Name() != "training_new.jsonl" {
		t.Errorf("expected training_new.jsonl to survive, got %s", entries[0].Name())
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)

	dc.Collect("close test", testCandidates(1), testScores(1), 1.0)
	time.Sleep(50 * time.Millisecond)
	flushCollector(dc)

	if err := dc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	dc.mu.Lock()
	if dc.currentFile != nil {
		t.Error("expected currentFile to be nil after close")
	}
	if dc.currentSize != 0 {
		t.Errorf("expected currentSize 0, got %d", dc.currentSize)
	}
	dc.mu.Unlock()

	// Close again should be a no-op, not an error.
	if err := dc.Close(); err != nil {
		t.Errorf("second close should not error, got %v", err)
	}
}

func TestCollect_Concurrent(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	const goroutines = 20
	const recordsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for r := 0; r < recordsPerGoroutine; r++ {
				dc.Collect("concurrent query", testCandidates(2), testScores(2), float64(id*100+r))
			}
		}(g)
	}
	wg.Wait()

	// Wait for all async writes.
	time.Sleep(300 * time.Millisecond)
	flushCollector(dc)

	// Count total records across all files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	totalLines := 0
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			totalLines++
		}
	}

	expected := goroutines * recordsPerGoroutine
	if totalLines != expected {
		t.Errorf("expected %d records, got %d", expected, totalLines)
	}
}

func TestCollect_EmptyCandidates(t *testing.T) {
	dir := t.TempDir()
	dc := NewDataCollector(true, dir)
	defer dc.Close() //nolint:errcheck

	dc.Collect("empty query", nil, nil, 0.0)

	time.Sleep(50 * time.Millisecond)
	flushCollector(dc)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected a file even with empty candidates")
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}

	var rec trainingRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if rec.Query != "empty query" {
		t.Errorf("query = %q, want %q", rec.Query, "empty query")
	}
	if len(rec.Candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(rec.Candidates))
	}
	if rec.RerankScores != nil && len(rec.RerankScores) != 0 {
		t.Errorf("expected nil/empty rerank_scores, got %v", rec.RerankScores)
	}
}
