package hidden

import (
	"strconv"
	"testing"
)

func itoa(i int) string { return strconv.Itoa(i) }

func TestNewCatalog_BuildsLookupTables(t *testing.T) {
	// Build a full 256-entry catalog with specific known bits at the
	// positions we'll assert on, padding the gaps with synthetic entries
	// so each Ref string is unique within its kind.
	known := map[uint16]BitEntry{
		0:   {Position: 0, Kind: BitKindRoleTypeLayer, Ref: "leaf|0", Token: "(leaf, L0)"},
		1:   {Position: 1, Kind: BitKindRoleTypeLayer, Ref: "hidden|1", Token: "(hidden, L1)"},
		32:  {Position: 32, Kind: BitKindTag, Ref: "typescript", Token: "typescript"},
		33:  {Position: 33, Kind: BitKindTag, Ref: "error-handling", Token: "error-handling"},
		64:  {Position: 64, Kind: BitKindPath, Ref: "/foo/bar.ts", Token: "/foo/bar.ts"},
		200: {Position: 200, Kind: BitKindSymbol, Ref: "n_abc123", Token: "ErrorHandler"},
	}
	bits := make([]BitEntry, 256)
	for i := uint16(0); i < 256; i++ {
		if e, ok := known[i]; ok {
			bits[i] = e
			continue
		}
		// pad with kind-appropriate synthetic entries
		switch {
		case i < 32:
			bits[i] = BitEntry{Position: i, Kind: BitKindRoleTypeLayer, Ref: "rt" + itoa(int(i)), Token: "pad"}
		case i < 64:
			bits[i] = BitEntry{Position: i, Kind: BitKindTag, Ref: "tag" + itoa(int(i)), Token: "pad"}
		case i < 200:
			bits[i] = BitEntry{Position: i, Kind: BitKindPath, Ref: "/p/" + itoa(int(i)), Token: "pad"}
		default:
			bits[i] = BitEntry{Position: i, Kind: BitKindSymbol, Ref: "sym" + itoa(int(i)), Token: "pad"}
		}
	}
	SortBitsByPosition(bits)

	c, err := NewCatalog("whk-wms", 1, 256, bits)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	// RoleTypeLayer lookups
	if bit, ok := c.RoleTypeLayerBit("leaf", 0); !ok || bit != 0 {
		t.Errorf("RoleTypeLayerBit(leaf, 0) = (%d, %v); want (0, true)", bit, ok)
	}
	if bit, ok := c.RoleTypeLayerBit("hidden", 1); !ok || bit != 1 {
		t.Errorf("RoleTypeLayerBit(hidden, 1) = (%d, %v); want (1, true)", bit, ok)
	}
	if _, ok := c.RoleTypeLayerBit("unknown", 99); ok {
		t.Errorf("RoleTypeLayerBit(unknown, 99) should miss")
	}

	// Tag lookups
	if bit, ok := c.TagBit("typescript"); !ok || bit != 32 {
		t.Errorf("TagBit(typescript) = (%d, %v); want (32, true)", bit, ok)
	}
	if _, ok := c.TagBit("not-in-catalog"); ok {
		t.Errorf("TagBit(not-in-catalog) should miss")
	}

	// Path lookups
	if bit, ok := c.PathBit("/foo/bar.ts"); !ok || bit != 64 {
		t.Errorf("PathBit(/foo/bar.ts) = (%d, %v); want (64, true)", bit, ok)
	}

	// Symbol lookups
	if bit, ok := c.SymbolBit("n_abc123"); !ok || bit != 200 {
		t.Errorf("SymbolBit(n_abc123) = (%d, %v); want (200, true)", bit, ok)
	}

	// RoleBit always returns (0, false) — see godoc
	if _, ok := c.RoleBit("anything"); ok {
		t.Errorf("RoleBit should always return (0, false) until role data exists")
	}
	// LayerBit always returns (0, false) — see godoc
	if _, ok := c.LayerBit(0); ok {
		t.Errorf("LayerBit should always return (0, false) until split out")
	}
}

func TestNewCatalog_RejectsLengthMismatch(t *testing.T) {
	bits := []BitEntry{{Position: 0, Kind: BitKindPath, Ref: "/p", Token: "/p"}}
	_, err := NewCatalog("s", 1, 256, bits) // 1 != 256
	if err == nil {
		t.Fatalf("expected error on len(bits)=1 != totalBits=256")
	}
}

func TestNewCatalog_RejectsEmptySpace(t *testing.T) {
	_, err := NewCatalog("", 1, 0, nil)
	if err == nil {
		t.Fatalf("expected error on empty space_id")
	}
}

func TestNewCatalog_RejectsUnknownKind(t *testing.T) {
	bits := make([]BitEntry, 1)
	bits[0] = BitEntry{Position: 0, Kind: BitKind("bogus"), Ref: "x", Token: "x"}
	_, err := NewCatalog("s", 1, 1, bits)
	if err == nil {
		t.Fatalf("expected error on unknown BitKind")
	}
}

func TestCatalog_NilSafe(t *testing.T) {
	var c *Catalog // nil receiver
	if _, ok := c.PathBit("/x"); ok {
		t.Errorf("nil PathBit should return false")
	}
	if _, ok := c.TagBit("x"); ok {
		t.Errorf("nil TagBit should return false")
	}
	if _, ok := c.RoleTypeLayerBit("x", 0); ok {
		t.Errorf("nil RoleTypeLayerBit should return false")
	}
	if _, ok := c.SymbolBit("x"); ok {
		t.Errorf("nil SymbolBit should return false")
	}
}

func TestSortBitsByPosition_Stable(t *testing.T) {
	bits := []BitEntry{
		{Position: 5, Kind: BitKindPath, Ref: "/c", Token: "c"},
		{Position: 1, Kind: BitKindTag, Ref: "a", Token: "a"},
		{Position: 3, Kind: BitKindRoleTypeLayer, Ref: "leaf|0", Token: "b"},
	}
	SortBitsByPosition(bits)
	if bits[0].Position != 1 || bits[1].Position != 3 || bits[2].Position != 5 {
		t.Errorf("sort failed: %+v", bits)
	}
}
