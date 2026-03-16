package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarQuickstartCmd() *cobra.Command {
	var (
		profile  string
		agents   string
		endpoint string
		format   string
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "One-command sidecar setup",
		Long: `Run the full sidecar onboarding sequence in one command.

Equivalent to running:
  mdemg sidecar init --defaults
  mdemg sidecar install
  mdemg sidecar up
  mdemg sidecar attach-agent <agents>
  mdemg sidecar generate-hooks

Each step checks current state and skips if already done.
Stops on the first failure and reports what succeeded.

Examples:
  mdemg sidecar quickstart
  mdemg sidecar quickstart --profile local --agents claude-code
  mdemg sidecar quickstart --endpoint http://macstudio.local:9999
  mdemg sidecar quickstart --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarQuickstart(sidecarQuickstartFlags{
				profile:  profile,
				agents:   agents,
				endpoint: endpoint,
				format:   format,
				dryRun:   dryRun,
			})
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "local", "Runtime profile (local, studio-remote)")
	cmd.Flags().StringVar(&agents, "agents", "claude-code", "Comma-separated adapter names (claude-code, codex)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override sidecar API endpoint")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without making changes")

	return cmd
}

type sidecarQuickstartFlags struct {
	profile  string
	agents   string
	endpoint string
	format   string
	dryRun   bool
}

// quickstartStep tracks the outcome of each step in the quickstart sequence.
type quickstartStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "done", "skipped", "failed"
	Message string `json:"message"`
}

// quickstartReport is the JSON-serializable report for the quickstart command.
type quickstartReport struct {
	sidecar.ReportEnvelope
	Steps []quickstartStep `json:"steps"`
}

func runSidecarQuickstart(flags sidecarQuickstartFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	var steps []quickstartStep
	failed := false

	printStep := func(icon, name, msg string) {
		if flags.format != "json" {
			fmt.Printf("  %s %-16s %s\n", icon, name, msg)
		}
	}

	if flags.format != "json" {
		fmt.Println("Sidecar Quickstart")
		fmt.Println("==================")
		fmt.Println()
	}

	// --- Step 1: init ---
	state := sidecar.CurrentStateFrom(cwd)
	if state != sidecar.StateUninitialized {
		steps = append(steps, quickstartStep{"init", "skipped", fmt.Sprintf("already in state %q", state)})
		printStep("-", "init", fmt.Sprintf("skipped (state: %s)", state))
	} else {
		if flags.dryRun {
			steps = append(steps, quickstartStep{"init", "dry-run", "would initialize sidecar"})
			printStep("~", "init", "would initialize sidecar")
		} else {
			initErr := runSidecarInit(sidecarInitFlags{
				profile:  flags.profile,
				agents:   flags.agents,
				defaults: true,
				format:   "json", // suppress text output
				endpoint: flags.endpoint,
			})
			if initErr != nil {
				steps = append(steps, quickstartStep{"init", "failed", initErr.Error()})
				printStep("✗", "init", initErr.Error())
				failed = true
			} else {
				steps = append(steps, quickstartStep{"init", "done", "initialized"})
				printStep("✓", "init", "initialized")
			}
		}
	}

	// --- Step 2: install ---
	if !failed {
		state = sidecar.CurrentStateFrom(cwd)
		if state == sidecar.StateInstalled || state == sidecar.StateRunning || state == sidecar.StateStopped || state == sidecar.StateDegraded {
			steps = append(steps, quickstartStep{"install", "skipped", fmt.Sprintf("already in state %q", state)})
			printStep("-", "install", fmt.Sprintf("skipped (state: %s)", state))
		} else {
			if flags.dryRun {
				steps = append(steps, quickstartStep{"install", "dry-run", "would install dependencies"})
				printStep("~", "install", "would install dependencies")
			} else {
				installErr := runSidecarInstall(sidecarInstallFlags{
					format: "json",
				})
				if installErr != nil {
					steps = append(steps, quickstartStep{"install", "failed", installErr.Error()})
					printStep("✗", "install", installErr.Error())
					failed = true
				} else {
					steps = append(steps, quickstartStep{"install", "done", "installed"})
					printStep("✓", "install", "installed")
				}
			}
		}
	}

	// --- Step 3: up ---
	if !failed {
		state = sidecar.CurrentStateFrom(cwd)
		if state == sidecar.StateRunning {
			steps = append(steps, quickstartStep{"up", "skipped", "already running"})
			printStep("-", "up", "skipped (already running)")
		} else {
			if flags.dryRun {
				steps = append(steps, quickstartStep{"up", "dry-run", "would start services"})
				printStep("~", "up", "would start services")
			} else {
				upErr := runSidecarUp(sidecarUpFlags{
					format: "json",
				})
				if upErr != nil {
					steps = append(steps, quickstartStep{"up", "failed", upErr.Error()})
					printStep("✗", "up", upErr.Error())
					failed = true
				} else {
					steps = append(steps, quickstartStep{"up", "done", "services started"})
					printStep("✓", "up", "services started")
				}
			}
		}
	}

	// --- Step 4: attach-agent ---
	if !failed {
		var adapterNames []string
		for a := range strings.SplitSeq(flags.agents, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				adapterNames = append(adapterNames, a)
			}
		}

		for _, name := range adapterNames {
			if flags.dryRun {
				steps = append(steps, quickstartStep{
					"attach-agent", "dry-run",
					fmt.Sprintf("would attach %s", name),
				})
				printStep("~", "attach-agent", fmt.Sprintf("would attach %s", name))
			} else {
				attachErr := runSidecarAttachAgent(sidecarAttachAgentFlags{
					adapterName: name,
					format:      "json",
				})
				if attachErr != nil {
					steps = append(steps, quickstartStep{
						"attach-agent", "failed",
						fmt.Sprintf("%s: %v", name, attachErr),
					})
					printStep("✗", "attach-agent", fmt.Sprintf("%s: %v", name, attachErr))
					failed = true
					break
				}
				steps = append(steps, quickstartStep{
					"attach-agent", "done",
					fmt.Sprintf("%s attached", name),
				})
				printStep("✓", "attach-agent", fmt.Sprintf("%s attached", name))
			}
		}
	}

	// --- Step 5: generate-hooks ---
	if !failed {
		if flags.dryRun {
			steps = append(steps, quickstartStep{"generate-hooks", "dry-run", "would generate hooks"})
			printStep("~", "generate-hooks", "would generate hooks")
		} else {
			hookErr := runSidecarGenerateHooks("json", false)
			if hookErr != nil {
				steps = append(steps, quickstartStep{"generate-hooks", "failed", hookErr.Error()})
				printStep("✗", "generate-hooks", hookErr.Error())
				failed = true
			} else {
				steps = append(steps, quickstartStep{"generate-hooks", "done", "hooks generated"})
				printStep("✓", "generate-hooks", "hooks generated")
			}
		}
	}

	// Build final report
	stateAfter := sidecar.CurrentStateFrom(cwd)
	exitCode := sidecar.ExitSuccess
	if failed {
		exitCode = sidecar.ExitRuntime
	}

	report := quickstartReport{
		ReportEnvelope: sidecar.NewReportEnvelope("mdemg sidecar quickstart", sidecar.StateUninitialized, stateAfter, exitCode),
		Steps:          steps,
	}

	if failed {
		report.NextActions = []string{"Fix the failed step above and re-run: mdemg sidecar quickstart"}
	} else if flags.dryRun {
		report.Result = "dry-run"
		report.NextActions = []string{"Run without --dry-run to apply"}
	} else {
		projectDir := cwd
		report.NextActions = []string{
			"Sidecar is ready! Start Claude Code in this project directory.",
			fmt.Sprintf("Config:  %s", filepath.Join(projectDir, ".mdemg", "sidecar.yaml")),
			fmt.Sprintf("MCP:     %s", filepath.Join(projectDir, ".claude", "mcp.json")),
			fmt.Sprintf("Hooks:   %s", filepath.Join(projectDir, ".claude", "hooks")),
			"Status:  mdemg sidecar status",
		}
	}

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text summary
	fmt.Println()
	if failed {
		fmt.Println("  Quickstart stopped due to failure.")
		fmt.Println("  Fix the issue and re-run: mdemg sidecar quickstart")
	} else if flags.dryRun {
		fmt.Println("  [dry-run] No changes made.")
		fmt.Println("  Run without --dry-run to apply.")
	} else {
		fmt.Println("  Sidecar is ready!")
		fmt.Println()
		fmt.Println("  Next steps:")
		for _, a := range report.NextActions[1:] {
			fmt.Printf("    %s\n", a)
		}
	}
	fmt.Println()

	if failed {
		return fmt.Errorf("quickstart failed")
	}
	return nil
}
