package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditFileStatusCurrent(t *testing.T) {
	meta := map[string]interface{}{
		"line_count":       100,
		"last_ingested_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"content_hash":     "sha256:abc123",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	client := srv.Client()
	status := auditFileStatus(client, srv.URL, "test-space", "CLAUDE.md", 100, 24*time.Hour)
	if status != "current" {
		t.Errorf("expected 'current', got %q", status)
	}
}

func TestAuditFileStatusStale(t *testing.T) {
	meta := map[string]interface{}{
		"line_count":       100,
		"last_ingested_at": time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
		"content_hash":     "sha256:abc123",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	client := srv.Client()
	status := auditFileStatus(client, srv.URL, "test-space", "CLAUDE.md", 100, 24*time.Hour)
	if status != "STALE (ingested 2d ago)" {
		t.Errorf("expected 'STALE (ingested 2d ago)', got %q", status)
	}
}

func TestAuditFileStatusShrank(t *testing.T) {
	meta := map[string]interface{}{
		"line_count":       200,
		"last_ingested_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"content_hash":     "sha256:abc123",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	client := srv.Client()
	status := auditFileStatus(client, srv.URL, "test-space", "CLAUDE.md", 50, 24*time.Hour)
	if status != "SHRANK (200 → 50 lines)" {
		t.Errorf("expected shrinkage status, got %q", status)
	}
}

func TestAuditFileStatusNotIngested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := srv.Client()
	status := auditFileStatus(client, srv.URL, "test-space", "CLAUDE.md", 100, 24*time.Hour)
	if status != "NOT INGESTED" {
		t.Errorf("expected 'NOT INGESTED', got %q", status)
	}
}

func TestAuditFileStatusServerDown(t *testing.T) {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	status := auditFileStatus(client, "http://127.0.0.1:1", "test-space", "CLAUDE.md", 100, 24*time.Hour)
	if status != "error (unreachable)" {
		t.Errorf("expected 'error (unreachable)', got %q", status)
	}
}

func TestCountFileLines(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	count := countFileLines(f)
	if count != 3 {
		t.Errorf("expected 3 lines, got %d", count)
	}
}

func TestCountBufferEntries(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "buffer.jsonl")
	content := `{"path":"a.md","content":"test"}
{"path":"b.md","content":"test2"}

{"path":"c.md","content":"test3"}
`
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	count := countBufferEntries(f)
	if count != 3 {
		t.Errorf("expected 3 entries (skipping blank lines), got %d", count)
	}
}

func TestCountBufferEntriesMissing(t *testing.T) {
	count := countBufferEntries("/nonexistent/buffer.jsonl")
	if count != 0 {
		t.Errorf("expected 0 for missing file, got %d", count)
	}
}

func TestFormatAgeDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{2 * time.Hour, "2h"},
		{23 * time.Hour, "23h"},
		{25 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{72 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		got := formatAgeDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatAgeDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
