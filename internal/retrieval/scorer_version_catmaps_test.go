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

// RETRIEVAL-TYPED-EDGES-001: flipping RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED or
// tuning any of the 7 semantic-edge weights MUST flip the cache namespace.
func TestScorerVersion_FlipsOnTypedEdges(t *testing.T) {
	base := config.Config{RetrievalColumnVotingEnabled: true}
	off := (&Service{cfg: base}).scorerVersion()

	on := base
	on.RetrievalGraphTypedEdgesEnabled = true
	on.EdgeAttentionAnalogousTo = 0.55
	vOn := (&Service{cfg: on}).scorerVersion()
	if vOn == off {
		t.Fatal("scorerVersion must change when typed edges are enabled")
	}

	tuned := on
	tuned.EdgeAttentionAnalogousTo = 0.80 // tune one weight
	if (&Service{cfg: tuned}).scorerVersion() == vOn {
		t.Fatal("scorerVersion must change when a semantic-edge weight is tuned")
	}
}
