// Package cli — `mdemg beta-share` opt-in training-data submission CLI.
//
// Beta pipeline Sprint C: wraps `mdemg data export` for beta testers who
// have opted-in to sharing their training data. Adds a submission receipt
// with a CUIDv2 submission_id, a 30-day retention notice, and clear
// deletion-request instructions. Produces a single tar.gz local file that
// the tester attaches to a GitHub issue — no infra required.
//
// PRIVACY CONTRACT:
//   - Opt-in per-run (interactive prompt OR --yes flag).
//   - Wraps the shipped TSDB privacy-scrub gate (`internal/tsdb`'s
//     RunExport). If any PII survives the scan the underlying export
//     BLOCKS and no archive is produced.
//   - Bundle includes a plaintext receipt naming the retention window +
//     deletion-request address. Retention is on the maintainer's side
//     (bundles kept ≤30 days from receipt); the tester keeps their local
//     copy as long as they like — the "30-day" period is the maintainer's
//     handling commitment, not a self-destruct on the tester's file.
package cli

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	cuid2 "github.com/nrednav/cuid2"
	"github.com/spf13/cobra"

	"mdemg/internal/tsdb"
)

const betaSubmissionReceiptSchemaVersion = "1"

// newBetaShareCmd is the parent `mdemg beta-share` command. Kept as a
// single leaf-verb command (no subcommands) — the operator experience is
// "one command, one bundle, one submission_id."
func newBetaShareCmd() *cobra.Command {
	var (
		outPath     string
		sinceDays   int
		yes         bool
		noEmbedding bool
		dryRun      bool
	)
	cmd := &cobra.Command{
		Use:   "beta-share",
		Short: "Opt-in: bundle recent training data for the beta maintainer",
		Long: `Package recent LLM interaction + retrieval + embedding events into a
tar.gz you can attach to a GitHub issue. Helps the maintainer diagnose
issues you're hitting AND contributes to MDEMG training if you consent.

WHAT IT DOES
  - Confirms opt-in (interactive; skippable with --yes for scripts)
  - Runs the shipped TSDB export (same privacy scrubber as prod)
  - Adds a submission_receipt.json with a unique submission_id
  - Adds a README-BETA.md explaining what's inside + retention policy
  - Bundles everything as one tar.gz for GitHub issue attachment

RETENTION & DELETION
  The maintainer retains submitted bundles for 30 days from receipt.
  To request earlier deletion, email rogerhenley345@gmail.com with the
  submission_id printed by this command.

PRIVACY
  Every text field passes through the same privacy scrubber that gates
  production exports. If any PII survives the scan the export BLOCKS
  and no archive is produced. Review the bundle contents before uploading
  if you're unsure — run with --dry-run first to see row counts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
			}

			// Opt-in gate — interactive UNLESS --yes was passed.
			if !yes && !dryRun {
				if !promptOptIn(cmd.OutOrStdout(), cmd.InOrStdin(), spaceID, sinceDays) {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled — no data was exported.")
					return nil
				}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
			defer cancel()

			// Underlying export config — beta defaults favour a small,
			// recent window (30 days) and skip embeddings (they inflate
			// bundle size dramatically without helping most bug triage).
			// The operator can override via flags.
			now := time.Now().UTC()
			from := now.AddDate(0, 0, -sinceDays)

			tables := []string{"llm_interactions", "retrieval_events"}
			if !noEmbedding {
				tables = append(tables, "embedding_events")
			}

			instanceID := resolveInstanceID("", spaceID)

			// Dry-run: show row counts, exit — do NOT produce a bundle.
			if dryRun {
				tsdbCfg := tsdbConfigFromEnv()
				client, err := tsdb.NewClient(ctx, tsdbCfg)
				if err != nil {
					return fmt.Errorf("connect to TSDB: %w", err)
				}
				defer client.Close()
				cfg := tsdb.ExportConfig{
					SpaceID:    spaceID,
					From:       from,
					To:         now,
					Tables:     tables,
					InstanceID: instanceID,
				}
				counts, err := tsdb.DryRunExport(ctx, client.Pool(), cfg)
				if err != nil {
					return fmt.Errorf("dry run: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "beta-share dry-run — would export:\n")
				var total int64
				for t, c := range counts {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-25s %d rows\n", t, c)
					total += c
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-25s %d rows\n", "TOTAL", total)
				fmt.Fprintf(cmd.OutOrStdout(), "\nRe-run without --dry-run (and confirm at the prompt) to produce a bundle.\n")
				return nil
			}

			// Resolve bundle output path. The default lives under
			// ~/.mdemg/beta-share/ so testers can find and clean up
			// old submissions.
			if outPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home: %w", err)
				}
				dir := filepath.Join(home, ".mdemg", "beta-share")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("mkdir %s: %w", dir, err)
				}
				stamp := now.Format("20060102-150405")
				outPath = filepath.Join(dir, fmt.Sprintf("mdemg-beta-share-%s.tar.gz", stamp))
			}

			// Step 1: run the shipped export into a scratch inner archive.
			// The inner archive is what `mdemg data export` produces
			// (UTDS-compliant + manifest); we then re-bundle it with the
			// receipt + README into the outer tar.gz.
			tmpDir, err := os.MkdirTemp("", "mdemg-beta-share-*")
			if err != nil {
				return fmt.Errorf("mktemp: %w", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			innerPath := filepath.Join(tmpDir, "utds-export.tar.gz")

			cfg := tsdb.ExportConfig{
				SpaceID:    spaceID,
				From:       from,
				To:         now,
				Tables:     tables,
				InstanceID: instanceID,
				OutputPath: innerPath,
			}

			tsdbCfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, tsdbCfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()

			fmt.Fprintf(cmd.OutOrStdout(), "Running privacy-scrubbed export (space=%s, window=%dd)...\n", spaceID, sinceDays)
			result, err := tsdb.RunExport(ctx, client.Pool(), cfg, Version, Commit)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}

			// Step 2: mint a submission_id + receipt + README.
			submissionID := cuid2.Generate()
			receipt := betaSubmissionReceipt{
				SchemaVersion:  betaSubmissionReceiptSchemaVersion,
				SubmissionID:   submissionID,
				ProducedAt:     now.Format(time.RFC3339),
				MDEMGVersion:   Version,
				SpaceID:        spaceID,
				InstanceID:     instanceID,
				WindowFromRFC:  from.Format(time.RFC3339),
				WindowToRFC:    now.Format(time.RFC3339),
				WindowDays:     sinceDays,
				Tables:         tables,
				TotalRows:      result.TotalRows,
				PrivacyScrub:   result.Manifest.Quality.PrivacyScrubViolations,
				RetentionDays:  30,
				DeletionEmail:  "rogerhenley345@gmail.com",
				DeletionSubject: fmt.Sprintf("MDEMG beta-share deletion request: %s", submissionID),
			}
			receiptJSON, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal receipt: %w", err)
			}

			readme := renderBetaShareReadme(receipt, filepath.Base(innerPath))

			// Step 3: assemble the outer tar.gz.
			if err := writeOuterBundle(outPath, innerPath, receiptJSON, readme); err != nil {
				return fmt.Errorf("write outer bundle: %w", err)
			}

			// Receipt is the operator-visible "you did the thing" surface.
			// Print submission_id LOUDLY so it's easy to copy into the GH
			// issue AND easy to reference in a deletion request later.
			fmt.Fprintf(cmd.OutOrStdout(), "\nBeta-share bundle created:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  Path:          %s\n", outPath)
			fmt.Fprintf(cmd.OutOrStdout(), "  Submission ID: %s\n", submissionID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Rows exported: %d (across %d tables)\n", result.TotalRows, len(tables))
			fmt.Fprintf(cmd.OutOrStdout(), "  Window:        %dd (%s → %s)\n", sinceDays, from.Format("2006-01-02"), now.Format("2006-01-02"))
			fmt.Fprintf(cmd.OutOrStdout(), "  Privacy scrub: %d violations blocked\n\n", result.Manifest.Quality.PrivacyScrubViolations)
			fmt.Fprintf(cmd.OutOrStdout(), "NEXT STEPS\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  1. Attach %s to a GitHub issue at:\n", filepath.Base(outPath))
			fmt.Fprintf(cmd.OutOrStdout(), "       https://github.com/reh3376/mdemg/issues/new/choose\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  2. Paste the Submission ID in the issue body so the maintainer\n")
			fmt.Fprintf(cmd.OutOrStdout(), "     can correlate it.\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  3. To request deletion later, email rogerhenley345@gmail.com with:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "       Subject: %s\n", receipt.DeletionSubject)
			fmt.Fprintf(cmd.OutOrStdout(), "\nSee README-BETA.md inside the bundle for the full retention policy.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Bundle output path (default: ~/.mdemg/beta-share/mdemg-beta-share-<ts>.tar.gz)")
	cmd.Flags().IntVar(&sinceDays, "since-days", 30, "Include events from the last N days")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive opt-in prompt (assume yes)")
	cmd.Flags().BoolVar(&noEmbedding, "no-embedding", true, "Exclude embedding_events (default true — reduces bundle size dramatically; set --no-embedding=false to include)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show row counts without producing a bundle")
	return cmd
}

// betaSubmissionReceipt is the JSON receipt written into every bundle AND
// printed to stdout by summary. Consumers: the maintainer's triage script
// keys on submission_id to correlate an issue attachment with a deletion
// request.
type betaSubmissionReceipt struct {
	SchemaVersion   string   `json:"schema_version"`
	SubmissionID    string   `json:"submission_id"`
	ProducedAt      string   `json:"produced_at"`
	MDEMGVersion    string   `json:"mdemg_version"`
	SpaceID         string   `json:"space_id"`
	InstanceID      string   `json:"instance_id"`
	WindowFromRFC   string   `json:"window_from_rfc3339"`
	WindowToRFC     string   `json:"window_to_rfc3339"`
	WindowDays      int      `json:"window_days"`
	Tables          []string `json:"tables"`
	TotalRows       int64    `json:"total_rows"`
	PrivacyScrub    int      `json:"privacy_scrub_violations_blocked"`
	RetentionDays   int      `json:"retention_days_maintainer_side"`
	DeletionEmail   string   `json:"deletion_request_email"`
	DeletionSubject string   `json:"deletion_request_subject_line"`
}

// promptOptIn is the interactive opt-in gate. Prints WHAT will be
// exported + retention policy, then waits for a `y`/`yes` on stdin.
// Any other answer (including empty line) is treated as "no."
func promptOptIn(out io.Writer, in io.Reader, spaceID string, sinceDays int) bool {
	fmt.Fprintf(out, "\nMDEMG Beta Share — Opt-in confirmation\n")
	fmt.Fprintf(out, "======================================\n\n")
	fmt.Fprintf(out, "This will bundle the following for the beta maintainer:\n")
	fmt.Fprintf(out, "  - LLM interaction events (last %d days, space_id=%s)\n", sinceDays, spaceID)
	fmt.Fprintf(out, "  - Retrieval events (same window)\n")
	fmt.Fprintf(out, "  - Optional embedding events (excluded by default; toggled via --no-embedding=false)\n\n")
	fmt.Fprintf(out, "Privacy:\n")
	fmt.Fprintf(out, "  - The shipped TSDB privacy scrubber runs first — if any PII is\n")
	fmt.Fprintf(out, "    detected in a text field, the export BLOCKS and no bundle is\n")
	fmt.Fprintf(out, "    produced.\n\n")
	fmt.Fprintf(out, "Retention:\n")
	fmt.Fprintf(out, "  - The maintainer keeps received bundles for 30 days from receipt.\n")
	fmt.Fprintf(out, "  - You can request earlier deletion by emailing rogerhenley345@gmail.com\n")
	fmt.Fprintf(out, "    with the submission_id this command will print.\n\n")
	fmt.Fprintf(out, "Proceed with export? [y/N]: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return ans == "y" || ans == "yes"
}

// renderBetaShareReadme returns the human-readable README bundled inside
// the tar.gz. Keeps every operator-visible policy statement in one place
// (JSON receipt has structured fields; README has the plain-English
// version).
func renderBetaShareReadme(r betaSubmissionReceipt, innerName string) []byte {
	var b strings.Builder
	b.WriteString("# MDEMG Beta Share Bundle\n\n")
	fmt.Fprintf(&b, "**Submission ID:** `%s`\n\n", r.SubmissionID)
	fmt.Fprintf(&b, "**Produced:** %s  \n", r.ProducedAt)
	fmt.Fprintf(&b, "**MDEMG version:** %s  \n", r.MDEMGVersion)
	fmt.Fprintf(&b, "**Window:** %d days (%s → %s)  \n", r.WindowDays, r.WindowFromRFC, r.WindowToRFC)
	fmt.Fprintf(&b, "**Rows:** %d (across %d tables)  \n\n", r.TotalRows, len(r.Tables))

	b.WriteString("## What's inside\n\n")
	b.WriteString("| File | Purpose |\n|------|---------|\n")
	b.WriteString("| `README-BETA.md` | This file |\n")
	b.WriteString("| `submission_receipt.json` | Structured receipt with submission_id, retention policy, deletion contact |\n")
	fmt.Fprintf(&b, "| `%s` | The UTDS-compliant training-data export (has its own manifest.json inside) |\n\n", innerName)

	b.WriteString("## Privacy\n\n")
	b.WriteString("Every text field in the UTDS export passed through the shipped\n")
	b.WriteString("MDEMG privacy scrubber before archiving. If ANY PII had been detected\n")
	b.WriteString("in a text field, the export would have BLOCKED and this bundle\n")
	fmt.Fprintf(&b, "would not exist. Scrub violations blocked: **%d**.\n\n", r.PrivacyScrub)

	b.WriteString("## Retention & deletion\n\n")
	fmt.Fprintf(&b, "The maintainer keeps received bundles for **%d days from receipt**.\n\n", r.RetentionDays)
	b.WriteString("To request deletion earlier than that:\n\n")
	fmt.Fprintf(&b, "  Email: **%s**  \n", r.DeletionEmail)
	fmt.Fprintf(&b, "  Subject: `%s`\n\n", r.DeletionSubject)
	b.WriteString("The submission_id in the subject line lets the maintainer locate\n")
	b.WriteString("and delete the correct submission without asking you follow-up\n")
	b.WriteString("questions.\n\n")

	b.WriteString("## How to submit\n\n")
	b.WriteString("1. Open https://github.com/reh3376/mdemg/issues/new/choose\n")
	b.WriteString("2. Pick the template matching your situation (bug report / install report / feature friction).\n")
	b.WriteString("3. Drag this `.tar.gz` file into the issue body.\n")
	fmt.Fprintf(&b, "4. Paste the submission_id (`%s`) in the issue body so the maintainer can correlate it.\n\n", r.SubmissionID)

	b.WriteString("Thank you for helping make MDEMG better.\n")
	return []byte(b.String())
}

// writeOuterBundle assembles the final tar.gz containing README-BETA.md,
// submission_receipt.json, and the inner UTDS archive as-is.
func writeOuterBundle(outPath, innerPath string, receiptJSON, readme []byte) (err error) {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	gz := gzip.NewWriter(f)
	defer func() {
		if cerr := gz.Close(); err == nil {
			err = cerr
		}
	}()
	tw := tar.NewWriter(gz)
	defer func() {
		if cerr := tw.Close(); err == nil {
			err = cerr
		}
	}()

	if err := writeTarEntry(tw, "README-BETA.md", readme); err != nil {
		return fmt.Errorf("write README: %w", err)
	}
	if err := writeTarEntry(tw, "submission_receipt.json", receiptJSON); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}

	// Stream the inner archive as-is (it's already gzipped; the outer
	// gzip layer wraps but doesn't recompress — tarballs nested in a
	// tarball are the canonical way to bundle a submission with metadata).
	innerData, err := os.ReadFile(innerPath) //nolint:gosec // G304: path is server-generated tmpfile
	if err != nil {
		return fmt.Errorf("read inner: %w", err)
	}
	if err := writeTarEntry(tw, filepath.Base(innerPath), innerData); err != nil {
		return fmt.Errorf("write inner: %w", err)
	}
	return nil
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
