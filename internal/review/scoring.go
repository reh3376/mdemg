package review

import "fmt"

// GradeSubmission is the raw human input for one grade, validated + scored
// against a dataset's Rubric by Rubric.Score.
type GradeSubmission struct {
	DatasetID string
	ItemID    string
	// Rated rubrics: dimension key → 0..4 score.
	Dimensions map[string]int
	// Ranked (DPO) rubrics: the chosen/rejected alternative ids + an optional
	// 0..4 confidence.
	ChosenAltID   string
	RejectedAltID string
	Confidence    int // 0..4; 0 = unset → treated as max confidence
}

// Score validates a submission against the rubric SHAPE and returns the
// normalized gold_score ∈ [0,1] + the gold_dimensions object persisted as jsonb.
//
//   - Rated:  every rubric dimension must be scored 0..4; gold = mean(scores)/4.
//   - Ranked: a chosen ≠ rejected alt id is required; gold encodes the ordering
//     (chosen preferred), scaled by confidence; dims = {chosen, rejected, confidence}.
//
// A submission whose shape mismatches the rubric kind (ranked input to a rated
// rubric, or vice-versa) is rejected — the endpoint maps this to a 400.
func (r Rubric) Score(g GradeSubmission) (gold float64, dims map[string]any, err error) {
	switch r.Kind {
	case RubricRated:
		if g.ChosenAltID != "" || g.RejectedAltID != "" {
			return 0, nil, fmt.Errorf("rated rubric %q: got a ranked submission (chosen/rejected set)", r.Version)
		}
		if len(r.Dimensions) == 0 {
			return 0, nil, fmt.Errorf("rated rubric %q has no dimensions", r.Version)
		}
		sum := 0
		out := make(map[string]any, len(r.Dimensions)+1)
		for _, dim := range r.Dimensions {
			v, ok := g.Dimensions[dim.Key]
			if !ok {
				return 0, nil, fmt.Errorf("rated rubric %q: missing dimension %q", r.Version, dim.Key)
			}
			if v < 0 || v > 4 {
				return 0, nil, fmt.Errorf("dimension %q score %d out of range [0,4]", dim.Key, v)
			}
			sum += v
			out[dim.Key] = v
		}
		gold = float64(sum) / float64(len(r.Dimensions)*4)
		out["_kind"] = "rated"
		return gold, out, nil

	case RubricRanked:
		if len(g.Dimensions) > 0 {
			return 0, nil, fmt.Errorf("ranked rubric %q: got a rated submission (dimensions set)", r.Version)
		}
		if g.ChosenAltID == "" || g.RejectedAltID == "" {
			return 0, nil, fmt.Errorf("ranked rubric %q: chosen and rejected alt ids are required", r.Version)
		}
		if g.ChosenAltID == g.RejectedAltID {
			return 0, nil, fmt.Errorf("ranked rubric %q: chosen and rejected must differ", r.Version)
		}
		conf := g.Confidence
		if conf < 0 || conf > 4 {
			return 0, nil, fmt.Errorf("ranked confidence %d out of range [0,4]", conf)
		}
		// gold encodes the preference strength: a clear pick (conf unset/0 → max)
		// is 1.0; lower confidence pulls toward 0.5 (no preference).
		if conf == 0 {
			gold = 1.0
		} else {
			gold = 0.5 + 0.5*float64(conf)/4.0
		}
		return gold, map[string]any{
			"_kind":      "ranked",
			"chosen":     g.ChosenAltID,
			"rejected":   g.RejectedAltID,
			"confidence": conf,
		}, nil

	default:
		return 0, nil, fmt.Errorf("unknown rubric kind %v", r.Kind)
	}
}

// LLMOutputRubric is the rated rubric for reviewing an LLM call site's output —
// shared across all 16 MDEMG call sites. Reviewing produces gold SFT/quality
// data for the recursive-retraining loop.
func LLMOutputRubric(version string) Rubric {
	if version == "" {
		version = "gr-v1"
	}
	return Rubric{
		Version: version,
		Kind:    RubricRated,
		Dimensions: []RubricDimension{
			{Key: "correctness", Anchors: [5]string{
				"wrong — the output is incorrect / hallucinated",
				"mostly wrong",
				"mixed — partly right",
				"mostly correct",
				"correct — fully right for this input",
			}},
			{Key: "format_validity", Anchors: [5]string{
				"malformed — wrong shape / unparseable",
				"major format issues",
				"parseable with issues",
				"valid with minor nits",
				"perfectly-formed for the call site's contract",
			}},
			{Key: "helpfulness", Anchors: [5]string{
				"useless — adds nothing / misleading",
				"weak",
				"adequate",
				"good",
				"excellent — exactly what the call site needs",
			}},
		},
	}
}

// GuidanceRubric is the rated rubric for the guidance corpus dataset. The
// outcome_label_correctness dimension is what the guidance reinforcement sink
// (Epic 5) reads to derive the corrected outcome. Anchors are the reproducible
// standard — see docs/development/hitl-review-001/rubric_v1.md.
func GuidanceRubric(version string) Rubric {
	if version == "" {
		version = "gr-v1"
	}
	return Rubric{
		Version: version,
		Kind:    RubricRated,
		Dimensions: []RubricDimension{
			{Key: "relevance", Anchors: [5]string{
				"off-topic — unrelated to the agent's task",
				"tangential — same area, not this task",
				"related — touches the task obliquely",
				"on-topic — clearly about this task",
				"precise — directly addresses this exact task",
			}},
			{Key: "actionability", Anchors: [5]string{
				"advisory prose — no action implied",
				"vague principle — no concrete step",
				"general direction — a step could be inferred",
				"specific guidance — a clear step is implied",
				"executable directive — names a specific, executable action",
			}},
			{Key: "outcome_label_correctness", Anchors: [5]string{
				"exactly wrong — the auto verdict is the opposite of the truth",
				"mostly wrong",
				"unclear — the auto verdict is defensible either way",
				"mostly right",
				"exactly right — the auto verdict matches what the agent actually did",
			}},
		},
	}
}
