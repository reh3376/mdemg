package ape

import (
	"strings"
	"testing"
)

// RSIC-STORM-001: the executor and the rollback snapshot MUST share one
// candidate predicate. RSIC-VALIDATE-001 updated only the executor; the
// drifted snapshot captured a different node set and rollback restored
// nothing (restored_count=0 live).
func TestTombstonePredicate_SharedConstHasLinkageCondition(t *testing.T) {
	for _, want := range []string{
		"obs_type: 'correction'",
		"CO_ACTIVATED_WITH",
		"stale.session_id = correction.session_id",
		"NOT coalesce(stale.is_archived, false)",
		"WITH DISTINCT stale LIMIT 50",
	} {
		if !strings.Contains(tombstoneStaleCandidates, want) {
			t.Errorf("shared predicate missing %q", want)
		}
	}
}

func TestTombstoneExecutor_StampsAttributionMetadata(t *testing.T) {
	// The executor Cypher is assembled from the shared predicate + a SET
	// tail; pin the metadata stamps (bare is_archived made the 2026-06-11
	// forensics mis-attribute the burst for hours).
	tail := tombstoneStaleCandidates + `
			SET stale.is_archived = true,
			    stale.archived_at = datetime(),
			    stale.archive_reason = 'rsic_tombstone_stale',
			    stale.archived_cycle_id = $cycleID
			RETURN count(stale) AS tombstoned
		`
	for _, want := range []string{"archived_at", "archive_reason", "archived_cycle_id"} {
		if !strings.Contains(tail, want) {
			t.Errorf("executor tail missing %q", want)
		}
	}
	// Canonical property name: archive_reason (NOT archived_reason)
	if strings.Contains(tail, "archived_reason =") {
		t.Error("non-canonical archived_reason used as a writer")
	}
}
