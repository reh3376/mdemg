package review

import "context"

// Grade is the in-memory representation of a certified review grade — what the
// platform passes to a ReinforcementSink. It carries the graded item so the
// sink can read its Meta (constraint_code, guidance_id, node ids) and the
// gold_dimensions to decide what to apply.
type Grade struct {
	GradeID        string
	DatasetID      string
	ItemID         string
	SpaceID        string
	GraderID       string
	RubricVersion  string
	GoldScore      float64
	GoldDimensions map[string]any
	Item           ReviewItem
}

// ReinforcementSink defines what "apply this grade to the live system" MEANS for
// a dataset. Implementations MUST be idempotent (keyed by Grade.GradeID),
// reversible, and support a dry-run preview. NoopSink makes a dataset gold-only.
type ReinforcementSink interface {
	// SinkID is a stable identifier recorded in the audit detail.
	SinkID() string
	// Preview returns a human-readable description of what Apply WOULD do plus
	// the structured detail Apply would persist — WITHOUT mutating anything.
	Preview(ctx context.Context, g Grade) (ReinforcementPreview, error)
	// Apply mutates the live system per the grade. MUST be idempotent on
	// g.GradeID and MUST capture, in the returned detail, EVERYTHING needed to
	// Reverse it (prior values, created edge ids, the verb). Respects
	// protected-space rules (add/update only; never delete protected nodes).
	Apply(ctx context.Context, g Grade) (ReinforcementDetail, error)
	// Reverse undoes a prior Apply using its persisted detail, restoring prior
	// state. Idempotent: reversing an already-reversed application is a no-op.
	Reverse(ctx context.Context, detail ReinforcementDetail) error
}

// ReinforcementPreview is the dry-run result: what Apply would do, no mutation.
type ReinforcementPreview struct {
	Summary string              `json:"summary"`
	Detail  ReinforcementDetail `json:"detail"`
}

// ReinforcementDetail is the audit + reversal payload persisted in
// review_grades.reinforcement_detail. PriorState is everything needed to undo
// the apply.
type ReinforcementDetail struct {
	SinkID     string         `json:"sink_id"`
	GradeID    string         `json:"grade_id"`
	Verb       string         `json:"verb"` // e.g. "guidance_outcome:followed"
	PriorState map[string]any `json:"prior_state"`
	Applied    map[string]any `json:"applied"`
}

// NonReinforcingApplier is an optional sink capability for datasets whose
// verdicts split into "reinforcing" (mutates the substrate — Apply) and
// "non-reinforcing" (status-only cleanup that touches ONLY the dataset's own
// row, not the substrate — e.g. marking a draft dismissed). Sinks that
// implement this let the auto-grader drain their non-mutating verdicts even
// when reinforce=false; the substrate-mutation invariant is preserved because
// the caller only invokes ApplyNonReinforcing when reinforce=false, and this
// method is contractually restricted to operations that do NOT mutate the
// cognitive substrate (Neo4j nodes, trust scores, edges, embeddings).
//
// Return ok=true if the verdict was HANDLED (status updated, cleanup done);
// ok=false means the verdict requires reinforcement and the caller should skip
// (no auto-effect for that verdict). Sinks whose ALL verdicts are reinforcing
// (or whose ALL verdicts are non-reinforcing) do NOT need this — Apply handles
// them directly under reinforce=true.
//
// HITL-AUTO-DISMISS-001 (2026-08-04).
type NonReinforcingApplier interface {
	ApplyNonReinforcing(ctx context.Context, g Grade) (detail ReinforcementDetail, ok bool, err error)
}

// NoopSink makes a dataset gold-only (grades persist but never touch the live
// system).
type NoopSink struct{}

func (NoopSink) SinkID() string { return "noop" }

func (NoopSink) Preview(_ context.Context, _ Grade) (ReinforcementPreview, error) {
	return ReinforcementPreview{Summary: "no-op (gold-only dataset — grade is not reinforced)"}, nil
}

func (NoopSink) Apply(_ context.Context, g Grade) (ReinforcementDetail, error) {
	return ReinforcementDetail{SinkID: "noop", GradeID: g.GradeID, Verb: "noop"}, nil
}

func (NoopSink) Reverse(_ context.Context, _ ReinforcementDetail) error { return nil }
