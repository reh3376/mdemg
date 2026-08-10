// Package cli — `mdemg beta-janitor` retention janitor for beta-import.
//
// Sprint B5c (2026-08-10). Companion to `mdemg beta-import` (B5b) — the
// receiver imports beta-tester bundles to space_id="beta-tester-<sid>";
// this command deletes them either on-demand (--submission-id) or in bulk
// by age (--older-than-days). The 30-day-retention promise the shipped
// beta-share bundle makes is a maintainer-side commitment, not a
// substrate-enforced rule — this command is how the operator honors it.
//
// Deletion is safe because:
//   - Every beta-import row has space_id LIKE 'beta-tester-%' (import-time
//     invariant enforced in importGuidanceRows: destSpace is always the
//     "beta-tester-" prefix + the submission_id suffix).
//   - Deletion by space_id (LIKE prefix or exact match) never touches the
//     operator's own space (mdemg-dev) or any other non-beta-tester space.
//   - The exporter's guidance_training_rows spec does NOT project row_id
//     (receiver re-mints via cuid2 per B5a/B5b contract), so deleting
//     imported rows can never affect original tester-side rows.
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/tsdb"
)

// newBetaJanitorCmd is `mdemg beta-janitor` (parent) + `delete` + `sweep` subcommands.
func newBetaJanitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "beta-janitor",
		Short: "Retention janitor for imported beta-tester bundles",
		Long: `Manages the maintainer-side 30-day retention promise on beta-tester
bundles imported via mdemg beta-import.

Subcommands:
  delete   — remove one specific submission by submission_id
  sweep    — remove all submissions older than N days (default 30)

Both operate ONLY on rows whose space_id starts with "beta-tester-" (an
import-time invariant of beta-import). Rows in the operator's own space
(e.g. mdemg-dev) are never touched.`,
	}
	cmd.AddCommand(newBetaJanitorDeleteCmd())
	cmd.AddCommand(newBetaJanitorSweepCmd())
	return cmd
}

func newBetaJanitorDeleteCmd() *cobra.Command {
	var (
		submissionID string
		dryRun       bool
		yes          bool
	)
	cmd := &cobra.Command{
		Use:   "delete --submission-id <sid>",
		Short: "Delete rows for a specific submission_id",
		Long: `Remove all guidance_training_rows imported via beta-import for the
given submission_id. Targets space_id="beta-tester-<sid>" exactly.

Idempotent: unknown submission_id → no-op, exit 0 with "0 rows deleted".`,
		Example: `  # Preview only
  mdemg beta-janitor delete --submission-id dt9j2e5lypsq1x6uuebrurwj --dry-run

  # Real delete
  mdemg beta-janitor delete --submission-id dt9j2e5lypsq1x6uuebrurwj --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if submissionID == "" {
				return fmt.Errorf("--submission-id is required")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			return runBetaJanitorDelete(ctx, cmd, submissionID, dryRun, yes)
		},
	}
	cmd.Flags().StringVar(&submissionID, "submission-id", "", "CUIDv2 submission_id (the space_id will be beta-tester-<submission-id>)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Count matching rows without deleting")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

func newBetaJanitorSweepCmd() *cobra.Command {
	var (
		olderThanDays int
		dryRun        bool
		yes           bool
	)
	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "Delete all beta-tester rows older than N days",
		Long: `Bulk-delete guidance_training_rows for ALL beta-tester spaces
where the row's time is older than the specified retention window.

Targets rows matching:
  space_id LIKE 'beta-tester-%' AND time < now() - interval '<N> days'

Safe: never touches the operator's own space or any non-beta-tester space.
Idempotent: no matching rows → no-op, exit 0 with "0 rows deleted".`,
		Example: `  # Standard 30-day sweep (dry-run first)
  mdemg beta-janitor sweep --older-than-days 30 --dry-run
  mdemg beta-janitor sweep --older-than-days 30 --yes

  # Aggressive sweep (delete everything older than 7 days)
  mdemg beta-janitor sweep --older-than-days 7 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if olderThanDays <= 0 {
				return fmt.Errorf("--older-than-days must be positive")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()
			return runBetaJanitorSweep(ctx, cmd, olderThanDays, dryRun, yes)
		},
	}
	cmd.Flags().IntVar(&olderThanDays, "older-than-days", 30, "Delete rows whose time is older than this (days)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Count matching rows without deleting")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

func runBetaJanitorDelete(ctx context.Context, cmd *cobra.Command, submissionID string, dryRun, yes bool) error {
	targetSpace := "beta-tester-" + submissionID
	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Beta Janitor: Delete\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Target space:  %s\n", targetSpace)

	tsdbCfg := tsdbConfigFromEnv()
	client, err := tsdb.NewClient(ctx, tsdbCfg)
	if err != nil {
		return fmt.Errorf("connect to TSDB: %w", err)
	}
	defer client.Close()

	var count int64
	if err := client.Pool().QueryRow(ctx,
		`SELECT count(*) FROM guidance_training_rows WHERE space_id = $1`,
		targetSpace).Scan(&count); err != nil {
		return fmt.Errorf("count: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Matching rows: %d\n", count)

	if count == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(nothing to delete — no rows for that submission_id)")
		return nil
	}
	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no rows deleted; re-run without --dry-run to delete)")
		return nil
	}
	if !yes {
		fmt.Fprintf(cmd.OutOrStdout(), "\nDelete %d rows from space=%q? [y/N]: ", count, targetSpace)
		var ans string
		if _, _ = fmt.Fscanln(cmd.InOrStdin(), &ans); !isYes(ans) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
	}
	tag, err := client.Pool().Exec(ctx,
		`DELETE FROM guidance_training_rows WHERE space_id = $1`, targetSpace)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Deleted %d rows from space=%q\n", tag.RowsAffected(), targetSpace)
	return nil
}

func runBetaJanitorSweep(ctx context.Context, cmd *cobra.Command, olderThanDays int, dryRun, yes bool) error {
	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Beta Janitor: Sweep\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Cutoff:  rows older than %d days\n", olderThanDays)
	fmt.Fprintln(cmd.OutOrStdout(), "Scope:   space_id LIKE 'beta-tester-%' ONLY")

	tsdbCfg := tsdbConfigFromEnv()
	client, err := tsdb.NewClient(ctx, tsdbCfg)
	if err != nil {
		return fmt.Errorf("connect to TSDB: %w", err)
	}
	defer client.Close()

	// Preview counts BEFORE deletion, grouped by space_id, so operator
	// sees exactly which tester-bundles will be pruned.
	countSQL := `SELECT space_id, count(*) FROM guidance_training_rows
	             WHERE space_id LIKE 'beta-tester-%' AND time < now() - ($1 || ' days')::interval
	             GROUP BY space_id ORDER BY 2 DESC`
	rows, err := client.Pool().Query(ctx, countSQL, fmt.Sprintf("%d", olderThanDays))
	if err != nil {
		return fmt.Errorf("preview count: %w", err)
	}
	var total int64
	type spaceCount struct {
		space string
		n     int64
	}
	var seen []spaceCount
	for rows.Next() {
		var sc spaceCount
		if err := rows.Scan(&sc.space, &sc.n); err != nil {
			rows.Close()
			return fmt.Errorf("scan preview: %w", err)
		}
		seen = append(seen, sc)
		total += sc.n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("preview iter: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nTotal matching rows: %d across %d beta-tester spaces\n", total, len(seen))
	for _, sc := range seen {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-45s %d rows\n", sc.space, sc.n)
	}
	if total == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(nothing older than the cutoff — sweep is a no-op)")
		return nil
	}
	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no rows deleted; re-run without --dry-run to delete)")
		return nil
	}
	if !yes {
		fmt.Fprintf(cmd.OutOrStdout(), "\nDelete %d rows across %d spaces? [y/N]: ", total, len(seen))
		var ans string
		if _, _ = fmt.Fscanln(cmd.InOrStdin(), &ans); !isYes(ans) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
	}
	tag, err := client.Pool().Exec(ctx,
		`DELETE FROM guidance_training_rows
		 WHERE space_id LIKE 'beta-tester-%' AND time < now() - ($1 || ' days')::interval`,
		fmt.Sprintf("%d", olderThanDays))
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Deleted %d rows across %d beta-tester spaces\n", tag.RowsAffected(), len(seen))
	return nil
}

func isYes(s string) bool {
	t := ""
	for _, c := range s {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		t += string(c)
	}
	t2 := ""
	for _, c := range t {
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		t2 += string(c)
	}
	return t2 == "y" || t2 == "yes"
}
