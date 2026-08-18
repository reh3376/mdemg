package retrieval

import (
	"context"
	"log/slog"
)

// ActivationSeed is a lightweight seed for [Service.ExpandSeedsByActivation] —
// consumers pass the node IDs they already care about (e.g. Jiminy's
// role-filtered actionables from Lever C) plus an initial activation score in
// [0,1]. The service does NOT re-derive the seed set; it expands it via one
// 1-hop edge fetch + SpreadingActivationWithAttention over the query context.
//
// Kept small so consumers outside this package can construct seeds without
// building a full [Candidate] (JIMINY-SUBSTRATE-NATIVE-001 Phase B1 —
// ACTIVATION-DRIVEN-DISCOVERY-001).
type ActivationSeed struct {
	NodeID string
	Score  float64
}

// ExpandSeedsByActivation runs one 1-hop [Service.fetchOutgoingEdges] against
// the given seeds and returns their post-spread activation map (seeds +
// newly-activated neighbors). Query-context-aware edge attention weights are
// computed from queryText via [ComputeEdgeAttention] against the service
// config; steps + lambda come from JIMINY_LEVER_C_ACTIVATION_STEPS / _LAMBDA
// (fallback: 2, 0.5). Fail-open: any error returns nil map + logs; the caller
// falls back to its raw seeds.
//
// This method is the substrate primitive Phase B1 exposes to Jiminy. It does
// NOT gate itself on any Jiminy config — the caller (jiminy.Service) owns the
// flag decision. Keeping the primitive unconditional lets future non-Jiminy
// callers (RSIC self-reflection, retrieval audit) reuse it.
func (s *Service) ExpandSeedsByActivation(ctx context.Context, spaceID string, seeds []ActivationSeed, queryText string) (map[string]float64, error) {
	if len(seeds) == 0 {
		return map[string]float64{}, nil
	}

	// Convert seeds → []Candidate (SpreadingActivationWithAttention only reads
	// NodeID + VectorSim). Clamp Score into [0, 1] on the way in.
	cands := make([]Candidate, 0, len(seeds))
	seedIDs := make([]string, 0, len(seeds))
	for _, sd := range seeds {
		if sd.NodeID == "" {
			continue
		}
		v := sd.Score
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		cands = append(cands, Candidate{NodeID: sd.NodeID, VectorSim: v})
		seedIDs = append(seedIDs, sd.NodeID)
	}
	if len(cands) == 0 {
		return map[string]float64{}, nil
	}

	edges, _, err := s.fetchOutgoingEdges(ctx, []string{spaceID}, seedIDs)
	if err != nil {
		// Fail-open: caller keeps raw seed scores. Log at WARN — a persistently
		// erroring edge-fetch degrades this primitive to identity.
		slog.Warn("ExpandSeedsByActivation: edge fetch failed", "space_id", spaceID, "seed_count", len(seedIDs), "err", err)
		return nil, err
	}

	// Steps + lambda: read from Jiminy Lever C config (single source; a
	// future non-Jiminy consumer that wants different values will get its
	// own knobs).
	steps := s.cfg.JiminyLeverCActivationSteps
	if steps <= 0 {
		steps = 2
	}
	lambda := s.cfg.JiminyLeverCActivationLambda
	if lambda < 0 || lambda > 0.9 {
		lambda = 0.5
	}

	// Jiminy's request.Context is human-language action text, not a code or
	// architecture query — so leave the query-flavor bits off. The default
	// edge attention weights already favor CO_ACTIVATED_WITH, which is the
	// dominant signal here (228k edges on mdemg-dev vs ~thousands of
	// typed-semantic edges).
	attention := ComputeEdgeAttention(QueryContext{QueryText: queryText}, s.cfg)
	act := SpreadingActivationWithAttention(cands, edges, steps, lambda, attention, []float64{0.0, 0.05})
	return act, nil
}
