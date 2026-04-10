package cli

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// newDataCurateCmd creates the `data curate` subcommand.
// Shells out to paradigm_router.py for spec-driven training data curation.
func newDataCurateCmd() *cobra.Command {
	var (
		specPath  string
		inputDir  string
		outputDir string
		version   string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "curate",
		Short: "Run spec-driven training data curation pipeline",
		Long: `Run the UAITS paradigm router to curate training data across all
paradigms declared in a UAITS spec (SFT, DPO, RAFT, curriculum).

The pipeline reads exported JSONL data, routes each dataset through its
paradigm-specific processing, and writes curated output.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			absSpec, err := filepath.Abs(specPath)
			if err != nil {
				return err
			}
			args := []string{"-m", "training.paradigm_router",
				"--spec", absSpec,
				"--input-dir", inputDir,
				"--output-dir", outputDir,
				"--version", version,
			}
			if dryRun {
				args = append(args, "--dry-run")
			}

			c := exec.Command("python3", args...) //nolint:gosec // G204: args from flags
			c.Dir = filepath.Join(".", "neural")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().StringVar(&specPath, "spec",
		"docs/tests/uaits/specs/mdemg.uaits.json",
		"UAITS spec file path")
	cmd.Flags().StringVar(&inputDir, "input-dir", "",
		"Directory with exported JSONL data")
	cmd.Flags().StringVar(&outputDir, "output-dir", "",
		"Output directory for curated datasets")
	cmd.Flags().StringVar(&version, "version", "v1",
		"Dataset version label (default: v1)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Show what would be curated without writing")

	_ = cmd.MarkFlagRequired("input-dir")
	_ = cmd.MarkFlagRequired("output-dir")

	return cmd
}
