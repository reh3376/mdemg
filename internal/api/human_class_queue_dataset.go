package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// Sprint JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (Q5 §3 #2) — 4th HITL
// dataset, the human-class pending queue. Reads guidance_training_rows
// where outcome_type='pending_human_review' + verifiability_class='human'
// (written by RecordOutcome when JIMINY_HUMAN_CLASS_QUEUE_ENABLED=true).
// Sink writes the operator's grade to constraint_outcomes with
// verifiability_class='human' + classifier_source='operator' — the
// existing GuidanceEffectivenessByClass UNION path picks it up and the
// mdemg_jiminy_follow_rate_human gauge moves off 0.
//
// Auto-grader is REJECTED at the sink by construction: human-class rules
// are the class the LLM cannot verify (that's the taxonomy). Sink refuses
// grader_id LIKE 'auto:%' with a named error (HITL-CURATION-002 invariant
// inversion — every other dataset ALLOWS auto:*; this one is the sole
// operator-only reservation).

type humanClassQueueDataset struct {
	pool          *pgxpool.Pool
	writer        *tsdb.ConstraintOutcomesWriter // sink writes here with class='human'
	rubricVersion string
	instanceID    string
}

func (d *humanClassQueueDataset) ID() string          { return "human_class_queue" }
func (d *humanClassQueueDataset) DisplayName() string { return "Human-class pending queue" }
func (d *humanClassQueueDataset) Description() string {
	return "Rules whose verifiability class is 'human' (18 seeded — meta/process/epistemic " +
		"directives the LLM classifier cannot verify from action-text). Operator grades " +
		"whether the agent's action followed the rule on a single 0-4 dimension. Grade lands " +
		"in constraint_outcomes with verifiability_class='human' + classifier_source='operator'; " +
		"the mdemg_jiminy_follow_rate_human gauge moves as the operator grades. Auto-grader " +
		"REJECTED at the sink by construction (LLM cannot verify by design)."
}
func (d *humanClassQueueDataset) Rubric() review.Rubric {
	return review.HumanClassQueueRubric(d.rubricVersion)
}
func (d *humanClassQueueDataset) Sink() review.ReinforcementSink {
	return humanClassQueueSink{writer: d.writer, instanceID: d.instanceID}
}

// AutogradePromptHint returns "" — the platform's autograder-preview path
// still calls hint-resolution but the sink refuses auto:* grader_ids, so
// no hint is meaningful for this dataset. Kept for interface completeness.
func (d *humanClassQueueDataset) AutogradePromptHint() string {
	return ""
}

// FetchCandidates returns pending human-class rows not yet graded at the
// current rubric_version (LEFT JOIN review_grades). Uses the SAME dedup
// pattern as HITL-CURATION-003 guidanceDataset — a row exits the queue
// automatically the moment operator grade lands in review_grades. No
// source-row mutation needed.
func (d *humanClassQueueDataset) FetchCandidates(ctx context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	order := "DESC"
	if q.SampleStrategy == review.SampleStrategyOldestUngraded {
		order = "ASC"
	}
	rows, err := d.pool.Query(ctx, `
		SELECT `+guidanceItemCols+`
		FROM guidance_training_rows g
		LEFT JOIN review_grades r
		  ON r.dataset_id = 'human_class_queue' AND r.item_id = g.row_id
		 AND r.reversed = FALSE AND r.rubric_version = $2
		WHERE g.space_id = $1
		  AND g.outcome_type = 'pending_human_review'
		  AND g.verifiability_class = 'human'
		  AND r.item_id IS NULL
		ORDER BY g.time `+order+`
		LIMIT $3`, q.SpaceID, d.rubricVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("human_class_queue: fetch candidates: %w", err)
	}
	defer rows.Close()
	var items []review.ReviewItem
	for rows.Next() {
		r, err := scanGuidance(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r.toItem())
	}
	return items, rows.Err()
}

// FetchItem returns one pending row by row_id. Consumes any status —
// operator can revisit an already-graded row for reference.
func (d *humanClassQueueDataset) FetchItem(ctx context.Context, _ string, itemID string) (review.ReviewItem, bool, error) {
	r, err := scanGuidance(d.pool.QueryRow(ctx, `
		SELECT `+guidanceItemCols+`
		FROM guidance_training_rows g
		WHERE g.row_id = $1
		ORDER BY g.time DESC LIMIT 1`, itemID))
	if err != nil {
		if isNoRowsErr(err) {
			return review.ReviewItem{}, false, nil
		}
		return review.ReviewItem{}, false, err
	}
	return r.toItem(), true, nil
}

// humanClassQueueSink translates the operator's `followed` dim to the
// shipped constraint_outcomes outcome_type enum + writes with
// verifiability_class='human'. Auto-grader (grader_id LIKE 'auto:%') is
// refused at Apply — the sink is operator-only by construction.
type humanClassQueueSink struct {
	writer     *tsdb.ConstraintOutcomesWriter
	instanceID string
}

func (humanClassQueueSink) SinkID() string { return "human_class_queue" }

// humanClassOutcomeFor picks the shipped outcome_type enum value from the
// operator's grade. Empty string = "no verdict; defer" — Preview/Apply
// treat that as a no-op (mirrors contradictedDraftsSink dim==2 defer).
func humanClassOutcomeFor(g review.Grade) string {
	v, ok := review.DimInt(g.GoldDimensions, "followed")
	if !ok {
		return ""
	}
	switch {
	case v >= 3:
		return "followed"
	case v <= 1:
		return "ignored"
	default:
		// v == 2 → the anchor says "unclear / not applicable"; skip the
		// substrate write. The operator will grade again if they want a
		// definitive signal.
		return ""
	}
}

// isAutoGrader detects the HITL-CURATION-002 auto:* prefix. Rejecting at
// the sink lets the platform-side autograder machinery still attempt
// (and fail cleanly) so an operator misinvoking `mdemg review autograde
// --dataset human_class_queue` gets a named error instead of silent
// substrate pollution.
func isAutoGrader(g review.Grade) bool {
	return strings.HasPrefix(g.GraderID, review.AutoGraderPrefix)
}

// errAutograderRejected is the named error returned when Apply refuses
// an auto:* grader_id. Package-level sentinel so tests can errors.Is
// against it. HITL-ERROR-VISIBLE-001 (2026-09-20) wrapped this in a
// clientVisibleError so the taxonomy-citing text surfaces to the API
// client (was previously opaque "internal error during …" per the
// sanitizeError default).
var errAutograderRejected = &clientVisibleError{
	msg: "human_class_queue: auto-grader rejected — human-verifiability class is " +
		"operator-only by construction (LLM cannot verify these rules; see " +
		"JIMINY-METRIC-DENOMINATOR-DESIGN-001 taxonomy)",
}

// clientVisibleError implements the ClientVisibleError interface (defined
// in server.go). Sinks/handlers use this to opt errors into surfacing at
// the API boundary while every other error path remains sanitized.
type clientVisibleError struct {
	msg string
}

func (e *clientVisibleError) Error() string          { return e.msg }
func (e *clientVisibleError) ClientVisible() string  { return e.msg }

func (s humanClassQueueSink) Preview(_ context.Context, g review.Grade) (review.ReinforcementPreview, error) {
	if isAutoGrader(g) {
		return review.ReinforcementPreview{}, errAutograderRejected
	}
	outcome := humanClassOutcomeFor(g)
	if outcome == "" {
		return review.ReinforcementPreview{
			Summary: fmt.Sprintf("no-op: grade dim `followed` unclear (2) — no substrate write for grade %s", g.GradeID),
			Detail: review.ReinforcementDetail{
				SinkID: "human_class_queue", GradeID: g.GradeID, Verb: "human_outcome:defer",
			},
		}, nil
	}
	code := g.Item.Meta["constraint_code"]
	if code == "" {
		code = "(unknown-code)"
	}
	return review.ReinforcementPreview{
		Summary: fmt.Sprintf(
			"would write constraint_outcomes row: outcome_type=%s, verifiability_class=human, "+
				"classifier_source=operator, constraint_code=%s, guidance_id=%s. "+
				"Row appears in the human-class follow-rate gauge on the next assessment tick.",
			outcome, code, g.Item.Meta["guidance_id"]),
		Detail: review.ReinforcementDetail{
			SinkID: "human_class_queue", GradeID: g.GradeID, Verb: "human_outcome:" + outcome,
		},
	}, nil
}

func (s humanClassQueueSink) Apply(_ context.Context, g review.Grade) (review.ReinforcementDetail, error) {
	if isAutoGrader(g) {
		return review.ReinforcementDetail{}, errAutograderRejected
	}
	outcome := humanClassOutcomeFor(g)
	if outcome == "" {
		// No-op: dim==2 defer. Detail captured so a subsequent grade at
		// dim<=1 or >=3 is a fresh apply (idempotency keyed on GradeID).
		return review.ReinforcementDetail{
			SinkID: "human_class_queue", GradeID: g.GradeID, Verb: "human_outcome:defer",
		}, nil
	}
	// Fields per Grade.Item.Meta shape from guidanceRow.toItem(). Missing
	// meta keys fall back to empty string — the writer accepts empty
	// constraint_id/code; the row still contributes to the class gauge.
	if s.writer == nil {
		return review.ReinforcementDetail{}, fmt.Errorf("human_class_queue: constraint_outcomes writer not wired")
	}
	s.writer.RecordOutcome(
		g.SpaceID,
		g.Item.Meta["source_node_id"],  // constraintID: the source node the operator judged
		g.Item.Meta["constraint_code"], // constraintCode
		g.Item.Meta["guidance_id"],
		g.Item.Meta["session_id"],
		outcome, // outcome_type: followed / ignored (partial_compliance not used at v1)
		g.Item.Meta["guidance_type"],
		s.instanceID,
		"operator", // classifier_source
		"human",    // verifiability_class
		g.GoldScore,
	)
	return review.ReinforcementDetail{
		SinkID:  "human_class_queue",
		GradeID: g.GradeID,
		Verb:    "human_outcome:" + outcome,
		Applied: map[string]any{
			"constraint_outcome_row": map[string]any{
				"outcome_type":        outcome,
				"verifiability_class": "human",
				"classifier_source":   "operator",
				"constraint_code":     g.Item.Meta["constraint_code"],
				"guidance_id":         g.Item.Meta["guidance_id"],
				"space_id":            g.SpaceID,
			},
		},
	}, nil
}

// Reverse: no-op. constraint_outcomes rows are append-only; reversing a
// grade is handled by writing an offsetting operator grade if the operator
// changes their mind. The sink does not delete substrate rows.
// (Symmetric with GuidanceSink's approach for the guidance dataset — trust
// EMA is restored on reverse, but the outcome-writer rows are historical.)
func (humanClassQueueSink) Reverse(_ context.Context, _ review.ReinforcementDetail) error {
	return nil
}
