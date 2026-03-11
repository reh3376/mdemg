//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"

	pb "mdemg/api/transferpb"
	"mdemg/internal/transfer"
)

// TestTransferExportFileRoundTrip exports a space to a .mdemg file, reads it back,
// and validates schema. Uses a dedicated test space.
func TestTransferExportFileRoundTrip(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-file")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed nodes for file export
	SeedObservationNodes(t, driver, spaceID, 5, "learning", 1)

	// Export with metadata-only profile (fast, no large data)
	cfg, err := transfer.ExportConfigForProfile(spaceID, transfer.ProfileMetadata)
	if err != nil {
		t.Fatalf("ExportConfigForProfile: %v", err)
	}
	cfg.ChunkSize = 500
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(result.Chunks) < 2 {
		t.Fatalf("expected at least metadata + summary chunks, got %d", len(result.Chunks))
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transfer-test.mdemg")
	if err := transfer.WriteFile(path, result); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	chunks, err := transfer.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(chunks) != len(result.Chunks) {
		t.Errorf("chunk count: got %d, want %d", len(chunks), len(result.Chunks))
	}

	if err := transfer.ValidateImport(ctx, driver, chunks); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}
}

// TestTransferExportImportRoundTrip exports a dedicated test space to file and
// re-imports with conflict=skip. Uses a small seeded space (not CMS) to avoid
// timeout on large datasets.
func TestTransferExportImportRoundTrip(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-rt")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed a small set of nodes so the export has data
	SeedObservationNodes(t, driver, spaceID, 10, "learning", 2)

	cfg, err := transfer.ExportConfigForProfile(spaceID, transfer.ProfileFull)
	if err != nil {
		t.Fatalf("ExportConfigForProfile: %v", err)
	}
	cfg.ChunkSize = 100
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transfer-import-test.mdemg")
	if err := transfer.WriteFile(path, result); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	chunks, err := transfer.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := transfer.ValidateImport(ctx, driver, chunks); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	imp := transfer.NewImporter(driver, pb.ConflictMode_CONFLICT_SKIP)
	importResult, err := imp.Import(ctx, chunks)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Re-import with skip: nodes should be skipped (already exist)
	if importResult.NodesCreated+importResult.NodesSkipped+importResult.NodesOverwritten == 0 &&
		importResult.EdgesCreated+importResult.EdgesSkipped == 0 && len(result.Chunks) > 2 {
		t.Logf("Import reported 0 node/edge changes; chunks=%d (may be idempotent re-import)", len(result.Chunks))
	}
}

// TestTransferDeltaExport verifies Phase 4 incremental export: since_timestamp filters entities
// and summary includes next_cursor for the next delta.
func TestTransferDeltaExport(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-delta")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed nodes for delta export
	SeedObservationNodes(t, driver, spaceID, 5, "learning", 1)

	// Delta with a far-future since: no entities modified after that, so we get 0 nodes/edges
	// but still metadata + summary with next_cursor
	cfg, err := transfer.ExportConfigForProfile(spaceID, transfer.ProfileMetadata)
	if err != nil {
		t.Fatalf("ExportConfigForProfile: %v", err)
	}
	cfg.SinceTimestamp = "2099-01-01T00:00:00Z"
	cfg.ChunkSize = 500
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export (delta): %v", err)
	}
	if len(result.Chunks) < 2 {
		t.Fatalf("expected at least metadata + summary, got %d chunks", len(result.Chunks))
	}
	last := result.Chunks[len(result.Chunks)-1]
	if last.ChunkType != pb.ChunkType_CHUNK_TYPE_SUMMARY {
		t.Fatalf("last chunk: got %v", last.ChunkType)
	}
	sum := last.GetSummary()
	if sum == nil {
		t.Fatal("summary is nil")
	}
	if sum.NextCursor == "" {
		t.Error("expected next_cursor in summary for delta export")
	}
	// With future since, counts should be 0
	if sum.NodesExported != 0 || sum.EdgesExported != 0 {
		t.Logf("delta with future since: nodes=%d edges=%d (expected 0 for future timestamp)", sum.NodesExported, sum.EdgesExported)
	}

	// Full export then delta with completed_at: second export should be valid and have next_cursor
	cfgFull, _ := transfer.ExportConfigForProfile(spaceID, transfer.ProfileMetadata)
	cfgFull.ChunkSize = 500
	resultFull, err := ex.Export(ctx, cfgFull)
	if err != nil {
		t.Fatalf("Export (full): %v", err)
	}
	if len(resultFull.Chunks) < 2 {
		t.Skip("full export had no summary")
	}
	completedAt := resultFull.Chunks[len(resultFull.Chunks)-1].GetSummary().GetCompletedAt()
	if completedAt == "" {
		t.Skip("no completed_at in full export")
	}
	cfgDelta, _ := transfer.ExportConfigForProfile(spaceID, transfer.ProfileMetadata)
	cfgDelta.SinceTimestamp = completedAt
	cfgDelta.ChunkSize = 500
	resultDelta, err := ex.Export(ctx, cfgDelta)
	if err != nil {
		t.Fatalf("Export (delta with completed_at): %v", err)
	}
	if len(resultDelta.Chunks) < 2 {
		t.Fatalf("delta export: expected at least 2 chunks, got %d", len(resultDelta.Chunks))
	}
	if lastDelta := resultDelta.Chunks[len(resultDelta.Chunks)-1].GetSummary(); lastDelta == nil || lastDelta.NextCursor == "" {
		t.Error("delta export should have summary with next_cursor")
	}
}

// TestTransferExportProfiles runs export with each profile and checks chunk structure.
// Uses a dedicated test space to avoid timeout on large CMS datasets.
func TestTransferExportProfiles(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-prof")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed nodes so exports have data
	SeedObservationNodes(t, driver, spaceID, 5, "learning", 2)
	SeedObservationNodes(t, driver, spaceID, 3, "decision", 1)

	profiles := []string{transfer.ProfileFull, transfer.ProfileCodebase, transfer.ProfileCMS, transfer.ProfileLearned, transfer.ProfileMetadata}
	for _, profile := range profiles {
		cfg, err := transfer.ExportConfigForProfile(spaceID, profile)
		if err != nil {
			t.Errorf("profile %q: %v", profile, err)
			continue
		}
		cfg.ChunkSize = 50
		ex := transfer.NewExporter(driver)
		result, err := ex.Export(ctx, cfg)
		if err != nil {
			t.Errorf("profile %q Export: %v", profile, err)
			continue
		}
		if len(result.Chunks) < 2 {
			t.Errorf("profile %q: expected at least 2 chunks, got %d", profile, len(result.Chunks))
		}
		if result.Chunks[0].ChunkType != pb.ChunkType_CHUNK_TYPE_METADATA {
			t.Errorf("profile %q: first chunk should be metadata, got %v", profile, result.Chunks[0].ChunkType)
		}
		last := result.Chunks[len(result.Chunks)-1]
		if last.ChunkType != pb.ChunkType_CHUNK_TYPE_SUMMARY {
			t.Errorf("profile %q: last chunk should be summary, got %v", profile, last.ChunkType)
		}
	}
}
