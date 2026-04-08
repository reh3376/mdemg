package ape

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// FileSynergyReader reads Claude Code integration files from disk to
// supply synergy metrics for RSIC health assessment.
type FileSynergyReader struct {
	claudeMDPath        string
	memoryMDPath        string
	jiminyCheck         func() bool
	memoryLineThreshold int // from cfg.SynergyMemoryLineThreshold
	overlapSampleSize   int // from cfg.SynergyOverlapSampleSize
}

// NewFileSynergyReader creates a reader for Claude Code synergy files.
// Paths may be empty — auto-detection via detectClaudeMD/detectMemoryMD
// is attempted for any empty path.
func NewFileSynergyReader(claudePath, memoryPath string, jiminyCheck func() bool,
	memoryLineThreshold, overlapSampleSize int) *FileSynergyReader {
	if claudePath == "" {
		claudePath = detectClaudeMD()
	}
	if memoryPath == "" {
		memoryPath = detectMemoryMD()
	}
	return &FileSynergyReader{
		claudeMDPath:        claudePath,
		memoryMDPath:        memoryPath,
		jiminyCheck:         jiminyCheck,
		memoryLineThreshold: memoryLineThreshold,
		overlapSampleSize:   overlapSampleSize,
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
	// G1: Compute overflow rate as raw excess lines above threshold
	if r.memoryLineThreshold > 0 && sm.MemoryMDLines > r.memoryLineThreshold {
		sm.OverflowRate = float64(sm.MemoryMDLines - r.memoryLineThreshold)
	}
	// G2: Compute overlap score between MEMORY.md and CLAUDE.md
	sm.OverlapScore = r.computeOverlap()
	return sm
}

// computeOverlap samples content lines from MEMORY.md and checks what fraction
// appear verbatim in CLAUDE.md. Returns 0 if either file is unreadable.
func (r *FileSynergyReader) computeOverlap() float64 {
	if r.claudeMDPath == "" || r.memoryMDPath == "" {
		return 0
	}
	claudeData, err := os.ReadFile(r.claudeMDPath)
	if err != nil {
		return 0
	}
	memData, err := os.ReadFile(r.memoryMDPath)
	if err != nil {
		return 0
	}

	claudeContent := string(claudeData)
	lines := strings.Split(string(memData), "\n")

	// Extract meaningful content lines (skip empty, headers, separators, short lines)
	var candidates []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if len(trimmed) >= 20 {
			candidates = append(candidates, trimmed)
		}
	}
	if len(candidates) == 0 {
		return 0
	}

	// Evenly distribute sample across candidates
	sampleSize := r.overlapSampleSize
	if sampleSize <= 0 {
		sampleSize = 5
	}
	var samples []string
	if len(candidates) <= sampleSize {
		samples = candidates
	} else {
		step := float64(len(candidates)) / float64(sampleSize)
		for i := range sampleSize {
			idx := int(float64(i) * step)
			samples = append(samples, candidates[idx])
		}
	}

	matches := 0
	for _, s := range samples {
		if strings.Contains(claudeContent, s) {
			matches++
		}
	}
	return float64(matches) / float64(len(samples))
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
	slog.Warn("synergy: MEMORY.md not found at expected path", "path", candidate)
	return ""
}
