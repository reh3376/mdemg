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
	"strings"

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

	// Path bit (full-path match against catalog path bits)
	var filePath string
	if obs.Metadata != nil {
		if pathRaw, ok := obs.Metadata["file_path"]; ok {
			if path, ok := pathRaw.(string); ok && path != "" {
				filePath = path
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

	// Tag bits — intersection of obs.Tags ∩ catalog.tagToBit.
	// Phase 14.2.2: catalog tag refs are now path-segment tokens (Builder
	// retune). Match obs.Tags first (legacy, harmless if no overlap), then
	// also tokenize the obs's file_path on '/' and match each segment.
	// This is the observe-time mirror of the Builder's path-segment
	// collection; without it, fingerprints of new observations would never
	// hit the path-segment tag bits.
	for _, tag := range obs.Tags {
		if pos, ok := cat.TagBit(tag); ok {
			bits[pos] = struct{}{}
		}
	}
	if filePath != "" {
		for _, seg := range pathSegments(filePath) {
			if pos, ok := cat.TagBit(seg); ok {
				bits[pos] = struct{}{}
			}
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

// pathSegments splits a file path on '/' and drops empty segments. Mirrors
// the Cypher `split(m.path, "/")` used by the Builder's path-segment tag
// collection (Phase 14.2.2). Segments must be ≥ 2 chars to match the
// Builder's filter (single-char segments are too noisy to be useful bits).
func pathSegments(path string) []string {
	parts := strings.Split(path, "/")
	out := parts[:0]
	for _, p := range parts {
		if len(p) >= 2 {
			out = append(out, p)
		}
	}
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

// RecomputeStaleFingerprints recomputes observe-time fingerprints for up to
// maxNodes MemoryNodes whose stored context_fingerprint_version is older
// than cat.Version, writing the new bits + version in batches.
//
// CONTEXT-LIVE-001: this — NOT RefineWithCoactivations — is the version-skew
// healer. Refine MERGES existing bits and bumps the version, which would
// relabel old-catalog bit semantics as current; recomputation derives fresh
// bits from the node's path/role/layer/tags against the new catalog. Shared
// by the stage-6 RSIC hook (budget-bounded, resumable across cycles) and
// conceptually mirrors `mdemg migrate context-fingerprint`.
//
// Returns (scanned, updated, skipped): skipped counts nodes whose recompute
// produced no bits (left untouched at their old version so a later catalog
// with better coverage can still claim them).
func RecomputeStaleFingerprints(
	ctx context.Context,
	driver neo4j.DriverWithContext,
	spaceID string,
	cat *hidden.Catalog,
	batchSize, maxNodes int,
) (scanned, updated, skipped int, err error) {
	if cat == nil {
		return 0, 0, 0, fmt.Errorf("RecomputeStaleFingerprints: nil catalog")
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	if maxNodes <= 0 {
		maxNodes = 2000
	}

	type row struct {
		nodeID   string
		roleType string
		layer    int
		tags     []string
		filePath string
	}
	readQ := `
		MATCH (m:MemoryNode {space_id: $space_id})
		WHERE coalesce(m.context_fingerprint_version, 0) < $target_version
		  AND m.role_type IS NOT NULL
		RETURN m.node_id AS node_id,
		       coalesce(m.role_type, 'conversation_observation') AS role_type,
		       coalesce(m.layer, 0) AS layer,
		       coalesce(m.tags, []) AS tags,
		       coalesce(m.path, '') AS file_path
		LIMIT $max_nodes
	`
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	rowsRaw, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, readQ, map[string]any{
			"space_id":       spaceID,
			"target_version": int64(cat.Version),
			"max_nodes":      int64(maxNodes),
		})
		if err != nil {
			return nil, err
		}
		var rows []row
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			if nodeID == nil {
				continue
			}
			roleType, _ := rec.Get("role_type")
			layerRaw, _ := rec.Get("layer")
			tagsRaw, _ := rec.Get("tags")
			pathRaw, _ := rec.Get("file_path")
			r := row{nodeID: fmt.Sprint(nodeID), roleType: fmt.Sprint(roleType), filePath: fmt.Sprint(pathRaw)}
			if li, ok := layerRaw.(int64); ok {
				r.layer = int(li)
			}
			if tarr, ok := tagsRaw.([]any); ok {
				for _, t := range tarr {
					if s, ok := t.(string); ok {
						r.tags = append(r.tags, s)
					}
				}
			}
			rows = append(rows, r)
		}
		return rows, res.Err()
	})
	_ = sess.Close(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("scan stale fingerprints: %w", err)
	}
	rows, _ := rowsRaw.([]row)
	scanned = len(rows)
	if scanned == 0 {
		return 0, 0, 0, nil
	}

	writeQ := `
		UNWIND $rows AS row
		MATCH (m:MemoryNode {node_id: row.node_id})
		SET m.context_fingerprint_active = row.bits,
		    m.context_fingerprint_version = $version
	`
	var pendingIDs []string
	var pendingBits [][]int64
	flush := func() error {
		if len(pendingIDs) == 0 {
			return nil
		}
		writeRows := make([]map[string]any, len(pendingIDs))
		for i, id := range pendingIDs {
			writeRows[i] = map[string]any{"node_id": id, "bits": pendingBits[i]}
		}
		ws := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, werr := ws.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, writeQ, map[string]any{
				"rows":    writeRows,
				"version": int64(cat.Version),
			})
			return nil, err
		})
		_ = ws.Close(ctx)
		if werr != nil {
			return werr
		}
		updated += len(pendingIDs)
		pendingIDs, pendingBits = nil, nil
		return nil
	}

	for _, r := range rows {
		if ctx.Err() != nil {
			// Budget exhausted — flush what we have; the next cycle resumes.
			break
		}
		obs := &Observation{
			Tags: r.tags,
			Metadata: map[string]any{
				"role_type": r.roleType,
				"layer":     r.layer,
			},
		}
		if r.filePath != "" {
			obs.Metadata["file_path"] = r.filePath
		}
		fp := ComputeContextFingerprintLocal(obs, cat)
		if fp == nil {
			skipped++
			continue
		}
		pendingIDs = append(pendingIDs, r.nodeID)
		pendingBits = append(pendingBits, asInt64Slice(fp))
		if len(pendingIDs) >= batchSize {
			if err := flush(); err != nil {
				return scanned, updated, skipped, fmt.Errorf("flush: %w", err)
			}
		}
	}
	if err := flush(); err != nil {
		return scanned, updated, skipped, fmt.Errorf("final flush: %w", err)
	}
	return scanned, updated, skipped, nil
}
