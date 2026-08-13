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
	// SCRUB-IDEMPOTENT-001 (2026-08-13): do NOT early-return when the
	// guidance_training_rows branch is clean — the retrieval_events and
	// llm_interactions branches below MUST still run. The previous
	// early-return silently skipped them.
	if len(dirties) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(nothing to do in guidance_training_rows)")
	}

	// Guidance-specific preview + apply. Only runs when dirties > 0 —
	// pre-fix (before SCRUB-IDEMPOTENT-001) this returned early on the
	// empty case, silently skipping the retrieval_events + llm_interactions
	// branches. Also drops the dry-run early-return so dry-run now walks
	// ALL three branches (retrieval + llm branches have their own dry-run
	// guards that DON'T return early).
	if len(dirties) > 0 {
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
			fmt.Fprintf(cmd.OutOrStdout(), "(dry-run — %d guidance_training_rows dirty rows would be scrubbed)\n", len(dirties))
		} else {
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
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Scrubbed %d/%d dirty rows in guidance_training_rows for space=%q\n", applied, len(dirties), spaceID)
		}
	}

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
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Sample (first 3):")
		for i, d := range retDirties {
			if i >= 3 {
				break
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  event_id=%s\n    query_text: %q → %q\n", d.id, short(d.oldQT, 80), short(d.newQT, 80))
		}
		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "(dry-run — would scrub %d retrieval_events rows)\n", len(retDirties))
		} else {
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
		}
	}

	// SCRUB-IDEMPOTENT-001 E4 (2026-08-13): also scrub
	// llm_interactions.user_prompt. Same predicate as the exporter's spec
	// (textFields[6] = nil = apply ALL patterns). Historical rows
	// pre-dating SCRUB-IDEMPOTENT-001's regex tightening got stored with
	// non-idempotent scrub output; the shipped intake scrub still runs
	// only one pass so those need a manual sweep.
	fmt.Fprintln(cmd.OutOrStdout(), "\n--- llm_interactions.user_prompt ---")
	llmScanSQL := `SELECT trace_id, user_prompt FROM llm_interactions
	               WHERE space_id = $1 AND time >= now() - $2::interval AND user_prompt IS NOT NULL`
	llmScanArgs := []any{spaceID, interval}
	if limit > 0 {
		llmScanSQL += fmt.Sprintf(" LIMIT %d", limit)
	}
	llmRows, err := client.Pool().Query(ctx, llmScanSQL, llmScanArgs...)
	if err != nil {
		return fmt.Errorf("scan llm_interactions: %w", err)
	}
	type llmDirty struct {
		id     string
		oldUP  string
		newUP  string
	}
	var llmDirties []llmDirty
	var llmScanned int
	for llmRows.Next() {
		var id, up string
		if err := llmRows.Scan(&id, &up); err != nil {
			llmRows.Close()
			return fmt.Errorf("scan llm row: %w", err)
		}
		llmScanned++
		newUP := llmclient.ScrubStringExcluding(up, nil) // full scrub, matches exporter textFields[6]=nil
		if newUP != up {
			llmDirties = append(llmDirties, llmDirty{id: id, oldUP: up, newUP: newUP})
		}
	}
	llmRows.Close()
	if err := llmRows.Err(); err != nil {
		return fmt.Errorf("scan llm iter: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Scanned:  %d rows\nDirty:    %d rows\n", llmScanned, len(llmDirties))
	if len(llmDirties) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(nothing to do in llm_interactions)")
		return nil
	}
	// Preview
	fmt.Fprintln(cmd.OutOrStdout(), "Sample (first 3):")
	for i, d := range llmDirties {
		if i >= 3 {
			break
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  trace_id=%s\n    user_prompt: %q → %q\n", d.id, short(d.oldUP, 80), short(d.newUP, 80))
	}
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "(dry-run — would scrub %d llm_interactions rows)\n", len(llmDirties))
		return nil
	}
	// TimescaleDB: historical llm_interactions chunks are compressed; raise
	// the per-txn decompression cap for this session so UPDATEs on old
	// chunks don't fail with SQLSTATE 53400. Best-effort; the UPDATEs below
	// still surface any real errors per row.
	if _, err := client.Pool().Exec(ctx, `SET timescaledb.max_tuples_decompressed_per_dml_transaction = 0`); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "(warn: could not raise decompression cap: %v)\n", err)
	}
	llmUpdateSQL := `UPDATE llm_interactions SET user_prompt = $2 WHERE trace_id = $1 AND space_id = $3`
	var llmApplied int
	for _, d := range llmDirties {
		tag, err := client.Pool().Exec(ctx, llmUpdateSQL, d.id, d.newUP, spaceID)
		if err != nil {
			return fmt.Errorf("update llm trace_id=%s: %w", d.id, err)
		}
		if tag.RowsAffected() > 0 {
			llmApplied++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Scrubbed %d/%d dirty rows in llm_interactions for space=%q\n", llmApplied, len(llmDirties), spaceID)
	return nil
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
