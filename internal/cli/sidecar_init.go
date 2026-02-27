package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarInitCmd() *cobra.Command {
	var (
		profile  string
		agents   string
		dryRun   bool
		format   string
		defaults bool
		endpoint string
		host     string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize sidecar configuration",
		Long: `Initialize MDEMG sidecar in the current repository.

Creates .mdemg/sidecar.yaml and .mdemg/sidecar.lock with the
specified profile and agent adapters.

Examples:
  mdemg sidecar init --defaults
  mdemg sidecar init --profile local --agents claude-code
  mdemg sidecar init --profile studio-remote --agents claude-code,codex --host macstudio.local
  mdemg sidecar init --dry-run --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarInit(sidecarInitFlags{
				profile:  profile,
				agents:   agents,
				dryRun:   dryRun,
				format:   format,
				defaults: defaults,
				endpoint: endpoint,
				host:     host,
			})
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "local", "Runtime profile (local, studio-remote)")
	cmd.Flags().StringVar(&agents, "agents", "claude-code", "Comma-separated adapter names (claude-code, codex)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate without writing files")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&defaults, "defaults", false, "Non-interactive with sensible defaults")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override sidecar API endpoint")
	cmd.Flags().StringVar(&host, "host", "", "Remote runtime host (required for studio-remote)")

	return cmd
}

type sidecarInitFlags struct {
	profile  string
	agents   string
	dryRun   bool
	format   string
	defaults bool
	endpoint string
	host     string
}

func runSidecarInit(flags sidecarInitFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	mdemgPath := filepath.Join(cwd, ".mdemg")
	stateBefore := sidecar.CurrentStateFrom(cwd)

	// Check for existing config
	existingConfig := sidecar.FindConfigFileFrom(cwd)
	if existingConfig != "" && !flags.defaults && !flags.dryRun {
		fmt.Println("Found existing sidecar configuration.")
		answer := promptLine("Reconfigure? (yes/no)", "no")
		if answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Parse profile
	prof := sidecar.Profile(flags.profile)
	if !prof.IsValid() {
		return fmt.Errorf("unknown profile %q (valid: local, studio-remote)", flags.profile)
	}

	// Parse adapters
	var adapterNames []sidecar.AdapterName
	for a := range strings.SplitSeq(flags.agents, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		adapterNames = append(adapterNames, sidecar.AdapterName(a))
	}

	// Generate config
	cfg := sidecar.GenerateConfig(prof, adapterNames, flags.endpoint, flags.host)

	// Validate
	errs := sidecar.ValidateConfig(cfg)
	if sidecar.HasErrors(errs) {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar init", stateBefore, stateBefore, sidecar.ExitValidation)
			for _, e := range errs {
				env.Issues = append(env.Issues, sidecar.ReportIssue{
					Code:        "CONFIG_VALIDATION",
					Severity:    e.Level,
					Message:     fmt.Sprintf("%s: %s", e.Field, e.Message),
					Remediation: fmt.Sprintf("Fix the %s field in sidecar.yaml or pass the correct flag", e.Field),
				})
			}
			return sidecar.PrintJSON(env)
		}
		fmt.Println("Configuration validation errors:")
		for _, e := range errs {
			fmt.Printf("  [%s] %s: %s\n", e.Level, e.Field, e.Message)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(errs))
	}

	// Marshal YAML
	yamlData, err := sidecar.MarshalConfigYAML(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configHash := sidecar.HashConfig(yamlData)

	// Dry run — report what would happen
	if flags.dryRun {
		stateAfter := sidecar.StateInitialized
		if stateBefore != sidecar.StateUninitialized {
			stateAfter = stateBefore
		}

		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar init", stateBefore, stateAfter, sidecar.ExitSuccess)
			env.Changes = []sidecar.ReportChange{
				{Path: filepath.Join(mdemgPath, "sidecar.yaml"), Action: "would-create"},
				{Path: filepath.Join(mdemgPath, "sidecar.lock"), Action: "would-create"},
			}
			env.NextActions = []string{"Run without --dry-run to apply"}
			return sidecar.PrintJSON(env)
		}
		fmt.Println("[dry-run] Would create:")
		fmt.Printf("  %s\n", filepath.Join(mdemgPath, "sidecar.yaml"))
		fmt.Printf("  %s\n", filepath.Join(mdemgPath, "sidecar.lock"))
		fmt.Printf("\nProfile:  %s\n", cfg.Profile)
		fmt.Printf("Endpoint: %s\n", cfg.Runtime.Endpoint)
		fmt.Printf("Adapters: %s\n", flags.agents)
		return nil
	}

	// Write files
	if err := os.MkdirAll(mdemgPath, 0755); err != nil {
		return fmt.Errorf("create .mdemg/: %w", err)
	}

	configPath := filepath.Join(mdemgPath, "sidecar.yaml")
	if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
		return fmt.Errorf("write sidecar.yaml: %w", err)
	}

	lock := sidecar.NewLockFile(string(cfg.Profile), cfg.Runtime.Endpoint, configHash)
	lockPath := filepath.Join(mdemgPath, "sidecar.lock")
	if err := sidecar.WriteLock(lockPath, lock); err != nil {
		return fmt.Errorf("write sidecar.lock: %w", err)
	}

	stateAfter := sidecar.StateInitialized

	if flags.format == "json" {
		env := sidecar.NewReportEnvelope("mdemg sidecar init", stateBefore, stateAfter, sidecar.ExitSuccess)
		env.Changes = []sidecar.ReportChange{
			{Path: configPath, Action: "created"},
			{Path: lockPath, Action: "created"},
		}
		env.NextActions = []string{"mdemg sidecar install"}
		return sidecar.PrintJSON(env)
	}

	// Text output
	fmt.Println("Sidecar Initialization")
	fmt.Println("======================")
	fmt.Println()
	fmt.Printf("  Created %s\n", configPath)
	fmt.Printf("  Created %s\n", lockPath)
	fmt.Println()
	fmt.Printf("  Profile:  %s\n", cfg.Profile)
	fmt.Printf("  Endpoint: %s\n", cfg.Runtime.Endpoint)
	fmt.Printf("  Adapters: %s\n", flags.agents)
	fmt.Printf("  State:    %s → %s\n", stateBefore, stateAfter)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  mdemg sidecar install")
	return nil
}
