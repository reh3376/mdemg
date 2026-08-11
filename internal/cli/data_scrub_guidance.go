// Sprint EXPORT-SCRUB-INTAKE-001 — one-shot backfill for guidance_training_rows.
//
// The intake scrub landed in JIMINY-RELEVANCE-001's writer is FORWARD-only.
// Rows written before EXPORT-SCRUB-INTAKE-001 shipped still carry raw
// abs_paths; the export scan-gate blocks on them → chronic export-auto HIGH
// alert. This command UPDATE-in-place scrubs matching historical rows so
// export-auto succeeds without waiting for the 30-day retention window
// to age them out.
//
// Safety:
//   - Dry-run default; --yes required for real writes.
//   - Bounded to the caller's --space-id (never touches other spaces).
//   - Optional --since duration to bound the scan window.
//   - Reports matched-row count + a small sample preview BEFORE applying.
//   - Not reversible (scrub is one-way).

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/llmclient"
	"mdemg/internal/tsdb"
)

func newDataScrubExportTablesCmd() *cobra.Command {
	var (
		spaceID string
		since   time.Duration
		dryRun  bool
		yes     bool
		limit   int
	)
	cmd := &cobra.Command{
		Use:   "scrub-export-tables",
		Short: "Backfill scrub PII in export tables (guidance_training_rows + retrieval_events) — EXPORT-SCRUB-INTAKE-001",
		Long: `Rewrite historical guidance_training_rows + retrieval_events rows in place to scrub
abs_paths from action_summary + guidance_content, so the export scan-gate
does not block on rows written before EXPORT-SCRUB-INTAKE-001's intake
scrub shipped.

Scrub semantics (must stay in sync with GuidanceTrainingRowsWriter.Record):
  - action_summary  → full scrub (all privacy patterns)
  - guidance_content → scrub with abs_path SKIPPED (operator-authored
    rules legitimately reference paths)

Safe:
  - Dry-run default. --yes required to write.
  - Bounded to the given --space-id.
  - Optional --since (default: 30d to match the exporter's default
    lookback).
  - Never touches other spaces.`,
		Example: `  # Preview what would be scrubbed on mdemg-dev
  mdemg data scrub-guidance-rows --space-id mdemg-dev

  # Real scrub
  mdemg data scrub-guidance-rows --space-id mdemg-dev --yes

  # Only scan the last day
  mdemg data scrub-guidance-rows --space-id mdemg-dev --since 24h --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			return runDataScrubExportTables(ctx, cmd, spaceID, since, dryRun, yes, limit)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID to scrub (required; never touches other spaces)")
	cmd.Flags().DurationVar(&since, "since", 30*24*time.Hour, "Only scan rows this recent (matches exporter default lookback)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Report what would change without writing (default true — pass --yes to actually write)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Actually apply the scrub (implies --dry-run=false)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap scanned rows to this count (0 = no limit)")
	return cmd
}

func runDataScrubExportTables(ctx context.Context, cmd *cobra.Command, spaceID string, since time.Duration, dryRun, yes bool, limit int) error {
	if yes {
		dryRun = false
	}
	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Guidance-Row Scrub Backfill\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Space:  %s\n", spaceID)
	fmt.Fprintf(cmd.OutOrStdout(), "Window: last %s\n", since)
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Mode:   DRY RUN (no writes — pass --yes to apply)\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Mode:   APPLY (rows will be rewritten in place)\n")
	}

	client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv())
	if err != nil {
		return fmt.Errorf("connect to TSDB: %w", err)
	}
	defer client.Close()

	// Scan candidate rows.
	scanSQL := `SELECT row_id, action_summary, guidance_content
	            FROM guidance_training_rows
	            WHERE space_id = $1 AND time >= now() - $2::interval`
	interval := fmt.Sprintf("%d seconds", int(since.Seconds()))
	scanArgs := []any{spaceID, interval}
	if limit > 0 {
		scanSQL += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := client.Pool().Query(ctx, scanSQL, scanArgs...)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	type dirty struct {
		id                string
		newAction, newGC  string
		oldAction, oldGC  string
		changedAction, changedGC bool
	}
	var dirties []dirty
	var scanned int
	for rows.Next() {
		var id, act, gc string
		if err := rows.Scan(&id, &act, &gc); err != nil {
			rows.Close()
			return fmt.Errorf("scan row: %w", err)
		}
		scanned++
		newAct := llmclient.ScrubStringExcluding(act, nil)
		newGC := llmclient.ScrubStringExcluding(gc, []string{"abs_path"})
		if newAct != act || newGC != gc {
			dirties = append(dirties, dirty{
				id:            id,
				newAction:     newAct,
				newGC:         newGC,
				oldAction:     act,
				oldGC:         gc,
				changedAction: newAct != act,
				changedGC:     newGC != gc,
			})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("scan iter: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nScanned:  %d rows\n", scanned)
	fmt.Fprintf(cmd.OutOrStdout(), "Dirty:    %d rows (need scrub)\n", len(dirties))
	if len(dirties) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(nothing to do — no rows contain unscrubbed PII)")
		return nil
	}

	// Preview first 3 dirties.
	fmt.Fprintln(cmd.OutOrStdout(), "\nSample (first 3):")
	for i, d := range dirties {
		if i >= 3 {
			break
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  row_id=%s\n", d.id)
		if d.changedAction {
			fmt.Fprintf(cmd.OutOrStdout(), "    action_summary: %q → %q\n", short(d.oldAction, 80), short(d.newAction, 80))
		}
		if d.changedGC {
			fmt.Fprintf(cmd.OutOrStdout(), "    guidance_content: %q → %q\n", short(d.oldGC, 80), short(d.newGC, 80))
		}
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "\n(dry-run — no rows written; re-run with --yes to apply the scrub to all %d dirty rows)\n", len(dirties))
		return nil
	}

	// APPLY. UPDATE-in-place per row. Bounded by the scanned set (we don't
	// re-scan; we UPDATE by row_id + time via the primary key).
	updateSQL := `UPDATE guidance_training_rows
	              SET action_summary = $2, guidance_content = $3
	              WHERE row_id = $1 AND space_id = $4`
	var applied int
	for _, d := range dirties {
		tag, err := client.Pool().Exec(ctx, updateSQL, d.id, d.newAction, d.newGC, spaceID)
		if err != nil {
			return fmt.Errorf("update row_id=%s: %w", d.id, err)
		}
		if tag.RowsAffected() > 0 {
			applied++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Scrubbed %d/%d dirty rows in guidance_training_rows for space=%q\n", applied, len(dirties), spaceID)

	// EXPORT-SCRUB-INTAKE-001 E2b: also scrub retrieval_events.query_text.
	// Same pattern; skip list matches retrievalEventsSpec.textFields (abs_path).
	fmt.Fprintln(cmd.OutOrStdout(), "\n--- retrieval_events.query_text ---")
	retScanSQL := `SELECT event_id, query_text FROM retrieval_events
	               WHERE space_id = $1 AND time >= now() - $2::interval AND query_text IS NOT NULL`
	retScanArgs := []any{spaceID, interval}
	if limit > 0 {
		retScanSQL += fmt.Sprintf(" LIMIT %d", limit)
	}
	retRows, err := client.Pool().Query(ctx, retScanSQL, retScanArgs...)
	if err != nil {
		return fmt.Errorf("scan retrieval_events: %w", err)
	}
	type retDirty struct {
		id     string
		oldQT  string
		newQT  string
	}
	var retDirties []retDirty
	var retScanned int
	for retRows.Next() {
		var id, qt string
		if err := retRows.Scan(&id, &qt); err != nil {
			retRows.Close()
			return fmt.Errorf("scan retrieval row: %w", err)
		}
		retScanned++
		newQT := llmclient.ScrubStringExcluding(qt, []string{"abs_path"})
		if newQT != qt {
			retDirties = append(retDirties, retDirty{id: id, oldQT: qt, newQT: newQT})
		}
	}
	retRows.Close()
	if err := retRows.Err(); err != nil {
		return fmt.Errorf("scan retrieval iter: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Scanned:  %d rows\nDirty:    %d rows\n", retScanned, len(retDirties))
	if len(retDirties) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(nothing to do in retrieval_events)")
		return nil
	}
	// Preview
	fmt.Fprintln(cmd.OutOrStdout(), "Sample (first 3):")
	for i, d := range retDirties {
		if i >= 3 {
			break
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  event_id=%s\n    query_text: %q → %q\n", d.id, short(d.oldQT, 80), short(d.newQT, 80))
	}
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "(dry-run — would scrub %d retrieval_events rows)\n", len(retDirties))
		return nil
	}
	retUpdateSQL := `UPDATE retrieval_events SET query_text = $2 WHERE event_id = $1 AND space_id = $3`
	var retApplied int
	for _, d := range retDirties {
		tag, err := client.Pool().Exec(ctx, retUpdateSQL, d.id, d.newQT, spaceID)
		if err != nil {
			return fmt.Errorf("update retrieval event_id=%s: %w", d.id, err)
		}
		if tag.RowsAffected() > 0 {
			retApplied++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Scrubbed %d/%d dirty rows in retrieval_events for space=%q\n", retApplied, len(retDirties), spaceID)
	return nil
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
