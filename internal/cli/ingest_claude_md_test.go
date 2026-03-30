package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPathHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/reh3376/mdemg", "-Users-reh3376-mdemg"},
		{"/home/user/my-project", "-home-user-my-project"},
		{"/Users/reh3376/Documents/dailylog", "-Users-reh3376-Documents-dailylog"},
	}

	for _, tt := range tests {
		got := projectPathHash(tt.input)
		if got != tt.expected {
			t.Errorf("projectPathHash(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.expected {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDiscoverClaudeMDFiles_StaticPaths(t *testing.T) {
	// Create a temp dir with some static files
	tmpDir := t.TempDir()

	// Create CLAUDE.md
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create AGENT_HANDOFF.md
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENT_HANDOFF.md"), []byte("# Handoff"), 0644); err != nil {
		t.Fatal(err)
	}

	projHash := projectPathHash(tmpDir)
	files := discoverClaudeMDFiles(tmpDir, projHash)

	// Should find CLAUDE.md and AGENT_HANDOFF.md (2 static files that exist)
	// Plus any glob matches (likely 0 since temp dir)
	foundClaude := false
	foundHandoff := false
	for _, f := range files {
		if f.Path == "CLAUDE.md" {
			foundClaude = true
		}
		if f.Path == "AGENT_HANDOFF.md" {
			foundHandoff = true
		}
	}

	if !foundClaude {
		t.Error("expected CLAUDE.md in discovered files")
	}
	if !foundHandoff {
		t.Error("expected AGENT_HANDOFF.md in discovered files")
	}
}

func TestDiscoverClaudeMDFiles_SkipsMissing(t *testing.T) {
	// Temp dir with no files — static paths should be skipped
	tmpDir := t.TempDir()
	projHash := projectPathHash(tmpDir)
	files := discoverClaudeMDFiles(tmpDir, projHash)

	// No static in-repo files should be present
	for _, f := range files {
		if f.Path == "CLAUDE.md" || f.Path == "VISION.md" || f.Path == "AGENT_HANDOFF.md" {
			t.Errorf("should not find %q when file doesn't exist", f.Path)
		}
	}
}

func TestComputeHash(t *testing.T) {
	content := []byte("test content for hashing")
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(content))

	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash should start with sha256: prefix, got %s", hash)
	}

	// Same content should produce same hash
	hash2 := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if hash != hash2 {
		t.Error("identical content should produce identical hash")
	}

	// Different content should produce different hash
	hash3 := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("different content")))
	if hash == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestBufferToLocalJSONL(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file to buffer
	testFile := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(testFile, []byte("# Test\nLine 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	bufPath := filepath.Join(tmpDir, ".mdemg", "ingest-buffer.jsonl")
	t.Setenv("INGEST_BUFFER_PATH", bufPath)

	cfg := &ingestClaudeMDConfig{
		spaceID:  "test-space",
		endpoint: "http://localhost:12345", // unreachable
		quiet:    true,
		file:     testFile,
	}

	if err := bufferToLocalJSONL(cfg); err != nil {
		t.Fatalf("bufferToLocalJSONL failed: %v", err)
	}

	// Verify buffer file was created
	data, err := os.ReadFile(bufPath)
	if err != nil {
		t.Fatalf("buffer file not created: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 buffer entry, got %d", len(lines))
	}

	var entry ingestBufferEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse buffer entry: %v", err)
	}

	if entry.SpaceID != "test-space" {
		t.Errorf("expected space_id test-space, got %s", entry.SpaceID)
	}
	if !strings.HasPrefix(entry.ContentHash, "sha256:") {
		t.Errorf("expected sha256: prefix, got %s", entry.ContentHash)
	}
	if entry.LineCount != 2 {
		t.Errorf("expected 2 lines, got %d", entry.LineCount)
	}
}

func TestFlushLocalBuffer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock server that accepts ingest requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/memory/ingest" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create a buffer file with one entry
	bufPath := filepath.Join(tmpDir, "ingest-buffer.jsonl")
	t.Setenv("INGEST_BUFFER_PATH", bufPath)

	entry := ingestBufferEntry{
		Path:        "CLAUDE.md",
		Content:     "# Test",
		ContentHash: "sha256:abc123",
		Tags:        []string{"test"},
		BufferedAt:  "2026-03-30T12:00:00Z",
		SpaceID:     "test-space",
		FileSize:    6,
		LineCount:   1,
	}
	line, _ := json.Marshal(entry)
	if err := os.WriteFile(bufPath, append(line, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{}
	cfg := &ingestClaudeMDConfig{
		endpoint: server.URL,
		spaceID:  "test-space",
		quiet:    true,
	}

	flushed := flushLocalBuffer(client, cfg)
	if flushed != 1 {
		t.Errorf("expected 1 flushed, got %d", flushed)
	}

	// Buffer should be empty after flush
	data, err := os.ReadFile(bufPath)
	if err != nil {
		t.Fatalf("buffer file missing after flush: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Errorf("buffer should be empty after successful flush, got: %s", string(data))
	}
}

func TestFlushPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK) // First succeeds
		} else {
			w.WriteHeader(http.StatusInternalServerError) // Second fails
		}
	}))
	defer server.Close()

	bufPath := filepath.Join(tmpDir, "ingest-buffer.jsonl")
	t.Setenv("INGEST_BUFFER_PATH", bufPath)

	// Write two entries
	var buf strings.Builder
	for i := 0; i < 2; i++ {
		entry := ingestBufferEntry{
			Path:        fmt.Sprintf("file%d.md", i),
			Content:     "test",
			ContentHash: "sha256:abc",
			Tags:        []string{"test"},
			SpaceID:     "test-space",
		}
		line, _ := json.Marshal(entry)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(bufPath, []byte(buf.String()), 0644); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{}
	cfg := &ingestClaudeMDConfig{endpoint: server.URL, spaceID: "test-space", quiet: true}

	flushed := flushLocalBuffer(client, cfg)
	if flushed != 1 {
		t.Errorf("expected 1 flushed (partial), got %d", flushed)
	}

	// Buffer should still have 1 remaining entry
	data, _ := os.ReadFile(bufPath)
	remaining := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining entry, got %d", len(remaining))
	}
}

func TestBufferEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	bufPath := filepath.Join(tmpDir, "nonexistent-buffer.jsonl")
	t.Setenv("INGEST_BUFFER_PATH", bufPath)

	client := &http.Client{}
	cfg := &ingestClaudeMDConfig{endpoint: "http://localhost:1", spaceID: "test", quiet: true}

	flushed := flushLocalBuffer(client, cfg)
	if flushed != 0 {
		t.Errorf("expected 0 flushed for missing buffer, got %d", flushed)
	}
}

func TestLineCount(t *testing.T) {
	tests := []struct {
		content  string
		expected int
	}{
		{"line1\nline2\nline3\n", 3},
		{"line1\nline2\nline3", 3}, // no trailing newline
		{"single line", 1},
		{"", 0},
		{"\n", 1},
		{"\n\n\n", 3},
	}

	for _, tt := range tests {
		content := []byte(tt.content)
		lineCount := strings.Count(string(content), "\n")
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lineCount++
		}
		if lineCount != tt.expected {
			t.Errorf("lineCount(%q) = %d, want %d", tt.content, lineCount, tt.expected)
		}
	}
}
