package cli

// Sprint JIMINY-STRUCTURED-CORRECTION-001 Epic 3 — one-time backfill CLI.
//
// Populates structured_data.correction on L0 obs_type='correction' rows
// authored BEFORE the E1 wiring (which persisted the structured form
// prospectively). Parses the joined content template via a regex, merges
// with any existing structured_data (preserves constraint-detector keys),
// and also propagates the three fields to any linked L1 role_type='correction'
// node via IMPLEMENTS_CORRECTION. Idempotent + dry-run-preview + batched.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
)

// correctionContentRegex matches the joined shape produced by
// conversation.buildCorrectionObserveRequest:
//   "CORRECTION: Incorrect: <X> | Correct: <Y>"
//   "CORRECTION: Incorrect: <X> | Correct: <Y> | Context: <Z>"
// The " | Context: " tail is optional. Uses lazy match on Y so an embedded
// " | " inside a manually-authored Correct doesn't get truncated.
var correctionContentRegex = regexp.MustCompile(
	`^CORRECTION: Incorrect: (.*?) \| Correct: (.*?)(?: \| Context: (.*))?$`)

func newCorrectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "corrections",
		Short: "L0/L1 correction node maintenance",
		Long:  "Backfill + audit tools for the correction ontology (JIMINY-STRUCTURED-CORRECTION-001).",
	}
	cmd.AddCommand(newCorrectionsRehydrateStructuredCmd())
	return cmd
}

func newCorrectionsRehydrateStructuredCmd() *cobra.Command {
	var spaceID string
	var dryRun bool
	var batchSize int
	cmd := &cobra.Command{
		Use:   "rehydrate-structured",
		Short: "Populate structured_data.correction on pre-E1 L0 correction obs + linked L1 nodes",
		Long: `Finds L0 conversation_observation nodes with obs_type='correction'
whose structured_data does NOT already carry the "correction" sub-object
(pre-JIMINY-STRUCTURED-CORRECTION-001 rows), parses the joined content
via the template regex, and populates:

  L0.structured_data.correction = {incorrect, correct, context}
  L1.correction_incorrect / L1.correction_correct / L1.correction_context
     (for any linked role_type='correction' node)

Idempotent — re-running skips rows that already carry the structured form.
Unparseable content is logged (WARN) and skipped.

  mdemg corrections rehydrate-structured --space-id mdemg-dev              # Preview
  mdemg corrections rehydrate-structured --space-id mdemg-dev --dry-run=false  # Execute`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			ctx := context.Background()
			driver, err := newDriver()
			if err != nil {
				return fmt.Errorf("neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("neo4j connectivity: %w", err)
			}
			return runCorrectionsRehydrate(ctx, driver, spaceID, dryRun, batchSize)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to rehydrate (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Update in batches of this size")
	return cmd
}

// rehydrateCandidate is one L0 obs eligible for structured backfill.
type rehydrateCandidate struct {
	nodeID       string
	content      string
	existingSD   string // may be "" or a JSON string; merge preserved
	linkedL1     string // linked correction node id (empty if none)
	incorrect    string
	correct      string
	context      string
	parsable     bool
}

// parseCorrectionContent tries the template regex. Returns (incorrect,
// correct, context, ok).
func parseCorrectionContent(content string) (string, string, string, bool) {
	m := correctionContentRegex.FindStringSubmatch(content)
	if m == nil {
		return "", "", "", false
	}
	// m[1]=incorrect, m[2]=correct, m[3]=context (may be empty when trailing
	// " | Context: <Z>" is absent).
	return m[1], m[2], m[3], true
}

// mergeStructuredCorrection merges the parsed fields into an existing
// structured_data JSON string, preserving other keys. Returns the new
// JSON string. Empty/malformed existing structured_data starts fresh.
func mergeStructuredCorrection(existingSD, incorrect, correct, contextS string) (string, error) {
	sd := map[string]any{}
	if existingSD != "" {
		if err := json.Unmarshal([]byte(existingSD), &sd); err != nil {
			// Preserve nothing; log via WARN at caller. Start fresh.
			sd = map[string]any{}
		}
	}
	sd["correction"] = map[string]any{
		"incorrect": incorrect,
		"correct":   correct,
		"context":   contextS,
	}
	b, err := json.Marshal(sd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func runCorrectionsRehydrate(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, dryRun bool, batchSize int) error {
	fmt.Println("MDEMG Corrections Rehydrate-Structured")
	fmt.Println("======================================")
	fmt.Printf("Space: %s\n", spaceID)
	if dryRun {
		fmt.Println("Mode:  DRY RUN (no changes)")
	} else {
		fmt.Println("Mode:  LIVE (structured_data will be populated on L0 + linked L1)")
	}
	fmt.Println()

	// Find L0 corrections missing structured "correction" sub-object.
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	findQuery := `
		MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
		WHERE obs.obs_type = 'correction'
		  AND NOT coalesce(obs.is_archived, false)
		  AND (obs.structured_data IS NULL
		       OR NOT (obs.structured_data CONTAINS '"correction"'))
		OPTIONAL MATCH (obs)-[:IMPLEMENTS_CORRECTION]->(c:MemoryNode {role_type: 'correction'})
		RETURN obs.node_id AS obsId, obs.content AS content,
		       coalesce(obs.structured_data, '') AS sd,
		       coalesce(c.node_id, '') AS linkedL1
		ORDER BY obs.created_at ASC
	`
	res, err := sess.Run(ctx, findQuery, map[string]any{"spaceId": spaceID})
	if err != nil {
		_ = sess.Close(ctx)
		return fmt.Errorf("find query: %w", err)
	}

	var cands []rehydrateCandidate
	scanned := 0
	unparseable := 0
	for res.Next(ctx) {
		scanned++
		rec := res.Record()
		obsID, _ := rec.Get("obsId")
		content, _ := rec.Get("content")
		sd, _ := rec.Get("sd")
		linkedL1, _ := rec.Get("linkedL1")
		c := rehydrateCandidate{
			nodeID:     fmt.Sprintf("%v", obsID),
			content:    fmt.Sprintf("%v", content),
			existingSD: fmt.Sprintf("%v", sd),
			linkedL1:   fmt.Sprintf("%v", linkedL1),
		}
		inc, cor, ctxS, ok := parseCorrectionContent(c.content)
		c.incorrect, c.correct, c.context, c.parsable = inc, cor, ctxS, ok
		if !ok {
			unparseable++
			fmt.Printf("  WARN unparseable content: %s — %.80s...\n", c.nodeID, c.content)
			continue
		}
		cands = append(cands, c)
	}
	_ = sess.Close(ctx)

	fmt.Printf("Scanned:      %d L0 correction obs missing structured\n", scanned)
	fmt.Printf("Parseable:    %d\n", len(cands))
	fmt.Printf("Unparseable:  %d (skipped)\n", unparseable)
	linkedCount := 0
	for _, c := range cands {
		if c.linkedL1 != "" {
			linkedCount++
		}
	}
	fmt.Printf("With L1 link: %d (will get L1 property backfill too)\n", linkedCount)

	if dryRun {
		fmt.Println("\nDry run — re-run with --dry-run=false to write.")
		return nil
	}

	if len(cands) == 0 {
		fmt.Println("\nNothing to do.")
		return nil
	}

	wsess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = wsess.Close(ctx) }()

	written := 0
	l1Updated := 0
	for i := 0; i < len(cands); i += batchSize {
		end := i + batchSize
		if end > len(cands) {
			end = len(cands)
		}
		for _, c := range cands[i:end] {
			newSD, err := mergeStructuredCorrection(c.existingSD, c.incorrect, c.correct, c.context)
			if err != nil {
				fmt.Printf("  WARN merge failed for %s: %v (skipped)\n", c.nodeID, err)
				continue
			}
			_, err = wsess.Run(ctx, `
				MATCH (obs:MemoryNode {node_id: $obsId, space_id: $spaceId})
				SET obs.structured_data = $sd, obs.updated_at = datetime()
			`, map[string]any{"obsId": c.nodeID, "spaceId": spaceID, "sd": newSD})
			if err != nil {
				fmt.Printf("  ERR update L0 %s: %v\n", c.nodeID, err)
				continue
			}
			written++
			if c.linkedL1 != "" {
				_, err = wsess.Run(ctx, `
					MATCH (c:MemoryNode {node_id: $l1Id, space_id: $spaceId, role_type: 'correction'})
					SET c.correction_incorrect = $incorrect,
					    c.correction_correct   = $correct,
					    c.correction_context   = $ctx,
					    c.updated_at = datetime()
				`, map[string]any{
					"l1Id":      c.linkedL1,
					"spaceId":   spaceID,
					"incorrect": c.incorrect,
					"correct":   c.correct,
					"ctx":       c.context,
				})
				if err != nil {
					fmt.Printf("  WARN update L1 %s: %v\n", c.linkedL1, err)
					continue
				}
				l1Updated++
			}
		}
		fmt.Printf("  batch %d..%d: wrote %d L0 (L1 updates: %d)\n", i, end, written, l1Updated)
	}
	fmt.Printf("\nDone. Wrote %d L0 obs (structured.correction populated); updated %d linked L1 correction nodes.\n", written, l1Updated)
	return nil
}
