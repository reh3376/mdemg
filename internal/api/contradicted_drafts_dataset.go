package api

import (
	"context"
	"fmt"

	"mdemg/internal/conversation"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// Sprint JIMINY-CONTRADICTED-BRIDGE-001 Epic 3: the contradicted-outcome
// correction-draft dataset. FetchCandidates reads pending V0030 drafts via
// the tsdb writer's flush-then-query API; the sink hands approved drafts to
// conversation.Service.Correct — the L0 obs it mints is then promoted to an
// L1 role_type='correction' node by JIMINY-CORRECTION-PRODUCER-001 on the
// next consolidation cycle. Lives in internal/api because it needs both the
// tsdb writer AND the conversation service.

// contradictedDraftsDataset implements review.ReviewableDataset.
type contradictedDraftsDataset struct {
	writer        *tsdb.ContradictedDraftsWriter
	rubricVersion string
	sink          contradictedDraftsSink
}

func (d *contradictedDraftsDataset) ID() string          { return "contradicted_drafts" }
func (d *contradictedDraftsDataset) DisplayName() string { return "Contradicted-outcome correction drafts" }
func (d *contradictedDraftsDataset) Description() string {
	return "Draft corrections auto-generated from Jiminy contradicted-outcome verdicts. " +
		"JUDGE: durable_rule (is this a rule worth remembering?) and phrasing_quality (edit as needed). " +
		"Approve (durable_rule >= 3) mints an L0 correction obs via /v1/conversation/correct — " +
		"JIMINY-CORRECTION-PRODUCER-001 then promotes to L1 on the next consolidation."
}
func (d *contradictedDraftsDataset) Rubric() review.Rubric {
	return review.ContradictedDraftsRubric(d.rubricVersion)
}
func (d *contradictedDraftsDataset) Sink() review.ReinforcementSink { return d.sink }

// FetchCandidates returns pending drafts for the given space, newest first.
// Reads via the writer so buffered rows are visible (flush-then-query).
func (d *contradictedDraftsDataset) FetchCandidates(ctx context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	if d.writer == nil {
		return nil, fmt.Errorf("contradicted_drafts: writer not wired")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.writer.FetchPendingBySpace(ctx, q.SpaceID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]review.ReviewItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, contradictedDraftItem(r))
	}
	return items, nil
}

// FetchItem returns a single draft by id (any status).
func (d *contradictedDraftsDataset) FetchItem(ctx context.Context, _ string, itemID string) (review.ReviewItem, bool, error) {
	if d.writer == nil {
		return review.ReviewItem{}, false, fmt.Errorf("contradicted_drafts: writer not wired")
	}
	r, err := d.writer.FetchByID(ctx, itemID)
	if err != nil {
		return review.ReviewItem{}, false, err
	}
	if r == nil {
		return review.ReviewItem{}, false, nil
	}
	return contradictedDraftItem(*r), true, nil
}

func contradictedDraftItem(r tsdb.ContradictedDraftRow) review.ReviewItem {
	return review.ReviewItem{
		ItemID:    r.ID,
		Content:   r.DraftCorrect,   // the proposed "correct" — what the operator will judge
		Context:   r.DraftIncorrect, // the observed "incorrect" — the action that violated
		AutoLabel: r.Status,
		AutoScore: r.Similarity,
		Stratum:   r.GuidanceType,
		Signals:   map[string]float64{"similarity": r.Similarity},
		Meta: map[string]string{
			"guidance_id":      r.GuidanceID,
			"guidance_type":    r.GuidanceType,
			"guidance_content": r.GuidanceContent,
			"action_summary":   r.ActionSummary,
			"source_node_id":   r.SourceNodeID,
			"session_id":       r.SessionID,
			"draft_incorrect":  r.DraftIncorrect,
			"draft_correct":    r.DraftCorrect,
			"action_hash":      r.ActionHash,
			"status":           r.Status,
			"applied_obs_id":   r.AppliedObsID,
			"applied_node_id":  r.AppliedNodeID,
			"recorded_at":      r.Time.Format("2006-01-02 15:04"),
		},
	}
}

// CorrectService is the sink's dependency on the conversation service — kept
// as an interface so tests can inject a mock.
type CorrectService interface {
	Correct(ctx context.Context, req conversation.CorrectRequest) (*conversation.ObserveResponse, error)
}

// contradictedDraftsSink applies a certified draft grade: on approve it hands
// the draft's Incorrect/Correct pair to conversation.Service.Correct (mints an
// L0 correction obs); on dismiss it marks the draft dismissed. Reverse flips
// the draft back to pending — the L0 obs it created (if any) stays and must
// be tombstoned separately if the operator wants full undo.
type contradictedDraftsSink struct {
	svc    CorrectService
	writer *tsdb.ContradictedDraftsWriter
}

func (contradictedDraftsSink) SinkID() string { return "contradicted_drafts" }

// draftVerdict picks the operator's decision from the rubric. durable_rule >= 3
// → approve; <= 1 → dismiss; ==2 (or missing) → "" (no-op / defer).
func draftVerdict(g review.Grade) string {
	corr, ok := review.DimInt(g.GoldDimensions, "durable_rule")
	if !ok {
		return ""
	}
	switch {
	case corr >= 3:
		return "approve"
	case corr <= 1:
		return "dismiss"
	default:
		return ""
	}
}

func (s contradictedDraftsSink) Preview(_ context.Context, g review.Grade) (review.ReinforcementPreview, error) {
	verdict := draftVerdict(g)
	switch verdict {
	case "approve":
		return review.ReinforcementPreview{
			Summary: fmt.Sprintf("would call conversation.Correct with Incorrect=%q Correct=%q (creates an L0 obs_type='correction' MemoryNode; the next consolidation cycle promotes to L1 role_type='correction'). Draft %s → status='approved'.",
				g.Item.Meta["draft_incorrect"], g.Item.Meta["draft_correct"], g.ItemID),
			Detail: review.ReinforcementDetail{
				SinkID: "contradicted_drafts", GradeID: g.GradeID, Verb: "correction_draft:approve",
			},
		}, nil
	case "dismiss":
		return review.ReinforcementPreview{
			Summary: fmt.Sprintf("would mark draft %s status='dismissed' (no substrate mutation).", g.ItemID),
			Detail: review.ReinforcementDetail{
				SinkID: "contradicted_drafts", GradeID: g.GradeID, Verb: "correction_draft:dismiss",
			},
		}, nil
	default:
		return review.ReinforcementPreview{
			Summary: "no action — durable_rule is unclear (score 2) or missing; draft stays pending.",
		}, nil
	}
}

func (s contradictedDraftsSink) Apply(ctx context.Context, g review.Grade) (review.ReinforcementDetail, error) {
	d := review.ReinforcementDetail{
		SinkID: "contradicted_drafts", GradeID: g.GradeID,
		PriorState: map[string]any{"draft_id": g.ItemID, "prior_status": g.Item.Meta["status"]},
		Applied:    map[string]any{},
	}
	if s.writer == nil {
		return d, fmt.Errorf("contradicted_drafts sink: writer not wired")
	}
	verdict := draftVerdict(g)
	switch verdict {
	case "approve":
		if s.svc == nil {
			return d, fmt.Errorf("contradicted_drafts sink: CorrectService not wired")
		}
		req := conversation.CorrectRequest{
			SpaceID:   g.SpaceID,
			SessionID: g.Item.Meta["session_id"],
			Incorrect: g.Item.Meta["draft_incorrect"],
			Correct:   g.Item.Meta["draft_correct"],
			Context:   g.Item.Meta["guidance_content"],
		}
		resp, err := s.svc.Correct(ctx, req)
		if err != nil {
			return d, fmt.Errorf("contradicted_drafts sink: conversation.Correct: %w", err)
		}
		var obsID, nodeID string
		if resp != nil {
			obsID = resp.ObsID
			nodeID = resp.NodeID
			d.Applied["obs_id"] = resp.ObsID
			d.Applied["node_id"] = resp.NodeID
		}
		if err := s.writer.MarkApproved(ctx, g.ItemID, obsID, nodeID); err != nil {
			// The L0 obs is already created; the status flip failing is
			// non-fatal (a subsequent grade retry will re-mark; DB uniqueness
			// isn't broken because MarkApproved is UPDATE, not INSERT). Log
			// the residue in Applied so ops can spot it.
			d.Applied["status_mark_error"] = err.Error()
		}
		d.Verb = "correction_draft:approve"
		return d, nil
	case "dismiss":
		if err := s.writer.MarkDismissed(ctx, g.ItemID); err != nil {
			return d, fmt.Errorf("contradicted_drafts sink: MarkDismissed: %w", err)
		}
		d.Verb = "correction_draft:dismiss"
		return d, nil
	default:
		d.Verb = "noop:unclear"
		return d, nil
	}
}

func (s contradictedDraftsSink) Reverse(ctx context.Context, detail review.ReinforcementDetail) error {
	if s.writer == nil {
		return fmt.Errorf("contradicted_drafts sink: writer not wired")
	}
	draftID, _ := detail.PriorState["draft_id"].(string)
	if draftID == "" {
		return nil
	}
	// Reset draft status to pending regardless of the applied verb. The L0
	// obs that Apply may have created is intentionally left in place — the
	// operator uses mdemg concepts tombstone to remove it if desired.
	return s.writer.ResetToPending(ctx, draftID)
}
