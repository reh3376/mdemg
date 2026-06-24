package review

import (
	"context"
	"fmt"
)

// GuidanceReinforcer is the live-substrate primitive set the GuidanceSink needs,
// implemented by an api-layer adapter over jiminy.Service (keeps internal/review
// free of the jiminy import). Outcomes are canonical strings; the adapter maps
// them to jiminy.GuidanceOutcome.
type GuidanceReinforcer interface {
	GetTrust(sessionID string) float64
	RecordTrust(sessionID, outcome string) float64 // EMA toward the outcome anchor
	SetTrust(sessionID string, score float64)       // exact restore (reversal only)
	AdjustConfidence(ctx context.Context, nodeID string, delta float64) error
}

// GuidanceSink applies a certified guidance grade to the LIVE substrate —
// reversibly. It moves per-session J17 trust (EMA) toward the corrected outcome
// and nudges the source node's confidence; the reversal restores the captured
// prior trust and applies the inverse confidence delta.
//
// Scope (disclosed): trust + confidence ship here. The GUIDANCE_OUTCOME edge
// sink — which needs careful create-vs-update + delete-only-self-created logic
// against the constraint node — is a documented follow-up (hitl-review-002).
//
// Idempotency is enforced at the endpoint layer (a non-reversed grade at the
// current rubric_version → 409), so Apply runs at most once per grade.
type GuidanceSink struct {
	R               GuidanceReinforcer
	ConfidenceNudge float64
}

func (GuidanceSink) SinkID() string { return "guidance" }

func (s GuidanceSink) Apply(ctx context.Context, g Grade) (ReinforcementDetail, error) {
	d := ReinforcementDetail{
		SinkID: "guidance", GradeID: g.GradeID,
		PriorState: map[string]any{}, Applied: map[string]any{},
	}
	if s.R == nil {
		return d, fmt.Errorf("guidance sink: reinforcer not wired")
	}
	outcome := correctedOutcome(g)
	if outcome == "" {
		d.Verb = "noop:unclear"
		return d, nil
	}
	session := guidanceSession(g)
	d.Verb = "guidance_outcome:" + outcome

	// Trust — capture prior BEFORE mutating so Reverse can restore it exactly.
	prior := s.R.GetTrust(session)
	newTrust := s.R.RecordTrust(session, outcome)
	d.PriorState["trust"] = prior
	d.PriorState["session"] = session
	d.Applied["trust"] = newTrust

	// Confidence — symmetric nudge (reverse = inverse delta).
	nodeID := g.Item.Meta["source_node_id"]
	if nodeID != "" && s.ConfidenceNudge != 0 {
		delta := s.ConfidenceNudge
		if outcome == "ignored" || outcome == "contradicted" {
			delta = -delta
		}
		if err := s.R.AdjustConfidence(ctx, nodeID, delta); err != nil {
			d.Applied["confidence_error"] = err.Error() // trust still applied + recorded
		} else {
			d.PriorState["confidence_node"] = nodeID
			d.PriorState["confidence_delta"] = delta
			d.Applied["confidence_delta"] = delta
		}
	}
	return d, nil
}

func (s GuidanceSink) Reverse(ctx context.Context, detail ReinforcementDetail) error {
	if s.R == nil {
		return fmt.Errorf("guidance sink: reinforcer not wired")
	}
	// Trust — restore the captured prior exactly.
	if prior, ok := floatFrom(detail.PriorState["trust"]); ok {
		if session, _ := detail.PriorState["session"].(string); session != "" {
			s.R.SetTrust(session, prior)
		}
	}
	// Confidence — apply the inverse delta (only if a delta was applied).
	if node, _ := detail.PriorState["confidence_node"].(string); node != "" {
		if delta, ok := floatFrom(detail.PriorState["confidence_delta"]); ok {
			return s.R.AdjustConfidence(ctx, node, -delta)
		}
	}
	return nil
}

func (s GuidanceSink) Preview(_ context.Context, g Grade) (ReinforcementPreview, error) {
	outcome := correctedOutcome(g)
	if outcome == "" {
		return ReinforcementPreview{Summary: "no reinforcement — outcome_label_correctness is unclear"}, nil
	}
	session := guidanceSession(g)
	var priorTrust float64
	if s.R != nil {
		priorTrust = s.R.GetTrust(session) // read-only, no mutation
	}
	nodeID := g.Item.Meta["source_node_id"]
	summary := fmt.Sprintf("would record outcome %q for session %s (trust now %.3f → EMA toward the %q anchor)",
		outcome, session, priorTrust, outcome)
	if nodeID != "" && s.ConfidenceNudge != 0 {
		summary += fmt.Sprintf("; confidence nudge %+.3f on node %s", s.ConfidenceNudge, nodeID)
	}
	return ReinforcementPreview{
		Summary: summary,
		Detail: ReinforcementDetail{
			SinkID: "guidance", GradeID: g.GradeID, Verb: "guidance_outcome:" + outcome,
			PriorState: map[string]any{"trust": priorTrust, "session": session},
		},
	}, nil
}

// guidanceSession resolves the session to reinforce: the item's session_id,
// falling back to the space id.
func guidanceSession(g Grade) string {
	if s := g.Item.Meta["session_id"]; s != "" {
		return s
	}
	return g.SpaceID
}

// correctedOutcome derives the outcome to reinforce from the grade's
// outcome_label_correctness (0..4) against the item's auto_label:
//   - ≥3 (auto verdict right): affirm the auto_label
//   - ≤1 (auto verdict wrong): invert it
//   - ==2 (unclear): "" → no reinforcement
func correctedOutcome(g Grade) string {
	corr, ok := dimInt(g.GoldDimensions, "outcome_label_correctness")
	if !ok {
		return ""
	}
	auto := normalizeOutcome(g.Item.AutoLabel)
	switch {
	case corr >= 3:
		return auto
	case corr <= 1:
		return invertOutcome(auto)
	default:
		return ""
	}
}

func normalizeOutcome(s string) string {
	switch s {
	case "followed", "partial_compliance", "ignored", "contradicted", "not_applicable":
		return s
	case "partial":
		return "partial_compliance"
	default:
		return s
	}
}

func invertOutcome(s string) string {
	switch s {
	case "followed":
		return "ignored"
	case "ignored", "not_applicable":
		return "followed"
	case "contradicted":
		return "followed"
	default:
		return s // partial/unknown — no clean inverse
	}
}

func dimInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func floatFrom(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
