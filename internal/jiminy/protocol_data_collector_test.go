package jiminy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProtocolDataCollector_ExpandedRecord(t *testing.T) {
	dir := t.TempDir()
	dc := NewProtocolDataCollector(dir)
	defer dc.Close()

	rec := protocolTrainingRecord{
		ConstraintCode:     "no-force-push",
		ConstraintNodeID:   "node-abc123",
		ConstraintText:     "Never force push to main",
		TierUsed:           1,
		TokenCount:         15,
		AgentAction:        "pushed to dev branch",
		Outcome:            "followed",
		ComprehensionScore: 0.95,
		TrustScore:         0.72,
		SessionID:          "session-1",
		// NS-03: Expanded fields
		ApplicabilityScore: 1.0,
		ScoreSource:        "nli",
		MLTier:             2,
		RuleTier:           1,
		MLConfidence:       0.78,
		TierSource:         "rule",
		SidecarMode:        "compare",
	}

	rec.GuidanceID = "guid-test-001"

	dc.Collect(rec)

	// Wait for async write
	time.Sleep(100 * time.Millisecond)
	dc.Close()

	// Read the JSONL file
	files, err := filepath.Glob(filepath.Join(dir, "protocol-*.jsonl"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one JSONL file")
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var parsed protocolTrainingRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify original fields
	if parsed.ConstraintCode != "no-force-push" {
		t.Errorf("constraint_code = %q, want 'no-force-push'", parsed.ConstraintCode)
	}
	if parsed.TierUsed != 1 {
		t.Errorf("tier_used = %d, want 1", parsed.TierUsed)
	}
	if parsed.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}

	// Verify guidance_id correlation field
	if parsed.GuidanceID != "guid-test-001" {
		t.Errorf("guidance_id = %q, want 'guid-test-001'", parsed.GuidanceID)
	}

	// Verify expanded fields (NS-03)
	if parsed.ApplicabilityScore != 1.0 {
		t.Errorf("applicability_score = %f, want 1.0", parsed.ApplicabilityScore)
	}
	if parsed.ScoreSource != "nli" {
		t.Errorf("score_source = %q, want 'nli'", parsed.ScoreSource)
	}
	if parsed.MLTier != 2 {
		t.Errorf("ml_tier = %d, want 2", parsed.MLTier)
	}
	if parsed.RuleTier != 1 {
		t.Errorf("rule_tier = %d, want 1", parsed.RuleTier)
	}
	if parsed.MLConfidence != 0.78 {
		t.Errorf("ml_confidence = %f, want 0.78", parsed.MLConfidence)
	}
	if parsed.TierSource != "rule" {
		t.Errorf("tier_source = %q, want 'rule'", parsed.TierSource)
	}
	if parsed.SidecarMode != "compare" {
		t.Errorf("sidecar_mode = %q, want 'compare'", parsed.SidecarMode)
	}
}

func TestProtocolDataCollector_ExpandedFieldsOmitEmpty(t *testing.T) {
	dir := t.TempDir()
	dc := NewProtocolDataCollector(dir)
	defer dc.Close()

	// Record with only original fields (expanded fields at zero values)
	rec := protocolTrainingRecord{
		ConstraintCode:     "test-code",
		TierUsed:           1,
		ComprehensionScore: 1.0,
		TrustScore:         0.5,
		SessionID:          "session-1",
	}

	dc.Collect(rec)
	time.Sleep(100 * time.Millisecond)
	dc.Close()

	files, err := filepath.Glob(filepath.Join(dir, "protocol-*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatal("expected JSONL file")
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	// Verify the expanded fields with omitempty are absent from JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Fields with omitempty and zero values should be absent
	for _, field := range []string{"applicability_score", "score_source", "ml_tier", "rule_tier", "ml_confidence", "tier_source", "sidecar_mode"} {
		if _, exists := raw[field]; exists {
			t.Errorf("expected field %q to be omitted (zero value), but it was present", field)
		}
	}
}

func TestProtocolDataCollector_RotationTriggersOnSize(t *testing.T) {
	dir := t.TempDir()
	dc := NewProtocolDataCollector(dir)
	// Set maxFileSize to 50 — each record is ~300+ bytes, so rotation triggers after first write.
	// Note: filename uses second-precision timestamps, so multiple rotations in <1s
	// reopen the same file with O_APPEND. This test verifies the rotation logic runs
	// (dc.written resets and dc.file is re-opened), not that unique filenames are created.
	dc.maxFileSize = 50
	defer dc.Close()

	rec := protocolTrainingRecord{
		ConstraintCode: "test-code",
		ConstraintText: "Long constraint text for rotation",
		TierUsed:       1,
		SessionID:      "session-1",
	}

	// First write: creates file, writes, dc.written > 50
	dc.writeRecord(rec)
	if dc.written == 0 {
		t.Error("expected dc.written > 0 after first write")
	}
	firstWritten := dc.written

	// Second write: should detect dc.written >= maxFileSize, close + reopen, reset dc.written
	dc.writeRecord(rec)

	// After rotation, dc.written should be less than firstWritten (reset + one new record)
	if dc.written >= firstWritten+firstWritten {
		t.Errorf("expected rotation to reset written counter, got written=%d (firstWritten=%d)", dc.written, firstWritten)
	}

	dc.Close()

	// Verify at least one file was created
	files, err := filepath.Glob(filepath.Join(dir, "protocol-*.jsonl"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected at least one JSONL file")
	}
}
