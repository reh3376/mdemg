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
		{"llm_interactions", 26},
		{"retrieval_events", 22},
		{"embedding_events", 23},
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
	}

	for _, tt := range tests {
		t.Run(tt.table+"_"+tt.colName, func(t *testing.T) {
			spec := tableSpecs[tt.table]
			if !spec.textFields[tt.idx] {
				t.Errorf("column %d (%s) should be a text field in %s", tt.idx, tt.colName, tt.table)
			}
			if spec.columns[tt.idx] != tt.colName {
				t.Errorf("column %d should be %s but is %s", tt.idx, tt.colName, spec.columns[tt.idx])
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
		SchemaVersion: 7,
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
	if manifest.SchemaVersion < 7 {
		t.Errorf("schema version must be >= 7, got: %d", manifest.SchemaVersion)
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
