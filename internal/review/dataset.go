// Package review is the general-purpose Human-in-the-Loop dataset review +
// live-reinforcement platform (HITL-REVIEW-001). A dataset implements
// ReviewableDataset once; the platform code (sampling, endpoints, persistence,
// reinforcement) is written once against the interface. The headline capability
// is that a certified human grade can reinforce the LIVE cognitive substrate —
// auditably, idempotently, and reversibly — not just sit as gold for a future
// retrain.
package review

import "context"

// ReviewableDataset is implemented once per dataset type (guidance, sft, dpo, …).
type ReviewableDataset interface {
	// ID is the stable dataset key (e.g. "guidance", "sft", "dpo").
	ID() string
	// DisplayName is shown in the UI dataset picker.
	DisplayName() string
	// FetchCandidates returns up to q.Limit un-graded (or re-gradable) review
	// items, honoring the sampling hints. The dataset decides where items come
	// from (a TSDB table, a file, …); the platform calls this.
	FetchCandidates(ctx context.Context, q CandidateQuery) ([]ReviewItem, error)
	// FetchItem returns a single item by id (for the grade flow — the sink reads
	// its Meta). ok=false when the item no longer exists.
	FetchItem(ctx context.Context, spaceID, itemID string) (item ReviewItem, ok bool, err error)
	// Rubric returns the versioned rubric SHAPE for this dataset (rated vs
	// ranked, its dimensions + anchors). Per-paradigm.
	Rubric() Rubric
	// Sink returns the ReinforcementSink defining what "apply this grade to the
	// live system" MEANS for this dataset. May return a NoopSink (gold-only).
	Sink() ReinforcementSink
}

// ReviewItem is one thing to be reviewed.
type ReviewItem struct {
	ItemID    string             `json:"item_id"`   // stable per-item id within the dataset
	Content   string             `json:"content"`   // the primary text under review
	Context   string             `json:"context"`   // surrounding context shown to the grader
	AutoLabel string             `json:"auto_label"` // current auto-classifier verdict (e.g. "ignored")
	AutoScore float64            `json:"auto_score"` // auto-classifier confidence [0,1]
	Signals   map[string]float64 `json:"signals,omitempty"` // secondary signals for active sampling
	Stratum   string             `json:"stratum,omitempty"` // natural stratum (for stratified sampling)
	// Alternatives holds the chosen/rejected pair for ranked paradigms (DPO).
	Alternatives []ReviewAlternative `json:"alternatives,omitempty"`
	Meta         map[string]string   `json:"meta,omitempty"` // dataset-specific passthrough
}

// ReviewAlternative is one option in a ranked (DPO) review item.
type ReviewAlternative struct {
	AltID   string `json:"alt_id"`
	Content string `json:"content"`
}

// CandidateQuery carries the sampling hints from the sampler.
type CandidateQuery struct {
	SpaceID         string
	Limit           int
	Strata          []string // dimensions to stratify across
	UncertaintyBand float64  // prefer items whose AutoScore is within this band of 0.5
	ExcludeGraded   bool     // skip items already graded at the current rubric_version
}
