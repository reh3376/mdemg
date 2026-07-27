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
	writer        contradictedDraftsWriterIface
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

// AutogradePromptHint teaches the autograder what a "durable rule" is vs
// what it isn't — the ContradictedDraftsRubric anchors describe the SCORE
// scale but not the TYPOLOGY, and the 2026-07-27 E1 dry-run showed the model
// scoring a "DATAPRUNE EXECUTED" log line as durable_rule=4/conf=0.90
// (high-confidence + structurally-wrong). This hint fixes that class.
// HITL-CURATION-002 E1 prompt tuning; implements review.AutogradePromptHinter.
func (d *contradictedDraftsDataset) AutogradePromptHint() string {
	return `You are grading DRAFT CORRECTIONS synthesized from a "contradicted-outcome"
verdict — i.e. Jiminy issued guidance, the agent's action contradicted it,
and this draft is a proposed CORRECTION rule capturing the lesson.

A DURABLE RULE describes an INVARIANT the agent should follow in FUTURE
work: "Always do X", "Never Y", "Prefer Z when W". It is prescriptive,
context-general, and time-independent.

The following are NOT durable rules — score their durable_rule dimension
LOW (0-1) EVEN IF they look well-formed:
  * Bash errors, command output, session traces (e.g. "Bash error in command: ...")
  * Completion logs (e.g. "EXECUTED: deleted N rows", "Sprint X COMPLETE", "SHIPPED 2026-...")
  * One-off decisions specific to a single moment ("I chose to skip Y today")
  * Sprint post-mortems, status updates, milestone announcements
  * Data dumps, JSON blobs, config snapshots
  * Content that describes what WAS done, not what SHOULD be done

The Content field is the proposed correction TEXT; the Context field is the
offending action that triggered the draft. If the Content reads as a rule
someone would want on their team's checklist, it scores durable_rule HIGH
(3-4). If it reads as a log line or a session artifact, it scores LOW (0-1).

phrasing_quality is a SEPARATE axis — the item can be a bad rule but well
phrased, or a good rule needing edits. Do not conflate.`
}

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

// contradictedDraftsWriterIface is the subset of *tsdb.ContradictedDraftsWriter
// methods the sink calls, extracted so tests can inject a capturing mock.
// *tsdb.ContradictedDraftsWriter satisfies this by shape.
type contradictedDraftsWriterIface interface {
	MarkApproved(ctx context.Context, id, appliedObsID, appliedNodeID string) error
	MarkDismissed(ctx context.Context, id string) error
	ResetToPending(ctx context.Context, id string) error
	FetchPendingBySpace(ctx context.Context, spaceID string, limit int) ([]tsdb.ContradictedDraftRow, error)
	FetchByID(ctx context.Context, id string) (*tsdb.ContradictedDraftRow, error)
}

// contradictedDraftsSink applies a certified draft grade: on approve it hands
// the draft's Incorrect/Correct pair to conversation.Service.Correct (mints an
// L0 correction obs); on dismiss it marks the draft dismissed. Reverse flips
// the draft back to pending — the L0 obs it created (if any) stays and must
// be tombstoned separately if the operator wants full undo.
type contradictedDraftsSink struct {
	svc    CorrectService
	writer contradictedDraftsWriterIface
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
