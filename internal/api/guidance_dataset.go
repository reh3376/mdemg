package api

import (
	"context"
	"errors"

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
func (d *guidanceDataset) Rubric() review.Rubric {
	return review.GuidanceRubric(d.rubricVersion)
}
func (d *guidanceDataset) Sink() review.ReinforcementSink { return d.sink }

const guidanceItemCols = `g.row_id, COALESCE(g.guidance_content,''), COALESCE(g.action_summary,''),
	COALESCE(g.guidance_type,''), COALESCE(g.outcome_type,''), COALESCE(g.similarity,0),
	COALESCE(g.constraint_code,''), COALESCE(g.guidance_id,''), COALESCE(g.session_id,''),
	COALESCE(g.source_node_id,'')`

func scanGuidanceItem(rowID, content, action, gtype, outcome, ccode, gid, sess, node string, sim float64) review.ReviewItem {
	return review.ReviewItem{
		ItemID:    rowID,
		Content:   content,
		Context:   action,
		AutoLabel: outcome,
		AutoScore: sim,
		Stratum:   gtype,
		Signals:   map[string]float64{"similarity": sim},
		Meta: map[string]string{
			"constraint_code": ccode,
			"guidance_id":     gid,
			"session_id":      sess,
			"source_node_id":  node,
			"guidance_type":   gtype,
		},
	}
}

// FetchCandidates returns guidance rows not yet graded at the current
// rubric_version (LEFT JOIN review_grades), most recent first.
func (d *guidanceDataset) FetchCandidates(ctx context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.pool.Query(ctx, `
		SELECT `+guidanceItemCols+`
		FROM guidance_training_rows g
		LEFT JOIN review_grades r
		  ON r.dataset_id = 'guidance' AND r.item_id = g.row_id
		 AND r.reversed = FALSE AND r.rubric_version = $2
		WHERE g.space_id = $1 AND r.item_id IS NULL
		ORDER BY g.time DESC
		LIMIT $3`, q.SpaceID, d.rubricVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []review.ReviewItem
	for rows.Next() {
		var rowID, content, action, gtype, outcome, ccode, gid, sess, node string
		var sim float64
		if err := rows.Scan(&rowID, &content, &action, &gtype, &outcome, &sim, &ccode, &gid, &sess, &node); err != nil {
			return nil, err
		}
		items = append(items, scanGuidanceItem(rowID, content, action, gtype, outcome, ccode, gid, sess, node, sim))
	}
	return items, rows.Err()
}

// FetchItem returns one guidance row by row_id.
func (d *guidanceDataset) FetchItem(ctx context.Context, spaceID, itemID string) (review.ReviewItem, bool, error) {
	var rowID, content, action, gtype, outcome, ccode, gid, sess, node string
	var sim float64
	err := d.pool.QueryRow(ctx, `
		SELECT `+guidanceItemCols+`
		FROM guidance_training_rows g
		WHERE g.row_id = $1
		ORDER BY g.time DESC LIMIT 1`, itemID).
		Scan(&rowID, &content, &action, &gtype, &outcome, &sim, &ccode, &gid, &sess, &node)
	if err != nil {
		if isNoRowsErr(err) {
			return review.ReviewItem{}, false, nil
		}
		return review.ReviewItem{}, false, err
	}
	return scanGuidanceItem(rowID, content, action, gtype, outcome, ccode, gid, sess, node, sim), true, nil
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
