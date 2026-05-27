// Sprint EVENTGRAPH-001 Epic 3 — Hebbian RETURN row parser.
//
// parseReinforcementRow translates one Neo4j RETURN row from the extended
// ApplyCoactivation Cypher (post-Epic-3 RETURN signature) into a
// tsdb.ReinforcementEventRow. Lives in its own file so the parser + its
// Tier 1 tests don't bloat service.go.
package learning

import (
	"mdemg/internal/tsdb"
)

// recordGetter mirrors the (any, bool) signature of neo4j.Record.Get so the
// parser is testable with a synthetic getter (no Neo4j driver in unit tests).
type recordGetter func(key string) (any, bool)

// parseReinforcementRow converts one Cypher RETURN row into the typed row
// the TSDB writer expects. Missing / wrong-typed keys land as zero values;
// the writer applies nullableFloat / nullableString so DB NULL is preserved
// for optional columns.
func parseReinforcementRow(get recordGetter) tsdb.ReinforcementEventRow {
	return tsdb.ReinforcementEventRow{
		SrcNodeID:          getString(get, "src_node_id"),
		DstNodeID:          getString(get, "dst_node_id"),
		PrevWeight:         getFloat(get, "prev_weight"),
		NewWeight:          getFloat(get, "new_weight"),
		DeltaWeight:        getFloat(get, "delta_weight"),
		EvidenceCountAfter: getInt(get, "evidence_count_after"),
		EtaEffective:       getFloat(get, "eta_effective"),
		SurpriseFactor:     getFloat(get, "surprise_factor"),
		ActivationProduct:  getFloat(get, "activation_product"),
		PathSim:            getFloat(get, "path_sim"),
		RoleA:              getString(get, "role_a"),
		RoleB:              getString(get, "role_b"),
		ObsTypeA:           getString(get, "obs_type_a"),
		ObsTypeB:           getString(get, "obs_type_b"),
		SessionID:          getString(get, "session_id"),
		Direction:          getString(get, "direction"),
		CreatedNewEdge:     getBool(get, "created_new_edge"),
		TriggerPath:        "apply_coactivation",
	}
}

func getString(get recordGetter, key string) string {
	v, ok := get(key)
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func getFloat(get recordGetter, key string) float64 {
	v, ok := get(key)
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func getInt(get recordGetter, key string) int {
	v, ok := get(key)
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func getBool(get recordGetter, key string) bool {
	v, ok := get(key)
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}
