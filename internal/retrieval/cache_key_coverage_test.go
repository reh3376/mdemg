package retrieval

import (
	"reflect"
	"testing"

	"mdemg/internal/models"
)

// cacheKeyNeutralFields are RetrieveRequest fields PROVEN result-neutral —
// they alter telemetry/explanation, not the ranked result set. Adding a new
// field to RetrieveRequest fails this test until you classify it here or in
// CacheKey (CACHE-KEY-002 forcing function: the unclassified-field class
// caused the v0.7.0 cache-key bug and recurred this sprint).
var cacheKeyNeutralFields = map[string]string{
	"JiminyEnabled":  "adds rationale/score breakdown to the response; ranking unchanged",
	"SessionID":      "TSDB telemetry attribution only",
	"PolicyContext":  "", // IN the key — listed for the reflect walk below
	"QueryEmbedding": "", // IN the key (hashed)
	"SpaceID":        "", "QueryText": "", "CandidateK": "", "TopK": "", "HopDepth": "",
	"IncludeEvidence": "", "IncludeGlobalSpace": "", "CodeOnly": "", "TranslateIntent": "",
	"IncludeExtensions": "", "ExcludeExtensions": "", "TemporalAfter": "", "TemporalBefore": "",
	// Caught by this test's FIRST run (beyond the audit's five): the sparse-
	// gate per-call overrides, pagination, and context-fingerprint params all
	// change the result set/page — now IN the key.
	"Cursor": "", "Limit": "", "SparseOverridePresent": "", "SparseEnabled": "",
	"SparsePercentile": "", "Category": "", "QueryContextFingerprint": "", "StrictContextMode": "",
}

func TestCacheKey_EveryRequestFieldClassified(t *testing.T) {
	rt := reflect.TypeOf(models.RetrieveRequest{})
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		if _, ok := cacheKeyNeutralFields[name]; !ok {
			t.Errorf("RetrieveRequest.%s is UNCLASSIFIED for cache-keying: add it to CacheKey (if it affects results) or to cacheKeyNeutralFields with a justification", name)
		}
	}
}

func TestCacheKey_DistinguishesAddedFields(t *testing.T) {
	base := models.RetrieveRequest{SpaceID: "s", QueryText: "q"}
	variants := []models.RetrieveRequest{
		{SpaceID: "s", QueryText: "q", IncludeExtensions: []string{"go"}},
		{SpaceID: "s", QueryText: "q", ExcludeExtensions: []string{"md"}},
		{SpaceID: "s", QueryText: "q", TemporalAfter: "2026-01-01T00:00:00Z"},
		{SpaceID: "s", QueryText: "q", TemporalBefore: "2026-06-01T00:00:00Z"},
		{SpaceID: "s", QueryText: "q", PolicyContext: map[string]any{"k": "v"}},
		{SpaceID: "s", QueryText: "", QueryEmbedding: []float32{0.1, 0.2}},
	}
	baseKey := CacheKey(base, "v1")
	seen := map[string]bool{baseKey: true}
	for i, v := range variants {
		k := CacheKey(v, "v1")
		if seen[k] {
			t.Errorf("variant %d collides with a prior key — field not in CacheKey", i)
		}
		seen[k] = true
	}
}
