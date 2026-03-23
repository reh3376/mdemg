package transfer

import (
	"strings"
	"testing"
)

func TestBuildNodeFilterClauses_Empty(t *testing.T) {
	cfg := ExportConfig{}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 0 {
		t.Errorf("expected 0 clauses, got %d: %v", len(r.Clauses), r.Clauses)
	}
	if len(r.Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(r.Params))
	}
}

func TestBuildNodeFilterClauses_ObsTypes(t *testing.T) {
	cfg := ExportConfig{ObsTypes: []string{"learning", "decision"}}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(r.Clauses))
	}
	c := r.Clauses[0]
	if !strings.Contains(c, "n.layer >= 1") {
		t.Error("ObsTypes clause should bypass for L1+")
	}
	if !strings.Contains(c, "$filterObsTypes") {
		t.Error("ObsTypes clause should reference $filterObsTypes param")
	}
	if !strings.Contains(c, "n.obs_type IN") {
		t.Error("ObsTypes clause should filter obs_type")
	}
	if v, ok := r.Params["filterObsTypes"]; !ok {
		t.Error("missing filterObsTypes param")
	} else {
		types := v.([]string)
		if len(types) != 2 || types[0] != "learning" || types[1] != "decision" {
			t.Errorf("unexpected obs types: %v", types)
		}
	}
}

func TestBuildNodeFilterClauses_Tags(t *testing.T) {
	cfg := ExportConfig{Tags: []string{"important", "shared"}}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(r.Clauses))
	}
	if !strings.Contains(r.Clauses[0], "$filterTags") {
		t.Error("Tags clause should reference $filterTags param")
	}
	if !strings.Contains(r.Clauses[0], "ANY(") {
		t.Error("Tags clause should use ANY()")
	}
}

func TestBuildNodeFilterClauses_ExcludeVolatile(t *testing.T) {
	cfg := ExportConfig{ExcludeVolatile: true}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(r.Clauses))
	}
	c := r.Clauses[0]
	if !strings.Contains(c, "n.volatile") {
		t.Error("ExcludeVolatile clause should check volatile property")
	}
	if !strings.Contains(c, "stability_score") {
		t.Error("ExcludeVolatile clause should check stability_score")
	}
	if !strings.Contains(c, "0.8") {
		t.Error("ExcludeVolatile clause should use 0.8 threshold")
	}
}

func TestBuildNodeFilterClauses_OnlyPinned(t *testing.T) {
	cfg := ExportConfig{OnlyPinned: true}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(r.Clauses))
	}
	c := r.Clauses[0]
	if !strings.Contains(c, "n.layer >= 1") {
		t.Error("OnlyPinned clause should bypass for L1+")
	}
	if !strings.Contains(c, "n.pinned") {
		t.Error("OnlyPinned clause should check pinned property")
	}
}

func TestBuildNodeFilterClauses_MinMaxLayer(t *testing.T) {
	cfg := ExportConfig{MinLayer: 1, MaxLayer: 3}
	r := BuildNodeFilterClauses(cfg)
	if len(r.Clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(r.Clauses))
	}
	if !strings.Contains(r.Clauses[0], "$filterMinLayer") {
		t.Error("MinLayer clause should reference $filterMinLayer")
	}
	if !strings.Contains(r.Clauses[1], "$filterMaxLayer") {
		t.Error("MaxLayer clause should reference $filterMaxLayer")
	}
	if r.Params["filterMinLayer"] != 1 {
		t.Errorf("expected filterMinLayer=1, got %v", r.Params["filterMinLayer"])
	}
	if r.Params["filterMaxLayer"] != 3 {
		t.Errorf("expected filterMaxLayer=3, got %v", r.Params["filterMaxLayer"])
	}
}

func TestBuildNodeFilterClauses_Combined(t *testing.T) {
	cfg := ExportConfig{
		ObsTypes:        []string{"learning"},
		Tags:            []string{"shared"},
		ExcludeVolatile: true,
		OnlyPinned:      true,
		MinLayer:        0,
		MaxLayer:        5,
	}
	r := BuildNodeFilterClauses(cfg)
	// ObsTypes + Tags + ExcludeVolatile + OnlyPinned + MaxLayer (MinLayer=0 is skipped)
	if len(r.Clauses) != 5 {
		t.Errorf("expected 5 clauses, got %d: %v", len(r.Clauses), r.Clauses)
	}
	// Params should include filterObsTypes, filterTags, filterMaxLayer
	if _, ok := r.Params["filterObsTypes"]; !ok {
		t.Error("missing filterObsTypes")
	}
	if _, ok := r.Params["filterTags"]; !ok {
		t.Error("missing filterTags")
	}
	if _, ok := r.Params["filterMaxLayer"]; !ok {
		t.Error("missing filterMaxLayer")
	}
}

func TestExportConfigForProfile_Shareable(t *testing.T) {
	cfg, err := ExportConfigForProfile("test-space", ProfileShareable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IncludeObservations {
		t.Error("shareable should include observations")
	}
	if cfg.IncludeSymbols {
		t.Error("shareable should exclude symbols")
	}
	if !cfg.IncludeLearnedEdges {
		t.Error("shareable should include learned edges")
	}
	if !cfg.ExcludeVolatile {
		t.Error("shareable should exclude volatile")
	}
	if len(cfg.ObsTypes) == 0 {
		t.Fatal("shareable should set obs types")
	}
	expectedTypes := map[string]bool{
		"learning": true, "decision": true, "correction": true,
		"technical_note": true, "insight": true, "preference": true,
		"constraint": true,
	}
	for _, ot := range cfg.ObsTypes {
		if !expectedTypes[ot] {
			t.Errorf("unexpected obs type in shareable profile: %q", ot)
		}
	}
	if len(cfg.ObsTypes) != len(expectedTypes) {
		t.Errorf("expected %d obs types, got %d", len(expectedTypes), len(cfg.ObsTypes))
	}
}

func TestAppendFilterWhere(t *testing.T) {
	base := "n.active = true"

	// No clauses
	result := appendFilterWhere(base, nil)
	if result != base {
		t.Errorf("expected %q, got %q", base, result)
	}

	// Single clause
	result = appendFilterWhere(base, []string{"n.layer >= 1"})
	expected := "n.active = true AND n.layer >= 1"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Multiple clauses
	result = appendFilterWhere(base, []string{"n.layer >= 1", "n.volatile = false"})
	expected = "n.active = true AND n.layer >= 1 AND n.volatile = false"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildEndpointFilterWhere(t *testing.T) {
	// No clauses
	result := buildEndpointFilterWhere(nil)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}

	// Single clause
	result = buildEndpointFilterWhere([]string{"n.layer >= 1"})
	if !strings.Contains(result, "a.layer >= 1") {
		t.Error("should rewrite n. to a.")
	}
	if !strings.Contains(result, "b.layer >= 1") {
		t.Error("should rewrite n. to b.")
	}
	if !strings.HasPrefix(result, " AND ") {
		t.Error("should start with AND")
	}
}
