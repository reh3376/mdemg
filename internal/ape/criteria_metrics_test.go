package ape

import "testing"

func TestReportMetricsMap_CoversCriteriaKeys(t *testing.T) {
	r := &SelfAssessmentReport{EdgeCount: 10, EdgesBelowThreshold: 3, ConsolidationAgeSec: 120, AvgEdgeWeight: 0.4, GuidanceHealth: 0.7}
	m := reportMetricsMap(r)
	// The keys task_spec criteria reference (post _delta strip) that the
	// report can resolve — the old inline maps missed most of these.
	for _, k := range []string{"edges_below_threshold", "total_edges", "consolidation_age_sec",
		"volatile_count", "correction_rate", "avg_edge_weight", "guidance_health"} {
		if _, ok := m[k]; !ok {
			t.Errorf("reportMetricsMap missing criteria key %q", k)
		}
	}
	if m["total_edges"] != 10 {
		t.Errorf("total_edges alias = %v, want 10", m["total_edges"])
	}
	if reportMetricsMap(nil) != nil {
		t.Errorf("nil report must yield nil map")
	}
}

func TestIsMutatingAction(t *testing.T) {
	for _, a := range []string{"tombstone_stale", "refresh_stale_edges", "prune_decayed_edges", "adjust_guidance_confidence"} {
		if !isMutatingAction(a) {
			t.Errorf("%s must be mutating", a)
		}
	}
	for _, a := range []string{"alert_jiminy_critical", "review_guidance_effectiveness", "alert_memory_bloat"} {
		if isMutatingAction(a) {
			t.Errorf("%s must NOT be mutating", a)
		}
	}
}
