package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

// claudeMDEntry represents a tracked .md file for CMS ingestion.
type claudeMDEntry struct {
	Path string   // File path (relative to repo root, or absolute with ~ expansion)
	Tags []string // CMS tags for this file
}

// Static paths relative to repo root — always checked, skipped if absent.
var claudeMDStaticPaths = []claudeMDEntry{
	{Path: "CLAUDE.md", Tags: []string{"claude-md", "config", "project"}},
	{Path: "AGENT_HANDOFF.md", Tags: []string{"claude-md", "handoff"}},
	{Path: "VISION.md", Tags: []string{"claude-md", "vision"}},
	{Path: ".claude/CLAUDE.md", Tags: []string{"claude-md", "personal"}},
	{Path: "CLAUDE.local.md", Tags: []string{"claude-md", "local"}},
	{Path: "AGENTS.md", Tags: []string{"claude-md", "cross-tool"}},
}

// Home-relative static paths.
var claudeMDHomePaths = []claudeMDEntry{
	{Path: "~/.claude/CLAUDE.md", Tags: []string{"claude-md", "global"}},
}

// Glob patterns for dynamic file discovery.
func claudeMDGlobs(projectHash string) []struct {
	Pattern string
	Tags    []string
} {
	homeDir, _ := os.UserHomeDir()
	return []struct {
		Pattern string
		Tags    []string
	}{
		{
			Pattern: filepath.Join(homeDir, ".claude", "projects", projectHash, "memory", "*.md"),
			Tags:    []string{"claude-md", "auto-memory"},
		},
		{
			Pattern: filepath.Join(homeDir, ".claude", "plans", "*.md"),
			Tags:    []string{"claude-md", "plan"},
		},
		{
			Pattern: filepath.Join(".claude", "rules", "**", "*.md"),
			Tags:    []string{"claude-md", "rules"},
		},
		{
			Pattern: filepath.Join(homeDir, ".claude", "projects", projectHash, "*", "session-memory", "summary.md"),
			Tags:    []string{"claude-md", "session-memory"},
		},
	}
}

// projectPathHash derives the Claude Code project hash from a directory path.
// e.g., "/Users/reh3376/mdemg" → "-Users-reh3376-mdemg"
func projectPathHash(dir string) string {
	cleaned := filepath.Clean(dir)
	return strings.ReplaceAll(cleaned, string(filepath.Separator), "-")
}

// expandHome replaces ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

type nodeMetaResponse struct {
	NodeID      string  `json:"node_id"`
	ContentHash string  `json:"content_hash"`
	FileSize    int64   `json:"file_size"`
	LineCount   int     `json:"line_count"`
	Status      string  `json:"status"`
	Error       string  `json:"error,omitempty"`
}

type ingestClaudeMDConfig struct {
	spaceID  string
	endpoint string
	force    bool
	quiet    bool
	dryRun   bool
	file     string // single-file mode
}

func newIngestClaudeMDCmd() *cobra.Command {
	cfg := &ingestClaudeMDConfig{}

	cmd := &cobra.Command{
		Use:   "ingest-claude-md",
		Short: "Ingest Claude .md files into CMS with content-hash change detection",
		Long: `Discovers and ingests all Claude Code .md files (CLAUDE.md, MEMORY.md,
plan files, memory topic files, rules, etc.) into CMS. Uses SHA256 content
hashing to skip unchanged files, avoiding observation bloat.

Called automatically by session-start and pre-compact hooks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if resolved := resolveSpaceID(cmd); resolved != "" {
				cfg.spaceID = resolved
			}
			if cfg.spaceID == "" {
				cfg.spaceID = "mdemg-dev"
			}
			return runIngestClaudeMD(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "MDEMG space ID (default: resolve from env/config)")
	cmd.Flags().StringVar(&cfg.endpoint, "endpoint", "", "MDEMG endpoint (default: from LISTEN_ADDR/.env)")
	cmd.Flags().BoolVar(&cfg.force, "force", false, "Force re-ingest even if content hash matches")
	cmd.Flags().BoolVar(&cfg.quiet, "quiet", false, "Suppress output except errors")
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "Show what would be ingested without doing it")
	cmd.Flags().StringVar(&cfg.file, "file", "", "Ingest a single file instead of the full registry")

	return cmd
}

// ingestBufferMaxEntries is the maximum number of buffered entries (FIFO eviction).
var ingestBufferMaxEntries = func() int {
	if v := os.Getenv("INGEST_BUFFER_MAX_ENTRIES"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 100
}()

// resolveIngestBufferPath returns the path to the local JSONL ingest buffer.
func resolveIngestBufferPath() string {
	if v := os.Getenv("INGEST_BUFFER_PATH"); v != "" {
		return v
	}
	return filepath.Join(".mdemg", "ingest-buffer.jsonl")
}

// ingestBufferEntry represents a single buffered ingest payload.
type ingestBufferEntry struct {
	Path        string   `json:"path"`
	Content     string   `json:"content"`
	ContentHash string   `json:"content_hash"`
	Tags        []string `json:"tags"`
	BufferedAt  string   `json:"buffered_at"`
	SpaceID     string   `json:"space_id"`
	FileSize    int64    `json:"file_size"`
	LineCount   int      `json:"line_count"`
}

// bufferToLocalJSONL writes pending ingests to .mdemg/ingest-buffer.jsonl when server is down.
func bufferToLocalJSONL(cfg *ingestClaudeMDConfig) error {
	bufPath := resolveIngestBufferPath()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	projHash := projectPathHash(cwd)

	var files []claudeMDEntry
	if cfg.file != "" {
		absPath := cfg.file
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, absPath)
		}
		files = append(files, claudeMDEntry{Path: absPath, Tags: []string{"claude-md", "targeted"}})
	} else {
		files = discoverClaudeMDFiles(cwd, projHash)
	}

	if len(files) == 0 {
		return nil
	}

	// Read existing buffer for FIFO enforcement
	var existingLines []string
	if data, err := os.ReadFile(bufPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				existingLines = append(existingLines, line)
			}
		}
	}

	buffered := 0
	for _, entry := range files {
		absPath := entry.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, absPath)
		}
		absPath = expandHome(absPath)

		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
		lineCount := strings.Count(string(content), "\n")
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lineCount++
		}

		cmsPath := absPath
		if rel, err := filepath.Rel(cwd, absPath); err == nil && !strings.HasPrefix(rel, "..") {
			cmsPath = rel
		}

		bufEntry := ingestBufferEntry{
			Path:        cmsPath,
			Content:     string(content),
			ContentHash: hash,
			Tags:        entry.Tags,
			BufferedAt:  time.Now().UTC().Format(time.RFC3339),
			SpaceID:     cfg.spaceID,
			FileSize:    info.Size(),
			LineCount:   lineCount,
		}

		line, err := json.Marshal(bufEntry)
		if err != nil {
			continue
		}
		existingLines = append(existingLines, string(line))
		buffered++
	}

	// FIFO eviction: keep only newest entries
	if len(existingLines) > ingestBufferMaxEntries {
		existingLines = existingLines[len(existingLines)-ingestBufferMaxEntries:]
	}

	// Write buffer file
	if err := os.MkdirAll(filepath.Dir(bufPath), 0755); err != nil {
		return fmt.Errorf("create buffer directory: %w", err)
	}

	var buf bytes.Buffer
	for _, line := range existingLines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	if err := os.WriteFile(bufPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write buffer file: %w", err)
	}

	if !cfg.quiet {
		fmt.Printf("Buffered %d files to %s\n", buffered, bufPath)
	}
	return nil
}

// flushLocalBuffer reads .mdemg/ingest-buffer.jsonl and ingests each entry.
// Returns count of successfully flushed entries. Partially-flushed buffers are preserved.
func flushLocalBuffer(client *http.Client, cfg *ingestClaudeMDConfig) int {
	bufPath := resolveIngestBufferPath()

	data, err := os.ReadFile(bufPath)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	var remaining []string
	flushed := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry ingestBufferEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			remaining = append(remaining, line) // Preserve malformed lines
			continue
		}

		// Attempt to ingest
		err := ingestClaudeMDFile(client, cfg.endpoint, entry.SpaceID, entry.Path,
			entry.Content, entry.ContentHash, entry.FileSize, entry.LineCount, entry.Tags)
		if err != nil {
			remaining = append(remaining, line) // Keep failed entries
			continue
		}
		flushed++
	}

	// Rewrite buffer with only remaining (unflushed) entries
	if flushed > 0 {
		var buf bytes.Buffer
		for _, line := range remaining {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
		_ = os.WriteFile(bufPath, buf.Bytes(), 0644)
	}

	return flushed
}

func runIngestClaudeMD(cfg *ingestClaudeMDConfig) error {
	if cfg.endpoint == "" {
		cfg.endpoint = config.ResolveEndpoint("http://localhost:9999")
	}

	// Check server health
	client := &http.Client{Timeout: 10 * time.Second}
	healthResp, err := client.Get(cfg.endpoint + "/healthz")
	if err != nil || healthResp.StatusCode != http.StatusOK {
		// Server down — buffer locally instead of failing
		if !cfg.quiet {
			fmt.Printf("⚠ Server unreachable at %s — buffering to %s\n",
				cfg.endpoint, resolveIngestBufferPath())
		}
		return bufferToLocalJSONL(cfg)
	}
	healthResp.Body.Close()

	// Server up — flush any buffered ingests first
	flushed := flushLocalBuffer(client, cfg)
	if flushed > 0 && !cfg.quiet {
		fmt.Printf("Flushed %d buffered ingests\n", flushed)
	}

	// Resolve project hash for glob expansion
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	projHash := projectPathHash(cwd)

	// Build file list
	var files []claudeMDEntry

	if cfg.file != "" {
		// Single-file mode
		absPath := cfg.file
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, absPath)
		}
		files = append(files, claudeMDEntry{
			Path: absPath,
			Tags: []string{"claude-md", "targeted"},
		})
	} else {
		files = discoverClaudeMDFiles(cwd, projHash)
	}

	if len(files) == 0 {
		if !cfg.quiet {
			fmt.Println("No .md files found to ingest")
		}
		return nil
	}

	// Process each file
	var ingested, skipped, errors, missing int
	for _, entry := range files {
		absPath := entry.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, absPath)
		}
		absPath = expandHome(absPath)

		// Check if file exists
		info, err := os.Stat(absPath)
		if err != nil {
			missing++
			slog.Debug("file not found, skipping", "path", absPath)
			continue
		}

		// Read content
		content, err := os.ReadFile(absPath)
		if err != nil {
			errors++
			slog.Warn("failed to read file", "path", absPath, "error", err)
			continue
		}

		// Compute metadata
		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
		fileSize := info.Size()
		lineCount := strings.Count(string(content), "\n")
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lineCount++ // Count last line without trailing newline
		}

		// Determine CMS path (use relative path for in-repo files, absolute for external)
		cmsPath := absPath
		if rel, err := filepath.Rel(cwd, absPath); err == nil && !strings.HasPrefix(rel, "..") {
			cmsPath = rel
		}

		// Check if content changed (unless --force)
		if !cfg.force {
			existingHash, err := getNodeContentHash(client, cfg.endpoint, cfg.spaceID, cmsPath)
			if err == nil && existingHash == hash {
				skipped++
				slog.Debug("unchanged, skipping", "path", cmsPath, "hash", hash[:20])
				continue
			}
		}

		if cfg.dryRun {
			fmt.Printf("[dry-run] Would ingest: %s (%d bytes, %d lines, hash=%s)\n",
				cmsPath, fileSize, lineCount, hash[:20])
			ingested++
			continue
		}

		// Ingest via observe endpoint
		err = ingestClaudeMDFile(client, cfg.endpoint, cfg.spaceID, cmsPath, string(content),
			hash, fileSize, lineCount, entry.Tags)
		if err != nil {
			errors++
			slog.Warn("failed to ingest", "path", cmsPath, "error", err)
			continue
		}

		ingested++
		slog.Debug("ingested", "path", cmsPath, "hash", hash[:20])
	}

	if !cfg.quiet {
		total := ingested + skipped + errors + missing
		fmt.Printf("Ingested %d/%d files (%d unchanged, %d missing, %d errors)\n",
			ingested, total, skipped, missing, errors)
	}

	// Always emit a summary line (even in quiet mode) for log audit trail.
	// This ensures LaunchAgent, hooks, and manual runs are all verifiable.
	fmt.Fprintf(os.Stderr, "[%s] ingest-claude-md: %d ingested, %d skipped, %d errors, %d missing (space: %s)\n",
		time.Now().UTC().Format(time.RFC3339), ingested, skipped, errors, missing, cfg.spaceID)

	return nil
}

// discoverClaudeMDFiles builds the complete list of .md files to track.
func discoverClaudeMDFiles(cwd, projHash string) []claudeMDEntry {
	var files []claudeMDEntry

	// Static in-repo paths (resolve relative to cwd for existence check)
	for _, entry := range claudeMDStaticPaths {
		absPath := filepath.Join(cwd, entry.Path)
		if _, err := os.Stat(absPath); err == nil {
			files = append(files, entry)
		}
	}

	// Home-relative static paths (only include if file exists)
	for _, entry := range claudeMDHomePaths {
		expanded := expandHome(entry.Path)
		if _, err := os.Stat(expanded); err == nil {
			files = append(files, claudeMDEntry{Path: expanded, Tags: entry.Tags})
		}
	}

	// Dynamic globs
	for _, g := range claudeMDGlobs(projHash) {
		matches, err := filepath.Glob(g.Pattern)
		if err != nil {
			slog.Debug("glob error", "pattern", g.Pattern, "error", err)
			continue
		}
		for _, match := range matches {
			files = append(files, claudeMDEntry{Path: match, Tags: g.Tags})
		}
	}

	return files
}

// getNodeContentHash fetches the content_hash for a node from the meta endpoint.
// Returns empty string if node doesn't exist or endpoint fails.
func getNodeContentHash(client *http.Client, endpoint, spaceID, path string) (string, error) {
	u := fmt.Sprintf("%s/v1/memory/node/meta?space_id=%s&path=%s",
		endpoint, url.QueryEscape(spaceID), url.QueryEscape(path))

	resp, err := client.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("node not found")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("meta endpoint returned %d", resp.StatusCode)
	}

	var meta nodeMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}

	return meta.ContentHash, nil
}

// ingestClaudeMDFile ingests a single .md file via the ingest endpoint.
// Uses POST /v1/memory/ingest which supports path-based merge and content_hash.
func ingestClaudeMDFile(client *http.Client, endpoint, spaceID, path, content,
	contentHash string, fileSize int64, lineCount int, tags []string) error {

	// Build a summary from the first 200 chars
	summary := content
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}

	payload := map[string]any{
		"space_id":     spaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"source":       "ingest-claude-md",
		"content":      content,
		"path":         path,
		"name":         filepath.Base(path),
		"summary":      summary,
		"tags":         tags,
		"content_hash": contentHash,
		"file_size":    fileSize,
		"line_count":   lineCount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := client.Post(endpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ingest returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
