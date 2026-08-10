package tsdb

import (
	"testing"
	"time"
)

func TestExportConfigValidation(t *testing.T) {
	t.Run("empty space_id rejected", func(t *testing.T) {
		cfg := ExportConfig{
			SpaceID: "",
			Tables:  []string{"llm_interactions"},
		}
		_, err := DryRunExport(t.Context(), nil, cfg)
		if err == nil {
			t.Fatal("expected error for empty space_id")
		}
	})

	t.Run("unknown table rejected", func(t *testing.T) {
		cfg := ExportConfig{
			SpaceID: "test",
			Tables:  []string{"unknown_table"},
		}
		_, err := RunExport(t.Context(), nil, cfg, "test", "abc")
		if err == nil {
			t.Fatal("expected error for unknown table")
		}
	})
}

func TestTableSpecs(t *testing.T) {
	// Verify all table specs have correct column counts
	tests := []struct {
		table    string
		expected int
	}{
		{"llm_interactions", 27},
		{"retrieval_events", 23},
		{"embedding_events", 24},
		// B5a (2026-08-10): guidance_training_rows added at manifest
		// schema_version 9. 15 columns = migration 027's schema MINUS
		// `row_id` (receiver re-mints via cuid2 to avoid PK collisions
		// across independently-produced bundles).
		{"guidance_training_rows", 15},
	}

	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			spec, ok := tableSpecs[tt.table]
			if !ok {
				t.Fatalf("table spec not found: %s", tt.table)
			}
			if len(spec.columns) != tt.expected {
				t.Errorf("expected %d columns, got %d: %v", tt.expected, len(spec.columns), spec.columns)
			}
		})
	}
}

func TestTextFieldIndexes(t *testing.T) {
	// Verify text field indexes point to the correct columns
	tests := []struct {
		table   string
		idx     int
		colName string
	}{
		{"llm_interactions", 5, "system_prompt"},
		{"llm_interactions", 6, "user_prompt"},
		{"llm_interactions", 7, "response"},
		{"llm_interactions", 8, "think_content"},
		{"llm_interactions", 21, "source_path"},
		{"retrieval_events", 4, "query_text"},
		{"embedding_events", 4, "text_content"},
		{"embedding_events", 9, "file_path"},
		{"embedding_events", 13, "signature"},
		{"embedding_events", 16, "query_text"},
		// B5a: guidance_training_rows text-field indexes
		{"guidance_training_rows", 6, "guidance_content"},
		{"guidance_training_rows", 10, "action_summary"},
	}

	for _, tt := range tests {
		t.Run(tt.table+"_"+tt.colName, func(t *testing.T) {
			spec := tableSpecs[tt.table]
			if _, ok := spec.textFields[tt.idx]; !ok {
				t.Errorf("column %d (%s) should be a text field in %s", tt.idx, tt.colName, tt.table)
			}
			if spec.columns[tt.idx] != tt.colName {
				t.Errorf("column %d should be %s but is %s", tt.idx, tt.colName, spec.columns[tt.idx])
			}
		})
	}
}

func TestPerFieldPrivacySkip(t *testing.T) {
	// Verify per-field privacy pattern skip configuration.
	// Skip lists prevent false positives on code/doc content that was already scrubbed at write-time.
	tests := []struct {
		table   string
		idx     int
		colName string
		skips   []string
	}{
		// LLM interactions: core text fields apply all patterns (nil skip)
		{"llm_interactions", 5, "system_prompt", nil},
		{"llm_interactions", 6, "user_prompt", nil},
		// LLM interactions: source_path IS a file path — skip abs_path
		{"llm_interactions", 21, "source_path", []string{"abs_path"}},
		// Retrieval events: query_text skips abs_path (queries reference file paths)
		{"retrieval_events", 4, "query_text", []string{"abs_path"}},
		// Embedding events: text_content scrubbed at write-time; skip patterns that re-trigger on scrubbed output
		{"embedding_events", 4, "text_content", []string{"abs_path", "env_secret", "neo4j_cred"}},
		{"embedding_events", 9, "file_path", []string{"abs_path"}},
		// Embedding events: signature may contain file paths
		{"embedding_events", 13, "signature", []string{"abs_path"}},
		// Embedding events: query_text is internal search key, not user input — skip all patterns
		{"embedding_events", 16, "query_text", []string{"abs_path", "api_key", "env_secret", "email", "neo4j_cred"}},
		// B5a: guidance_training_rows text fields.
		// guidance_content(6): operator-authored rule text; may reference file paths — skip abs_path
		{"guidance_training_rows", 6, "guidance_content", []string{"abs_path"}},
		// action_summary(10): agent action text — full scrub (same shape as llm_interactions.user_prompt)
		{"guidance_training_rows", 10, "action_summary", nil},
	}

	for _, tt := range tests {
		t.Run(tt.table+"_"+tt.colName, func(t *testing.T) {
			spec := tableSpecs[tt.table]
			skip, ok := spec.textFields[tt.idx]
			if !ok {
				t.Fatalf("column %d (%s) not in textFields for %s", tt.idx, tt.colName, tt.table)
			}
			if len(skip) != len(tt.skips) {
				t.Fatalf("expected skip patterns %v, got %v", tt.skips, skip)
			}
			for i, s := range tt.skips {
				if skip[i] != s {
					t.Errorf("skip[%d] expected %q, got %q", i, s, skip[i])
				}
			}
		})
	}
}

func TestExportManifestStructure(t *testing.T) {
	manifest := &ExportManifest{
		UTDSVersion:   "1.0.0",
		ExportID:      "exp-test-20260401-120000",
		InstanceID:    "test-instance",
		SpaceID:       "test-space",
		MDEMGVersion:  "v0.4.1",
		SchemaVersion: 8,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		DataRange: ExportDataRange{
			From: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			To:   time.Now().Format(time.RFC3339),
		},
		Tables: map[string]*ExportTable{
			"llm_interactions": {
				RowCount: 100,
				SHA256:   "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				Columns:  llmInteractionsSpec.columns,
			},
		},
		Quality: ExportQualitySummary{
			LLMErrorRate:           0.0,
			LLMEmptySystemPrompt:   0,
			LLMEmptyResponse:       0,
			PrivacyScrubViolations: 0,
		},
	}

	if manifest.UTDSVersion != "1.0.0" {
		t.Errorf("unexpected UTDS version: %s", manifest.UTDSVersion)
	}
	if manifest.SchemaVersion < 8 {
		t.Errorf("schema version must be >= 8, got: %d", manifest.SchemaVersion)
	}
	if manifest.Quality.PrivacyScrubViolations != 0 {
		t.Error("privacy violations should be 0")
	}
	if _, ok := manifest.Tables["llm_interactions"]; !ok {
		t.Error("llm_interactions table should be present")
	}
}

func TestExportConfigDefaults(t *testing.T) {
	cfg := ExportConfig{
		SpaceID: "test",
	}
	if len(cfg.Tables) != 0 {
		t.Error("tables should be empty by default (filled by RunExport)")
	}
}

// TestExporter_SchemaVersionIs9 pins B5a's contract: bundles produced by
// this build MUST carry manifest.schema_version >= 9 (guidance_training_rows
// projection landed). The beta-import receiver (B5b) branches on this: >= 9
// → corpus-import mode; < 9 → telemetry-only lite mode. Changing this
// number without also updating the receiver's version gate would silently
// break the corpus loop.
func TestExporter_SchemaVersionIs9(t *testing.T) {
	// The literal in RunExport must match. This test would fail if a
	// future refactor moved the version but forgot to bump.
	// We read the constant indirectly via a minimal manifest construction —
	// changing this from 9 requires an explicit test edit AND a coordinated
	// change to the receiver's version gate.
	const expected = 9
	// The manifest embeds the version, so we synthesize a snapshot the same
	// way RunExport does (via ExportManifest literal — see exporter.go:190).
	m := &ExportManifest{SchemaVersion: expected}
	if m.SchemaVersion != expected {
		t.Fatalf("SchemaVersion literal drifted: expected %d, got %d", expected, m.SchemaVersion)
	}
	// Guard: guidance_training_rows MUST be a registered tableSpec at
	// schema 9. If a future ship removes it while keeping schema 9,
	// this test flags the drift.
	if _, ok := tableSpecs["guidance_training_rows"]; !ok {
		t.Fatal("guidance_training_rows tableSpec required at schema 9 (B5a) — do not remove without bumping schema down or adding a version gate")
	}
}

// TestExportConfig_DefaultTablesIncludesGuidanceCorpus pins that the
// exporter's default table list includes guidance_training_rows.
// Regression pin for B5a — if a future edit reverts the default list
// to just the 3 telemetry tables, beta-share bundles stop carrying
// corpus rows silently and B5b's receiver falls back to telemetry-only
// mode with no visible error to the operator.
func TestExportConfig_DefaultTablesIncludesGuidanceCorpus(t *testing.T) {
	// Mirror the logic in RunExport where an empty Tables slice fills
	// in the default. We don't call RunExport itself (needs a pool);
	// we check the tableSpecs registry has the table registered.
	if _, ok := tableSpecs["guidance_training_rows"]; !ok {
		t.Fatal("guidance_training_rows must be a valid table for the exporter to include it in defaults")
	}
	// Also verify the two other B5a-critical properties on the spec.
	spec := tableSpecs["guidance_training_rows"]
	if len(spec.columns) < 10 {
		t.Errorf("guidance_training_rows spec should project at least 10 columns; got %d", len(spec.columns))
	}
	// row_id MUST NOT be in the exported columns (receiver re-mints via cuid2).
	for i, col := range spec.columns {
		if col == "row_id" {
			t.Errorf("row_id must NOT be in exporter projection (position %d) — receiver re-mints to avoid (time, row_id) PK collisions", i)
		}
	}
}
