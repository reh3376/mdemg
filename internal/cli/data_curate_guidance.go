package cli

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

// newDataCurateGuidanceCmd creates the `data curate-guidance` subcommand.
// Shells out to scripts/curate_guidance_corpus.py to build the labeled guidance
// corpus from the guidance_training_rows evidence table (JIMINY-RELEVANCE-001
// Epic 5). Distinct from `data curate` (the file-based UAITS paradigm router) —
// this source is TSDB and the output is a corpus artifact + manifest the router
// can later consume as a file input.
func newDataCurateGuidanceCmd() *cobra.Command {
	var (
		version             string
		spaceID             string
		lookbackHours       int
		minLabelQuality     string
		against             string
		outDir              string
		dryRun              bool
		minSuggestionLength int
	)

	cmd := &cobra.Command{
		Use:   "curate-guidance",
		Short: "Curate the labeled guidance corpus from guidance_training_rows (TSDB)",
		Long: `Build the labeled guidance training corpus from the guidance_training_rows
evidence table (the guidance text + action text + verdict persisted by
JIMINY-RELEVANCE-001 Epic 1).

Prefers human-certified GOLD labels (HITL-REVIEW-001 review_grades, when that
table exists) over the LLM/heuristic auto-labels, distribution-summarizes the
production guidance_type x outcome mix, optionally leak-audits the output, and
writes a versioned corpus.jsonl + manifest.json under the output dir. The
manifest's gold_fraction is the signal the retrain FUTURE-TRIGGER reads.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			args := []string{"scripts/curate_guidance_corpus.py",
				"--version", version,
				"--space-id", spaceID,
				"--lookback-hours", strconv.Itoa(lookbackHours),
				"--min-label-quality", minLabelQuality,
				"--out-dir", outDir,
				"--min-suggestion-length", strconv.Itoa(minSuggestionLength),
			}
			if against != "" {
				args = append(args, "--against", against)
			}
			if dryRun {
				args = append(args, "--dry-run")
			}
			c := exec.Command("python3", args...) //nolint:gosec // G204: args from flags
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().StringVar(&version, "version", "v1", "Corpus artifact version dir")
	cmd.Flags().StringVar(&spaceID, "space-id", "mdemg-dev", "Space to curate (empty = all spaces)")
	cmd.Flags().IntVar(&lookbackHours, "lookback-hours", 4320, "Evidence window in hours (default 180d)")
	cmd.Flags().StringVar(&minLabelQuality, "min-label-quality", "real",
		"any=include heuristic/blank; real=LLM/tier1/explicit/gold only; gold=human-graded only")
	cmd.Flags().StringVar(&against, "against", "",
		"Comma-separated JSONL paths to leak-audit the corpus against")
	cmd.Flags().StringVar(&outDir, "out-dir", "training_data/guidance_corpus", "Output directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compute + print the manifest without writing files")
	cmd.Flags().IntVar(&minSuggestionLength, "min-suggestion-length", 40,
		"Min char length for review_grades.suggested_guidance to emit as a synthetic "+
			"corpus row (filters triage notes). Set 0 to disable.")

	return cmd
}
