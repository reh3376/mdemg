package retrieval

import (
	"testing"

	"mdemg/internal/config"
)

// CONTEXT-LIVE-001: operator edits to the per-category context-weight map or
// the sparse-gate override map MUST flip the cache namespace (the v0.7.0
// cache-key class — config changes serving stale cached rankings).
func TestScorerVersion_FlipsOnCategoryMapChanges(t *testing.T) {
	base := config.Config{RetrievalColumnVotingEnabled: true}
	s1 := &Service{cfg: base}
	v1 := s1.scorerVersion()

	withWeights := base
	withWeights.RetrievalContextColumnCategoryWeights = map[string]float64{"relationship": 0.0}
	s2 := &Service{cfg: withWeights}
	if s2.scorerVersion() == v1 {
		t.Fatal("scorerVersion must change when per-category context weights change")
	}

	withOverrides := base
	min20 := 20
	withOverrides.SparseGateCategoryOverrides = map[string]config.SparseGateOverride{
		"data_flow_integration": {MinActive: &min20},
	}
	s3 := &Service{cfg: withOverrides}
	if s3.scorerVersion() == v1 {
		t.Fatal("scorerVersion must change when sparse-gate overrides change")
	}
	if s3.scorerVersion() == s2.scorerVersion() {
		t.Fatal("distinct maps must produce distinct versions")
	}
}
