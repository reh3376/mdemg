package retrieval

import (
	"math"

	"mdemg/internal/config"
)

// SpreadingActivation computes transient activation scores in-memory.
// - cands: candidates from vector recall (used for seeding)
// - edges: bounded neighborhood edges
// - steps: number of propagation steps (typ. 2-5)
// - lambda: step decay (prevents runaway)
func SpreadingActivation(cands []Candidate, edges []Edge, steps int, lambda float64, hopMinWeights []float64) map[string]float64 {
	act := map[string]float64{}
	if steps <= 0 {
		steps = 2
	}
	if lambda < 0 {
		lambda = 0
	}
	if lambda > 0.9 {
		lambda = 0.9
	}

	// Seed: ALL candidates seeded from max(VectorSim, BM25Score)
	// This enables Hebbian learning by ensuring all returned nodes have activation values.
	// Uses max of vector and BM25 signals so BM25-only candidates also seed properly.
	for _, c := range cands {
		v := c.VectorSim
		if c.BM25Score > v {
			v = c.BM25Score
		}
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		act[c.NodeID] = v
	}

	// Build incoming lists — only propagate through learned edges.
	// Structural edges (ASSOCIATED_WITH, GENERALIZES, etc.) are used for
	// graph expansion but NOT for activation spreading. This ensures that
	// learned co-activation patterns (CO_ACTIVATED_WITH) are the sole
	// driver of activation differentiation, preventing saturation from
	// dense structural connectivity.
	incoming := map[string][]Edge{}
	inhib := map[string][]Edge{}
	for _, e := range edges {
		if e.RelType == "CONTRADICTS" {
			inhib[e.Dst] = append(inhib[e.Dst], e)
			continue
		}
		if e.RelType != "CO_ACTIVATED_WITH" {
			continue
		}
		incoming[e.Dst] = append(incoming[e.Dst], e)
	}

	clamp01 := func(x float64) float64 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}

	for t := 0; t < steps; t++ {
		next := map[string]float64{}
		// Carry forward existing activations with decay
		for id, a := range act {
			next[id] = clamp01((1 - lambda) * a)
		}

		// Determine minimum weight threshold for this hop
		minWeight := 0.0
		if len(hopMinWeights) > 0 {
			idx := t
			if idx >= len(hopMinWeights) {
				idx = len(hopMinWeights) - 1
			}
			minWeight = hopMinWeights[idx]
		}

		// Apply incoming excitatory and inhibitory
		for dst, ins := range incoming {
			acc := next[dst]
			// Degree-normalized accumulation: divide by sqrt(degree) to prevent
			// high-degree nodes from saturating to 1.0. This preserves relative
			// signal strength — nodes with stronger/more relevant learned edges
			// get higher activation without all nodes converging to the same value.
			// Filter edges by minimum weight for this hop
			var filteredCount float64
			for _, e := range ins {
				if effectiveWeight(e) >= minWeight {
					filteredCount++
				}
			}
			degreeNorm := math.Sqrt(filteredCount)
			if degreeNorm < 1 {
				degreeNorm = 1
			}
			for _, e := range ins {
				w := effectiveWeight(e)
				if w < minWeight {
					continue
				}
				srcA := act[e.Src]
				acc += (srcA * w) / degreeNorm
			}
			// inhibitory edges
			for _, e := range inhib[dst] {
				srcA := act[e.Src]
				w := math.Abs(effectiveWeight(e))
				acc -= srcA * w
			}
			next[dst] = clamp01(acc)
		}

		act = next
	}
	return act
}

func effectiveWeight(e Edge) float64 {
	w := e.Weight
	if w < 0 {
		w = 0
	}
	// Dimension mix; if all dims are 0, treat as semantic=1.0
	mix := 0.0
	mix += 0.6 * e.DimSemantic
	mix += 0.2 * e.DimTemporal
	mix += 0.2 * e.DimCoactivation
	if mix == 0 {
		mix = 1.0
	}
	out := w * mix
	if out > 1 {
		out = 1
	}
	return out
}

// =============================================================================
// EDGE-TYPE ATTENTION (Phase 3)
// =============================================================================
// Attention-weighted activation spreading that considers different edge types
// with query-aware modulation. Replaces the CO_ACTIVATED_WITH-only spreading
// with weighted contributions from all relationship types.

// EdgeAttentionWeights holds per-edge-type attention weights for activation spreading
type EdgeAttentionWeights struct {
	CoActivated   float64 // Weight for CO_ACTIVATED_WITH edges
	Associated    float64 // Weight for ASSOCIATED_WITH edges
	Generalizes   float64 // Weight for GENERALIZES edges
	AbstractsTo   float64 // Weight for ABSTRACTS_TO edges
	Temporal      float64 // Weight for TEMPORALLY_ADJACENT edges
	AnalogousTo   float64 // Weight for ANALOGOUS_TO edges
	Bridges       float64 // Weight for BRIDGES edges
	ComposesWith  float64 // Weight for COMPOSES_WITH edges
	ContrastsWith float64 // Weight for CONTRASTS_WITH edges
	Influences    float64 // Weight for INFLUENCES edges
	DefinesSymbol float64 // Weight for DEFINES_SYMBOL edges
	ThemeOf       float64 // Weight for THEME_OF edges
	Imports       float64 // Weight for IMPORTS edges
	Calls         float64 // Weight for CALLS edges
	Extends       float64 // Weight for EXTENDS edges
	Implements    float64 // Weight for IMPLEMENTS edges
	GroundedBy    float64 // Weight for GROUNDED_BY edges
}

// QueryContext provides context for attention modulation
type QueryContext struct {
	QueryText   string
	IsCodeQuery bool
	IsArchQuery bool
}

// ComputeEdgeAttention returns attention weights adjusted for query context
func ComputeEdgeAttention(ctx QueryContext, cfg config.Config) EdgeAttentionWeights {
	weights := EdgeAttentionWeights{
		CoActivated: cfg.EdgeAttentionCoActivated,
		Associated:  cfg.EdgeAttentionAssociated,
		Generalizes: cfg.EdgeAttentionGeneralizes,
		AbstractsTo: cfg.EdgeAttentionAbstractsTo,
		Temporal:    cfg.EdgeAttentionTemporal,
	}

	if ctx.IsCodeQuery {
		// Code queries: boost CO_ACTIVATED (practical patterns), reduce hierarchical
		weights.CoActivated *= cfg.EdgeAttentionCodeBoost
		weights.Generalizes *= 0.6
		weights.AbstractsTo *= 0.5
		weights.Associated *= 0.8
	}

	if ctx.IsArchQuery {
		// Architecture queries: boost hierarchical edges, reduce CO_ACTIVATED
		weights.Generalizes *= cfg.EdgeAttentionArchBoost
		weights.AbstractsTo *= cfg.EdgeAttentionArchBoost
		weights.Associated *= 1.2
		weights.CoActivated *= 0.8
	}

	// Set weights for dynamic edge types (Phase 75)
	weights.AnalogousTo = 0.55
	weights.Bridges = 0.60
	weights.ComposesWith = 0.50
	weights.ContrastsWith = 0.40
	weights.Influences = 0.45
	weights.DefinesSymbol = 0.70
	weights.ThemeOf = 0.65

	// Set weights for parser-derived edge types (Phase 75A)
	weights.Imports = 0.50
	weights.Calls = 0.55
	weights.Extends = 0.70
	weights.Implements = 0.70

	// L0 Skip Connection weight (ANN Optimization Phase C)
	weights.GroundedBy = cfg.EdgeAttentionGroundedBy

	// Clamp all weights to [0, 1]
	weights.CoActivated = clampWeight(weights.CoActivated)
	weights.Associated = clampWeight(weights.Associated)
	weights.Generalizes = clampWeight(weights.Generalizes)
	weights.AbstractsTo = clampWeight(weights.AbstractsTo)
	weights.Temporal = clampWeight(weights.Temporal)
	weights.AnalogousTo = clampWeight(weights.AnalogousTo)
	weights.Bridges = clampWeight(weights.Bridges)
	weights.ComposesWith = clampWeight(weights.ComposesWith)
	weights.ContrastsWith = clampWeight(weights.ContrastsWith)
	weights.Influences = clampWeight(weights.Influences)
	weights.DefinesSymbol = clampWeight(weights.DefinesSymbol)
	weights.ThemeOf = clampWeight(weights.ThemeOf)
	weights.Imports = clampWeight(weights.Imports)
	weights.Calls = clampWeight(weights.Calls)
	weights.Extends = clampWeight(weights.Extends)
	weights.Implements = clampWeight(weights.Implements)
	weights.GroundedBy = clampWeight(weights.GroundedBy)

	return weights
}

// clampWeight clamps a weight to [0, 1]
func clampWeight(w float64) float64 {
	if w < 0 {
		return 0
	}
	if w > 1 {
		return 1
	}
	return w
}

// GetEdgeAttention returns the attention weight for a specific edge type
func (w EdgeAttentionWeights) GetEdgeAttention(relType string) float64 {
	switch relType {
	case "CO_ACTIVATED_WITH":
		return w.CoActivated
	case "ASSOCIATED_WITH":
		return w.Associated
	case "GENERALIZES":
		return w.Generalizes
	case "ABSTRACTS_TO":
		return w.AbstractsTo
	case "TEMPORALLY_ADJACENT":
		return w.Temporal
	case "ANALOGOUS_TO":
		return w.AnalogousTo
	case "BRIDGES":
		return w.Bridges
	case "COMPOSES_WITH":
		return w.ComposesWith
	case "CONTRASTS_WITH":
		return w.ContrastsWith
	case "INFLUENCES":
		return w.Influences
	case "DEFINES_SYMBOL":
		return w.DefinesSymbol
	case "THEME_OF":
		return w.ThemeOf
	case "IMPORTS":
		return w.Imports
	case "CALLS":
		return w.Calls
	case "EXTENDS":
		return w.Extends
	case "IMPLEMENTS":
		return w.Implements
	case "GROUNDED_BY":
		return w.GroundedBy
	default:
		return 0.5 // Unknown edge types get neutral weight
	}
}

// DefaultEdgeAttention returns attention weights that match original behavior
// (only CO_ACTIVATED_WITH edges contribute)
func DefaultEdgeAttention() EdgeAttentionWeights {
	return EdgeAttentionWeights{
		CoActivated:   1.0,
		Associated:    0.0,
		Generalizes:   0.0,
		AbstractsTo:   0.0,
		Temporal:      0.0,
		AnalogousTo:   0.0,
		Bridges:       0.0,
		ComposesWith:  0.0,
		ContrastsWith: 0.0,
		Influences:    0.0,
		DefinesSymbol: 0.0,
		ThemeOf:       0.0,
		Imports:       0.0,
		Calls:         0.0,
		Extends:       0.0,
		Implements:    0.0,
		GroundedBy:    0.0,
	}
}

// WeightedEdge combines an Edge with its computed attention weight
type WeightedEdge struct {
	Edge
	AttentionWeight float64
}

// SpreadingActivationWithAttention computes activation with edge-type attention.
// Unlike SpreadingActivation which only uses CO_ACTIVATED_WITH edges, this
// includes all edge types with query-aware attention weighting.
func SpreadingActivationWithAttention(cands []Candidate, edges []Edge, steps int, lambda float64, attention EdgeAttentionWeights, hopMinWeights []float64) map[string]float64 {
	act := map[string]float64{}
	if steps <= 0 {
		steps = 2
	}
	if lambda < 0 {
		lambda = 0
	}
	if lambda > 0.9 {
		lambda = 0.9
	}

	// Seed: ALL candidates seeded from max(VectorSim, BM25Score)
	for _, c := range cands {
		v := c.VectorSim
		if c.BM25Score > v {
			v = c.BM25Score
		}
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		act[c.NodeID] = v
	}

	// Build incoming lists with attention weights
	// KEY CHANGE: Include ALL edge types, not just CO_ACTIVATED_WITH
	incoming := map[string][]WeightedEdge{}
	inhib := map[string][]Edge{}

	for _, e := range edges {
		if e.RelType == "CONTRADICTS" {
			inhib[e.Dst] = append(inhib[e.Dst], e)
			continue
		}

		attnWeight := attention.GetEdgeAttention(e.RelType)
		if attnWeight > 0.01 { // Skip near-zero attention edges
			incoming[e.Dst] = append(incoming[e.Dst], WeightedEdge{
				Edge:            e,
				AttentionWeight: attnWeight,
			})
		}
	}

	clamp01 := func(x float64) float64 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}

	// Propagation steps
	for t := 0; t < steps; t++ {
		next := map[string]float64{}

		// Carry forward existing activations with decay
		for id, a := range act {
			next[id] = clamp01((1 - lambda) * a)
		}

		// Determine minimum weight threshold for this hop
		minWeight := 0.0
		if len(hopMinWeights) > 0 {
			idx := t
			if idx >= len(hopMinWeights) {
				idx = len(hopMinWeights) - 1
			}
			minWeight = hopMinWeights[idx]
		}

		// Apply incoming with attention-weighted aggregation
		for dst, ins := range incoming {
			acc := next[dst]

			// Attention-weighted degree normalization (filtered by hop min weight)
			var totalAttnWeight float64
			for _, we := range ins {
				if effectiveWeight(we.Edge) >= minWeight {
					totalAttnWeight += we.AttentionWeight
				}
			}
			degreeNorm := math.Sqrt(totalAttnWeight)
			if degreeNorm < 1 {
				degreeNorm = 1
			}

			for _, we := range ins {
				w := effectiveWeight(we.Edge)
				if w < minWeight {
					continue
				}
				srcA := act[we.Src]
				// KEY: Apply attention weight to edge contribution
				acc += (srcA * w * we.AttentionWeight) / degreeNorm
			}

			// Inhibitory edges (unchanged from original)
			for _, e := range inhib[dst] {
				srcA := act[e.Src]
				w := math.Abs(effectiveWeight(e))
				acc -= srcA * w
			}

			next[dst] = clamp01(acc)
		}

		act = next
	}

	return act
}
