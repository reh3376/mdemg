// Package cli — `mdemg beta-import` receiver CLI.
//
// Sprint B5b (2026-08-10). Consumes an mdemg-beta-share bundle
// (produced by internal/cli/beta_share.go, shipped in Sprint C of
// the beta pipeline) and imports its guidance corpus rows into the
// LOCAL guidance_training_rows TSDB table with per-tester space
// attribution.
//
// Bundle structure (double-tar-gz):
//   outer.tar.gz
//     ├── README-BETA.md                    (human-readable policy)
//     ├── submission_receipt.json           (structured receipt: sid, etc.)
//     └── utds-export.tar.gz                (streamed inner archive)
//          ├── manifest.json                (ExportManifest — schema 9 for B5)
//          ├── llm_interactions.jsonl       (telemetry, not imported by B5b)
//          ├── retrieval_events.jsonl       (telemetry, not imported by B5b)
//          ├── embedding_events.jsonl       (telemetry, not imported by B5b)
//          └── guidance_training_rows.jsonl (B5a-added — B5b imports THIS)
//
// Trust model:
//   - Operator manually invokes `mdemg beta-import <bundle>` against a
//     bundle they downloaded from a GH issue attachment. Same trust
//     level as `mdemg plugin validate <path>` (arbitrary local exec).
//   - Two defense-in-depth gates run BEFORE any DB write:
//       1. Per-JSONL SHA-256 vs manifest.tables[t].sha256 — proves the
//          JSONL wasn't modified in transit.
//       2. Privacy re-scrub over every text field — proves the sender's
//          exporter didn't have its scrubber bypassed (a hand-crafted
//          bundle could pass SHA-256 while carrying real PII).
//   - Any gate failure REJECTS the bundle before touching the DB.
//
// Attribution:
//   - Every imported row's `space_id` + `instance_id` is remapped to
//     "beta-tester-<submission_id>" (a CUIDv2-suffixed synthetic space).
//     The tester's original space_id (which might be "mdemg-dev" on their
//     side!) is DROPPED — this prevents corpus pollution of the
//     maintainer's own space and gives the janitor a keying prefix for
//     30-day retention sweep.
//   - Row `time` preserved (so time-window deletion works).
//   - Row `row_id` is RE-MINTED via cuid2.Generate() — the exporter
//     dropped the field to avoid (time, row_id) PK collisions across
//     independently-produced bundles.
package cli

import (
	"archive/tar"
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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"mdemg/internal/tsdb"
)

// newBetaImportCmd is `mdemg beta-import <bundle.tar.gz>`.
func newBetaImportCmd() *cobra.Command {
	var (
		dryRun       bool
		yes          bool
		spaceSuffix  string
		bundleGate   bool // internal: skip interactive opt-in when true (used by --yes flow)
	)
	_ = bundleGate // reserved for future automation
	cmd := &cobra.Command{
		Use:   "beta-import <bundle.tar.gz>",
		Short: "Import a beta tester's mdemg-beta-share bundle into the local guidance corpus",
		Args:  cobra.ExactArgs(1),
		Long: `Import a beta tester's mdemg-beta-share bundle into the local
guidance_training_rows TSDB table with per-tester space attribution.

Two integrity gates run BEFORE any DB write:
  1. Per-JSONL SHA-256 matches the bundle's manifest.
  2. Privacy re-scrub over every text field yields zero violations.

Any gate failure REJECTS the bundle. If both pass, rows are imported
to space_id "beta-tester-<submission_id>" (a synthetic space so tester
data cannot collide with the operator's own; also gives the janitor a
prefix for 30-day retention sweep).

Requires the bundle's manifest.schema_version >= 9 (B5a). Older bundles
produced by beta.1/beta.2/beta.3 don't carry the guidance_training_rows
JSONL — they're gracefully skipped with a "telemetry-only lite mode"
notice.

Deletion:
  mdemg beta janitor delete --submission-id <sid>
  mdemg beta janitor sweep  --older-than-days 30`,
		Example: `  # Preview only (no DB write)
  mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz --dry-run

  # Real import, interactive opt-in
  mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz

  # Script-friendly (skip prompt)
  mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz --yes

  # Force a specific synthetic-space suffix (repeat imports of same bundle)
  mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz --space-suffix custom-tag --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bundlePath := args[0]
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			return runBetaImport(ctx, cmd, bundlePath, betaImportOpts{
				dryRun:      dryRun,
				yes:         yes,
				spaceSuffix: spaceSuffix,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify the bundle + report row counts without writing anything")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive opt-in prompt (script-friendly)")
	cmd.Flags().StringVar(&spaceSuffix, "space-suffix", "", "Override the space suffix (default: submission_id from the receipt)")
	return cmd
}

type betaImportOpts struct {
	dryRun      bool
	yes         bool
	spaceSuffix string
}

type betaImportReceipt struct {
	SchemaVersion string `json:"schema_version"`
	SubmissionID  string `json:"submission_id"`
	ProducedAt    string `json:"produced_at"`
	MDEMGVersion  string `json:"mdemg_version"`
	SpaceID       string `json:"space_id"`
	InstanceID    string `json:"instance_id"`
	TotalRows     int64  `json:"total_rows"`
}

func runBetaImport(ctx context.Context, cmd *cobra.Command, bundlePath string, opt betaImportOpts) error {
	fmt.Fprintln(cmd.OutOrStdout(), "MDEMG Beta Bundle Import")
	fmt.Fprintln(cmd.OutOrStdout(), "========================")
	fmt.Fprintf(cmd.OutOrStdout(), "Bundle:         %s\n", bundlePath)

	// STEP 1 — Unpack outer bundle to tmpdir.
	tmpDir, err := os.MkdirTemp("", "mdemg-beta-import-*")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractTarGz(bundlePath, tmpDir); err != nil {
		return fmt.Errorf("extract outer bundle: %w", err)
	}

	// Verify the outer bundle's expected shape.
	readmePath := filepath.Join(tmpDir, "README-BETA.md")
	receiptPath := filepath.Join(tmpDir, "submission_receipt.json")
	innerPath := filepath.Join(tmpDir, "utds-export.tar.gz")
	for _, p := range []string{readmePath, receiptPath, innerPath} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("bundle missing expected file %s: %w", filepath.Base(p), err)
		}
	}

	// STEP 2 — Parse receipt.
	var receipt betaImportReceipt
	if data, err := os.ReadFile(receiptPath); err != nil {
		return fmt.Errorf("read receipt: %w", err)
	} else if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("parse receipt: %w", err)
	}
	if receipt.SubmissionID == "" {
		return fmt.Errorf("receipt missing submission_id (bundle malformed)")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Submission ID:  %s\n", receipt.SubmissionID)
	fmt.Fprintf(cmd.OutOrStdout(), "Produced at:    %s\n", receipt.ProducedAt)
	fmt.Fprintf(cmd.OutOrStdout(), "Sender version: %s\n", receipt.MDEMGVersion)

	// STEP 3 — Unpack inner archive to a sub-tmpdir.
	innerDir := filepath.Join(tmpDir, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		return fmt.Errorf("mkdir inner: %w", err)
	}
	if err := extractTarGz(innerPath, innerDir); err != nil {
		return fmt.Errorf("extract inner archive: %w", err)
	}

	// STEP 4 — Read + validate manifest.
	manifestPath := filepath.Join(innerDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest tsdb.ExportManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.UTDSVersion != "1.0.0" {
		return fmt.Errorf("unsupported utds_version %q (expected 1.0.0)", manifest.UTDSVersion)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "UTDS version:   %s\n", manifest.UTDSVersion)
	fmt.Fprintf(cmd.OutOrStdout(), "Schema version: %d\n", manifest.SchemaVersion)
	fmt.Fprintf(cmd.OutOrStdout(), "Export ID:      %s\n", manifest.ExportID)
	fmt.Fprintf(cmd.OutOrStdout(), "Tables:         %d\n", len(manifest.Tables))

	// STEP 4b — Manifest version gate.
	// Schema < 9 bundles predate B5a's guidance_training_rows projection.
	// They're valid bundles (telemetry-only) — we don't reject, just log
	// and enter lite mode (zero rows imported to the corpus).
	const minCorpusSchema = 9
	liteMode := manifest.SchemaVersion < minCorpusSchema
	if liteMode {
		fmt.Fprintf(cmd.OutOrStdout(), "\n⚠ Lite mode: bundle schema %d < %d (B5a). Bundle predates the guidance_training_rows projection — corpus import will be a no-op.\n\n",
			manifest.SchemaVersion, minCorpusSchema)
	}

	// STEP 5 — SHA-256 verify each JSONL against the manifest.
	// Runs even in lite mode — an operator receiving a schema-8 bundle
	// still wants to know if it's tampered.
	fmt.Fprintln(cmd.OutOrStdout(), "\nVerifying per-JSONL SHA-256...")
	for tableName, tbl := range manifest.Tables {
		if tbl == nil || tbl.SHA256 == "" {
			continue
		}
		jsonlPath := filepath.Join(innerDir, tableName+".jsonl")
		if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
			// Manifest lists a table with row_count > 0 but the JSONL is
			// missing — bundle is malformed (exporter deletes empty JSONLs
			// so we can't reach here on a legitimate zero-row table).
			return fmt.Errorf("manifest lists %s (%d rows) but %s.jsonl not in bundle", tableName, tbl.RowCount, tableName)
		}
		if err := tsdb.VerifyJSONLSHA256(jsonlPath, tbl.SHA256); err != nil {
			return fmt.Errorf("SHA verify: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %-25s %d rows  sha=%s...\n", tableName, tbl.RowCount, tbl.SHA256[:12])
	}

	// STEP 6 — Privacy re-scrub gate on every JSONL that carries text
	// fields (all four tables in the current spec).
	fmt.Fprintln(cmd.OutOrStdout(), "\nRe-running privacy scrubber (defense-in-depth against tampered bundles)...")
	for tableName := range manifest.Tables {
		jsonlPath := filepath.Join(innerDir, tableName+".jsonl")
		if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
			continue
		}
		report, err := tsdb.RescrubJSONL(jsonlPath, tableName)
		if err != nil {
			return fmt.Errorf("rescrub %s: %w", tableName, err)
		}
		if report.Violations > 0 {
			return fmt.Errorf("REJECTED: privacy scrub violations in %s: %d rows affected. First: %s",
				tableName, report.Violations, report.FirstViolation)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %-25s %d rows scanned, 0 violations\n", tableName, report.RowsScanned)
	}

	// STEP 7 — Resolve destination space + confirm.
	suffix := opt.spaceSuffix
	if suffix == "" {
		suffix = receipt.SubmissionID
	}
	destSpace := "beta-tester-" + suffix
	fmt.Fprintf(cmd.OutOrStdout(), "\nDestination space: %s\n", destSpace)

	if liteMode {
		fmt.Fprintln(cmd.OutOrStdout(), "\nLite-mode result: manifest gates PASSED. Zero corpus rows to import.")
		fmt.Fprintln(cmd.OutOrStdout(), "The sender should upgrade to mdemg v0.11.0-beta.3+ to produce schema-9 bundles carrying guidance_training_rows.")
		return nil
	}

	// Determine what we'd write.
	corpusTable := manifest.Tables["guidance_training_rows"]
	if corpusTable == nil || corpusTable.RowCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nManifest schema>=9 but zero guidance rows in this bundle. Nothing to import.")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Would import:  %d guidance_training_rows\n", corpusTable.RowCount)

	if opt.dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no rows written; re-run without --dry-run to import)")
		return nil
	}

	// STEP 8 — Interactive opt-in (skip on --yes).
	if !opt.yes {
		fmt.Fprintf(cmd.OutOrStdout(), "\nProceed with import? [y/N]: ")
		var ans string
		if _, _ = fmt.Fscanln(cmd.InOrStdin(), &ans); strings.TrimSpace(strings.ToLower(ans)) != "y" && strings.TrimSpace(strings.ToLower(ans)) != "yes" {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled — no rows written.")
			return nil
		}
	}

	// STEP 9 — Connect to TSDB + import.
	tsdbCfg := tsdbConfigFromEnv()
	client, err := tsdb.NewClient(ctx, tsdbCfg)
	if err != nil {
		return fmt.Errorf("connect to TSDB: %w", err)
	}
	defer client.Close()

	imported, err := importGuidanceRows(ctx, client.Pool(), filepath.Join(innerDir, "guidance_training_rows.jsonl"), destSpace)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Imported %d rows to space_id=%q\n", imported, destSpace)
	fmt.Fprintln(cmd.OutOrStdout(), "\nTo delete this submission later:")
	fmt.Fprintf(cmd.OutOrStdout(), "  mdemg beta-janitor delete --submission-id %s\n", suffix)
	fmt.Fprintln(cmd.OutOrStdout(), "\nRetention: 30 days per maintainer-side retention promise. Run `mdemg beta-janitor sweep --older-than-days 30` periodically.")
	return nil
}

// importGuidanceRows streams the guidance_training_rows JSONL and
// writes each row to the local table with space_id + instance_id
// remapped to destSpace. Row time is preserved; row_id is re-minted
// per-row via the DB's default (the schema doesn't have a DEFAULT for
// row_id, so we mint one in Go via cuid2 to stay consistent with the
// writer's own contract at internal/tsdb/guidance_training_rows_writer.go:129).
//
// Uses per-row INSERT (not CopyFrom) because we're transforming each
// row and the volume per bundle is bounded by the exporter's --since-days
// window (typically 100s–low-1000s of rows) — well below where per-row
// INSERT becomes the bottleneck.
func importGuidanceRows(ctx context.Context, pool *pgxpool.Pool, jsonlPath, destSpace string) (int, error) {
	f, err := os.Open(jsonlPath) //nolint:gosec // G304: operator-provided path (opened after tar-slip + SHA gates)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", jsonlPath, err)
	}
	defer func() { _ = f.Close() }()

	// row_id is NOT NULL in the schema (migration 027 line 41). The
	// exporter drops it from the projection so we mint fresh CUIDv2 here
	// — matches the writer's stamping pattern.
	insertSQL := `INSERT INTO guidance_training_rows
		(row_id, time, space_id, session_id, instance_id, guidance_id, guidance_type,
		 guidance_content, source_node_id, source_role_type, source_layer,
		 action_summary, outcome_type, similarity, classifier_source, constraint_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	imported := 0
	dec := json.NewDecoder(f)
	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			return imported, fmt.Errorf("decode row %d: %w", imported+1, err)
		}
		args := []any{
			mintRowID(),               // row_id — fresh CUIDv2 per row (avoids (time, row_id) PK collisions across bundles)
			stringField(row, "time"),
			destSpace,                 // space_id — remapped
			stringField(row, "session_id"),
			destSpace,                 // instance_id — remapped
			stringField(row, "guidance_id"),
			stringField(row, "guidance_type"),
			stringField(row, "guidance_content"),
			stringField(row, "source_node_id"),
			stringField(row, "source_role_type"),
			intField(row, "source_layer"),
			stringField(row, "action_summary"),
			stringField(row, "outcome_type"),
			floatField(row, "similarity"),
			stringField(row, "classifier_source"),
			stringField(row, "constraint_code"),
		}
		if _, err := pool.Exec(ctx, insertSQL, args...); err != nil {
			return imported, fmt.Errorf("INSERT row %d (guidance_id=%q): %w", imported+1, args[5], err)
		}
		imported++
	}
	return imported, nil
}

// mintRowID generates a fresh CUIDv2 for guidance_training_rows.row_id.
// Delegated to the shipped cuid2 library — per CLAUDE.md `must-use-cuid2-for-all-identifiers`.
func mintRowID() string {
	return cuid2.Generate()
}

// stringField returns m[key] as string, or "" if missing/nil.
func stringField(m map[string]any, key string) any {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// intField returns m[key] as an int-compatible value for pgx, or nil for NULL.
// source_layer is nullable in the schema, so we return nil (not 0) on absence.
func intField(m map[string]any, key string) any {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return n
	}
	return nil
}

// floatField returns m[key] as float64, or nil for NULL.
func floatField(m map[string]any, key string) any {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return nil
}

// extractTarGz unpacks src (a .tar.gz path) into destDir (which must
// exist). Only regular files at the top level are extracted; nested
// directories aren't expected in UTDS bundles. Aborts on tar-slip
// (a path that would escape destDir).
func extractTarGz(src, destDir string) error {
	f, err := os.Open(src) //nolint:gosec // G304: operator-provided path
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader for %s: %w", src, err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// Reject any path with .. or absolute-path traversal (defense
		// against a malicious bundle overwriting host files).
		name := filepath.Clean(hdr.Name)
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("tar-slip rejected: %q", hdr.Name)
		}
		targetPath := filepath.Join(destDir, name)
		// Belt-and-suspenders: verify the resolved path is within destDir.
		absDest, _ := filepath.Abs(destDir)
		absTarget, _ := filepath.Abs(targetPath)
		if !strings.HasPrefix(absTarget, absDest+string(os.PathSeparator)) && absTarget != absDest {
			return fmt.Errorf("tar-slip rejected (resolved): %q → %q", hdr.Name, absTarget)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", targetPath, err)
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // G304: destination is inside operator-provided tmpDir
			if err != nil {
				return fmt.Errorf("create %s: %w", targetPath, err)
			}
			// #nosec G110 — DoS via gzip bomb is a low-severity risk for a
			// CLI operator explicitly importing a locally-downloaded bundle.
			// The outer manifest's per-JSONL SHA gate would catch content
			// inflation before it lands in the DB.
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write %s: %w", targetPath, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close %s: %w", targetPath, err)
			}
		}
	}
	return nil
}
