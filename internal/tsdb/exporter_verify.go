// Package tsdb — B5b (2026-08-10): shared verify helpers for UTDS
// bundles. Extracted from the exporter's re-scrub + SHA logic so the
// receiver (mdemg beta-import) can enforce the same contracts on load.
//
// The two contracts:
//   1. Per-JSONL SHA-256 matches manifest.tables[t].sha256 — proves the
//      JSONL file wasn't modified between export and import.
//   2. Every text-field value re-scrubs to itself — proves the privacy
//      scrubber wasn't bypassed at export time (defense-in-depth: the
//      exporter blocks on violations, but a hand-edited bundle could
//      have both a tampered JSONL AND a matching manifest.sha256 —
//      this catches privacy leaks that a SHA check alone would miss).
//
// Both contracts are enforced at LOAD time by the receiver. Neither
// requires DB access, so this file has no pgx dependency.

package tsdb

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"mdemg/internal/llmclient"
)

// VerifyJSONLSHA256 recomputes the SHA-256 of the given JSONL file
// (streamed, no full-file load) and compares to expected. Returns nil
// on match, an error naming both digests on mismatch. Used by the
// receiver's manifest-integrity check.
//
// The exporter's per-JSONL hash is computed at write-time via
// io.MultiWriter(bufio, sha256.New()) (see exportTable). This function
// mirrors that shape on the read side.
func VerifyJSONLSHA256(path, expectedHex string) error {
	f, err := os.Open(path) //nolint:gosec // G304: path is caller-provided (operator CLI), same trust model as validator.go
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, bufio.NewReaderSize(f, 64*1024)); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if got != expectedHex {
		return fmt.Errorf("SHA-256 mismatch on %s: manifest=%s actual=%s", path, expectedHex, got)
	}
	return nil
}

// RescrubReport summarizes a privacy re-scrub pass against a JSONL
// file. Zero violations = the bundle's privacy scrub survived transit
// intact; any violations = the bundle should be REJECTED (either
// tampered or produced by a broken/absent scrubber).
type RescrubReport struct {
	Table          string
	RowsScanned    int64
	Violations     int
	FirstViolation string // one-line description of the first-encountered violation, for operator triage
}

// RescrubJSONL streams a JSONL file, decodes each row as a map, and
// re-runs the privacy scrubber over every text-field column defined in
// the table's spec. Returns a report; caller decides how to react
// (typically: any Violations > 0 → REJECT the bundle).
//
// The spec's per-field skipPatterns list is honored (mirrors the
// exporter's export-time scan). If the table isn't in tableSpecs, the
// function returns a report with 0 rows scanned + 0 violations —
// caller should reject unknown tables upstream (already done via the
// manifest.tables[] iteration).
func RescrubJSONL(path, tableName string) (RescrubReport, error) {
	report := RescrubReport{Table: tableName}
	spec, ok := tableSpecs[tableName]
	if !ok {
		return report, fmt.Errorf("unknown table: %s", tableName)
	}
	f, err := os.Open(path) //nolint:gosec // G304: path is caller-provided (operator CLI)
	if err != nil {
		return report, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// Column-name → column-index reverse lookup so the JSONL row (which
	// is a map keyed by column name, per exportTable's json.Marshal) can
	// be checked against textFields (which is keyed by index).
	colByName := make(map[string]int, len(spec.columns))
	for i, c := range spec.columns {
		colByName[c] = i
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // 8MB max line — some LLM prompts approach this
	for scanner.Scan() {
		report.RowsScanned++
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return report, fmt.Errorf("%s row %d: JSON decode failed: %w", tableName, report.RowsScanned, err)
		}
		for colName, idx := range colByName {
			skipPatterns, isTextField := spec.textFields[idx]
			if !isTextField {
				continue
			}
			val, ok := row[colName]
			if !ok || val == nil {
				continue
			}
			str, ok := val.(string)
			if !ok || str == "" {
				continue
			}
			scrubbed := llmclient.ScrubStringExcluding(str, skipPatterns)
			if scrubbed != str {
				report.Violations++
				if report.FirstViolation == "" {
					report.FirstViolation = fmt.Sprintf("%s row %d column %s: value differs after re-scrub", tableName, report.RowsScanned, colName)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return report, fmt.Errorf("scan %s: %w", path, err)
	}
	return report, nil
}

// TableColumns returns the ordered column list for a known table.
// Exposed so the receiver can build INSERT column lists without
// duplicating the exporter's tableSpecs registry. Returns nil for
// unknown tables (caller should have rejected upstream).
func TableColumns(tableName string) []string {
	spec, ok := tableSpecs[tableName]
	if !ok {
		return nil
	}
	// Return a defensive copy — callers should not mutate the registry.
	out := make([]string, len(spec.columns))
	copy(out, spec.columns)
	return out
}
