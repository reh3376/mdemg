package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarDetachAgentCmd() *cobra.Command {
	var (
		dryRun bool
		format string
	)

	cmd := &cobra.Command{
		Use:   "detach-agent <adapter-name>",
		Short: "Detach an agent adapter",
		Long: `Remove MDEMG's MCP config from an AI agent's configuration.

Cleanly removes the MDEMG section from the agent's config file.

Supported adapters: claude-code, codex

Examples:
  mdemg sidecar detach-agent claude-code
  mdemg sidecar detach-agent codex --dry-run
  mdemg sidecar detach-agent claude-code --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarDetachAgent(sidecarDetachAgentFlags{
				adapterName: args[0],
				dryRun:      dryRun,
				format:      format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would happen without making changes")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarDetachAgentFlags struct {
	adapterName string
	dryRun      bool
	format      string
}

func runSidecarDetachAgent(flags sidecarDetachAgentFlags) error {
	const command = "mdemg sidecar detach-agent"

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)

	// State guard: same as attach
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
				Message:     fmt.Sprintf("Cannot detach agent from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("cannot detach agent from state %q — %s", stateBefore, remediation)
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

	// Load sidecar config
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

	// Resolve project dir and agent config path
	projectDir := filepath.Dir(filepath.Dir(configPath))
	agentConfigPath := adapter.ConfigPath(projectDir)

	// Read existing agent config — must exist for detach
	existing, readErr := os.ReadFile(agentConfigPath)
	if readErr != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_NOT_FOUND",
				Severity:    "error",
				Message:     fmt.Sprintf("Agent config not found: %s", agentConfigPath),
				Remediation: fmt.Sprintf("No %s config to detach from", adapter.Name()),
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("agent config not found: %s", agentConfigPath)
	}

	// Compute removal
	cleaned, removeErr := adapter.RemoveFrom(existing)
	if removeErr != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope(command, stateBefore, stateBefore, sidecar.ExitRuntime)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "REMOVE_FAILED",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to remove config: %v", removeErr),
				Remediation: fmt.Sprintf("Check %s syntax", agentConfigPath),
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("remove config: %w", removeErr)
	}

	// --dry-run
	if flags.dryRun {
		report := sidecar.NewAttachAgentReport(command, stateBefore, stateBefore, sidecar.ExitSuccess)
		report.Result = "dry-run"
		report.AdapterName = string(adapter.Name())
		report.AdapterVersion = adapter.Version()
		report.ConfigPath = agentConfigPath
		report.ConfigFormat = adapter.ConfigFormat()
		report.DryRun = true
		report.MergeStrategy = "remove"
		report.NextActions = []string{
			fmt.Sprintf("Would remove MDEMG config from %s", agentConfigPath),
			"Would update sidecar.yaml adapters list",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Println("[dry-run] Detach Agent")
		fmt.Println("======================")
		fmt.Printf("  Adapter:  %s (%s)\n", adapter.Name(), adapter.Version())
		fmt.Printf("  Config:   %s\n", agentConfigPath)
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

	// Write cleaned config (or remove file if empty/minimal)
	isEmpty := isEmptyConfig(cleaned, adapter.ConfigFormat())
	if isEmpty {
		if rmErr := os.Remove(agentConfigPath); rmErr != nil {
			return fmt.Errorf("remove empty config: %w", rmErr)
		}
		changes = append(changes, sidecar.ReportChange{
			Path:   agentConfigPath,
			Action: "removed",
		})
	} else {
		if writeErr := os.WriteFile(agentConfigPath, cleaned, 0644); writeErr != nil {
			return fmt.Errorf("write cleaned config: %w", writeErr)
		}
		changes = append(changes, sidecar.ReportChange{
			Path:   agentConfigPath,
			Action: "updated",
		})
	}

	// Update sidecar.yaml adapters list
	adapterUpdated := updateSidecarAdapters(configPath, cfg, adapterName, false)
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
	report.MergeStrategy = "remove"
	report.Changes = changes
	report.NextActions = []string{
		"mdemg sidecar status",
		fmt.Sprintf("mdemg sidecar attach-agent %s", adapter.Name()),
	}

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println()
	fmt.Println("Detach Agent")
	fmt.Println("============")
	fmt.Printf("  Adapter:  %s (%s)\n", adapter.Name(), adapter.Version())
	fmt.Printf("  Config:   %s\n", agentConfigPath)
	if isEmpty {
		fmt.Println("  Action:   removed empty config file")
	} else {
		fmt.Println("  Action:   removed MDEMG section")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	for _, a := range report.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()

	return nil
}

// isEmptyConfig returns true if the config bytes represent an empty/minimal config.
func isEmptyConfig(data []byte, format string) bool {
	switch format {
	case "json":
		// "{}\n" or "{}" is empty
		trimmed := string(data)
		return trimmed == "{}\n" || trimmed == "{}"
	case "toml":
		// Empty TOML is just whitespace/newlines
		for _, b := range data {
			if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
				return false
			}
		}
		return true
	default:
		return len(data) == 0
	}
}
