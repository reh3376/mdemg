package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// synergyMigrationFlag is the JSON structure persisted in .mdemg/synergy-migrated.json.
type synergyMigrationFlag struct {
	Migrated         bool     `json:"migrated"`
	Version          string   `json:"version"`
	MigratedAt       string   `json:"migrated_at"`
	SpaceID          string   `json:"space_id"`
	FilesMigrated    []string `json:"files_migrated"`
	SectionsMigrated []string `json:"sections_migrated"`
}

func newSynergyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "synergy",
		Short: "Claude Code ↔ MDEMG synergy optimization",
		Long: `Manage the synergy between Claude Code's persistent state (.md files)
and MDEMG's CMS (Neo4j graph). Reduces token overhead by migrating
archival content to CMS and monitoring for re-bloat.`,
	}

	cmd.AddCommand(newSynergyStatusCmd())
	cmd.AddCommand(newSynergyMigrateCmd())
	cmd.AddCommand(newSynergyCheckCmd())

	return cmd
}

// ─── synergy status ───

func newSynergyStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display synergy health metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			return runSynergyStatus(spaceID, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().String("space-id", "", "Space ID (default: mdemg-dev)")

	return cmd
}

func runSynergyStatus(spaceID string, jsonOutput bool) error {
	endpoint := resolveEndpoint()

	// Call the synergy status API
	url := fmt.Sprintf("%s/v1/synergy/status?space_id=%s", endpoint, spaceID)
	resp, err := http.Get(url) //nolint:gosec // G107: localhost API call
	if err != nil {
		// Server not running — compute locally
		return runSynergyStatusLocal(spaceID, jsonOutput)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return runSynergyStatusLocal(spaceID, jsonOutput)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Data)
	}

	return printSynergyStatus(result.Data)
}

func runSynergyStatusLocal(spaceID string, jsonOutput bool) error {
	claudeMDPath := findClaudeMD()
	memoryMDPath := findMemoryMD()

	jiminyHealthy := checkJiminyHealth()
	claudeLines := countLines(claudeMDPath)
	memoryLines := countLines(memoryMDPath)
	autoFiles, autoLines := countAutoMemory(memoryMDPath)
	migStatus, migDate := readMigrationFlag()

	data := map[string]any{
		"jiminy_healthy":      jiminyHealthy,
		"claude_md_lines":     claudeLines,
		"memory_md_lines":     memoryLines,
		"auto_memory_files":   autoFiles,
		"auto_memory_lines":   autoLines,
		"overflow_events_24h": 0, // requires server
		"synergy_health":      computeLocalSynergyHealth(jiminyHealthy, claudeLines, memoryLines),
		"migration_status":    migStatus,
		"migration_date":      migDate,
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	return printSynergyStatus(data)
}

func printSynergyStatus(data map[string]any) error {
	fmt.Println("═══ SYNERGY STATUS ═══")

	jiminy := "UNKNOWN"
	if v, ok := data["jiminy_healthy"].(bool); ok {
		if v {
			jiminy = "HEALTHY"
		} else {
			jiminy = "UNHEALTHY"
		}
	}
	fmt.Printf("Jiminy:        %s\n", jiminy)

	claudeLines := toInt(data["claude_md_lines"])
	memoryLines := toInt(data["memory_md_lines"])
	autoFiles := toInt(data["auto_memory_files"])
	autoLines := toInt(data["auto_memory_lines"])

	fmt.Printf("CLAUDE.md:     %d lines (target: ≤150)\n", claudeLines)
	fmt.Printf("MEMORY.md:     %d lines (target: ≤120, cap: 200)\n", memoryLines)
	fmt.Printf("Auto-memory:   %d files, %d lines\n", autoFiles, autoLines)

	overflow := toInt(data["overflow_events_24h"])
	fmt.Printf("CMS overflow:  %d events in last 24h\n", overflow)

	health := toFloat(data["synergy_health"])
	fmt.Printf("Synergy:       %.2f\n", health)

	migStatus, _ := data["migration_status"].(string)
	migDate, _ := data["migration_date"].(string)
	if migStatus != "" {
		fmt.Printf("Migration:     %s (%s)\n", migStatus, migDate)
	} else {
		fmt.Println("Migration:     not migrated")
	}

	fmt.Println("═══ END ═══")
	return nil
}

// ─── synergy migrate ───

func newSynergyMigrateCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run synergy migration (ingest archival content to CMS)",
		Long: `Migrates archival content from Claude Code auto-memory files and
MEMORY.md sections into MDEMG CMS. Requires Jiminy to be healthy.
Content is moved to ~/mdemg/temp/ for dev safety.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			return runSynergyMigrate(spaceID, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without making changes")
	cmd.Flags().String("space-id", "", "Space ID (default: mdemg-dev)")

	return cmd
}

func runSynergyMigrate(spaceID string, dryRun bool) error {
	// Check migration flag
	flag := readMigrationFlagFull()
	if flag.Migrated {
		fmt.Printf("Already migrated (version: %s, date: %s)\n", flag.Version, flag.MigratedAt)
		fmt.Println("Use scripts/synergy-migrate.sh --force to re-run.")
		return nil
	}

	// Jiminy health gate
	if !checkJiminyHealth() {
		return fmt.Errorf("Jiminy health check failed — aborting to prevent catastrophic forgetting.\nStart Jiminy: mdemg start --auto-migrate")
	}

	endpoint := resolveEndpoint()

	// Files to migrate to CMS
	memoryDir := filepath.Dir(findMemoryMD())
	filesToMigrate := map[string]string{
		"cognitive-architecture.md":     "learning",
		"feedback_cuidv2_standard.md":   "constraint",
		"feedback_fix_all_failures.md":  "constraint",
		"feedback_live_testing_mode.md": "preference",
		"feedback_dev_plan_standard_tasks.md": "constraint",
		"project_mdemg_windows_docs.md":       "task",
		"project_menubar_multi_instance.md":   "task",
		"project_next_tasks_queue.md":         "task",
	}

	// Obsolete files to move without ingestion
	obsoleteFiles := []string{
		"vscode-extension.md",
		"feedback_plan_before_code.md",
	}

	if dryRun {
		fmt.Println("=== DRY RUN ===")
		fmt.Println("\nFiles to ingest then move to temp/migrated/:")
		for f, obsType := range filesToMigrate {
			path := filepath.Join(memoryDir, f)
			lines := countLines(path)
			fmt.Printf("  %s (%d lines) → obs_type: %s\n", f, lines, obsType)
		}
		fmt.Println("\nObsolete files to move to temp/obsolete/:")
		for _, f := range obsoleteFiles {
			fmt.Printf("  %s\n", f)
		}
		return nil
	}

	// Ensure temp directories exist
	tempDirs := []string{
		filepath.Join(os.Getenv("HOME"), "mdemg", "temp", "obsolete"),
		filepath.Join(os.Getenv("HOME"), "mdemg", "temp", "migrated"),
	}
	for _, d := range tempDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create temp dir %s: %w", d, err)
		}
	}

	migrated := []string{}
	// 1. Ingest and move files
	for f, obsType := range filesToMigrate {
		path := filepath.Join(memoryDir, f)
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  SKIP %s (not found)\n", f)
			continue
		}

		// Ingest to CMS
		if err := observeToCMS(endpoint, spaceID, string(content), obsType, []string{"synergy-migration", "v1"}); err != nil {
			fmt.Printf("  WARN %s: CMS ingest failed: %v\n", f, err)
			continue
		}

		// Move to temp
		dest := filepath.Join(os.Getenv("HOME"), "mdemg", "temp", "migrated", f)
		if err := os.Rename(path, dest); err != nil {
			fmt.Printf("  WARN %s: move failed: %v\n", f, err)
			continue
		}

		fmt.Printf("  OK %s → CMS (%s) + temp/migrated/\n", f, obsType)
		migrated = append(migrated, f)
	}

	// 2. Move obsolete files
	for _, f := range obsoleteFiles {
		path := filepath.Join(memoryDir, f)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		dest := filepath.Join(os.Getenv("HOME"), "mdemg", "temp", "obsolete", f)
		if err := os.Rename(path, dest); err != nil {
			fmt.Printf("  WARN %s: move failed: %v\n", f, err)
			continue
		}
		fmt.Printf("  OK %s → temp/obsolete/\n", f)
		migrated = append(migrated, f)
	}

	// 3. Write migration flag
	flagData := synergyMigrationFlag{
		Migrated:      true,
		Version:       "v1",
		MigratedAt:    time.Now().Format(time.RFC3339),
		SpaceID:       spaceID,
		FilesMigrated: migrated,
	}
	writeMigrationFlag(flagData)

	fmt.Printf("\nMigration complete: %d files processed.\n", len(migrated))
	return nil
}

// ─── synergy check ───

func newSynergyCheckCmd() *cobra.Command {
	var autoMode bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Health check for synergy (cron-compatible, exits non-zero if unhealthy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			return runSynergyCheck(spaceID, autoMode)
		},
	}

	cmd.Flags().BoolVar(&autoMode, "auto", false, "Automated mode (exit 1 if unhealthy, suppress output)")
	cmd.Flags().String("space-id", "", "Space ID (default: mdemg-dev)")

	return cmd
}

func runSynergyCheck(spaceID string, autoMode bool) error {
	jiminyHealthy := checkJiminyHealth()
	claudeLines := countLines(findClaudeMD())
	memoryLines := countLines(findMemoryMD())
	health := computeLocalSynergyHealth(jiminyHealthy, claudeLines, memoryLines)

	flag := readMigrationFlagFull()

	unhealthy := false

	// Critical: Jiminy down + migrated
	if !jiminyHealthy && flag.Migrated {
		unhealthy = true
		if !autoMode {
			fmt.Println("CRITICAL: Jiminy unhealthy + migration applied — catastrophic forgetting risk!")
		}
		// Alert CMS if server is up
		endpoint := resolveEndpoint()
		_ = observeToCMS(endpoint, spaceID,
			"SYNERGY ALERT: Jiminy unhealthy post-migration — catastrophic forgetting risk",
			"error", []string{"synergy-alert", "jiminy-unhealthy"})
	}

	// Warning: file bloat
	if claudeLines > 200 || memoryLines > 180 {
		unhealthy = true
		if !autoMode {
			fmt.Printf("WARNING: File bloat detected (CLAUDE.md: %d, MEMORY.md: %d)\n", claudeLines, memoryLines)
		}
	}

	if !autoMode {
		fmt.Printf("Synergy health: %.2f | Jiminy: %v | CLAUDE.md: %d | MEMORY.md: %d\n",
			health, jiminyHealthy, claudeLines, memoryLines)
	}

	if unhealthy {
		return fmt.Errorf("synergy health check failed")
	}
	return nil
}

// ─── Helpers ───

func resolveEndpoint() string {
	if v := os.Getenv("MDEMG_URL"); v != "" {
		return v
	}
	return "http://localhost:9999"
}

func checkJiminyHealth() bool {
	endpoint := resolveEndpoint()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/v1/jiminy/healthz", endpoint)) //nolint:gosec // G107: localhost health check
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var result struct {
		Enabled bool   `json:"enabled"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Enabled && result.Status == "ok"
}

func findClaudeMD() string {
	// 1. Config var
	if v := os.Getenv("SYNERGY_CLAUDE_MD_PATH"); v != "" {
		return v
	}
	// 2. Current directory
	if _, err := os.Stat("CLAUDE.md"); err == nil {
		abs, _ := filepath.Abs("CLAUDE.md")
		return abs
	}
	return ""
}

func findMemoryMD() string {
	// 1. Config var
	if v := os.Getenv("SYNERGY_MEMORY_MD_PATH"); v != "" {
		return v
	}
	// 2. Derive Claude Code project directory from CWD
	// Claude Code encodes the project path as hyphen-separated segments in the directory name
	// e.g., /Users/reh3376/mdemg → -Users-reh3376-mdemg
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	// Build expected project dir name from CWD: replace "/" with "-"
	if cwd != "" {
		projectDir := strings.ReplaceAll(cwd, "/", "-")
		candidate := filepath.Join(home, ".claude", "projects", projectDir, "memory", "MEMORY.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 3. Fallback: glob, prefer longest path (most specific)
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

func countLines(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return bytes.Count(data, []byte("\n"))
}

func countAutoMemory(memoryMDPath string) (files int, lines int) {
	if memoryMDPath == "" {
		return 0, 0
	}
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

func readMigrationFlag() (status string, date string) {
	flag := readMigrationFlagFull()
	if flag.Migrated {
		return flag.Version, flag.MigratedAt
	}
	return "", ""
}

func readMigrationFlagFull() synergyMigrationFlag {
	path := filepath.Join(".mdemg", "synergy-migrated.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return synergyMigrationFlag{}
	}
	var flag synergyMigrationFlag
	if err := json.Unmarshal(data, &flag); err != nil {
		return synergyMigrationFlag{}
	}
	return flag
}

func writeMigrationFlag(flag synergyMigrationFlag) {
	data, err := json.MarshalIndent(flag, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(".mdemg", 0755)
	_ = os.WriteFile(filepath.Join(".mdemg", "synergy-migrated.json"), data, 0644)
}

func computeLocalSynergyHealth(jiminyHealthy bool, claudeLines, memoryLines int) float64 {
	if !jiminyHealthy {
		return 0.0
	}
	score := 1.0
	if claudeLines > 200 {
		score -= 0.3
	} else if claudeLines > 150 {
		score -= 0.1
	}
	if memoryLines > 180 {
		score -= 0.3
	} else if memoryLines > 120 {
		score -= 0.1
	}
	if score < 0 {
		score = 0
	}
	return score
}

func observeToCMS(endpoint, spaceID, content, obsType string, tags []string) error {
	payload := map[string]any{
		"space_id":   spaceID,
		"session_id": "synergy-cli",
		"content":    content,
		"obs_type":   obsType,
		"tags":       tags,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post( //nolint:gosec // G107: localhost API call
		fmt.Sprintf("%s/v1/conversation/observe", endpoint),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("CMS observe returned %d", resp.StatusCode)
	}
	return nil
}

func toInt(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}
