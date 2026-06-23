package ape

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSynergyReader_WithFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a CLAUDE.md with known line count
	claudePath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a memory directory with MEMORY.md and one auto-memory file
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}
	memoryPath := filepath.Join(memDir, "MEMORY.md")
	if err := os.WriteFile(memoryPath, []byte("mem1\nmem2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	autoFile := filepath.Join(memDir, "feedback_testing.md")
	if err := os.WriteFile(autoFile, []byte("auto1\nauto2\nauto3\nauto4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := &FileSynergyReader{
		claudeMDPath:        claudePath,
		memoryMDPath:        memoryPath,
		jiminyCheck:         func() bool { return true },
		memoryLineThreshold: 120,
		overlapSampleSize:   5,
	}

	sm := reader.ReadSynergyMetrics()

	if sm.ClaudeMDLines != 3 {
		t.Errorf("ClaudeMDLines: want 3, got %d", sm.ClaudeMDLines)
	}
	if sm.MemoryMDLines != 2 {
		t.Errorf("MemoryMDLines: want 2, got %d", sm.MemoryMDLines)
	}
	if sm.AutoMemoryFiles != 1 {
		t.Errorf("AutoMemoryFiles: want 1, got %d", sm.AutoMemoryFiles)
	}
	if sm.AutoMemoryLines != 4 {
		t.Errorf("AutoMemoryLines: want 4, got %d", sm.AutoMemoryLines)
	}
	if !sm.JiminyHealthy {
		t.Error("JiminyHealthy: want true")
	}
}

func TestFileSynergyReader_MissingFiles(t *testing.T) {
	reader := &FileSynergyReader{
		claudeMDPath:        "/nonexistent/CLAUDE.md",
		memoryMDPath:        "/nonexistent/memory/MEMORY.md",
		jiminyCheck:         func() bool { return false },
		memoryLineThreshold: 120,
		overlapSampleSize:   5,
	}

	sm := reader.ReadSynergyMetrics()

	if sm.ClaudeMDLines != 0 {
		t.Errorf("ClaudeMDLines: want 0, got %d", sm.ClaudeMDLines)
	}
	if sm.MemoryMDLines != 0 {
		t.Errorf("MemoryMDLines: want 0, got %d", sm.MemoryMDLines)
	}
	if sm.AutoMemoryFiles != 0 {
		t.Errorf("AutoMemoryFiles: want 0, got %d", sm.AutoMemoryFiles)
	}
	if sm.JiminyHealthy {
		t.Error("JiminyHealthy: want false")
	}
}

func TestFileSynergyReader_EmptyPaths(t *testing.T) {
	reader := &FileSynergyReader{
		claudeMDPath:        "",
		memoryMDPath:        "",
		memoryLineThreshold: 120,
		overlapSampleSize:   5,
	}

	sm := reader.ReadSynergyMetrics()

	if sm.ClaudeMDLines != 0 {
		t.Errorf("ClaudeMDLines: want 0, got %d", sm.ClaudeMDLines)
	}
	if sm.MemoryMDLines != 0 {
		t.Errorf("MemoryMDLines: want 0, got %d", sm.MemoryMDLines)
	}
}

func TestFileSynergyReader_NilJiminyCheck(t *testing.T) {
	reader := &FileSynergyReader{
		claudeMDPath:        "",
		memoryMDPath:        "",
		jiminyCheck:         nil,
		memoryLineThreshold: 120,
		overlapSampleSize:   5,
	}

	sm := reader.ReadSynergyMetrics()
	// JIMINY-SIGNAL-001: a nil jiminyCheck means "cannot determine" — it must
	// default to HEALTHY (true), not the zero-value false (= "down"), which fired
	// a false catastrophic-forgetting CRITICAL ~8×/day. Real outages are covered
	// by /healthz + the watchdog; only a real check returning false marks it down.
	if !sm.JiminyHealthy {
		t.Error("JiminyHealthy: want true (default healthy) with nil check — false would re-introduce the false CRITICAL")
	}
}

func TestFileSynergyReader_OverflowRate(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)

	claudePath := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudePath, []byte("line\n"), 0644)
	memoryPath := filepath.Join(memDir, "MEMORY.md")

	tests := []struct {
		name      string
		lines     int
		threshold int
		want      float64
	}{
		{"below threshold", 100, 120, 0},
		{"at threshold", 120, 120, 0},
		{"5 over", 125, 120, 5},
		{"10 over", 130, 120, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate file with N lines
			var b strings.Builder
			for i := range tt.lines {
				fmt.Fprintf(&b, "line%d\n", i%10)
			}
			os.WriteFile(memoryPath, []byte(b.String()), 0644)

			reader := &FileSynergyReader{
				claudeMDPath:        claudePath,
				memoryMDPath:        memoryPath,
				jiminyCheck:         func() bool { return true },
				memoryLineThreshold: tt.threshold,
				overlapSampleSize:   5,
			}
			sm := reader.ReadSynergyMetrics()
			if sm.OverflowRate != tt.want {
				t.Errorf("OverflowRate: want %f, got %f", tt.want, sm.OverflowRate)
			}
		})
	}
}

func TestFileSynergyReader_OverlapScore(t *testing.T) {
	dir := t.TempDir()

	t.Run("no overlap", func(t *testing.T) {
		claudePath := filepath.Join(dir, "claude_no.md")
		memPath := filepath.Join(dir, "mem_no.md")
		os.WriteFile(claudePath, []byte("totally unique claude content here\n"), 0644)
		os.WriteFile(memPath, []byte("completely different memory text here\n"), 0644)

		reader := &FileSynergyReader{
			claudeMDPath:        claudePath,
			memoryMDPath:        memPath,
			jiminyCheck:         func() bool { return true },
			memoryLineThreshold: 120,
			overlapSampleSize:   5,
		}
		sm := reader.ReadSynergyMetrics()
		if sm.OverlapScore != 0.0 {
			t.Errorf("expected 0.0, got %f", sm.OverlapScore)
		}
	})

	t.Run("full overlap", func(t *testing.T) {
		shared := "this is shared content that appears in both files\n"
		claudePath := filepath.Join(dir, "claude_full.md")
		memPath := filepath.Join(dir, "mem_full.md")
		os.WriteFile(claudePath, []byte(shared), 0644)
		os.WriteFile(memPath, []byte(shared), 0644)

		reader := &FileSynergyReader{
			claudeMDPath:        claudePath,
			memoryMDPath:        memPath,
			jiminyCheck:         func() bool { return true },
			memoryLineThreshold: 120,
			overlapSampleSize:   5,
		}
		sm := reader.ReadSynergyMetrics()
		if sm.OverlapScore != 1.0 {
			t.Errorf("expected 1.0, got %f", sm.OverlapScore)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		claudePath := filepath.Join(dir, "claude_partial.md")
		memPath := filepath.Join(dir, "mem_partial.md")
		os.WriteFile(claudePath, []byte("line one matches in both files perfectly\nline two is only in claude file here\n"), 0644)
		os.WriteFile(memPath, []byte("line one matches in both files perfectly\nline three is only in memory file here\n"), 0644)

		reader := &FileSynergyReader{
			claudeMDPath:        claudePath,
			memoryMDPath:        memPath,
			jiminyCheck:         func() bool { return true },
			memoryLineThreshold: 120,
			overlapSampleSize:   5,
		}
		sm := reader.ReadSynergyMetrics()
		if sm.OverlapScore <= 0.0 || sm.OverlapScore >= 1.0 {
			t.Errorf("expected partial overlap (0 < x < 1), got %f", sm.OverlapScore)
		}
	})

	t.Run("missing files", func(t *testing.T) {
		reader := &FileSynergyReader{
			claudeMDPath:        "/nonexistent/a.md",
			memoryMDPath:        "/nonexistent/b.md",
			jiminyCheck:         func() bool { return true },
			memoryLineThreshold: 120,
			overlapSampleSize:   5,
		}
		sm := reader.ReadSynergyMetrics()
		if sm.OverlapScore != 0.0 {
			t.Errorf("expected 0.0 for missing files, got %f", sm.OverlapScore)
		}
	})
}

func TestCountFileLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", 0},
		{"one line no trailing newline", "hello", 0},
		{"one line with newline", "hello\n", 1},
		{"three lines", "a\nb\nc\n", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			if got := countFileLines(path); got != tt.want {
				t.Errorf("countFileLines() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountAutoMemoryFiles(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "MEMORY.md")

	// Create MEMORY.md (should be excluded)
	os.WriteFile(memPath, []byte("index\n"), 0644)
	// Create 2 auto-memory files
	os.WriteFile(filepath.Join(dir, "user_role.md"), []byte("a\nb\n"), 0644)
	os.WriteFile(filepath.Join(dir, "feedback_x.md"), []byte("c\n"), 0644)
	// Create a non-md file (should be excluded)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip\n"), 0644)

	files, lines := countAutoMemoryFiles(memPath)
	if files != 2 {
		t.Errorf("files: want 2, got %d", files)
	}
	if lines != 3 {
		t.Errorf("lines: want 3, got %d", lines)
	}
}
