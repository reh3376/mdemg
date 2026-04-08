package ape

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// FileSynergyReader reads Claude Code integration files from disk to
// supply synergy metrics for RSIC health assessment.
type FileSynergyReader struct {
	claudeMDPath string
	memoryMDPath string
	jiminyCheck  func() bool
}

// NewFileSynergyReader creates a reader for Claude Code synergy files.
// Paths may be empty — auto-detection via detectClaudeMD/detectMemoryMD
// is attempted for any empty path.
func NewFileSynergyReader(claudePath, memoryPath string, jiminyCheck func() bool) *FileSynergyReader {
	if claudePath == "" {
		claudePath = detectClaudeMD()
	}
	if memoryPath == "" {
		memoryPath = detectMemoryMD()
	}
	return &FileSynergyReader{
		claudeMDPath: claudePath,
		memoryMDPath: memoryPath,
		jiminyCheck:  jiminyCheck,
	}
}

// ReadSynergyMetrics reads line counts and health status from synergy files.
func (r *FileSynergyReader) ReadSynergyMetrics() SynergyMetrics {
	sm := SynergyMetrics{
		ClaudeMDLines: countFileLines(r.claudeMDPath),
		MemoryMDLines: countFileLines(r.memoryMDPath),
	}
	if r.memoryMDPath != "" {
		sm.AutoMemoryFiles, sm.AutoMemoryLines = countAutoMemoryFiles(r.memoryMDPath)
	}
	if r.jiminyCheck != nil {
		sm.JiminyHealthy = r.jiminyCheck()
	}
	return sm
}

// countFileLines counts newlines in a file. Returns 0 if the file is missing or unreadable.
func countFileLines(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return bytes.Count(data, []byte("\n"))
}

// countAutoMemoryFiles counts .md files (excluding MEMORY.md itself) and their
// total line count in the same directory as memoryMDPath.
func countAutoMemoryFiles(memoryMDPath string) (files int, lines int) {
	dir := filepath.Dir(memoryMDPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "MEMORY.md" {
			continue
		}
		files++
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			lines += bytes.Count(data, []byte("\n"))
		}
	}
	return files, lines
}

// detectClaudeMD locates CLAUDE.md using the same heuristic as the CLI:
// 1. CWD, 2. parent directories up to 3 levels.
func detectClaudeMD() string {
	if abs, err := filepath.Abs("CLAUDE.md"); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	// Walk up to 3 parent directories
	dir, _ := os.Getwd()
	for range 3 {
		dir = filepath.Dir(dir)
		candidate := filepath.Join(dir, "CLAUDE.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// detectMemoryMD locates MEMORY.md in the Claude Code project memory directory.
func detectMemoryMD() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Build expected project dir: /Users/x/foo → -Users-x-foo
	projectDir := strings.ReplaceAll(cwd, "/", "-")
	candidate := filepath.Join(home, ".claude", "projects", projectDir, "memory", "MEMORY.md")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// Fallback: glob for longest path match
	pattern := filepath.Join(home, ".claude", "projects", "*", "memory", "MEMORY.md")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return ""
	}
	best := matches[0]
	for _, m := range matches[1:] {
		if len(m) > len(best) {
			best = m
		}
	}
	return best
}
