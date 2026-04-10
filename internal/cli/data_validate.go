package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// newDataValidateCmd creates the `data validate` subcommand.
// Shells out to uaits_runner.py for spec schema and data validation.
func newDataValidateCmd() *cobra.Command {
	var (
		specPath string
		dataDir  string
		report   string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a UAITS spec and optionally check data compliance",
		Long: `Run the UAITS runner to validate a spec against the schema and
optionally check actual data compliance.

Without --data-dir, validates spec structure only (schema checks).
With --data-dir, also validates data files against the spec contract.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			args := []string{
				"docs/tests/uaits/runners/uaits_runner.py",
				"--spec", specPath,
			}
			if dataDir != "" {
				args = append(args, "--data-dir", dataDir)
			}
			if report != "" {
				args = append(args, "--report", report)
			}

			c := exec.Command("python3", args...) //nolint:gosec // G204: args from flags
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().StringVar(&specPath, "spec",
		"docs/tests/uaits/specs/mdemg.uaits.json",
		"UAITS spec file path (supports globs)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "",
		"Data directory for compliance validation")
	cmd.Flags().StringVar(&report, "report", "",
		"Output report JSON path")

	return cmd
}
