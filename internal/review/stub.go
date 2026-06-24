package review

import (
	"context"
	"fmt"
)

// StubDataset is a synthetic ReviewableDataset for self-test / dev, gated by
// REVIEW_STUB_DATASET_ENABLED. It is gold-only (NoopSink) so it never touches
// the live system, and it lets the platform's non-reinforcement gates
// (registry, sampler, endpoints, persistence) be exercised without depending on
// any real dataset being populated.
type StubDataset struct{}

func (StubDataset) ID() string          { return "stub" }
func (StubDataset) DisplayName() string { return "Stub (self-test)" }

func (StubDataset) Rubric() Rubric {
	return Rubric{
		Version: "gr-v1",
		Kind:    RubricRated,
		Dimensions: []RubricDimension{
			{Key: "quality", Anchors: [5]string{
				"unusable — wrong or empty",
				"poor — major problems",
				"acceptable — usable with edits",
				"good — minor nits",
				"excellent — ship as-is",
			}},
		},
	}
}

func (StubDataset) Sink() ReinforcementSink { return NoopSink{} }

func (StubDataset) FetchCandidates(_ context.Context, q CandidateQuery) ([]ReviewItem, error) {
	n := q.Limit
	if n <= 0 || n > 5 {
		n = 5
	}
	items := make([]ReviewItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, ReviewItem{
			ItemID:    fmt.Sprintf("stub-%d", i),
			Content:   fmt.Sprintf("synthetic review item %d", i),
			Context:   "self-test context",
			AutoLabel: "unknown",
			AutoScore: 0.5,
			Stratum:   "default",
		})
	}
	return items, nil
}
