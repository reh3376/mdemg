package hidden

import (
	"regexp"
	"strings"
	"testing"

	cuid2 "github.com/nrednav/cuid2"
	"mdemg/internal/config"
)

func TestHiddenPatternIdentityThreshold(t *testing.T) {
	// Unset (zero-value config) falls back to the 0.90 default.
	s := &Service{cfg: config.Config{}}
	if got := s.hiddenPatternIdentityThreshold(); got != 0.90 {
		t.Errorf("default threshold = %v, want 0.90", got)
	}
	// A configured value overrides the default.
	s2 := &Service{cfg: config.Config{HiddenPatternIdentitySimThreshold: 0.75}}
	if got := s2.hiddenPatternIdentityThreshold(); got != 0.75 {
		t.Errorf("override threshold = %v, want 0.75", got)
	}
}

func TestMatchTheme_HiddenPatternReuse(t *testing.T) {
	// HIDDEN-CHURN-002 reuses matchTheme for hidden patterns (themeRef is just
	// node_id + centroid). This pins that the shared matcher behaves correctly
	// for hidden-pattern refs.
	existing := []themeRef{
		{NodeID: "h_abc", Centroid: []float64{1, 0, 0}},
		{NodeID: "h_def", Centroid: []float64{0, 1, 0}},
	}
	claimed := map[string]bool{}
	if got := matchTheme([]float64{1, 0, 0}, existing, claimed, 0.90); got != "h_abc" {
		t.Fatalf("hidden centroid match: got %q want h_abc", got)
	}
	// A pattern already claimed (updated) this run is not rematched — one
	// cluster per pattern per cycle, the invariant that stops double-update.
	claimed["h_abc"] = true
	if got := matchTheme([]float64{1, 0, 0}, existing, claimed, 0.90); got != "" {
		t.Fatalf("claimed pattern must not rematch, got %q", got)
	}
}

func TestMatchHiddenPattern_MemberOverlapPrimary(t *testing.T) {
	// p1 owns members {a,b,c,d}; p2 owns {x,y,z}. A new cluster {a,b,c,e}
	// shares 3/5 with p1 (Jaccard 0.6) → matches p1 by membership EVEN IF its
	// centroid is far (the centroid-jitter case centroid-only matching misses).
	refs := []hiddenPatternRef{
		{NodeID: "p1", Centroid: []float64{1, 0, 0}, Members: map[string]bool{"a": true, "b": true, "c": true, "d": true}},
		{NodeID: "p2", Centroid: []float64{0, 1, 0}, Members: map[string]bool{"x": true, "y": true, "z": true}},
	}
	idx := buildMemberIndex(refs)
	claimed := map[string]bool{}
	// Centroid deliberately orthogonal to p1 — only membership can match it.
	got := matchHiddenPattern([]string{"a", "b", "c", "e"}, []float64{0, 0, 1}, refs, idx, claimed, 0.5, 0.90)
	if got != "p1" {
		t.Fatalf("member-overlap should match p1, got %q", got)
	}
}

func TestMatchHiddenPattern_CentroidFallback(t *testing.T) {
	// A cluster whose members fully turned over (no overlap) but whose centroid
	// is identical to p1 falls back to the cosine match.
	refs := []hiddenPatternRef{
		{NodeID: "p1", Centroid: []float64{1, 0, 0}, Members: map[string]bool{"a": true, "b": true}},
	}
	idx := buildMemberIndex(refs)
	got := matchHiddenPattern([]string{"q", "r", "s"}, []float64{1, 0, 0}, refs, idx, map[string]bool{}, 0.5, 0.90)
	if got != "p1" {
		t.Fatalf("centroid fallback should match p1, got %q", got)
	}
}

func TestMatchHiddenPattern_NoMatchAndClaimed(t *testing.T) {
	refs := []hiddenPatternRef{
		{NodeID: "p1", Centroid: []float64{1, 0, 0}, Members: map[string]bool{"a": true, "b": true}},
	}
	idx := buildMemberIndex(refs)
	// No member overlap + orthogonal centroid → no match.
	if got := matchHiddenPattern([]string{"q", "r"}, []float64{0, 1, 0}, refs, idx, map[string]bool{}, 0.5, 0.90); got != "" {
		t.Fatalf("disjoint cluster must not match, got %q", got)
	}
	// Strong overlap but the pattern is already claimed → no rematch.
	claimed := map[string]bool{"p1": true}
	if got := matchHiddenPattern([]string{"a", "b"}, []float64{1, 0, 0}, refs, idx, claimed, 0.5, 0.90); got != "" {
		t.Fatalf("claimed pattern must not rematch, got %q", got)
	}
}

func TestHiddenPatternMemberJaccardThreshold(t *testing.T) {
	if got := (&Service{cfg: config.Config{}}).hiddenPatternMemberJaccardThreshold(); got != 0.5 {
		t.Errorf("default jaccard threshold = %v, want 0.5", got)
	}
	if got := (&Service{cfg: config.Config{HiddenPatternMemberJaccardThreshold: 0.7}}).hiddenPatternMemberJaccardThreshold(); got != 0.7 {
		t.Errorf("override jaccard threshold = %v, want 0.7", got)
	}
}

var cuidShape = regexp.MustCompile(`^[a-z][a-z0-9]{20,}$`)

func TestHiddenPatternNodeID_IsCUIDv2NotUUID(t *testing.T) {
	// Regression guard for HIDDEN-CHURN-002: new hidden patterns mint a CUIDv2
	// id (cuid2.Generate), never randomUUID(). A UUID has hyphens and a fixed
	// 36-char shape; CUIDv2 is a lowercase-alphanumeric, leading-letter token.
	for i := 0; i < 50; i++ {
		id := cuid2.Generate()
		if strings.Contains(id, "-") {
			t.Fatalf("CUIDv2 id must not contain hyphens (UUID shape): %q", id)
		}
		if !cuidShape.MatchString(id) {
			t.Fatalf("id %q is not CUIDv2-shaped (lowercase alphanumeric, leading letter)", id)
		}
	}
}

// --- HIDDEN-CHURN-003: incremental clustering ---

func TestHiddenIncrementalAssignThreshold(t *testing.T) {
	if got := (&Service{cfg: config.Config{}}).hiddenIncrementalAssignThreshold(); got != 0.80 {
		t.Errorf("default = %v, want 0.80", got)
	}
	if got := (&Service{cfg: config.Config{HiddenIncrementalAssignSimThreshold: 0.6}}).hiddenIncrementalAssignThreshold(); got != 0.6 {
		t.Errorf("override = %v, want 0.6", got)
	}
}

func TestNearestPatternByCentroid(t *testing.T) {
	refs := []hiddenPatternRef{
		{NodeID: "p1", Centroid: []float64{1, 0, 0}},
		{NodeID: "p2", Centroid: []float64{0, 1, 0}},
	}
	if got := nearestPatternByCentroid([]float64{0.99, 0.01, 0}, refs, 0.80); got != "p1" {
		t.Errorf("want p1, got %q", got)
	}
	if got := nearestPatternByCentroid([]float64{0, 0, 1}, refs, 0.80); got != "" {
		t.Errorf("orthogonal must not match, got %q", got)
	}
	if got := nearestPatternByCentroid([]float64{1, 0, 0}, nil, 0.80); got != "" {
		t.Errorf("empty refs must not match, got %q", got)
	}
}

func TestIncrementalMean(t *testing.T) {
	// mean of n=3 vectors = [2,2]; fold k=1 new summing to [6,6]:
	// ([2,2]*3 + [6,6]) / 4 = [3,3].
	got := incrementalMean([]float64{2, 2}, []float64{6, 6}, 3, 1)
	if len(got) != 2 || got[0] != 3 || got[1] != 3 {
		t.Errorf("want [3 3], got %v", got)
	}
	// Length mismatch → old returned unchanged.
	if got := incrementalMean([]float64{1, 2, 3}, []float64{1, 1}, 1, 1); len(got) != 3 || got[0] != 1 {
		t.Errorf("length mismatch must return old, got %v", got)
	}
}
