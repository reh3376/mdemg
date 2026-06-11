package retrieval

import (
	"testing"
	"time"

	"mdemg/internal/config"
)

func TestParseTemporalIntent_NoneMode(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	tests := []string{
		"How does auth work?",
		"What is the architecture of the system?",
		"Show me the login function",
		"explain the database schema",
		"",
	}
	for _, query := range tests {
		intent := ParseTemporalIntent(query, now)
		if intent.Mode != TemporalModeNone {
			t.Errorf("query %q: expected mode=none, got mode=%s", query, intent.Mode)
		}
		if intent.Constraint != nil {
			t.Errorf("query %q: expected nil constraint, got %+v", query, intent.Constraint)
		}
	}
}

func TestParseTemporalIntent_SoftMode(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		query         string
		expectKeyword string
	}{
		{"recent changes to auth", "recent"},
		{"show me the latest updates", "latest"},
		{"what changed in the codebase", "what changed"},
		{"what's new in the API", "what's new"},
		{"newest files in the project", "newest"},
	}
	for _, tc := range tests {
		intent := ParseTemporalIntent(tc.query, now)
		if intent.Mode != TemporalModeSoft {
			t.Errorf("query %q: expected mode=soft, got mode=%s", tc.query, intent.Mode)
			continue
		}
		found := false
		for _, kw := range intent.Keywords {
			if kw == tc.expectKeyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("query %q: expected keyword %q in %v", tc.query, tc.expectKeyword, intent.Keywords)
		}
		if intent.Confidence < 0.5 {
			t.Errorf("query %q: expected confidence >= 0.5, got %.2f", tc.query, intent.Confidence)
		}
	}
}

func TestParseTemporalIntent_HardLastN(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		query     string
		daysDelta float64 // approximate days ago for After
	}{
		{"changes in the last 7 days", 7},
		{"what happened in the last 30 days", 30},
		{"updates in the last 2 weeks", 14},
		{"last 3 months of activity", 90},
	}
	for _, tc := range tests {
		intent := ParseTemporalIntent(tc.query, now)
		if intent.Mode != TemporalModeHard {
			t.Errorf("query %q: expected mode=hard, got mode=%s", tc.query, intent.Mode)
			continue
		}
		if intent.Constraint == nil {
			t.Errorf("query %q: expected non-nil constraint", tc.query)
			continue
		}
		if intent.Constraint.After == nil {
			t.Errorf("query %q: expected non-nil After", tc.query)
			continue
		}
		// Check that After is approximately the right number of days ago
		actualDays := now.Sub(*intent.Constraint.After).Hours() / 24
		if actualDays < tc.daysDelta-1 || actualDays > tc.daysDelta+1 {
			t.Errorf("query %q: expected After ~%v days ago, got ~%.1f days ago",
				tc.query, tc.daysDelta, actualDays)
		}
	}
}

func TestParseTemporalIntent_HardSince(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("since 2026-01-15", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil || intent.Constraint.After == nil {
		t.Fatal("expected non-nil constraint with After")
	}
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !intent.Constraint.After.Equal(expected) {
		t.Errorf("expected After=%v, got %v", expected, *intent.Constraint.After)
	}
}

func TestParseTemporalIntent_HardSinceMonth(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("since January 2026", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil || intent.Constraint.After == nil {
		t.Fatal("expected non-nil constraint with After")
	}
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, now.Location())
	if !intent.Constraint.After.Equal(expected) {
		t.Errorf("expected After=%v, got %v", expected, *intent.Constraint.After)
	}
}

func TestParseTemporalIntent_HardBefore(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("before December 2025", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil || intent.Constraint.Before == nil {
		t.Fatal("expected non-nil constraint with Before")
	}
	expected := time.Date(2025, 12, 1, 0, 0, 0, 0, now.Location())
	if !intent.Constraint.Before.Equal(expected) {
		t.Errorf("expected Before=%v, got %v", expected, *intent.Constraint.Before)
	}
}

func TestParseTemporalIntent_HardBeforeISO(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("before 2025-12-01", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil || intent.Constraint.Before == nil {
		t.Fatal("expected non-nil constraint with Before")
	}
	expected := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	if !intent.Constraint.Before.Equal(expected) {
		t.Errorf("expected Before=%v, got %v", expected, *intent.Constraint.Before)
	}
}

func TestParseTemporalIntent_HardBetween(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("between 2026-01-01 and 2026-01-31", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if intent.Constraint.After == nil || intent.Constraint.Before == nil {
		t.Fatal("expected both After and Before for between query")
	}
	afterExpected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beforeExpected := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	if !intent.Constraint.After.Equal(afterExpected) {
		t.Errorf("expected After=%v, got %v", afterExpected, *intent.Constraint.After)
	}
	if !intent.Constraint.Before.Equal(beforeExpected) {
		t.Errorf("expected Before=%v, got %v", beforeExpected, *intent.Constraint.Before)
	}
}

func TestParseTemporalIntent_ThisWeek(t *testing.T) {
	// Wednesday Feb 4, 2026
	now := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("what changed this week", now)
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected mode=hard, got %s", intent.Mode)
	}
	if intent.Constraint == nil || intent.Constraint.After == nil {
		t.Fatal("expected non-nil constraint with After")
	}
	// Monday of that week = Feb 2
	expected := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	if !intent.Constraint.After.Equal(expected) {
		t.Errorf("expected After=%v (Monday), got %v", expected, *intent.Constraint.After)
	}
}

func TestFilterCandidates_WithinRange(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	after := now.Add(-7 * 24 * time.Hour)

	cands := []Candidate{
		{NodeID: "new1", UpdatedAt: now.Add(-1 * 24 * time.Hour)},  // 1 day ago
		{NodeID: "new2", UpdatedAt: now.Add(-3 * 24 * time.Hour)},  // 3 days ago
		{NodeID: "edge", UpdatedAt: now.Add(-7 * 24 * time.Hour)},  // exactly 7 days ago (at boundary)
		{NodeID: "old1", UpdatedAt: now.Add(-10 * 24 * time.Hour)}, // 10 days ago
		{NodeID: "old2", UpdatedAt: now.Add(-30 * 24 * time.Hour)}, // 30 days ago
	}

	constraint := &TemporalConstraint{After: &after, Description: "last 7 days"}
	filtered := FilterCandidatesByTime(cands, constraint)

	// "edge" (exactly at After) should pass since it's not Before After
	if len(filtered) != 3 {
		t.Errorf("expected 3 candidates, got %d", len(filtered))
		for _, c := range filtered {
			t.Logf("  kept: %s (updated %v)", c.NodeID, c.UpdatedAt)
		}
	}

	// Verify the right ones survived
	ids := make(map[string]bool)
	for _, c := range filtered {
		ids[c.NodeID] = true
	}
	for _, expected := range []string{"new1", "new2", "edge"} {
		if !ids[expected] {
			t.Errorf("expected %s to survive filter", expected)
		}
	}
}

func TestFilterCandidates_NilConstraint(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a"},
		{NodeID: "b"},
		{NodeID: "c"},
		{NodeID: "d"},
		{NodeID: "e"},
	}
	filtered := FilterCandidatesByTime(cands, nil)
	if len(filtered) != 5 {
		t.Errorf("expected all 5 candidates with nil constraint, got %d", len(filtered))
	}
}

func TestFilterCandidates_BeforeConstraint(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	before := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	cands := []Candidate{
		{NodeID: "old", UpdatedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)}, // before cutoff
		{NodeID: "new", UpdatedAt: now},                                          // after cutoff
	}

	constraint := &TemporalConstraint{Before: &before}
	filtered := FilterCandidatesByTime(cands, constraint)
	if len(filtered) != 1 || filtered[0].NodeID != "old" {
		t.Errorf("expected only 'old' to pass before filter, got %v", filtered)
	}
}

func TestCleanTemporalKeywords(t *testing.T) {
	tests := []struct {
		query    string
		keywords []string
		expected string
	}{
		{"recent changes to auth", []string{"recent"}, "changes to auth"},
		{"what changed in the last 7 days", []string{"in the last 7 days"}, "what changed"},
		{"show me latest updates to API", []string{"latest"}, "show me updates to API"},
		{"how does auth work", nil, "how does auth work"},
		{"how does auth work", []string{}, "how does auth work"},
	}
	for _, tc := range tests {
		result := CleanTemporalKeywords(tc.query, tc.keywords)
		if result != tc.expected {
			t.Errorf("CleanTemporalKeywords(%q, %v) = %q, want %q",
				tc.query, tc.keywords, result, tc.expected)
		}
	}
}

func TestCleanTemporalKeywords_PreservesNonEmpty(t *testing.T) {
	// When cleaning removes everything, return original
	result := CleanTemporalKeywords("recent", []string{"recent"})
	if result != "recent" {
		t.Errorf("expected original query when cleaning produces empty, got %q", result)
	}
}

func TestBuildExplicitTemporalIntent(t *testing.T) {
	intent := BuildExplicitTemporalIntent("2026-01-15", "2026-02-01")
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected hard mode, got %s", intent.Mode)
	}
	if intent.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if intent.Constraint.After == nil || intent.Constraint.Before == nil {
		t.Fatal("expected both After and Before")
	}
	if intent.Confidence != 1.0 {
		t.Errorf("expected confidence=1.0, got %.2f", intent.Confidence)
	}
}

func TestBuildExplicitTemporalIntent_Empty(t *testing.T) {
	intent := BuildExplicitTemporalIntent("", "")
	if intent.Mode != TemporalModeNone {
		t.Errorf("expected none mode for empty strings, got %s", intent.Mode)
	}
}

func TestBuildExplicitTemporalIntent_RFC3339(t *testing.T) {
	intent := BuildExplicitTemporalIntent("2026-01-15T10:30:00Z", "")
	if intent.Mode != TemporalModeHard {
		t.Fatalf("expected hard mode, got %s", intent.Mode)
	}
	if intent.Constraint.After == nil {
		t.Fatal("expected After to be set")
	}
	expected := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	if !intent.Constraint.After.Equal(expected) {
		t.Errorf("expected After=%v, got %v", expected, *intent.Constraint.After)
	}
}

// TestScoring_NoneMode_Unchanged verifies that scoring is identical when temporal mode is "none".
// This is the key regression test: when no temporal language is detected, behavior must not change.
func TestScoring_NoneMode_Unchanged(t *testing.T) {
	cfg := config.Config{
		ScoringAlpha:                0.55,
		ScoringBeta:                 0.30,
		ScoringGamma:                0.10,
		ScoringDelta:                0.05,
		ScoringPhi:                  0.08,
		ScoringKappa:                0.12,
		ScoringRhoL0:                0.05,
		ScoringRhoL1:                0.02,
		ScoringRhoL2:                0.01,
		ScoringConfigBoost:          1.15,
		ScoringPathBoost:            0.15,
		TemporalEnabled:             true,
		TemporalSoftBoostMultiplier: 3.0,
		TemporalHardFilterEnabled:   true,
	}

	now := time.Now()
	cands := []Candidate{
		{NodeID: "a", VectorSim: 0.9, Confidence: 0.8, UpdatedAt: now.Add(-24 * time.Hour), Layer: 0, Path: "src/auth.go"},
		{NodeID: "b", VectorSim: 0.7, Confidence: 0.6, UpdatedAt: now.Add(-48 * time.Hour), Layer: 0, Path: "src/db.go"},
		{NodeID: "c", VectorSim: 0.5, Confidence: 0.5, UpdatedAt: now.Add(-72 * time.Hour), Layer: 1, Path: "concept/security"},
	}
	act := map[string]float64{"a": 0.5, "b": 0.3, "c": 0.2}
	edges := []Edge{}

	// Score with none-mode hints
	hintsNone := RetrievalHints{
		TemporalIntent: TemporalIntent{Mode: TemporalModeNone},
	}
	resultsNone := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "how does auth work", hintsNone)

	// Score with empty hints (default zero-value)
	hintsDefault := RetrievalHints{}
	resultsDefault := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "how does auth work", hintsDefault)

	// Scores must be identical (within floating point tolerance due to time.Now() calls in scoring)
	if len(resultsNone) != len(resultsDefault) {
		t.Fatalf("different result counts: %d vs %d", len(resultsNone), len(resultsDefault))
	}
	const tolerance = 1e-6
	for i := range resultsNone {
		diff := resultsNone[i].Score - resultsDefault[i].Score
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("result %d: none-mode score %.10f != default score %.10f (diff: %.10f)",
				i, resultsNone[i].Score, resultsDefault[i].Score, diff)
		}
		if resultsNone[i].Breakdown.TemporalBoost != 0 {
			t.Errorf("result %d: temporal boost should be 0 in none mode, got %.6f",
				i, resultsNone[i].Breakdown.TemporalBoost)
		}
	}
}

// TestScoring_SoftMode_BoostsRecent verifies that soft mode increases scores for recent items.
func TestScoring_SoftMode_BoostsRecent(t *testing.T) {
	cfg := config.Config{
		ScoringAlpha:                0.55,
		ScoringBeta:                 0.30,
		ScoringGamma:                0.10,
		ScoringDelta:                0.05,
		ScoringPhi:                  0.08,
		ScoringKappa:                0.12,
		ScoringRhoL0:                0.05,
		ScoringRhoL1:                0.02,
		ScoringRhoL2:                0.01,
		ScoringConfigBoost:          1.15,
		ScoringPathBoost:            0.15,
		TemporalEnabled:             true,
		TemporalSoftBoostMultiplier: 3.0,
		TemporalHardFilterEnabled:   true,
	}

	now := time.Now()
	cands := []Candidate{
		{NodeID: "recent", VectorSim: 0.7, Confidence: 0.6, UpdatedAt: now.Add(-1 * 24 * time.Hour), Layer: 0, Path: "src/new.go"},
		{NodeID: "old", VectorSim: 0.7, Confidence: 0.6, UpdatedAt: now.Add(-60 * 24 * time.Hour), Layer: 0, Path: "src/old.go"},
	}
	act := map[string]float64{"recent": 0.3, "old": 0.3}
	edges := []Edge{}

	// Score with none-mode
	hintsNone := RetrievalHints{
		TemporalIntent: TemporalIntent{Mode: TemporalModeNone},
	}
	resultsNone := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "auth module", hintsNone)

	// Score with soft-mode
	hintsSoft := RetrievalHints{
		TemporalIntent: TemporalIntent{Mode: TemporalModeSoft, Keywords: []string{"recent"}},
	}
	resultsSoft := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "recent auth module", hintsSoft)

	// In soft mode, the gap between recent and old should be larger
	noneGap := resultsNone[0].Score - resultsNone[1].Score
	softGap := resultsSoft[0].Score - resultsSoft[1].Score

	if softGap <= noneGap {
		t.Errorf("soft mode should increase gap between recent and old items: none_gap=%.4f, soft_gap=%.4f",
			noneGap, softGap)
	}

	// Verify temporal boost is non-zero in soft mode
	for _, r := range resultsSoft {
		if r.Breakdown.TemporalBoost == 0 {
			t.Errorf("expected non-zero temporal boost in soft mode for %s", r.NodeID)
		}
	}
}

func TestParseTemporalIntent_HardPriority(t *testing.T) {
	// When both hard and soft patterns match, hard should win
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	intent := ParseTemporalIntent("recent changes in the last 7 days", now)
	if intent.Mode != TemporalModeHard {
		t.Errorf("expected hard mode to take priority, got %s", intent.Mode)
	}
}

// Phase 2 Temporal Tests

func TestStaleReferencePenalty_Disabled(t *testing.T) {
	// staleDays <= 0 should return nil
	cands := []Candidate{{NodeID: "a", UpdatedAt: time.Now()}}
	edges := []Edge{{Src: "a", Dst: "b"}}
	result := StaleReferencePenalty(cands, edges, 0, 0.15)
	if result != nil {
		t.Errorf("expected nil map when staleDays=0, got %v", result)
	}
}

func TestStaleReferencePenalty_NoPenalty(t *testing.T) {
	// Both nodes are same age, no penalty
	now := time.Now()
	cands := []Candidate{
		{NodeID: "a", UpdatedAt: now},
		{NodeID: "b", UpdatedAt: now},
	}
	edges := []Edge{{Src: "a", Dst: "b"}}
	result := StaleReferencePenalty(cands, edges, 30, 0.15)
	if p := result["a"]; p > 0 {
		t.Errorf("expected no penalty when nodes are same age, got %.4f", p)
	}
}

func TestStaleReferencePenalty_PenalizeOld(t *testing.T) {
	// Src is 60 days old, dst is brand new, staleDays=30
	// daysDiff = 60, penalty = 0.05 * (60-30)/30 = 0.05
	now := time.Now()
	cands := []Candidate{
		{NodeID: "old", UpdatedAt: now.Add(-60 * 24 * time.Hour)},
		{NodeID: "new", UpdatedAt: now},
	}
	edges := []Edge{{Src: "old", Dst: "new"}}
	result := StaleReferencePenalty(cands, edges, 30, 0.15)
	penalty := result["old"]
	if penalty < 0.04 || penalty > 0.06 {
		t.Errorf("expected penalty ~0.05, got %.4f", penalty)
	}
}

func TestStaleReferencePenalty_CappedAtMax(t *testing.T) {
	// Very large age difference should be capped at maxPenalty
	now := time.Now()
	cands := []Candidate{
		{NodeID: "ancient", UpdatedAt: now.Add(-365 * 24 * time.Hour)},
		{NodeID: "fresh", UpdatedAt: now},
	}
	edges := []Edge{{Src: "ancient", Dst: "fresh"}}
	maxPen := 0.15
	result := StaleReferencePenalty(cands, edges, 30, maxPen)
	penalty := result["ancient"]
	if penalty != maxPen {
		t.Errorf("expected penalty capped at %.2f, got %.4f", maxPen, penalty)
	}
}

func TestStaleReferencePenalty_UsesCanonicalTime(t *testing.T) {
	// CanonicalTime should be used instead of UpdatedAt when available
	now := time.Now()
	cands := []Candidate{
		{
			NodeID:        "old-content",
			UpdatedAt:     now,                           // recently re-ingested
			CanonicalTime: now.Add(-90 * 24 * time.Hour), // but content is 90 days old
		},
		{
			NodeID:    "new-content",
			UpdatedAt: now,
		},
	}
	edges := []Edge{{Src: "old-content", Dst: "new-content"}}
	result := StaleReferencePenalty(cands, edges, 30, 0.15)
	penalty := result["old-content"]
	if penalty <= 0 {
		t.Errorf("expected penalty > 0 when CanonicalTime is old, got %.4f", penalty)
	}
}

func TestGetDecayRate_LayerOverridesSourceType(t *testing.T) {
	// L1+ nodes should always use layer-specific rates, even with source tags
	cfg := config.Config{
		TemporalSourceTypeDecayEnabled: true,
		ScoringRhoDocumentation:        0.01,
		ScoringRhoL0:                   0.05,
		ScoringRhoL1:                   0.02,
		ScoringRhoL2:                   0.01,
	}
	tags := []string{"documentation"}
	rate := getDecayRate(1, tags, cfg)
	if rate != 0.02 {
		t.Errorf("expected L1 rate 0.02 for layer 1 even with documentation tag, got %.4f", rate)
	}
}

func TestGetDecayRate_SourceTypeEnabled(t *testing.T) {
	cfg := config.Config{
		TemporalSourceTypeDecayEnabled: true,
		ScoringRhoDocumentation:        0.01,
		ScoringRhoConfig:               0.03,
		ScoringRhoConversation:         0.10,
		ScoringRhoChangelog:            0.08,
		ScoringRhoL0:                   0.05,
		ScoringRhoL1:                   0.02,
		ScoringRhoL2:                   0.01,
	}

	tests := []struct {
		tags []string
		want float64
		desc string
	}{
		{[]string{"documentation"}, 0.01, "documentation tag"},
		{[]string{"markdown"}, 0.01, "markdown tag"},
		{[]string{"config"}, 0.03, "config tag"},
		{[]string{"configuration"}, 0.03, "configuration tag"},
		{[]string{"conversation"}, 0.10, "conversation tag"},
		{[]string{"conversation_observation"}, 0.10, "conversation_observation tag"},
		{[]string{"changelog"}, 0.08, "changelog tag"},
		{[]string{"python"}, 0.05, "unmatched tag falls back to L0"},
		{nil, 0.05, "no tags falls back to L0"},
	}

	for _, tc := range tests {
		rate := getDecayRate(0, tc.tags, cfg)
		if rate != tc.want {
			t.Errorf("%s: expected %.4f, got %.4f", tc.desc, tc.want, rate)
		}
	}
}

func TestGetDecayRate_SourceTypeDisabled(t *testing.T) {
	// When disabled, should always use L0 rate regardless of tags
	cfg := config.Config{
		TemporalSourceTypeDecayEnabled: false,
		ScoringRhoDocumentation:        0.01,
		ScoringRhoL0:                   0.05,
		ScoringRhoL1:                   0.02,
		ScoringRhoL2:                   0.01,
	}
	rate := getDecayRate(0, []string{"documentation"}, cfg)
	if rate != 0.05 {
		t.Errorf("expected L0 rate 0.05 when source type decay disabled, got %.4f", rate)
	}
}

func TestFilterCandidatesByTime_UsesCanonicalTime(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	after := now.Add(-7 * 24 * time.Hour)

	cands := []Candidate{
		{
			NodeID:        "new-content",
			UpdatedAt:     now.Add(-30 * 24 * time.Hour), // UpdatedAt is old
			CanonicalTime: now.Add(-1 * 24 * time.Hour),  // but CanonicalTime is recent
		},
		{
			NodeID:        "old-content",
			UpdatedAt:     now.Add(-1 * 24 * time.Hour),  // UpdatedAt is recent
			CanonicalTime: now.Add(-30 * 24 * time.Hour), // but CanonicalTime is old
		},
	}

	constraint := &TemporalConstraint{After: &after}
	filtered := FilterCandidatesByTime(cands, constraint)

	// "new-content" should survive (CanonicalTime is recent)
	// "old-content" should be filtered out (CanonicalTime is old)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(filtered))
	}
	if filtered[0].NodeID != "new-content" {
		t.Errorf("expected new-content to survive, got %s", filtered[0].NodeID)
	}
}
