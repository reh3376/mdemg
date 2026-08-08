package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/jiminy"
	"mdemg/internal/review"
)

func isNoRowsErr(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// HITL-REVIEW-001 Epic 5 — the guidance corpus as the first reviewable dataset.
// FetchCandidates/FetchItem read JIMINY-RELEVANCE-001's guidance_training_rows;
// the sink is the live-reinforcement GuidanceSink. Lives in internal/api (not
// internal/review) because it needs both the TSDB pool and the jiminy service.

type guidanceDataset struct {
	pool          *pgxpool.Pool
	rubricVersion string
	sink          review.GuidanceSink
}

func (d *guidanceDataset) ID() string          { return "guidance" }
func (d *guidanceDataset) DisplayName() string { return "Guidance Corpus" }
func (d *guidanceDataset) Description() string {
	return "Jiminy guidance interactions (the surfaced guidance + the agent's action + the auto-classified outcome). " +
		"JUDGE: relevance, actionability, and whether the auto verdict (followed/ignored) was right. " +
		"Grading here can reinforce the LIVE substrate (trust + node confidence, reversible) and capture corrective guidance."
}
func (d *guidanceDataset) Rubric() review.Rubric {
	return review.GuidanceRubric(d.rubricVersion)
}
func (d *guidanceDataset) Sink() review.ReinforcementSink { return d.sink }

// AutogradePromptHint teaches the autograder guidance-dataset typology: what
// outcome_type values mean, what guidance_type says about the row, and how to
// judge the `outcome_label_correctness` rubric dim against the item's
// AutoLabel + Context. Mirrors contradictedDraftsDataset's per-dataset hint
// pattern (HITL-CURATION-002 E1) — the generic LLM prompt gives noisy grades
// on this dataset because the rubric axis is CORRECTNESS OF THE AUTO-LABEL,
// not correctness of the guidance itself.
//
// HITL-CURATION-003 (2026-08-08); implements review.AutogradePromptHinter.
func (d *guidanceDataset) AutogradePromptHint() string {
	return `You are grading GUIDANCE INTERACTIONS from Jiminy — each item is a
tuple of (surfaced guidance, the agent's next action, an auto-classified
outcome label). The rubric dim ` + "`outcome_label_correctness`" + ` measures
whether the AUTO-LABEL was RIGHT about what happened, NOT whether the
guidance itself was good.

Fields you see:
  * Content         = the guidance text Jiminy surfaced (a constraint /
                      correction / pattern / learning / concept / decision)
  * Context         = the agent's action_summary immediately after the surface
  * AutoLabel       = the classifier's outcome: one of
                      followed | ignored | contradicted | partial_compliance
                      (not_applicable rows are already filtered at write time)
  * Stratum         = the guidance_type — actionable classes are
                      "constraint" and "correction"; abstract classes are
                      "pattern" / "learning" / "concept" / "decision"

Score ` + "`outcome_label_correctness`" + ` (0-4):
  * 4 = auto-label clearly right (action obviously followed / obviously
        ignored the guidance)
  * 3 = auto-label right but action is ambiguous or partial
  * 2 = UNCLEAR — you cannot tell from Content + Context whether the label
        is right; the item lacks the information needed to judge
  * 1 = auto-label likely wrong (action appears to do the opposite of what
        the label claims)
  * 0 = auto-label clearly wrong (action directly contradicts the label)

Common misclassifications the label often gets wrong:
  * Abstract guidance (Stratum in pattern/learning/concept/decision) labeled
    "followed" when the action doesn't actually apply the abstraction — this
    is usually NA-eligible; score LOW (0-1) unless the action clearly does
    apply the abstraction
  * "must_not" constraints labeled "ignored" when the action simply didn't
    touch the mechanism — the rule wasn't violated because it wasn't in
    scope; score LOW (0-1) — a true violation would touch what the rule
    forbids
  * "correction" items where the auto-label rewards the corrected behavior
    even when the action pre-dates the correction being visible — score
    based on whether the action reflects the corrected behavior, not
    whether the correction was in the guidance corpus at action time`
}

// guidanceItemCols selects everything a human SME needs to judge the auto
// verdict: the guidance + action text, plus the auto-classifier's full picture
// (how it was labeled + how confident) and the guidance's provenance.
const guidanceItemCols = `g.row_id, COALESCE(g.guidance_content,''), COALESCE(g.action_summary,''),
	COALESCE(g.guidance_type,''), COALESCE(g.outcome_type,''), COALESCE(g.similarity,0),
	COALESCE(g.constraint_code,''), COALESCE(g.guidance_id,''), COALESCE(g.session_id,''),
	COALESCE(g.source_node_id,''), COALESCE(g.classifier_source,''), COALESCE(g.source_role_type,''),
	g.source_layer, to_char(g.time,'YYYY-MM-DD HH24:MI') `

type guidanceRow struct {
	rowID, content, action, gtype, outcome, ccode, gid, sess, node, clsfr, roleType, when string
	sim   float64
	layer *int
}

func (r guidanceRow) toItem() review.ReviewItem {
	layer := ""
	if r.layer != nil {
		layer = fmt.Sprintf("%d", *r.layer)
	}
	return review.ReviewItem{
		ItemID:    r.rowID,
		Content:   r.content,
		Context:   r.action,
		AutoLabel: r.outcome,
		AutoScore: r.sim,
		Stratum:   r.gtype,
		Signals:   map[string]float64{"similarity": r.sim},
		Meta: map[string]string{
			"constraint_code":   r.ccode,
			"guidance_id":       r.gid,
			"session_id":        r.sess,
			"source_node_id":    r.node,
			"guidance_type":     r.gtype,
			"classifier_source": r.clsfr,         // how the auto-label was produced (llm/tier1/heuristic/explicit)
			"similarity":        fmt.Sprintf("%.3f", r.sim),
			"source_role_type":  r.roleType,      // what the guidance was grounded in
			"source_layer":      layer,
			"recorded_at":       r.when,
		},
	}
}

func scanGuidance(sc interface{ Scan(...any) error }) (guidanceRow, error) {
	var r guidanceRow
	err := sc.Scan(&r.rowID, &r.content, &r.action, &r.gtype, &r.outcome, &r.sim,
		&r.ccode, &r.gid, &r.sess, &r.node, &r.clsfr, &r.roleType, &r.layer, &r.when)
	return r, err
}

// FetchCandidates returns guidance rows not yet graded at the current
// rubric_version (LEFT JOIN review_grades), most recent first.
func (d *guidanceDataset) FetchCandidates(ctx context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	// Order switches on sample strategy (HITL-CURATION-003):
	//   default / "newest"          → DESC (matches interactive-CLI attention)
	//   "oldest-ungraded"           → ASC  (starvation-free backfill for the
	//                                       scheduled autograder — the
	//                                       LEFT JOIN + IS NULL predicate
	//                                       already ensures "ungraded"; the
	//                                       ASC order pulls the oldest first
	//                                       so low-classifier-confidence tail
	//                                       rows aren't permanently starved)
	order := "DESC"
	if q.SampleStrategy == review.SampleStrategyOldestUngraded {
		order = "ASC"
	}
	rows, err := d.pool.Query(ctx, `
		SELECT `+guidanceItemCols+`
		FROM guidance_training_rows g
		LEFT JOIN review_grades r
		  ON r.dataset_id = 'guidance' AND r.item_id = g.row_id
		 AND r.reversed = FALSE AND r.rubric_version = $2
		WHERE g.space_id = $1 AND r.item_id IS NULL
		ORDER BY g.time `+order+`
		LIMIT $3`, q.SpaceID, d.rubricVersion, limit)
	if err != nil {
		return nil, err
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

// FetchItem returns one guidance row by row_id.
func (d *guidanceDataset) FetchItem(ctx context.Context, spaceID, itemID string) (review.ReviewItem, bool, error) {
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

// guidanceReinforcerAdapter implements review.GuidanceReinforcer over
// jiminy.Service (keeps internal/review free of the jiminy import).
type guidanceReinforcerAdapter struct{ svc *jiminy.Service }

func (a guidanceReinforcerAdapter) GetTrust(sessionID string) float64 {
	return a.svc.GetTrustScore(sessionID)
}
func (a guidanceReinforcerAdapter) RecordTrust(sessionID, outcome string) float64 {
	return a.svc.RecordTrustOutcome(sessionID, jiminy.GuidanceOutcome(outcome))
}
func (a guidanceReinforcerAdapter) SetTrust(sessionID string, score float64) {
	a.svc.SetTrustScore(sessionID, score)
}
func (a guidanceReinforcerAdapter) AdjustConfidence(ctx context.Context, nodeID string, delta float64) error {
	return a.svc.AdjustNodeConfidenceDirect(ctx, nodeID, delta)
}
