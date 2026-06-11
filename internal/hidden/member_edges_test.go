package hidden

import "testing"

func TestMemberEdgePairs_UniqueCUIDPerMember(t *testing.T) {
	ids := []string{"n1", "n2", "n3"}
	pairs := memberEdgePairs(ids)
	if len(pairs) != 3 {
		t.Fatalf("want 3 pairs, got %d", len(pairs))
	}
	seen := map[string]bool{}
	for i, p := range pairs {
		if p["memberId"] != ids[i] {
			t.Errorf("pair %d memberId = %v, want %s", i, p["memberId"], ids[i])
		}
		eid, _ := p["edgeId"].(string)
		if eid == "" {
			t.Fatalf("pair %d edgeId empty", i)
		}
		if seen[eid] {
			t.Fatalf("duplicate edgeId %q", eid)
		}
		seen[eid] = true
		// CUIDv2: lowercase alphanumeric, starts with a letter, no hyphens
		// (distinguishes from the old randomUUID format).
		for _, c := range eid {
			if c == '-' {
				t.Fatalf("edgeId %q looks like a UUID, want CUIDv2", eid)
			}
		}
	}
}

func TestMemberEdgePairs_Empty(t *testing.T) {
	if got := memberEdgePairs(nil); len(got) != 0 {
		t.Fatalf("nil input → %d pairs, want 0", len(got))
	}
}
