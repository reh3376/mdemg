package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// handleSynergyStatus handles GET /v1/synergy/status
// Returns synergy health metrics: file line counts, overflow events, migration status, and health score.
func (s *Server) handleSynergyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")

	// 1. Check Jiminy health
	jiminyHealthy := s.jiminySvc != nil && s.cfg.JiminyEnabled

	// 2. Resolve and count CLAUDE.md lines
	claudeMDPath := resolveSynergyPath(s.cfg.SynergyClaudeMDPath, "CLAUDE.md")
	claudeMDLines := countFileLines(claudeMDPath)

	// 3. Resolve and count MEMORY.md lines
	memoryMDPath := resolveMemoryMDPath(s.cfg.SynergyMemoryMDPath)
	memoryMDLines := countFileLines(memoryMDPath)

	// 4. Count auto-memory files (*.md in same directory as MEMORY.md, excluding MEMORY.md itself)
	autoMemoryFiles := 0
	autoMemoryLines := 0
	if memoryMDPath != "" {
		memoryDir := filepath.Dir(memoryMDPath)
		memoryBase := filepath.Base(memoryMDPath)
		matches, _ := filepath.Glob(filepath.Join(memoryDir, "*.md"))
		for _, m := range matches {
			if filepath.Base(m) == memoryBase {
				continue
			}
			autoMemoryFiles++
			autoMemoryLines += countFileLines(m)
		}
	}

	// 5. Query CMS for overflow events in last 24h
	overflowEvents24h := countOverflowEvents(r.Context(), s.driver, spaceID)

	// 6. Read migration status
	migrationStatus, migrationDate := readMigrationStatus()

	// 7. Calculate synergy health score (mirrors scoreSynergy in self_assess.go)
	synergyHealth := computeSynergyHealth(jiminyHealthy, claudeMDLines, memoryMDLines,
		overflowEvents24h, s.cfg.SynergyTargetClaudeLines, s.cfg.SynergyTargetMemoryLines)

	// 8. Count recovery buffer entries
	bufferSpaceEntries := countRecoveryBufferSpaceEntries(r.Context(), s.driver, s.cfg.SynergyRecoveryBufferSpace)
	bufferLocalEntries := countRecoveryBufferLocalEntries(s.cfg.SynergyRecoveryBufferPath)

	// 9. Build response
	data := map[string]any{
		"jiminy_healthy":                jiminyHealthy,
		"claude_md_lines":               claudeMDLines,
		"memory_md_lines":               memoryMDLines,
		"auto_memory_files":             autoMemoryFiles,
		"auto_memory_lines":             autoMemoryLines,
		"overflow_events_24h":           overflowEvents24h,
		"synergy_health":                synergyHealth,
		"recovery_buffer_space_entries": bufferSpaceEntries,
		"recovery_buffer_local_entries": bufferLocalEntries,
		"recovery_buffer_total":         bufferSpaceEntries + bufferLocalEntries,
	}
	if migrationStatus != "" {
		data["migration_status"] = migrationStatus
		data["migration_date"] = migrationDate
	} else {
		data["migration_status"] = ""
		data["migration_date"] = ""
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// resolveMemoryMDPath resolves the MEMORY.md path using CWD-based project directory matching.
func resolveMemoryMDPath(configured string) string {
	if configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Derive Claude Code project dir from CWD (e.g., /Users/reh3376/mdemg → -Users-reh3376-mdemg)
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		projectDir := strings.ReplaceAll(cwd, "/", "-")
		candidate := filepath.Join(home, ".claude", "projects", projectDir, "memory", "MEMORY.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: glob, prefer longest path
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "memory", "MEMORY.md"))
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

// resolveSynergyPath returns the configured path if non-empty, otherwise auto-detects via glob.
func resolveSynergyPath(configured string, autoDetectPattern string) string {
	if configured != "" {
		return configured
	}
	if autoDetectPattern == "" {
		return ""
	}
	matches, _ := filepath.Glob(autoDetectPattern)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// countFileLines reads a file and returns the number of newline characters.
// Returns 0 if the path is empty or the file cannot be read.
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

// countOverflowEvents queries Neo4j for auto-overflow observations created in the last 24 hours.
// Returns 0 on any error.
func countOverflowEvents(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) int {
	if driver == nil || spaceID == "" {
		return 0
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND n.created_at > datetime() - duration('PT24H')
			  AND ANY(t IN n.tags WHERE t = 'auto-overflow')
			RETURN count(n) AS overflow_count
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("overflow_count"); ok && v != nil {
				if count, ok := v.(int64); ok {
					return int(count), nil
				}
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0
	}
	if count, ok := result.(int); ok {
		return count
	}
	return 0
}

// migrationInfo represents the content of .mdemg/synergy-migrated.json.
type migrationInfo struct {
	Migrated   bool   `json:"migrated"`
	Version    string `json:"version"`
	MigratedAt string `json:"migrated_at"`
}

// readMigrationStatus reads the migration status from .mdemg/synergy-migrated.json.
// Returns empty strings if the file does not exist or cannot be parsed.
func readMigrationStatus() (status string, date string) {
	data, err := os.ReadFile(filepath.Join(".mdemg", "synergy-migrated.json"))
	if err != nil {
		return "", ""
	}
	var info migrationInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return "", ""
	}
	if !info.Migrated {
		return "", ""
	}
	return info.Version, info.MigratedAt
}

// countRecoveryBufferSpaceEntries queries Neo4j for observations in the recovery buffer space.
func countRecoveryBufferSpaceEntries(ctx context.Context, driver neo4j.DriverWithContext, bufferSpace string) int {
	if driver == nil || bufferSpace == "" {
		return 0
	}
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND ANY(t IN n.tags WHERE t = 'recovery-buffer')
			RETURN count(n) AS cnt
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": bufferSpace})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("cnt"); ok && v != nil {
				if count, ok := v.(int64); ok {
					return int(count), nil
				}
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0
	}
	if count, ok := result.(int); ok {
		return count
	}
	return 0
}

// countRecoveryBufferLocalEntries counts lines in the local JSONL recovery buffer file.
func countRecoveryBufferLocalEntries(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// computeSynergyHealth calculates the synergy health score.
// Mirrors the scoreSynergy logic in internal/ape/self_assess.go.
func computeSynergyHealth(jiminyHealthy bool, claudeLines, memoryLines, overflowEvents, targetClaude, targetMemory int) float64 {
	if !jiminyHealthy {
		return 0.0
	}
	score := 1.0

	// Penalise bloated CLAUDE.md
	if claudeLines > targetClaude+50 {
		score -= 0.3
	} else if claudeLines > targetClaude {
		score -= 0.1
	}

	// Penalise bloated MEMORY.md
	if memoryLines > targetMemory+60 {
		score -= 0.3
	} else if memoryLines > targetMemory {
		score -= 0.1
	}

	// Penalise high overflow rate
	if overflowEvents > 10 {
		score -= 0.3
	} else if overflowEvents > 5 {
		score -= 0.1
	}

	if score < 0 {
		score = 0
	}
	return score
}
