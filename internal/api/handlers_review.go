package api

import (
	"encoding/json"
	"net/http"

	"github.com/nrednav/cuid2"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// reviewGradeRow maps a review.Grade (+ marshalled jsonb + audit fields) to the
// tsdb row shape (keeps internal/tsdb free of internal/review). reversesGradeID
// non-empty marks a reversal row; the original is flagged via MarkReversed.
func reviewGradeRow(g review.Grade, dimsJSON string, applied bool, detailJSON, reversesGradeID, instanceID, suggested string) tsdb.ReviewGradeRow {
	return tsdb.ReviewGradeRow{
		GradeID:                 g.GradeID,
		DatasetID:               g.DatasetID,
		ItemID:                  g.ItemID,
		SpaceID:                 g.SpaceID,
		GoldScore:               g.GoldScore,
		GoldDimensionsJSON:      dimsJSON,
		GraderID:                g.GraderID,
		RubricVersion:           g.RubricVersion,
		ReinforcementApplied:    applied,
		ReinforcementDetailJSON: detailJSON,
		Reversed:                false,
		ReversesGradeID:         reversesGradeID,
		InstanceID:              instanceID,
		SuggestedGuidance:       suggested,
	}
}

// HITL-REVIEW-001 Epic 4 — /v1/review/* endpoints. All admin-gated
// (auth.ScopeAdminSpaces) at registration; REVIEW_ENABLED=false → 503.

func (s *Server) reviewReady(w http.ResponseWriter) bool {
	if !s.cfg.ReviewEnabled || s.reviewRegistry == nil || s.reviewWriter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "review platform is not enabled (set REVIEW_ENABLED=true)",
		})
		return false
	}
	return true
}

func (s *Server) sampler() review.Sampler {
	return review.Sampler{
		SampleSize:      s.cfg.ReviewSampleSize,
		UncertaintyBand: s.cfg.ReviewActiveUncertaintyBand,
		Seed:            s.cfg.ReviewSampleSeed,
	}
}

// GET /v1/review/datasets
func (s *Server) handleReviewDatasets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.reviewReady(w) {
		return
	}
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		spaceID = s.cfg.RSICWatchdogSpaceID
	}
	out := make([]map[string]any, 0)
	for _, d := range s.reviewRegistry.List() {
		rb := d.Rubric()
		cands, _ := d.FetchCandidates(r.Context(), review.CandidateQuery{
			SpaceID: spaceID, Limit: s.cfg.ReviewSampleSize, ExcludeGraded: true,
		})
		desc := ""
		if dd, ok := d.(interface{ Description() string }); ok {
			desc = dd.Description()
		}
		out = append(out, map[string]any{
			"id":              d.ID(),
			"display_name":    d.DisplayName(),
			"description":     desc,
			"rubric_version":  rb.Version,
			"rubric_kind":     rb.Kind.String(),
			"candidate_count": len(cands),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"datasets":                out,
			"current_rubric_version":  s.cfg.ReviewRubricVersion,
			"reinforce_default":       s.cfg.ReviewReinforceDefault,
		},
	})
}

// GET /v1/review/next?dataset_id=&space_id=
func (s *Server) handleReviewNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.reviewReady(w) {
		return
	}
	datasetID := r.URL.Query().Get("dataset_id")
	d, ok := s.reviewRegistry.Get(datasetID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown dataset_id"})
		return
	}
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		spaceID = s.cfg.RSICWatchdogSpaceID
	}
	cands, err := d.FetchCandidates(r.Context(), review.CandidateQuery{
		SpaceID: spaceID, Limit: s.cfg.ReviewSampleSize,
		UncertaintyBand: s.cfg.ReviewActiveUncertaintyBand, ExcludeGraded: true,
	})
	if err != nil {
		writeInternalError(w, err, "review fetch candidates")
		return
	}
	selected := s.sampler().Select(cands)
	if len(selected) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"item": nil, "message": "no candidates to review"},
		})
		return
	}
	item := selected[0]
	rb := d.Rubric()
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"item":           item,
			"rubric":         rb,
			"auto_label":     item.AutoLabel,
			"graded_count":   0,
			"sample_size":    s.cfg.ReviewSampleSize,
		},
	})
}

type reviewGradeRequest struct {
	DatasetID  string         `json:"dataset_id"`
	ItemID     string         `json:"item_id"`
	SpaceID    string         `json:"space_id"`
	GraderID   string         `json:"grader_id"`
	Dimensions map[string]int `json:"dimensions,omitempty"`
	Ranking    struct {
		Chosen     string `json:"chosen"`
		Rejected   string `json:"rejected"`
		Confidence int    `json:"confidence"`
	} `json:"ranking,omitempty"`
	Reinforce         *bool  `json:"reinforce,omitempty"`
	DryRun            *bool  `json:"dry_run,omitempty"`
	Force             bool   `json:"force,omitempty"`
	SuggestedGuidance string `json:"suggested_guidance,omitempty"` // SME corrective example
}

// POST /v1/review/grade
func (s *Server) handleReviewGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.reviewReady(w) {
		return
	}
	var req reviewGradeRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.DatasetID == "" || req.ItemID == "" || req.GraderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dataset_id, item_id, grader_id are required"})
		return
	}
	d, ok := s.reviewRegistry.Get(req.DatasetID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown dataset_id"})
		return
	}
	if req.SpaceID == "" {
		req.SpaceID = s.cfg.RSICWatchdogSpaceID
	}
	rb := d.Rubric()
	sub := review.GradeSubmission{
		DatasetID: req.DatasetID, ItemID: req.ItemID,
		Dimensions:    req.Dimensions,
		ChosenAltID:   req.Ranking.Chosen,
		RejectedAltID: req.Ranking.Rejected,
		Confidence:    req.Ranking.Confidence,
	}
	gold, dims, err := rb.Score(sub)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	item, _, _ := d.FetchItem(r.Context(), req.SpaceID, req.ItemID)
	grade := review.Grade{
		GradeID: cuid2.Generate(), DatasetID: req.DatasetID, ItemID: req.ItemID,
		SpaceID: req.SpaceID, GraderID: req.GraderID, RubricVersion: rb.Version,
		GoldScore: gold, GoldDimensions: dims, Item: item,
	}

	dryRun := false // writes happen unless the caller explicitly requests a dry run
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	reinforce := s.cfg.ReviewReinforceDefault
	if req.Reinforce != nil {
		reinforce = *req.Reinforce
	}

	// Dry-run: return the sink preview WITHOUT persisting or mutating.
	if dryRun {
		var preview review.ReinforcementPreview
		if reinforce {
			preview, _ = d.Sink().Preview(r.Context(), grade)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"dry_run": true, "gold_score": gold, "gold_dimensions": dims,
				"would_reinforce": reinforce, "preview": preview,
			},
		})
		return
	}

	// Idempotency: a non-reversed grade at the current rubric_version exists → 409.
	existing, err := s.reviewWriter.LatestGradeForItem(r.Context(), req.DatasetID, req.ItemID)
	if err != nil {
		writeInternalError(w, err, "review idempotency check")
		return
	}
	if existing != nil && existing.RubricVersion == rb.Version && !req.Force {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "item already graded at the current rubric_version (send force:true to re-grade)",
			"existing_grade_id": existing.GradeID,
		})
		return
	}

	applied := false
	detailJSON := ""
	if reinforce {
		detail, aerr := d.Sink().Apply(r.Context(), grade)
		if aerr != nil {
			writeInternalError(w, aerr, "review reinforcement apply")
			return
		}
		applied = true
		if b, mErr := json.Marshal(detail); mErr == nil {
			detailJSON = string(b)
		}
	}

	dimsJSON, _ := json.Marshal(dims)
	s.reviewWriter.Record(reviewGradeRow(grade, string(dimsJSON), applied, detailJSON, "", s.cfg.InstanceID, req.SuggestedGuidance))

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"grade_id": grade.GradeID, "gold_score": gold,
			"reinforcement_applied": applied,
		},
	})
}

// POST /v1/review/reverse  {grade_id}
func (s *Server) handleReviewReverse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.reviewReady(w) {
		return
	}
	var req struct {
		GradeID string `json:"grade_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.GradeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "grade_id is required"})
		return
	}
	row, err := s.reviewWriter.GradeByID(r.Context(), req.GradeID)
	if err != nil {
		writeInternalError(w, err, "review grade lookup")
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown grade_id"})
		return
	}
	if row.Reversed {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "grade already reversed"})
		return
	}
	// Undo the live reinforcement (if any) via the dataset's sink.
	if row.ReinforcementApplied && row.ReinforcementDetailJSON != "" {
		d, ok := s.reviewRegistry.Get(row.DatasetID)
		if ok {
			var detail review.ReinforcementDetail
			if uErr := json.Unmarshal([]byte(row.ReinforcementDetailJSON), &detail); uErr == nil {
				if rErr := d.Sink().Reverse(r.Context(), detail); rErr != nil {
					writeInternalError(w, rErr, "review reinforcement reverse")
					return
				}
			}
		}
	}
	// Write the reversal row + mark the original reversed.
	rev := review.Grade{
		GradeID: cuid2.Generate(), DatasetID: row.DatasetID, ItemID: row.ItemID,
		SpaceID: row.SpaceID, GraderID: "reversal", RubricVersion: row.RubricVersion,
		GoldScore: row.GoldScore,
	}
	s.reviewWriter.Record(reviewGradeRow(rev, "{}", false, "", req.GradeID, s.cfg.InstanceID, ""))
	if err := s.reviewWriter.MarkReversed(r.Context(), req.GradeID); err != nil {
		writeInternalError(w, err, "review mark reversed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"reversed": true, "grade_id": req.GradeID},
	})
}
