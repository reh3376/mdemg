package conversation

import (
	"testing"

	"mdemg/internal/hidden"
)

func makeFingerprintCatalog(t *testing.T) *hidden.Catalog {
	t.Helper()
	bits := make([]hidden.BitEntry, 256)
	bits[0] = hidden.BitEntry{Position: 0, Kind: hidden.BitKindRoleTypeLayer, Ref: "leaf|0", Token: "(leaf, L0)"}
	bits[1] = hidden.BitEntry{Position: 1, Kind: hidden.BitKindRoleTypeLayer, Ref: "conversation_observation|0", Token: "(conv_obs, L0)"}
	bits[32] = hidden.BitEntry{Position: 32, Kind: hidden.BitKindTag, Ref: "typescript", Token: "typescript"}
	bits[33] = hidden.BitEntry{Position: 33, Kind: hidden.BitKindTag, Ref: "validation", Token: "validation"}
	bits[34] = hidden.BitEntry{Position: 34, Kind: hidden.BitKindTag, Ref: "auth", Token: "auth"}
	bits[64] = hidden.BitEntry{Position: 64, Kind: hidden.BitKindPath, Ref: "/auth/login.go", Token: "/auth/login.go"}
	bits[200] = hidden.BitEntry{Position: 200, Kind: hidden.BitKindSymbol, Ref: "node-abc", Token: "ErrorHandler"}

	for i := uint16(0); i < 256; i++ {
		if bits[i].Token == "" {
			switch {
			case i < 32:
				bits[i] = hidden.BitEntry{Position: i, Kind: hidden.BitKindRoleTypeLayer, Ref: "rt" + itoa(i), Token: "pad"}
			case i < 64:
				bits[i] = hidden.BitEntry{Position: i, Kind: hidden.BitKindTag, Ref: "tag" + itoa(i), Token: "pad"}
			case i < 200:
				bits[i] = hidden.BitEntry{Position: i, Kind: hidden.BitKindPath, Ref: "/p/" + itoa(i), Token: "pad"}
			default:
				bits[i] = hidden.BitEntry{Position: i, Kind: hidden.BitKindSymbol, Ref: "sym" + itoa(i), Token: "pad"}
			}
		}
	}
	hidden.SortBitsByPosition(bits)
	cat, err := hidden.NewCatalog("test-space", 1, 256, bits)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return cat
}

// helper purely for test setup; intentionally tiny to avoid pulling in strconv
// at top of file (the real itoa helper lives elsewhere in the test tree).
func itoa(i uint16) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	buf := [6]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

func TestComputeContextFingerprintLocal_ConversationObs(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	obs := &Observation{
		Tags: []string{"typescript", "validation"},
		Metadata: map[string]any{
			"file_path": "/auth/login.go",
		},
	}
	got := ComputeContextFingerprintLocal(obs, cat)
	// Default role_type=conversation_observation, layer=0 → bit 1.
	// Tags: typescript=32, validation=33.
	// Path: /auth/login.go=64 (full-path bit).
	// Phase 14.2.2: path also splits to ["auth","login.go"]; "auth" matches
	// catalog tag bit 34 (set in makeFingerprintCatalog).
	want := []uint16{1, 32, 33, 34, 64}
	if len(got) != len(want) {
		t.Fatalf("len(bits) = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("bit[%d] = %d; want %d (got %v)", i, got[i], b, got)
		}
	}
}

func TestComputeContextFingerprintLocal_ExplicitRoleTypeAndLayer(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	obs := &Observation{
		Metadata: map[string]any{
			"role_type": "leaf",
			"layer":     0,
		},
	}
	got := ComputeContextFingerprintLocal(obs, cat)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("got %v; want [0] (leaf|0)", got)
	}
}

func TestComputeContextFingerprintLocal_NilCatalogReturnsNil(t *testing.T) {
	obs := &Observation{Tags: []string{"x"}}
	if got := ComputeContextFingerprintLocal(obs, nil); got != nil {
		t.Errorf("expected nil for nil catalog; got %v", got)
	}
}

func TestComputeContextFingerprintLocal_NilObsReturnsNil(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	if got := ComputeContextFingerprintLocal(nil, cat); got != nil {
		t.Errorf("expected nil for nil obs; got %v", got)
	}
}

func TestComputeContextFingerprintLocal_NoMatches(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	obs := &Observation{
		Tags: []string{"unknown-tag-not-in-catalog"},
		Metadata: map[string]any{
			"role_type": "absent_role",
			"file_path": "/never/seen.go",
		},
	}
	got := ComputeContextFingerprintLocal(obs, cat)
	if got != nil {
		t.Errorf("expected nil (no matches); got %v", got)
	}
}

func TestComputeContextFingerprintLocal_DeterministicOrder(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	// Tags supplied in reverse order of catalog positions; result must be sorted.
	obs := &Observation{
		Tags: []string{"validation", "auth", "typescript"},
	}
	got := ComputeContextFingerprintLocal(obs, cat)
	want := []uint16{1, 32, 33, 34} // role_type bit 1 (default conv_obs|0) + tags
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bit[%d] = %d; want %d", i, got[i], want[i])
		}
	}
}

func TestComputeContextFingerprintLocal_LayerInt64Metadata(t *testing.T) {
	cat := makeFingerprintCatalog(t)
	// Some callers pass layer as int64 (e.g. Neo4j-derived). Verify both paths.
	obs := &Observation{
		Metadata: map[string]any{
			"role_type": "leaf",
			"layer":     int64(0),
		},
	}
	got := ComputeContextFingerprintLocal(obs, cat)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("int64 layer path failed: got %v; want [0]", got)
	}
}

func TestAsInt64Slice(t *testing.T) {
	in := []uint16{0, 32, 200}
	got := asInt64Slice(in)
	if len(got) != 3 || got[0] != 0 || got[1] != 32 || got[2] != 200 {
		t.Errorf("got %v; want [0 32 200]", got)
	}
	// Empty input → empty (not nil) slice; Neo4j-safe parameter binding.
	if got := asInt64Slice(nil); got == nil || len(got) != 0 {
		t.Errorf("nil input expected empty slice; got %v", got)
	}
}
