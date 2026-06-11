package learning

import "testing"

func mapGetter(m map[string]any) recordGetter {
	return func(key string) (any, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestParseReinforcementRow_HappyPath_BidirectionalCreate(t *testing.T) {
	// Symmetric mode: r.direction is set to "bidirectional" by ApplyCoactivation
	// when LearningAsymmetricEnabled=false. evidence_count_after = 1 → ON CREATE.
	rec := map[string]any{
		"src_node_id":          "node-a",
		"dst_node_id":          "node-b",
		"prev_weight":          0.10,
		"new_weight":           0.18,
		"delta_weight":         0.08,
		"evidence_count_after": int64(1),
		"eta_effective":        0.045,
		"surprise_factor":      1.5,
		"activation_product":   0.6,
		"path_sim":             0.7,
		"role_a":               "code",
		"role_b":               "conversation_observation",
		"obs_type_a":           "",
		"obs_type_b":           "learning",
		"session_id":           "session-42",
		"direction":            "bidirectional",
		"created_new_edge":     true,
	}
	row := parseReinforcementRow(mapGetter(rec))

	if row.SrcNodeID != "node-a" || row.DstNodeID != "node-b" {
		t.Errorf("src/dst = %s/%s, want node-a/node-b", row.SrcNodeID, row.DstNodeID)
	}
	if row.PrevWeight != 0.10 || row.NewWeight != 0.18 || row.DeltaWeight != 0.08 {
		t.Errorf("weights mismatch: prev=%v new=%v delta=%v", row.PrevWeight, row.NewWeight, row.DeltaWeight)
	}
	if row.EvidenceCountAfter != 1 || !row.CreatedNewEdge {
		t.Errorf("create-branch detection failed: evidence=%d created=%v", row.EvidenceCountAfter, row.CreatedNewEdge)
	}
	if row.Direction != "bidirectional" {
		t.Errorf("direction = %q, want bidirectional", row.Direction)
	}
	if row.TriggerPath != "apply_coactivation" {
		t.Errorf("trigger_path = %q, want apply_coactivation", row.TriggerPath)
	}
}

func TestParseReinforcementRow_OnMatchPath_AsymmetricForward(t *testing.T) {
	// Asymmetric mode: r.direction = "forward". evidence_count_after > 1 → ON MATCH.
	rec := map[string]any{
		"src_node_id":          "n1",
		"dst_node_id":          "n2",
		"prev_weight":          0.4,
		"new_weight":           0.46,
		"delta_weight":         0.06,
		"evidence_count_after": int64(7),
		"eta_effective":        0.05,
		"surprise_factor":      1.0,
		"activation_product":   0.3,
		"path_sim":             0.2,
		"role_a":               "code",
		"role_b":               "code",
		"obs_type_a":           "",
		"obs_type_b":           "",
		"session_id":           "",
		"direction":            "forward",
		"created_new_edge":     false,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.CreatedNewEdge {
		t.Errorf("expected ON MATCH (created_new_edge=false), got true")
	}
	if row.EvidenceCountAfter != 7 {
		t.Errorf("evidence_count_after = %d, want 7", row.EvidenceCountAfter)
	}
	if row.Direction != "forward" {
		t.Errorf("direction = %q, want forward", row.Direction)
	}
	if row.SessionID != "" {
		t.Errorf("session_id should be empty string for no-session pairs, got %q", row.SessionID)
	}
}

func TestParseReinforcementRow_MissingOptionalFields_ZeroValuedRow(t *testing.T) {
	// Only required keys present. Optional float / string fields absent →
	// parser must return zero values, which the writer's nullable* helpers
	// will serialize as DB NULL.
	rec := map[string]any{
		"src_node_id":          "a",
		"dst_node_id":          "b",
		"prev_weight":          0.0,
		"new_weight":           0.1,
		"delta_weight":         0.1,
		"evidence_count_after": int64(1),
		"direction":            "bidirectional",
		"created_new_edge":     true,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.EtaEffective != 0 || row.SurpriseFactor != 0 || row.ActivationProduct != 0 || row.PathSim != 0 {
		t.Errorf("optional floats should be zero when key missing, got eta=%v surprise=%v ap=%v ps=%v",
			row.EtaEffective, row.SurpriseFactor, row.ActivationProduct, row.PathSim)
	}
	if row.RoleA != "" || row.RoleB != "" || row.ObsTypeA != "" || row.ObsTypeB != "" || row.SessionID != "" {
		t.Errorf("optional strings should be empty when key missing, got %+v", row)
	}
}

func TestParseReinforcementRow_NeoTypeCoercion(t *testing.T) {
	// Neo4j driver may return numeric values as int64 OR float64 depending on
	// the source Cypher expression. Verify both paths coerce correctly.
	rec := map[string]any{
		"src_node_id":          "a",
		"dst_node_id":          "b",
		"prev_weight":          float64(0.1),
		"new_weight":           float64(0.2),
		"delta_weight":         float64(0.1),
		"evidence_count_after": int64(3),  // Neo4j int → Go int64
		"direction":            "forward",
		"created_new_edge":     false,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.EvidenceCountAfter != 3 {
		t.Errorf("int64→int coercion failed: got %d, want 3", row.EvidenceCountAfter)
	}
}

func TestParseReinforcementRow_NilValuesHandledAsZero(t *testing.T) {
	rec := map[string]any{
		"src_node_id":          "a",
		"dst_node_id":          "b",
		"prev_weight":          nil, // simulate Cypher COALESCE returning NULL
		"new_weight":           0.1,
		"delta_weight":         0.1,
		"evidence_count_after": nil,
		"role_a":               nil,
		"created_new_edge":     nil,
		"direction":            nil,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.PrevWeight != 0 || row.EvidenceCountAfter != 0 || row.RoleA != "" || row.CreatedNewEdge || row.Direction != "" {
		t.Errorf("nil values should land as zero/empty, got %+v", row)
	}
}

func TestParseReinforcementRow_WrongType_FallsBackToZero(t *testing.T) {
	rec := map[string]any{
		"src_node_id":  "a",
		"dst_node_id":  "b",
		"prev_weight":  "not-a-number", // wrong type; parser should not panic
		"new_weight":   0.2,
		"delta_weight": 0.2,
		"role_a":       42, // wrong type
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.PrevWeight != 0 {
		t.Errorf("wrong-typed float should fall back to 0, got %v", row.PrevWeight)
	}
	if row.RoleA != "" {
		t.Errorf("wrong-typed string should fall back to empty, got %q", row.RoleA)
	}
}

func TestParseReinforcementRow_ContradictCreate_EventGraph004(t *testing.T) {
	// EVENTGRAPH-004 contradict statement, ON CREATE branch: a CONTRADICTS edge
	// is born at weight=negWeight. prev=0, delta=+negWeight (the edge's OWN
	// weight delta — negative-feedback semantics live in trigger_path, which the
	// caller overrides to apply_negative_feedback_contradict after parsing).
	rec := map[string]any{
		"src_node_id":          "query-node",
		"dst_node_id":          "rejected-node",
		"prev_weight":          0.0,
		"new_weight":           0.15,
		"delta_weight":         0.15,
		"evidence_count_after": int64(1),
		"eta_effective":        nil,
		"surprise_factor":      nil,
		"activation_product":   nil,
		"path_sim":             nil,
		"role_a":               "conversation_observation",
		"role_b":               "conversation_observation",
		"obs_type_a":           "note",
		"obs_type_b":           "note",
		"session_id":           "eg004-probe",
		"direction":            "forward",
		"created_new_edge":     true,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if !row.CreatedNewEdge {
		t.Fatalf("ON CREATE branch: created_new_edge should be true")
	}
	if row.PrevWeight != 0.0 || row.NewWeight != 0.15 || row.DeltaWeight != 0.15 {
		t.Errorf("create weights: prev=%v new=%v delta=%v, want 0/0.15/0.15", row.PrevWeight, row.NewWeight, row.DeltaWeight)
	}
	if row.EvidenceCountAfter != 1 {
		t.Errorf("evidence_count_after = %d, want 1", row.EvidenceCountAfter)
	}
	// Hebbian-only fields are NULL for contradict rows → zero values, which the
	// writer's nullableFloat maps back to SQL NULL.
	if row.EtaEffective != 0 || row.SurpriseFactor != 0 || row.ActivationProduct != 0 || row.PathSim != 0 {
		t.Errorf("contradict rows must zero the Hebbian-only fields, got eta=%v surprise=%v act=%v sim=%v",
			row.EtaEffective, row.SurpriseFactor, row.ActivationProduct, row.PathSim)
	}
}

func TestParseReinforcementRow_ContradictRematch_EventGraph004(t *testing.T) {
	// EVENTGRAPH-004 contradict statement, ON MATCH branch: evidence_count
	// increments, weight unchanged → delta=0, created_new_edge=false.
	rec := map[string]any{
		"src_node_id":          "query-node",
		"dst_node_id":          "rejected-node",
		"prev_weight":          0.15,
		"new_weight":           0.15,
		"delta_weight":         0.0,
		"evidence_count_after": int64(2),
		"eta_effective":        nil,
		"surprise_factor":      nil,
		"activation_product":   nil,
		"path_sim":             nil,
		"role_a":               "conversation_observation",
		"role_b":               "conversation_observation",
		"obs_type_a":           "note",
		"obs_type_b":           "note",
		"session_id":           "eg004-probe",
		"direction":            "forward",
		"created_new_edge":     false,
	}
	row := parseReinforcementRow(mapGetter(rec))
	if row.CreatedNewEdge {
		t.Fatalf("ON MATCH branch: created_new_edge should be false")
	}
	if row.DeltaWeight != 0.0 || row.PrevWeight != 0.15 || row.NewWeight != 0.15 {
		t.Errorf("re-match weights: prev=%v new=%v delta=%v, want 0.15/0.15/0", row.PrevWeight, row.NewWeight, row.DeltaWeight)
	}
	if row.EvidenceCountAfter != 2 {
		t.Errorf("evidence_count_after = %d, want 2", row.EvidenceCountAfter)
	}
}
