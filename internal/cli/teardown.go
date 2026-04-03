package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"mdemg/internal/api"
	"mdemg/internal/db"
	"mdemg/internal/secrets"
	"mdemg/internal/sidecar"
	"mdemg/internal/transfer"

	osExec "os/exec"
)

type teardownFlags struct {
	dryRun   bool
	format   string
	yes      bool
	force    bool
	full     bool
	export   bool
	keepData bool
	spaceID  string
}

// TeardownReport extends ReportEnvelope with teardown-specific fields.
type TeardownReport struct {
	sidecar.ReportEnvelope
	Scope      string `json:"scope"`
	Force      bool   `json:"force"`
	KeepData   bool   `json:"keep_data"`
	BackupPath string `json:"backup_path"`
	ExportPath string `json:"export_path,omitempty"`
}

// NewTeardownReport creates a TeardownReport with all slices initialized to [].
func NewTeardownReport(exitCode int, scope string, force, keepData bool, backupPath, exportPath string) TeardownReport {
	return TeardownReport{
		ReportEnvelope: sidecar.NewReportEnvelope("mdemg teardown", "", "", exitCode),
		Scope:          scope,
		Force:          force,
		KeepData:       keepData,
		BackupPath:     backupPath,
		ExportPath:     exportPath,
	}
}

func newTeardownCmd() *cobra.Command {
	var flags teardownFlags

	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove all MDEMG artifacts from this project",
		Long: `Completely remove all traces of MDEMG from the current project.

Instance scope (default):
  Stops server, removes Docker container/volume, deletes Neo4j space data,
  removes keyring secrets, uninstalls hooks, cleans up IDE configs, removes
  .mdemg/ directory, and deregisters from sidebar/menubar apps.

Full scope (--full):
  Additionally removes system-level artifacts: binary, plugins, man pages,
  shell completions, and systemd units. Use this when you want MDEMG
  completely gone from this machine.

Backs up .mdemg/ before removal as a safety net.

Examples:
  mdemg teardown                    # Interactive teardown of this instance
  mdemg teardown --dry-run          # Preview what would be removed
  mdemg teardown --yes              # Skip confirmation prompt
  mdemg teardown --export           # Export CMS data before teardown
  mdemg teardown --keep-data        # Preserve Neo4j volume
  mdemg teardown --full --yes       # Complete system removal
  mdemg teardown --format json      # Machine-readable output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.spaceID == "" {
				flags.spaceID = resolveSpaceID(cmd)
			}
			return runTeardown(flags)
		},
	}

	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Report what would be removed without executing")
	cmd.Flags().StringVar(&flags.format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVarP(&flags.yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Allow deletion of protected spaces (mdemg-dev, mdemg-global)")
	cmd.Flags().BoolVar(&flags.full, "full", false, "Include system-level cleanup (binary, plugins, systemd, man pages)")
	cmd.Flags().BoolVar(&flags.export, "export", false, "Export CMS/RSIC/Jiminy data before teardown")
	cmd.Flags().BoolVar(&flags.keepData, "keep-data", false, "Keep Neo4j volume (preserve graph data)")
	cmd.Flags().StringVar(&flags.spaceID, "space-id", "", "Space ID for Neo4j data cleanup (default: auto-detect)")

	return cmd
}

func runTeardown(flags teardownFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	scope := "instance"
	if flags.full {
		scope = "full"
	}

	// Auto-detect space ID from config or directory name
	spaceID := flags.spaceID
	if spaceID == "" {
		spaceID = detectSpaceID(cwd)
	}

	// Resolve container/volume names
	containerName, volumeName, _ := resolveProjectContainer()

	// Build dry-run preview
	if flags.dryRun {
		return runTeardownDryRun(cwd, flags, scope, spaceID, containerName, volumeName)
	}

	// Confirmation prompt
	if !flags.yes {
		if err := teardownConfirm(scope, cwd, spaceID); err != nil {
			return err
		}
	}

	var changes []sidecar.ReportChange
	var exportPath string

	// Phase 0: Pre-export
	if flags.export {
		fmt.Println("=== Phase 0: Exporting data ===")
		ep, err := teardownExport(cwd, spaceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: export failed: %v (continuing with teardown)\n", err)
		} else if ep != "" {
			exportPath = ep
			changes = append(changes, sidecar.ReportChange{Path: ep, Action: "exported"})
			fmt.Printf("Exported to: %s\n", ep)
		}
		fmt.Println()
	}

	// Phase 1: Stop running MDEMG server
	fmt.Println("=== Phase 1: Stopping MDEMG server ===")
	if c := teardownStopServer(); c != nil {
		changes = append(changes, *c)
	}
	fmt.Println()

	// Phase 2: Stop and remove Docker container
	fmt.Println("=== Phase 2: Removing Docker container ===")
	if c := teardownRemoveContainer(containerName, cwd, flags.keepData); c != nil {
		changes = append(changes, *c)
	}
	fmt.Println()

	// Phase 3: Remove Docker volume
	fmt.Println("=== Phase 3: Removing Docker volume ===")
	if !flags.keepData {
		if c := teardownRemoveVolume(volumeName); c != nil {
			changes = append(changes, *c)
		}
	} else {
		fmt.Println("Keeping Neo4j volume (--keep-data)")
	}
	fmt.Println()

	// Phase 4: Delete Neo4j space data
	fmt.Println("=== Phase 4: Cleaning Neo4j space data ===")
	if spaceID != "" {
		if c := teardownDeleteSpace(spaceID, flags.force); c != nil {
			changes = append(changes, *c)
		}
	} else {
		fmt.Println("No space ID detected — skipping")
	}
	fmt.Println()

	// Phase 5: Remove keyring secrets
	fmt.Println("=== Phase 5: Removing keyring secrets ===")
	changes = append(changes, teardownRemoveSecrets()...)
	fmt.Println()

	// Phase 6: Uninstall hooks
	fmt.Println("=== Phase 6: Uninstalling hooks ===")
	changes = append(changes, teardownUninstallHooks(cwd)...)
	fmt.Println()

	// Phase 7: Remove IDE configs (MCP)
	fmt.Println("=== Phase 7: Removing IDE configs ===")
	changes = append(changes, teardownRemoveMCPConfigs(cwd)...)
	fmt.Println()

	// Phase 8: Backup and remove .mdemg/
	fmt.Println("=== Phase 8: Removing .mdemg/ directory ===")
	backupPath := ""
	if c, bp := teardownRemoveMdemgDir(cwd); c != nil {
		changes = append(changes, *c)
		backupPath = bp
	}
	fmt.Println()

	// Phase 9: Remove .mdemgignore
	fmt.Println("=== Phase 9: Removing .mdemgignore ===")
	if c := teardownRemoveFile(filepath.Join(cwd, ".mdemgignore")); c != nil {
		changes = append(changes, *c)
	}
	// Warn about .env but don't remove it
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		fmt.Println("Note: .env file preserved (may contain MDEMG variables — review manually)")
	}
	fmt.Println()

	// Phase 10: Deregister from sidebar/menubar
	fmt.Println("=== Phase 10: Deregistering from sidebar apps ===")
	if c := teardownDeregisterSidebar(cwd); c != nil {
		changes = append(changes, *c)
	}
	fmt.Println()

	// Phase 11: Clean up Claude settings
	fmt.Println("=== Phase 11: Cleaning Claude settings ===")
	if c := teardownCleanClaudeSettings(cwd); c != nil {
		changes = append(changes, *c)
	}
	fmt.Println()

	// Full scope: system-level cleanup
	if flags.full {
		// Phase 12: Remove system binary
		fmt.Println("=== Phase 12: System binary ===")
		changes = append(changes, teardownSystemBinary()...)
		fmt.Println()

		// Phase 13: Remove system-level artifacts (Linux)
		fmt.Println("=== Phase 13: System artifacts ===")
		changes = append(changes, teardownSystemArtifacts()...)
		fmt.Println()
	}

	// Build report
	report := NewTeardownReport(sidecar.ExitSuccess, scope, flags.force, flags.keepData, backupPath, exportPath)
	report.Changes = changes
	report.NextActions = buildTeardownNextActions(backupPath, exportPath, flags.full)

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text summary
	printTeardownSummary(report, changes)
	return nil
}

// --- Dry-run ---

func runTeardownDryRun(cwd string, flags teardownFlags, scope, spaceID, containerName, volumeName string) error {
	report := NewTeardownReport(sidecar.ExitSuccess, scope, flags.force, flags.keepData, "", "")
	report.Result = "dry-run"

	actions := []string{}

	if flags.export {
		actions = append(actions, fmt.Sprintf("Would export space %q before teardown", spaceID))
	}

	// Server
	pidPath := pidFilePath()
	if pid, err := readPID(pidPath); err == nil && isProcessAlive(pid) {
		actions = append(actions, fmt.Sprintf("Would stop MDEMG server (pid=%d)", pid))
	}

	// Docker
	if DockerAvailable() {
		if state, err := InspectContainer(containerName); err == nil && state.Exists {
			actions = append(actions, fmt.Sprintf("Would remove container: %s", containerName))
		}
		if !flags.keepData && volumeName != "" {
			actions = append(actions, fmt.Sprintf("Would remove volume: %s", volumeName))
		} else if flags.keepData {
			actions = append(actions, "Would keep Neo4j volume (--keep-data)")
		}
	}

	// Space
	if spaceID != "" {
		if api.IsProtectedSpace(spaceID) && !flags.force {
			actions = append(actions, fmt.Sprintf("Would skip protected space: %s (use --force to delete)", spaceID))
		} else {
			actions = append(actions, fmt.Sprintf("Would delete Neo4j space data: %s", spaceID))
		}
	}

	// Secrets
	for key := range secrets.KnownSecrets {
		actions = append(actions, fmt.Sprintf("Would remove keyring secret: %s", key))
	}

	// Hooks
	if isMdemgHook(filepath.Join(cwd, ".git", "hooks", "post-commit")) {
		actions = append(actions, "Would remove git post-commit hook")
	}
	for _, hf := range claudeHookFiles() {
		hookPath := filepath.Join(cwd, ".claude", "hooks", hf.Template)
		if isMdemgHook(hookPath) {
			actions = append(actions, fmt.Sprintf("Would remove Claude hook: %s", hf.Template))
		}
	}

	// MCP configs
	for _, mcpPath := range mcpConfigPaths(cwd) {
		if _, err := os.Stat(mcpPath); err == nil {
			rel, _ := filepath.Rel(cwd, mcpPath)
			actions = append(actions, fmt.Sprintf("Would clean MCP config: %s", rel))
		}
	}

	// .mdemg/
	mdemgDir := filepath.Join(cwd, ".mdemg")
	if _, err := os.Stat(mdemgDir); err == nil {
		actions = append(actions, "Would backup .mdemg/ to .mdemg-backup-<timestamp>/")
		actions = append(actions, fmt.Sprintf("Would remove %s", mdemgDir))
	}

	// .mdemgignore
	if _, err := os.Stat(filepath.Join(cwd, ".mdemgignore")); err == nil {
		actions = append(actions, "Would remove .mdemgignore")
	}

	// Sidebar
	for _, regPath := range sidebarRegistryPaths() {
		if _, err := os.Stat(regPath); err == nil {
			actions = append(actions, fmt.Sprintf("Would deregister from: %s", regPath))
		}
	}

	// Claude settings
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "settings.local.json")); err == nil {
		actions = append(actions, "Would clean MDEMG entries from .claude/settings.local.json")
	}

	// Full scope
	if flags.full {
		actions = append(actions, "Would remove system binary (or advise brew uninstall)")
		if runtime.GOOS == "linux" {
			actions = append(actions, "Would remove systemd units, man pages, completions, plugins")
		}
	}

	report.NextActions = actions

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	fmt.Println("[dry-run] MDEMG Teardown")
	fmt.Println("========================")
	fmt.Printf("  Scope:     %s\n", scope)
	fmt.Printf("  Space:     %s\n", spaceID)
	fmt.Printf("  Force:     %v\n", flags.force)
	fmt.Printf("  Keep data: %v\n", flags.keepData)
	fmt.Printf("  Export:    %v\n", flags.export)
	fmt.Println()
	fmt.Println("  Would perform:")
	for _, a := range actions {
		fmt.Printf("    - %s\n", a)
	}
	fmt.Println()
	return nil
}

// --- Confirmation ---

func teardownConfirm(scope, cwd, spaceID string) error {
	fmt.Println("MDEMG Teardown")
	fmt.Println("==============")
	fmt.Printf("  Scope:   %s\n", scope)
	fmt.Printf("  Project: %s\n", cwd)
	if spaceID != "" {
		fmt.Printf("  Space:   %s\n", spaceID)
	}
	fmt.Println()

	if scope == "full" {
		fmt.Print("This will remove MDEMG from your system. Type 'teardown' to confirm: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(input) != "teardown" {
			return fmt.Errorf("teardown cancelled")
		}
	} else {
		fmt.Print("This will remove all MDEMG artifacts from this project. Continue? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			return fmt.Errorf("teardown cancelled")
		}
	}
	fmt.Println()
	return nil
}

// --- Phase implementations ---

func teardownExport(cwd, spaceID string) (string, error) {
	if spaceID == "" {
		return "", fmt.Errorf("no space ID for export")
	}

	cfg, err := loadConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	origOutput := log.Writer()
	log.SetOutput(io.Discard)
	driver, err := db.NewDriver(cfg)
	log.SetOutput(origOutput)
	if err != nil {
		return "", fmt.Errorf("connect to Neo4j: %w", err)
	}
	defer driver.Close(ctx)

	ts := time.Now().Format("20060102T150405")
	outputPath := filepath.Join(cwd, fmt.Sprintf("%s-teardown-%s.mdemg", spaceID, ts))

	exporter := transfer.NewExporter(driver)
	exportCfg := transfer.DefaultExportConfig(spaceID)

	result, err := exporter.Export(ctx, exportCfg)
	if err != nil {
		return "", err
	}

	if err := transfer.WriteFile(outputPath, result); err != nil {
		return "", fmt.Errorf("write export file: %w", err)
	}
	return outputPath, nil
}

func teardownStopServer() *sidecar.ReportChange {
	pidPath := pidFilePath()
	pid, err := readPID(pidPath)
	if err != nil || !isProcessAlive(pid) {
		fmt.Println("Server not running")
		// Clean up stale PID/port files
		if err == nil {
			_ = removePID(pidPath)
			_ = removePortFile()
		}
		return nil
	}

	fmt.Printf("Stopping server (pid=%d)...\n", pid)
	if err := terminateProcess(pid); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: terminate failed: %v\n", err)
	}

	// Wait for graceful shutdown (up to 15s)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if isProcessAlive(pid) {
		fmt.Println("Warning: graceful shutdown timed out, force killing")
		_ = forceKillProcess(pid)
		time.Sleep(1 * time.Second)
	}

	_ = removePID(pidPath)
	_ = removePortFile()
	fmt.Println("Server stopped")

	return &sidecar.ReportChange{Path: pidPath, Action: "stopped"}
}

func teardownRemoveContainer(containerName, cwd string, keepData bool) *sidecar.ReportChange {
	if !DockerAvailable() {
		fmt.Println("Docker not available — skipping")
		return nil
	}

	// Docker Compose deployment: stop all services
	composePath := filepath.Join(cwd, "docker-compose.yml")
	if _, statErr := os.Stat(composePath); statErr == nil {
		fmt.Println("Docker Compose deployment detected")
		args := []string{"compose", "-f", composePath, "down"}
		if !keepData {
			args = append(args, "-v")
		}
		cmd := osExec.Command("docker", args...)
		cmd.Dir = cwd
		out, composeErr := cmd.CombinedOutput()
		if composeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: docker compose down failed: %s\n", strings.TrimSpace(string(out)))
			// Fall through to legacy cleanup
		} else {
			fmt.Println("Stopped and removed Docker Compose services")
			return &sidecar.ReportChange{Path: "docker-compose", Action: "removed"}
		}
	}

	// Legacy single-container cleanup
	state, err := InspectContainer(containerName)
	if err != nil || !state.Exists {
		fmt.Printf("Container '%s' not found — skipping\n", containerName)
		return nil
	}

	if state.Running {
		fmt.Printf("Stopping container '%s'...\n", containerName)
		if _, err := RunDockerCommand("stop", containerName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: container stop failed: %v\n", err)
		}
	}

	fmt.Printf("Removing container '%s'...\n", containerName)
	if _, err := RunDockerCommand("rm", "-f", containerName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: container removal failed: %v\n", err)
		return nil
	}

	fmt.Println("Container removed")
	return &sidecar.ReportChange{Path: containerName, Action: "removed"}
}

func teardownRemoveVolume(volumeName string) *sidecar.ReportChange {
	if !DockerAvailable() || volumeName == "" {
		fmt.Println("Skipping volume removal")
		return nil
	}

	fmt.Printf("Removing volume '%s'...\n", volumeName)
	if _, err := RunDockerCommand("volume", "rm", volumeName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: volume removal failed: %v (may not exist)\n", err)
		return nil
	}

	fmt.Println("Volume removed")
	return &sidecar.ReportChange{Path: volumeName, Action: "removed"}
}

func teardownDeleteSpace(spaceID string, force bool) *sidecar.ReportChange {
	if api.IsProtectedSpace(spaceID) {
		if !force {
			fmt.Printf("Space %q is protected — skipping (use --force to delete)\n", spaceID)
			return nil
		}
		fmt.Printf("Warning: deleting protected space %q (--force)\n", spaceID)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot load config for Neo4j connection: %v\n", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	origOutput := log.Writer()
	log.SetOutput(io.Discard)
	driver, err := db.NewDriver(cfg)
	log.SetOutput(origOutput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot connect to Neo4j: %v — space data not cleaned\n", err)
		return nil
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Batch delete nodes in the space
	totalDeleted := 0
	batchSize := 1000
	for {
		raw, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			result, err := tx.Run(ctx,
				"MATCH (n {space_id: $spaceID}) WITH n LIMIT $limit DETACH DELETE n RETURN count(*) AS deleted",
				map[string]any{"spaceID": spaceID, "limit": batchSize},
			)
			if err != nil {
				return nil, err
			}
			rec, err := result.Single(ctx)
			if err != nil {
				return nil, err
			}
			return rec.Values[0].(int64), nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: space deletion query failed: %v\n", err)
			break
		}
		deleted := raw.(int64)
		totalDeleted += int(deleted)
		if deleted < int64(batchSize) {
			break
		}
	}

	if totalDeleted > 0 {
		fmt.Printf("Deleted %d nodes from space %q\n", totalDeleted, spaceID)
		return &sidecar.ReportChange{Path: spaceID, Action: fmt.Sprintf("deleted %d nodes", totalDeleted)}
	}
	fmt.Printf("No nodes found in space %q\n", spaceID)
	return nil
}

func teardownRemoveSecrets() []sidecar.ReportChange {
	var changes []sidecar.ReportChange
	for key := range secrets.KnownSecrets {
		err := secrets.Delete(key)
		if err != nil {
			if err != secrets.ErrNotFound {
				fmt.Fprintf(os.Stderr, "Warning: could not remove secret %q: %v\n", key, err)
			}
			continue
		}
		fmt.Printf("Removed secret: %s\n", key)
		changes = append(changes, sidecar.ReportChange{Path: "keyring:" + key, Action: "removed"})
	}
	if len(changes) == 0 {
		fmt.Println("No keyring secrets found")
	}
	return changes
}

func teardownUninstallHooks(cwd string) []sidecar.ReportChange {
	var changes []sidecar.ReportChange

	// Git hooks
	removed, err := UninstallGitHook(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: git hook uninstall: %v\n", err)
	} else if removed {
		fmt.Println("Removed git post-commit hook")
		changes = append(changes, sidecar.ReportChange{Path: ".git/hooks/post-commit", Action: "removed"})
	}

	// Claude hooks
	claudeRemoved, err := UninstallClaudeHooks(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Claude hook uninstall: %v\n", err)
	} else if claudeRemoved > 0 {
		fmt.Printf("Removed %d Claude hook(s)\n", claudeRemoved)
		changes = append(changes, sidecar.ReportChange{Path: ".claude/hooks/", Action: fmt.Sprintf("removed %d hooks", claudeRemoved)})
	}

	if len(changes) == 0 {
		fmt.Println("No MDEMG hooks found")
	}
	return changes
}

func teardownRemoveMCPConfigs(cwd string) []sidecar.ReportChange {
	var changes []sidecar.ReportChange
	for _, mcpPath := range mcpConfigPaths(cwd) {
		removed, fileDeleted, err := removeMDEMGFromMCPConfig(mcpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: MCP cleanup %s: %v\n", mcpPath, err)
			continue
		}
		if removed {
			rel, _ := filepath.Rel(cwd, mcpPath)
			if fileDeleted {
				fmt.Printf("Removed MCP config: %s\n", rel)
				changes = append(changes, sidecar.ReportChange{Path: rel, Action: "removed"})
			} else {
				fmt.Printf("Cleaned MDEMG from: %s\n", rel)
				changes = append(changes, sidecar.ReportChange{Path: rel, Action: "cleaned"})
			}
		}
	}
	if len(changes) == 0 {
		fmt.Println("No MCP configs found")
	}
	return changes
}

func teardownRemoveMdemgDir(cwd string) (*sidecar.ReportChange, string) {
	mdemgDir := filepath.Join(cwd, ".mdemg")
	if _, err := os.Stat(mdemgDir); os.IsNotExist(err) {
		fmt.Println("No .mdemg/ directory found")
		return nil, ""
	}

	// Backup
	ts := time.Now().Format("20060102T150405")
	backupPath := filepath.Join(cwd, fmt.Sprintf(".mdemg-backup-%s", ts))
	fmt.Printf("Backing up %s → %s\n", mdemgDir, backupPath)

	if err := os.Rename(mdemgDir, backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: backup rename failed: %v, removing directly\n", err)
		if err := os.RemoveAll(mdemgDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove .mdemg/: %v\n", err)
			return nil, ""
		}
		fmt.Println("Removed .mdemg/ (backup failed)")
		return &sidecar.ReportChange{Path: mdemgDir, Action: "removed"}, ""
	}

	fmt.Println("Backed up and removed .mdemg/")
	return &sidecar.ReportChange{Path: mdemgDir, Action: "backed-up", BackupPath: backupPath}, backupPath
}

func teardownRemoveFile(path string) *sidecar.ReportChange {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", path, err)
		return nil
	}
	fmt.Printf("Removed %s\n", filepath.Base(path))
	return &sidecar.ReportChange{Path: path, Action: "removed"}
}

func teardownDeregisterSidebar(cwd string) *sidecar.ReportChange {
	removed, err := deregisterFromSidebar(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sidebar deregistration: %v\n", err)
		return nil
	}
	if removed > 0 {
		fmt.Printf("Deregistered from %d sidebar registry/registries\n", removed)
		return &sidecar.ReportChange{Path: "sidebar-registry", Action: fmt.Sprintf("deregistered %d entries", removed)}
	}
	fmt.Println("Not registered in any sidebar app")
	return nil
}

func teardownCleanClaudeSettings(cwd string) *sidecar.ReportChange {
	cleaned, err := cleanupClaudeSettings(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Claude settings cleanup: %v\n", err)
		return nil
	}
	if cleaned {
		fmt.Println("Cleaned MDEMG entries from .claude/settings.local.json")
		return &sidecar.ReportChange{Path: ".claude/settings.local.json", Action: "cleaned"}
	}
	fmt.Println("No MDEMG entries in Claude settings")
	return nil
}

func teardownSystemBinary() []sidecar.ReportChange {
	var changes []sidecar.ReportChange

	// Check if installed via Homebrew
	if _, err := RunBrewCommand("list", "mdemg"); err == nil {
		fmt.Println("MDEMG installed via Homebrew — run: brew uninstall mdemg")
		fmt.Println("(Not removing automatically — Homebrew manages this package)")
		return changes
	}

	// Check common binary locations
	binaryPaths := []string{
		"/usr/local/bin/mdemg",
		filepath.Join(homeDir(), ".local", "bin", "mdemg"),
	}

	for _, binPath := range binaryPaths {
		if _, err := os.Stat(binPath); err == nil {
			if os.Getuid() != 0 && strings.HasPrefix(binPath, "/usr/local") {
				fmt.Printf("Found %s — requires sudo to remove\n", binPath)
				fmt.Printf("Run: sudo rm %s\n", binPath)
			} else {
				if err := os.Remove(binPath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", binPath, err)
				} else {
					fmt.Printf("Removed %s\n", binPath)
					changes = append(changes, sidecar.ReportChange{Path: binPath, Action: "removed"})
				}
			}
		}
	}

	if len(changes) == 0 {
		fmt.Println("No system binary found to remove")
	}
	return changes
}

func teardownSystemArtifacts() []sidecar.ReportChange {
	if runtime.GOOS != "linux" {
		fmt.Println("System artifact cleanup is Linux-only — skipping on", runtime.GOOS)
		return nil
	}

	var changes []sidecar.ReportChange

	// Systemd units — check both /etc/systemd/system (tarball installs) and
	// /usr/lib/systemd/system (.deb installs)
	systemdUnits := []string{
		"/etc/systemd/system/mdemg@.service",
		"/etc/systemd/system/mdemg-rsic@.service",
		"/etc/systemd/system/mdemg-rsic@.timer",
		"/usr/lib/systemd/system/mdemg@.service",
		"/usr/lib/systemd/system/mdemg-rsic@.service",
		"/usr/lib/systemd/system/mdemg-rsic@.timer",
	}
	reloadSystemd := false
	for _, unit := range systemdUnits {
		if _, err := os.Stat(unit); err == nil {
			if os.Getuid() != 0 {
				fmt.Printf("Found %s — requires sudo to remove\n", unit)
			} else {
				if err := os.Remove(unit); err == nil {
					changes = append(changes, sidecar.ReportChange{Path: unit, Action: "removed"})
					reloadSystemd = true
				}
			}
		}
	}
	if reloadSystemd {
		fmt.Println("Reloading systemd daemon...")
		_ = runSystemctl("daemon-reload")
	}

	// Systemd share directory (persisted copies from install.sh)
	systemdShareDir := "/usr/local/share/mdemg/systemd"
	if _, err := os.Stat(systemdShareDir); err == nil {
		if os.Getuid() != 0 {
			fmt.Printf("Found %s — requires sudo to remove\n", systemdShareDir)
		} else if err := os.RemoveAll(systemdShareDir); err == nil {
			changes = append(changes, sidecar.ReportChange{Path: systemdShareDir, Action: "removed"})
		}
	}

	// Man pages
	manDir := "/usr/local/share/man/man1"
	if entries, err := filepath.Glob(filepath.Join(manDir, "mdemg*.1")); err == nil {
		for _, entry := range entries {
			if os.Getuid() != 0 {
				fmt.Printf("Found %s — requires sudo to remove\n", entry)
			} else if err := os.Remove(entry); err == nil {
				changes = append(changes, sidecar.ReportChange{Path: entry, Action: "removed"})
			}
		}
	}

	// Shell completions
	completionPaths := []string{
		"/etc/bash_completion.d/mdemg",
		"/usr/local/share/zsh/site-functions/_mdemg",
		"/usr/local/share/fish/vendor_completions.d/mdemg.fish",
	}
	for _, cp := range completionPaths {
		if _, err := os.Stat(cp); err == nil {
			if os.Getuid() != 0 {
				fmt.Printf("Found %s — requires sudo to remove\n", cp)
			} else if err := os.Remove(cp); err == nil {
				changes = append(changes, sidecar.ReportChange{Path: cp, Action: "removed"})
			}
		}
	}

	// Plugins directory
	pluginDir := "/usr/local/share/mdemg"
	if _, err := os.Stat(pluginDir); err == nil {
		if os.Getuid() != 0 {
			fmt.Printf("Found %s — requires sudo to remove\n", pluginDir)
		} else if err := os.RemoveAll(pluginDir); err == nil {
			changes = append(changes, sidecar.ReportChange{Path: pluginDir, Action: "removed"})
		}
	}

	if len(changes) == 0 && os.Getuid() != 0 {
		fmt.Println("System artifacts require root — run: sudo mdemg teardown --full --yes")
	}
	return changes
}

// --- Helpers ---

func detectSpaceID(cwd string) string {
	// Try .mdemg/config.yaml
	configPath := filepath.Join(cwd, ".mdemg", "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "space_id:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "space_id:"))
				val = strings.Trim(val, `"'`)
				if val != "" {
					return val
				}
			}
		}
	}

	// Fall back to directory name
	return filepath.Base(cwd)
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/home/user"
	}
	return home
}

// RunBrewCommand executes a brew command and returns its output.
func RunBrewCommand(args ...string) (string, error) {
	cmd := osExec.Command("brew", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("brew %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

func runSystemctl(args ...string) error {
	cmd := osExec.Command("systemctl", args...)
	return cmd.Run()
}

func buildTeardownNextActions(backupPath, exportPath string, full bool) []string {
	var actions []string
	if backupPath != "" {
		actions = append(actions, fmt.Sprintf("Backup saved at: %s", backupPath))
	}
	if exportPath != "" {
		actions = append(actions, fmt.Sprintf("Export saved at: %s", exportPath))
	}
	actions = append(actions, "mdemg init  (to re-initialize)")
	if full {
		actions = append(actions, "Re-install: curl -fsSL https://github.com/reh3376/mdemg/releases/latest/download/install.sh | bash")
	}
	return actions
}

func printTeardownSummary(report TeardownReport, changes []sidecar.ReportChange) {
	fmt.Println()
	fmt.Println("MDEMG Teardown Complete")
	fmt.Println("=======================")
	fmt.Printf("  Scope:     %s\n", report.Scope)
	fmt.Printf("  Keep data: %v\n", report.KeepData)
	if report.BackupPath != "" {
		fmt.Printf("  Backup:    %s\n", report.BackupPath)
	}
	if report.ExportPath != "" {
		fmt.Printf("  Export:    %s\n", report.ExportPath)
	}
	fmt.Println()

	if len(changes) > 0 {
		fmt.Println("  Changes:")
		for _, c := range changes {
			if c.BackupPath != "" {
				fmt.Printf("    %s: %s → %s\n", c.Action, c.Path, c.BackupPath)
			} else {
				fmt.Printf("    %s: %s\n", c.Action, c.Path)
			}
		}
		fmt.Println()
	}

	fmt.Println("  Next steps:")
	for _, a := range report.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()
}
