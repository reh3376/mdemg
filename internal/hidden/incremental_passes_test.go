package hidden

import (
	"strings"
	"testing"
)

// CONSOLIDATE-PERF-002: the legacy paginated queries now compose from the
// shared aggregation-body consts. These goldens are the ORIGINAL pre-refactor
// literals — byte-identical composition proves flag-off behavior unchanged,
// and any future math edit must touch the shared const (one source of truth
// for both paths) AND these goldens together.

const goldenLegacyForwardHidden = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WITH h ORDER BY h.node_id SKIP $skip LIMIT $limit
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[r:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, collect({emb: b.embedding, weight: coalesce(r.weight, 1.0)}) AS neighbors
WHERE size(neighbors) > 0
WITH h, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH h, neighbors, totalWeight,
     [i IN range(0, size(h.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $alpha * coalesce(h.embedding[i], 0) + $beta * aggregated[i]
    ],
    h.last_forward_pass = datetime(),
    h.aggregation_count = size(neighbors)
RETURN count(h) AS updated`

const goldenLegacyForwardConcept = `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2
WITH c ORDER BY c.node_id SKIP $skip LIMIT $limit
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})-[r:ABSTRACTS_TO]->(c)
WHERE h.message_pass_embedding IS NOT NULL OR h.embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(h.message_pass_embedding, h.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`

const goldenLegacyBackward = `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WITH h ORDER BY h.node_id SKIP $skip LIMIT $limit
OPTIONAL MATCH (h)-[rUp:ABSTRACTS_TO]->(c:MemoryNode)
WHERE c.layer >= 2 AND (c.message_pass_embedding IS NOT NULL OR c.embedding IS NOT NULL)
WITH h, collect(coalesce(c.message_pass_embedding, c.embedding)) AS conceptEmbs
OPTIONAL MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[rDown:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, conceptEmbs, collect(b.embedding) AS baseEmbs
WHERE size(conceptEmbs) > 0 OR size(baseEmbs) > 0
WITH h, conceptEmbs, baseEmbs,
     CASE WHEN size(conceptEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN conceptEmbs | sum + e[i]) / size(conceptEmbs)
       ]
     ELSE null END AS conceptSignal,
     CASE WHEN size(baseEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN baseEmbs | sum + e[i]) / size(baseEmbs)
       ]
     ELSE null END AS baseSignal
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $selfW * coalesce(h.embedding[i], 0) +
      $baseW * coalesce(baseSignal[i], h.embedding[i]) +
      $concW * coalesce(conceptSignal[i], h.embedding[i])
    ],
    h.last_backward_pass = datetime()
RETURN count(h) AS updated`

func TestLegacyPassCypherComposition(t *testing.T) {
	cases := []struct {
		name     string
		composed string
		golden   string
	}{
		{"forward_hidden", "\nMATCH (h:MemoryNode {space_id: $spaceId, layer: 1})\nWITH h ORDER BY h.node_id SKIP $skip LIMIT $limit\n" + fwdHiddenAggBody, goldenLegacyForwardHidden},
		{"forward_concept", "\nMATCH (c:MemoryNode {space_id: $spaceId})\nWHERE c.layer >= 2\nWITH c ORDER BY c.node_id SKIP $skip LIMIT $limit\n" + fwdConceptAggBody, goldenLegacyForwardConcept},
		{"backward", "\nMATCH (h:MemoryNode {space_id: $spaceId, layer: 1})\nWITH h ORDER BY h.node_id SKIP $skip LIMIT $limit\n" + backwardAggBody, goldenLegacyBackward},
	}
	for _, tc := range cases {
		if tc.composed != tc.golden {
			t.Errorf("%s: composed legacy Cypher diverged from the pre-refactor golden\ncomposed:\n%s\ngolden:\n%s", tc.name, tc.composed, tc.golden)
		}
	}
}

// The incremental by-IDs queries must share the exact aggregation bodies and
// target explicit id sets (never SKIP/LIMIT — stamping mutates the gated set
// mid-pagination and SKIP would jump over still-pending nodes).
func TestIncrementalByIDQueriesShape(t *testing.T) {
	for name, q := range map[string]string{
		"forward_hidden":  forwardHiddenByIDsCypher,
		"forward_concept": forwardConceptByIDsCypher,
		"backward":        backwardHiddenByIDsCypher,
	} {
		if !strings.Contains(q, "IN $ids") {
			t.Errorf("%s: missing id-set targeting", name)
		}
		if strings.Contains(q, "SKIP") || strings.Contains(q, "LIMIT") {
			t.Errorf("%s: must not paginate (gate+SKIP interaction)", name)
		}
	}
	if !strings.Contains(forwardHiddenByIDsCypher, fwdHiddenAggBody) ||
		!strings.Contains(forwardConceptByIDsCypher, fwdConceptAggBody) ||
		!strings.Contains(backwardHiddenByIDsCypher, backwardAggBody) {
		t.Error("incremental queries must embed the shared aggregation bodies")
	}
}

// Gate predicates must cover every input path: null-stamp bootstrap, member
// node recency, membership (edge) recency, and — for the layers that consume
// a lower layer's output — cascade advancement.
func TestIncrementalGatePredicates(t *testing.T) {
	for name, tc := range map[string]struct {
		q     string
		wants []string
	}{
		"pending_forward_hidden": {pendingForwardHiddenCypher, []string{
			"last_forward_pass IS NULL", "nb.updated_at > h.last_forward_pass", "nr.created_at > h.last_forward_pass"}},
		"pending_forward_concept": {pendingForwardConceptCypher, []string{
			"last_forward_pass IS NULL", "nh.last_forward_pass > c.last_forward_pass", "nh.updated_at > c.last_forward_pass", "nr.created_at > c.last_forward_pass"}},
		"pending_backward": {pendingBackwardHiddenCypher, []string{
			"last_backward_pass IS NULL", "nb.updated_at > h.last_backward_pass", "nc.last_forward_pass > h.last_backward_pass"}},
	} {
		for _, w := range tc.wants {
			if !strings.Contains(tc.q, w) {
				t.Errorf("%s: missing gate clause %q", name, w)
			}
		}
	}
}
