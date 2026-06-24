package review

// RubricKind distinguishes a rated rubric (per-dimension 0–4, for SFT/guidance)
// from a ranked rubric (chosen/rejected ordering, for DPO).
type RubricKind int

const (
	RubricRated RubricKind = iota
	RubricRanked
)

func (k RubricKind) String() string {
	switch k {
	case RubricRanked:
		return "ranked"
	default:
		return "rated"
	}
}

// Rubric is the versioned grading shape for a dataset. The scoring engine
// (Epic 2) turns a GradeSubmission into the normalized 0–1 gold_score +
// gold_dimensions per this shape.
type Rubric struct {
	Version    string            `json:"version"`            // pinned on every grade (e.g. "gr-v1")
	Kind       RubricKind        `json:"kind"`               // Rated or Ranked
	Dimensions []RubricDimension `json:"dimensions,omitempty"` // for Rated; Ranked ignores this
}

// RubricDimension is one 0–4 rated axis with a WRITTEN anchor per level — a
// reproducible standard, not a subjective vibe.
type RubricDimension struct {
	Key     string    `json:"key"`     // e.g. "relevance", "actionability", "outcome_label_correctness"
	Anchors [5]string `json:"anchors"` // written description for levels 0,1,2,3,4
}
