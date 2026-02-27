package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarAttachAgentCmd() *cobra.Command {
	var (
		dryRun    bool
		printOnly bool
		format    string
	)

	cmd := &cobra.Command{
		Use:   "attach-agent <adapter-name>",
		Short: "Attach an agent adapter",
		Long: `Wire MDEMG's MCP server into an AI agent's configuration.

Merges MDEMG config into the agent's config file with backup,
idempotency, and dry-run support.

Supported adapters: claude-code, codex

Examples:
  mdemg sidecar attach-agent claude-code
  mdemg sidecar attach-agent codex --dry-run
  mdemg sidecar attach-agent claude-code --print-only
  mdemg sidecar attach-agent claude-code --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarAttachAgent(sidecarAttachAgentFlags{
				adapterName: args[0],
				dryRun:      dryRun,
				printOnly:   printOnly,
				format:      format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would happen without making changes")
	cmd.Flags().BoolVar(&printOnly, "print-only", false, "Print the MCP config payload to stdout and exit")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarAttachAgentFlags struct {
	adapterName string
	dryRun      bool
	printOnly   bool
	format      string
}

func runSidecarAttachAgent(flags sidecarAttachAgentFlags) error {
	const command = "mdemg sidecar attach-agent"

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)

	// State guard: must be installed, running, stopped, or degraded
	switch stateBefore {
	case sidecar.StateInstalled, sidecar.StateRunning, sidecar.StateStopped, sidecar.StateDegraded:
		// valid
	default:
		remediation := "mdemg sidecar install"
		if stateBefore == sidecar.StateUninitialized {
			remediation = "mdemg sidecar init"
		} else if stateBefore == sidecar.StateInitialized {
			remediation = "mdemg sidecar install"
		}
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot attach agent from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("cannot attach agent from state %q — %s", stateBefore, remediation)
	}

	// Validate adapter name
	adapterName := sidecar.AdapterName(flags.adapterName)
	adapter, err := sidecar.NewAdapter(adapterName)
	if err != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "INVALID_ADAPTER",
				Severity:    "error",
				Message:     err.Error(),
				Remediation: "Valid adapters: claude-code, codex",
			})
			return sidecar.PrintJSON(env)
		}
		return err
	}

	// Load sidecar config for endpoint
	configPath := sidecar.FindConfigFileFrom(cwd)
	if configPath == "" {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_NOT_FOUND",
				Severity:    "error",
				Message:     "No sidecar.yaml found",
				Remediation: "Run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("no sidecar.yaml found — run: mdemg sidecar init")
	}

	cfg, err := sidecar.LoadConfig(configPath)
	if err != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_READ_ERROR",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to read config: %v", err),
				Remediation: "Check .mdemg/sidecar.yaml syntax",
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("read config: %w", err)
	}

	endpoint := cfg.Runtime.Endpoint

	// Resolve project dir from config path
	projectDir := filepath.Dir(filepath.Dir(configPath)) // .mdemg/sidecar.yaml → project root
	agentConfigPath := adapter.ConfigPath(projectDir)

	// --print-only: just print the payload and exit
	if flags.printOnly {
		payload, buildErr := adapter.BuildPayload(endpoint)
		if buildErr != nil {
			return buildErr
		}
		fmt.Print(string(payload))
		return nil
	}

	// Read existing agent config (may not exist)
	var existing []byte
	if data, readErr := os.ReadFile(agentConfigPath); readErr == nil {
		existing = data
	}

	// Compute what the merge would produce
	merged, strategy, mergeErr := adapter.MergeInto(existing, endpoint)
	if mergeErr != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitRuntime)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "MERGE_FAILED",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to merge config: %v", mergeErr),
				Remediation: fmt.Sprintf("Check %s syntax", agentConfigPath),
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("merge config: %w", mergeErr)
	}

	// --dry-run: report what would happen
	if flags.dryRun {
		report := sidecar.NewAttachAgentReport(command, stateBefore, stateBefore, sidecar.ExitSuccess)
		report.Result = "dry-run"
		report.AdapterName = string(adapter.Name())
		report.AdapterVersion = adapter.Version()
		report.ConfigPath = agentConfigPath
		report.ConfigFormat = adapter.ConfigFormat()
		report.DryRun = true
		report.MergeStrategy = strategy
		report.NextActions = []string{
			fmt.Sprintf("Would %s %s", strategy, agentConfigPath),
			"Would backup existing file",
			"Would update sidecar.yaml adapters list",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Println("[dry-run] Attach Agent")
		fmt.Println("======================")
		fmt.Printf("  Adapter:  %s (%s)\n", adapter.Name(), adapter.Version())
		fmt.Printf("  Config:   %s\n", agentConfigPath)
		fmt.Printf("  Strategy: %s\n", strategy)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range report.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	// --- Mutating path ---

	var changes []sidecar.ReportChange
	var backupManifest sidecar.BackupManifest

	// Backup existing file if it exists
	if len(existing) > 0 {
		backupDir := filepath.Join(projectDir, ".mdemg", "backups")
		if mkErr := os.MkdirAll(backupDir, 0755); mkErr != nil {
			return fmt.Errorf("create backup dir: %w", mkErr)
		}

		ts := time.Now().UTC().Format("20060102T150405Z")
		backupName := fmt.Sprintf("%s.%s", filepath.Base(agentConfigPath), ts)
		backupPath := filepath.Join(backupDir, backupName)

		if writeErr := os.WriteFile(backupPath, existing, 0644); writeErr != nil {
			return fmt.Errorf("write backup: %w", writeErr)
		}

		hash := sha256.Sum256(existing)
		backupManifest = sidecar.BackupManifest{
			BackupPath:   backupPath,
			OriginalHash: fmt.Sprintf("%x", hash),
			Timestamp:    ts,
		}

		changes = append(changes, sidecar.ReportChange{
			Path:       agentConfigPath,
			Action:     "backed-up",
			BackupPath: backupPath,
		})
	}

	// Ensure parent directory exists
	if mkErr := os.MkdirAll(filepath.Dir(agentConfigPath), 0755); mkErr != nil {
		return fmt.Errorf("create config dir: %w", mkErr)
	}

	// Write merged config
	if writeErr := os.WriteFile(agentConfigPath, merged, 0644); writeErr != nil {
		return fmt.Errorf("write config: %w", writeErr)
	}

	changes = append(changes, sidecar.ReportChange{
		Path:   agentConfigPath,
		Action: strategy + "d", // "created" or "merged"
	})

	// Update sidecar.yaml adapters list
	adapterUpdated := updateSidecarAdapters(configPath, cfg, adapterName, true)
	if adapterUpdated {
		changes = append(changes, sidecar.ReportChange{
			Path:   configPath,
			Action: "updated",
		})
	}

	// Build report
	report := sidecar.NewAttachAgentReport(command, stateBefore, stateBefore, sidecar.ExitSuccess)
	report.AdapterName = string(adapter.Name())
	report.AdapterVersion = adapter.Version()
	report.ConfigPath = agentConfigPath
	report.ConfigFormat = adapter.ConfigFormat()
	report.MergeStrategy = strategy
	report.BackupManifest = backupManifest
	report.Changes = changes
	report.NextActions = []string{
		"mdemg sidecar status",
		fmt.Sprintf("mdemg sidecar detach-agent %s", adapter.Name()),
	}

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println()
	fmt.Println("Attach Agent")
	fmt.Println("============")
	fmt.Printf("  Adapter:  %s (%s)\n", adapter.Name(), adapter.Version())
	fmt.Printf("  Config:   %s\n", agentConfigPath)
	fmt.Printf("  Strategy: %s\n", strategy)
	if backupManifest.BackupPath != "" {
		fmt.Printf("  Backup:   %s\n", backupManifest.BackupPath)
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	for _, a := range report.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()

	return nil
}

// updateSidecarAdapters adds or enables an adapter in the sidecar.yaml config.
// Returns true if the config was modified and saved.
func updateSidecarAdapters(configPath string, cfg *sidecar.Config, name sidecar.AdapterName, enabled bool) bool {
	found := false
	changed := false
	for i, a := range cfg.Adapters {
		if a.Name == name {
			found = true
			if a.Enabled != enabled {
				cfg.Adapters[i].Enabled = enabled
				changed = true
			}
			break
		}
	}
	if !found {
		cfg.Adapters = append(cfg.Adapters, sidecar.AdapterConfig{
			Name:    name,
			Enabled: enabled,
		})
		changed = true
	}

	if !changed {
		return false
	}

	data, err := sidecar.MarshalConfigYAML(cfg)
	if err != nil {
		return false
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return false
	}
	return true
}
