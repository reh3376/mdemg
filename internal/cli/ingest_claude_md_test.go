package cli

import (
	"crypto/sha256"
	"fmt"
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
