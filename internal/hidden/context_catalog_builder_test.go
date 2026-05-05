package hidden

import (
	"testing"
)

// TestAllocateBits_WhkWmsShape replicates the per-Phase-14.2-Epic-0 forensic
// data for whk-wms (5 role_type×layer tuples with frequencies, 32 tags, 192
// paths) and asserts the allocator produces 256 bits with the correct shape.
func TestAllocateBits_WhkWmsShape(t *testing.T) {
	b := &neo4jBuilder{
		opts: BuilderOpts{
			BitBudget:         256,
			TopNPaths:         192,
			TopNTags:          32,
			FloorBitsPerKind:  16,
			RoleTypeLayerBits: 32,
		},
	}

	// Synthesize density data resembling whk-wms (per phase_14_2_forensic.md).
	d := &densityResult{
		topRoleTypeLayer: counted([]string{
			"leaf|0", "hidden|0", "hidden|1", "concept|2", "comparison|2",
		}),
		topTags: counted([]string{
			"typescript", "error-handling", "validation", "logging", "authentication",
			"module", "temporal", "authorization", "class", "interface",
			"sync", "queue", "graphql", "websocket", "api",
			"controller", "service", "repository", "schema", "model",
			"middleware", "guard", "factory", "test", "spec",
			"util", "helper", "config", "type", "constant",
			"enum", "constructor",
		}),
		topPaths:        synthPaths(192),
		distinctPaths:   8360,
		distinctTags:    150, // hypothetical
		distinctSymbols: 0,   // confirmed by forensic
	}

	bits := b.allocateBits(d)

	if len(bits) != 256 {
		t.Fatalf("got %d bits, want 256", len(bits))
	}
	rtl, tag, path, sym := summarizeAllocation(bits)
	if rtl != 5 {
		t.Errorf("role_type×layer bits = %d; want 5 (only 5 tuples seeded)", rtl)
	}
	if tag != 32 {
		t.Errorf("tag bits = %d; want 32", tag)
	}
	if sym != 0 {
		t.Errorf("symbol bits = %d; want 0 (no symbols in whk-wms)", sym)
	}
	// Path bits = 256 - 5 (rtl) - 32 (tags) = 219 (the 27 unused rtl slots
	// reclaimed by paths because we cap at len(d.topRoleTypeLayer)).
	if path != 219 {
		t.Errorf("path bits = %d; want 219 (256 - 5 used rtl - 32 tags)", path)
	}

	// Bits must be sorted by Position 0..255 with no gaps.
	for i, b := range bits {
		if int(b.Position) != i {
			t.Errorf("bits[%d].Position = %d; want %d", i, b.Position, i)
		}
	}
}

// TestAllocateBits_UbtsLoadShape replicates the ubts-load edge case:
// 1 role_type, 1 layer, 0 tags, 3009 paths. All available bits should
// flow to paths.
func TestAllocateBits_UbtsLoadShape(t *testing.T) {
	b := &neo4jBuilder{
		opts: BuilderOpts{
			BitBudget:         256,
			TopNPaths:         192,
			TopNTags:          32,
			FloorBitsPerKind:  16,
			RoleTypeLayerBits: 32,
		},
	}

	d := &densityResult{
		topRoleTypeLayer: counted([]string{"leaf|0"}),
		topTags:          nil, // no tags
		topPaths:         synthPaths(192),
		distinctPaths:    3009,
		distinctTags:     0,
		distinctSymbols:  0,
	}

	bits := b.allocateBits(d)

	if len(bits) != 256 {
		t.Fatalf("got %d bits, want 256", len(bits))
	}
	rtl, tag, path, _ := summarizeAllocation(bits)
	if rtl != 1 {
		t.Errorf("rtl = %d; want 1", rtl)
	}
	if tag != 0 {
		t.Errorf("tag = %d; want 0", tag)
	}
	// All 256 - 1 = 255 remaining bits should land on paths (we have 192
	// path entries from synthPaths; the rest become empty BitKindPath
	// placeholders).
	if path != 255 {
		t.Errorf("path bits = %d; want 255", path)
	}
}

// TestAllocateBits_Determinism: same inputs → same output.
func TestAllocateBits_Determinism(t *testing.T) {
	b := &neo4jBuilder{
		opts: BuilderOpts{
			BitBudget:         128,
			TopNPaths:         96,
			TopNTags:          16,
			FloorBitsPerKind:  8,
			RoleTypeLayerBits: 16,
		},
	}
	d := &densityResult{
		topRoleTypeLayer: counted([]string{"leaf|0", "hidden|1"}),
		topTags:          counted([]string{"a", "b", "c"}),
		topPaths:         counted([]string{"/p/x", "/p/y"}),
		distinctPaths:    100, distinctTags: 50, distinctSymbols: 0,
	}
	bits1 := b.allocateBits(d)
	bits2 := b.allocateBits(d)
	if len(bits1) != len(bits2) {
		t.Fatalf("len mismatch: %d vs %d", len(bits1), len(bits2))
	}
	for i := range bits1 {
		if bits1[i] != bits2[i] {
			t.Errorf("bits[%d] mismatch: %+v vs %+v", i, bits1[i], bits2[i])
		}
	}
}

// TestAllocateBits_BudgetBoundaries: ensure exact-budget invariant under
// edge cases.
func TestAllocateBits_BudgetBoundaries(t *testing.T) {
	cases := []struct {
		name string
		opts BuilderOpts
		d    *densityResult
	}{
		{
			name: "small budget all kinds populated",
			opts: BuilderOpts{BitBudget: 32, TopNPaths: 16, TopNTags: 8, FloorBitsPerKind: 4, RoleTypeLayerBits: 8},
			d: &densityResult{
				topRoleTypeLayer: counted([]string{"r1|0", "r2|1"}),
				topTags:          counted([]string{"t1", "t2", "t3", "t4"}),
				topPaths:         counted([]string{"/p1", "/p2", "/p3"}),
				distinctPaths:    10, distinctTags: 4, distinctSymbols: 0,
			},
		},
		{
			name: "empty density (cold-start space)",
			opts: BuilderOpts{BitBudget: 256, TopNPaths: 192, TopNTags: 32, FloorBitsPerKind: 16, RoleTypeLayerBits: 32},
			d:    &densityResult{},
		},
		{
			name: "more rtl tuples than budget allows",
			opts: BuilderOpts{BitBudget: 64, TopNPaths: 32, TopNTags: 16, FloorBitsPerKind: 8, RoleTypeLayerBits: 8},
			d: &densityResult{
				topRoleTypeLayer: counted([]string{"r1|0", "r2|0", "r3|0", "r4|0", "r5|0", "r6|0", "r7|0", "r8|0", "r9|0", "r10|0"}), // 10 > 8
				topTags:          counted([]string{"t1"}),
				topPaths:         counted([]string{"/p1"}),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &neo4jBuilder{opts: tc.opts}
			bits := b.allocateBits(tc.d)
			if len(bits) != tc.opts.BitBudget {
				t.Errorf("got %d bits, want %d", len(bits), tc.opts.BitBudget)
			}
			// Position 0..N-1 contiguous
			for i, e := range bits {
				if int(e.Position) != i {
					t.Errorf("bits[%d].Position = %d", i, e.Position)
				}
			}
		})
	}
}

// TestTopRefs: extract top-N Refs of a kind from sorted bits.
func TestTopRefs(t *testing.T) {
	bits := []BitEntry{
		{Position: 0, Kind: BitKindRoleTypeLayer, Ref: "rtl1", Token: "r"},
		{Position: 1, Kind: BitKindTag, Ref: "tag1", Token: "t"},
		{Position: 2, Kind: BitKindTag, Ref: "tag2", Token: "t"},
		{Position: 3, Kind: BitKindPath, Ref: "/p1", Token: "p"},
		{Position: 4, Kind: BitKindPath, Ref: "", Token: ""}, // placeholder; should be skipped
		{Position: 5, Kind: BitKindPath, Ref: "/p2", Token: "p"},
	}
	tags := topRefs(bits, BitKindTag, 5)
	if len(tags) != 2 || tags[0] != "tag1" || tags[1] != "tag2" {
		t.Errorf("topRefs(tags) = %v; want [tag1 tag2]", tags)
	}
	paths := topRefs(bits, BitKindPath, 1)
	if len(paths) != 1 || paths[0] != "/p1" {
		t.Errorf("topRefs(paths, 1) = %v; want [/p1]", paths)
	}
	// Empty placeholder is correctly skipped:
	allPaths := topRefs(bits, BitKindPath, 10)
	if len(allPaths) != 2 {
		t.Errorf("topRefs(paths, 10) = %v; want 2 entries (empty placeholder skipped)", allPaths)
	}
}

func TestStableSortCounted(t *testing.T) {
	in := []countedRef{
		{Ref: "b", Count: 5}, {Ref: "a", Count: 5}, {Ref: "c", Count: 10},
	}
	out := stableSortCounted(in)
	// Expected: c (10), a (5, alphabetic before b), b (5)
	if out[0].Ref != "c" || out[1].Ref != "a" || out[2].Ref != "b" {
		t.Errorf("stableSortCounted = %+v; want c,a,b order", out)
	}
}

// helper: produce a slice of countedRef from bare strings (Count = len-i so
// the list is already in "frequency desc" order).
func counted(refs []string) []countedRef {
	out := make([]countedRef, len(refs))
	for i, r := range refs {
		out[i] = countedRef{Ref: r, Token: r, Count: int64(len(refs) - i)}
	}
	return out
}

// helper: synthesize N path entries with monotonically-decreasing counts.
func synthPaths(n int) []countedRef {
	out := make([]countedRef, n)
	for i := 0; i < n; i++ {
		ref := "/p/" + itoa(i)
		out[i] = countedRef{Ref: ref, Token: ref, Count: int64(n - i)}
	}
	return out
}
