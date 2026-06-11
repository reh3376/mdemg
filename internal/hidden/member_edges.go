// HIDDEN-WEIGHT-001 — CUIDv2 edge ids for abstraction-edge creation.
//
// Cypher cannot mint CUIDv2 (randomUUID() violated the project identifier
// standard), so edge ids are generated Go-side and zipped with member ids
// for UNWIND-based CREATE.
package hidden

import cuid2 "github.com/nrednav/cuid2"

// memberEdgePairs zips member node ids with freshly minted CUIDv2 edge ids.
// One pair per member; ids are unique per call.
func memberEdgePairs(memberIDs []string) []map[string]any {
	pairs := make([]map[string]any, 0, len(memberIDs))
	for _, id := range memberIDs {
		pairs = append(pairs, map[string]any{
			"memberId": id,
			"edgeId":   cuid2.Generate(),
		})
	}
	return pairs
}
