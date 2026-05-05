// Phase 14.2 Note 05 — Two-phase context fingerprint computation.
//
// Phase A (observe-time, this file): ComputeContextFingerprintLocal walks
// observation-local features (path, role_type, layer, top-N tags) and emits
// the sorted set of bit positions whose value is 1 in the 256-bit sparse
// fingerprint. No co-activation lookups happen here; cold-start works
// (every observation gets *some* fingerprint, even when the CO_ACTIVATED_WITH
// graph is empty).
//
// Phase B (post-hoc refinement, RefineWithCoactivations): walks the graph
// for co-activated MemoryNodes and adds symbol bits, bumping the version.
// Called by the CycleOrchestrator stage 6 hook for observations whose
// version is older than the active catalog. Every space surveyed in Phase
// 14.2 Epic 0 has 0 distinct symbols, so this is currently a no-op for
// production data — kept for forward compatibility with code-rich spaces.
//
// Cycle-safety: this file imports internal/hidden (one-way) and is imported
// by service.go at observe-time. No back-edge into internal/retrieval.
package conversation

import (
	"context"
	"fmt"
	"slices"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/hidden"
)

// ComputeContextFingerprintLocal returns the sorted set of bit positions
// whose value is 1 in the observation's local fingerprint. Bits are
// determined by the active catalog's reverse-lookup tables.
//
// Inputs walked:
//   - obs.Metadata["file_path"] (string) → BitKindPath lookup
//   - obs.Metadata["role_type"] + obs.Metadata["layer"] (or sane defaults
//     for ConversationObs) → BitKindRoleTypeLayer lookup
//   - obs.Tags → BitKindTag lookup (intersection)
//
// Returns nil for either nil cat (cold-start) or no matches. A returned
// slice is sorted ascending and contains no duplicates.
func ComputeContextFingerprintLocal(obs *Observation, cat *hidden.Catalog) []uint16 {
	if cat == nil || obs == nil {
		return nil
	}

	bits := make(map[uint16]struct{}, 8)

	// Path bit
	if obs.Metadata != nil {
		if pathRaw, ok := obs.Metadata["file_path"]; ok {
			if path, ok := pathRaw.(string); ok && path != "" {
				if pos, ok := cat.PathBit(path); ok {
					bits[pos] = struct{}{}
				}
			}
		}
	}

	// Role-type × layer bit. Conversation observations default to
	// ("conversation_observation", 0) per createObservationNode Cypher.
	roleType, layer := observationRoleTypeLayer(obs)
	if pos, ok := cat.RoleTypeLayerBit(roleType, layer); ok {
		bits[pos] = struct{}{}
	}

	// Tag bits — intersection of obs.Tags ∩ catalog.tagToBit
	for _, tag := range obs.Tags {
		if pos, ok := cat.TagBit(tag); ok {
			bits[pos] = struct{}{}
		}
	}

	if len(bits) == 0 {
		return nil
	}
	out := make([]uint16, 0, len(bits))
	for b := range bits {
		out = append(out, b)
	}
	slices.Sort(out)
	return out
}

// observationRoleTypeLayer extracts the (role_type, layer) tuple used as
// the BitKindRoleTypeLayer lookup key. Conversation observations default to
// ("conversation_observation", 0) — matches the createObservationNode Cypher.
// Phase 14.2.1+ may carry richer role_type values when callers populate
// metadata explicitly.
func observationRoleTypeLayer(obs *Observation) (string, int) {
	roleType := "conversation_observation"
	layer := 0
	if obs.Metadata != nil {
		if rt, ok := obs.Metadata["role_type"].(string); ok && rt != "" {
			roleType = rt
		}
		if l, ok := obs.Metadata["layer"].(int); ok {
			layer = l
		} else if l64, ok := obs.Metadata["layer"].(int64); ok {
			layer = int(l64)
		}
	}
	return roleType, layer
}

// RefineWithCoactivations is the Phase B (post-hoc) refinement: walks
// CO_ACTIVATED_WITH edges from obsID and adds symbol bits for each
// co-activated MemoryNode whose node_id has a catalog symbol bit. Bumps
// the version field on the observation node in the same transaction.
//
// Idempotent: re-running on the same (obs_id, catalog_version) pair is a
// no-op at the bit level. Returns the count of bits added (zero for spaces
// with no symbol bits, which is every surveyed production space today).
func RefineWithCoactivations(ctx context.Context, driver neo4j.DriverWithContext, obsID string, cat *hidden.Catalog) (int, error) {
	if cat == nil {
		return 0, fmt.Errorf("RefineWithCoactivations: nil catalog")
	}
	if obsID == "" {
		return 0, fmt.Errorf("RefineWithCoactivations: empty obs_id")
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	added, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// 1. Read current fingerprint + co-activated node_ids.
		readQ := `
			MATCH (o:MemoryNode {obs_id: $obs_id})
			OPTIONAL MATCH (o)-[:CO_ACTIVATED_WITH]-(other:MemoryNode)
			RETURN o.context_fingerprint_active AS active,
			       o.context_fingerprint_version AS version,
			       collect(DISTINCT other.node_id) AS coIds
		`
		res, err := tx.Run(ctx, readQ, map[string]any{"obs_id": obsID})
		if err != nil {
			return 0, err
		}
		if !res.Next(ctx) {
			return 0, fmt.Errorf("obs %q not found", obsID)
		}
		rec := res.Record()
		var current []uint16
		if raw, ok := rec.Get("active"); ok && raw != nil {
			if arr, ok := raw.([]any); ok {
				for _, v := range arr {
					if i, ok := v.(int64); ok {
						current = append(current, uint16(i))
					}
				}
			}
		}
		var coIDs []string
		if raw, ok := rec.Get("coIds"); ok && raw != nil {
			if arr, ok := raw.([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok && s != "" {
						coIDs = append(coIDs, s)
					}
				}
			}
		}

		// 2. Compute the new bit set: existing bits ∪ symbol bits for co-IDs.
		merged := make(map[uint16]struct{}, len(current)+4)
		for _, b := range current {
			merged[b] = struct{}{}
		}
		addedCount := 0
		for _, id := range coIDs {
			if pos, ok := cat.SymbolBit(id); ok {
				if _, dup := merged[pos]; !dup {
					merged[pos] = struct{}{}
					addedCount++
				}
			}
		}

		// 3. Bump version regardless (mature observations under the new
		// catalog version), even when no new bits were added — this keeps
		// the refresh hook from re-walking the same observation forever.
		next := make([]uint16, 0, len(merged))
		for b := range merged {
			next = append(next, b)
		}
		slices.Sort(next)

		writeQ := `
			MATCH (o:MemoryNode {obs_id: $obs_id})
			SET o.context_fingerprint_active = $active,
			    o.context_fingerprint_version = $version
		`
		_, err = tx.Run(ctx, writeQ, map[string]any{
			"obs_id":  obsID,
			"active":  asInt64Slice(next),
			"version": int64(cat.Version),
		})
		if err != nil {
			return 0, err
		}
		return addedCount, nil
	})
	if err != nil {
		return 0, err
	}
	if added == nil {
		return 0, nil
	}
	if n, ok := added.(int); ok {
		return n, nil
	}
	return 0, nil
}

// asInt64Slice converts []uint16 → []int64 for Neo4j parameter binding.
// Neo4j integer arrays are int64 on the wire.
func asInt64Slice(in []uint16) []int64 {
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}
