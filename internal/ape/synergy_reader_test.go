package ape

import (
	"os"
	"path/filepath"
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
		claudeMDPath: claudePath,
		memoryMDPath: memoryPath,
		jiminyCheck:  func() bool { return true },
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
		claudeMDPath: "/nonexistent/CLAUDE.md",
		memoryMDPath: "/nonexistent/memory/MEMORY.md",
		jiminyCheck:  func() bool { return false },
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
		claudeMDPath: "",
		memoryMDPath: "",
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
		claudeMDPath: "",
		memoryMDPath: "",
		jiminyCheck:  nil,
	}

	sm := reader.ReadSynergyMetrics()
	if sm.JiminyHealthy {
		t.Error("JiminyHealthy: want false with nil check")
	}
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
