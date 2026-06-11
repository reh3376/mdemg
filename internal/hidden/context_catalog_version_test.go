package hidden

import (
	"encoding/json"
	"testing"
)

// TSDB-CONSUME-001: the V0020 row flattening — per-kind counts must sum to
// the allocation, top-ref arrays follow their kinds, JSON round-trips.
func TestCatalogVersionRecord_Flatten(t *testing.T) {
	bits := []BitEntry{
		{Position: 0, Kind: BitKindRoleTypeLayer, Ref: "decision|0", Token: "(decision, L0)"},
		{Position: 1, Kind: BitKindRoleTypeLayer, Ref: "constraint|0", Token: "(constraint, L0)"},
		{Position: 2, Kind: BitKindTag, Ref: "go", Token: "go"},
		{Position: 3, Kind: BitKindPath, Ref: "internal/api/server.go", Token: "server.go"},
		{Position: 4, Kind: BitKindPath, Ref: "internal/tsdb/client.go", Token: "client.go"},
		{Position: 5, Kind: BitKindSymbol, Ref: "n_abc123", Token: "NewServer"},
	}
	rec := catalogVersionRecord("test-space", 7, 256, bits)

	if rec.SpaceID != "test-space" || rec.Version != 7 || rec.TotalBits != 256 {
		t.Errorf("header fields = %+v", rec)
	}
	if rec.BitsRoleTypeLayer != 2 || rec.BitsTag != 1 || rec.BitsPath != 2 || rec.BitsSymbol != 1 {
		t.Errorf("kind counts = rtl:%d tag:%d path:%d sym:%d",
			rec.BitsRoleTypeLayer, rec.BitsTag, rec.BitsPath, rec.BitsSymbol)
	}
	if len(rec.TopPaths) != 2 || rec.TopPaths[0] != "internal/api/server.go" {
		t.Errorf("TopPaths = %v", rec.TopPaths)
	}
	if len(rec.TopSymbols) != 1 || len(rec.TopTags) != 1 {
		t.Errorf("TopSymbols/TopTags = %v/%v", rec.TopSymbols, rec.TopTags)
	}
	var back []BitEntry
	if err := json.Unmarshal(rec.AllocationJSON, &back); err != nil || len(back) != 6 {
		t.Errorf("AllocationJSON round-trip: err=%v len=%d", err, len(back))
	}
}

// Nil recorder (zero-value BuilderOpts) must stay a no-op path — the CLI
// backfill (migrate_context_fingerprint) intentionally builds without one.
func TestBuilderOpts_NilVersionRecorderIsValid(t *testing.T) {
	var opts BuilderOpts
	if opts.VersionRecorder != nil {
		t.Fatal("zero-value BuilderOpts must have nil VersionRecorder")
	}
}
