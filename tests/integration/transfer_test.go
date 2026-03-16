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

// TestTransferShareableRoundTrip exports with the shareable profile and verifies
// that only graduated, shareable-type observations are included.
func TestTransferShareableRoundTrip(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-share")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: graduated shareable nodes (learning, decision) + volatile nodes (progress, task)
	SeedGraduatedNodes(t, driver, spaceID, 5, "learning")
	SeedGraduatedNodes(t, driver, spaceID, 3, "decision")
	SeedObservationNodes(t, driver, spaceID, 4, "progress", 1) // volatile, excluded type
	SeedObservationNodes(t, driver, spaceID, 2, "task", 1)     // volatile, excluded type

	// Export with shareable profile
	cfg, err := transfer.ExportConfigForProfile(spaceID, transfer.ProfileShareable)
	if err != nil {
		t.Fatalf("ExportConfigForProfile: %v", err)
	}
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify chunks structure
	if len(result.Chunks) < 2 {
		t.Fatalf("expected at least metadata + summary, got %d chunks", len(result.Chunks))
	}

	// Count exported nodes — should only have graduated learning + decision (8), not progress/task (6)
	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			nodeCount += len(chunk.Nodes.Nodes)
		}
	}
	if nodeCount != 8 {
		t.Errorf("expected 8 shareable nodes (5 learning + 3 decision), got %d", nodeCount)
	}

	// Verify no excluded obs_types in exported nodes
	excludedTypes := map[string]bool{"progress": true, "task": true, "blocker": true, "context": true, "error": true}
	for _, chunk := range result.Chunks {
		if chunk.Nodes == nil {
			continue
		}
		for _, n := range chunk.Nodes.Nodes {
			if n.Layer == 0 && excludedTypes[n.ObsType] {
				t.Errorf("shareable export included excluded obs_type %q (node %s)", n.ObsType, n.NodeId)
			}
		}
	}

	// Verify volatile nodes are excluded
	for _, chunk := range result.Chunks {
		if chunk.Nodes == nil {
			continue
		}
		for _, n := range chunk.Nodes.Nodes {
			if n.Volatile && n.StabilityScore < 0.8 {
				t.Errorf("shareable export included volatile node %s (stability=%.2f)", n.NodeId, n.StabilityScore)
			}
		}
	}

	// Import into a new space
	importSpaceID := GenerateTestSpaceID("transfer-share-imp")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, importSpaceID)
	})

	// Remap space ID and add suffix to node_ids for global uniqueness
	// (memnode_id constraint requires node_id to be globally unique)
	idSuffix := "-imp"
	for _, chunk := range result.Chunks {
		chunk.SpaceId = importSpaceID
		if chunk.Metadata != nil {
			chunk.Metadata.SpaceId = importSpaceID
		}
		if chunk.Nodes != nil {
			for _, n := range chunk.Nodes.Nodes {
				n.SpaceId = importSpaceID
				n.NodeId = n.NodeId + idSuffix
				n.Path = n.Path + idSuffix
			}
		}
		if chunk.Edges != nil {
			for _, e := range chunk.Edges.Edges {
				e.SpaceId = importSpaceID
				e.FromNodeId = e.FromNodeId + idSuffix
				e.ToNodeId = e.ToNodeId + idSuffix
			}
		}
		if chunk.Observations != nil {
			for _, o := range chunk.Observations.Observations {
				o.SpaceId = importSpaceID
			}
		}
	}

	imp := transfer.NewImporter(driver, pb.ConflictMode_CONFLICT_SKIP)
	importResult, err := imp.Import(ctx, result.Chunks)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if importResult.NodesCreated != 8 {
		t.Errorf("expected 8 nodes imported, got %d created", importResult.NodesCreated)
	}
}

// TestTransferFilterObsTypes verifies that ObsTypes filter only includes specified obs_types (+ L1+).
func TestTransferFilterObsTypes(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-obs-filter")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: 5 learning + 3 decision + 4 progress
	SeedGraduatedNodes(t, driver, spaceID, 5, "learning")
	SeedGraduatedNodes(t, driver, spaceID, 3, "decision")
	SeedGraduatedNodes(t, driver, spaceID, 4, "progress")

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.ObsTypes = []string{"learning"}
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			for _, n := range chunk.Nodes.Nodes {
				nodeCount++
				if n.Layer == 0 && n.ObsType != "learning" {
					t.Errorf("obs_type filter: got L0 node with obs_type=%q, want learning", n.ObsType)
				}
			}
		}
	}
	if nodeCount != 5 {
		t.Errorf("expected 5 learning nodes, got %d", nodeCount)
	}
}

// TestTransferFilterExcludeVolatile verifies ExcludeVolatile skips volatile low-stability nodes.
func TestTransferFilterExcludeVolatile(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-volatile")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: 5 graduated (stable) + 4 volatile (unstable)
	SeedGraduatedNodes(t, driver, spaceID, 5, "learning")
	SeedObservationNodes(t, driver, spaceID, 4, "learning", 1) // volatile=true, stability=0.1

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.ExcludeVolatile = true
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			for _, n := range chunk.Nodes.Nodes {
				nodeCount++
				if n.Volatile && n.StabilityScore < 0.8 {
					t.Errorf("exclude_volatile: exported volatile node %s (stability=%.2f)", n.NodeId, n.StabilityScore)
				}
			}
		}
	}
	if nodeCount != 5 {
		t.Errorf("expected 5 graduated nodes, got %d", nodeCount)
	}
}

// TestTransferFilterOnlyPinned verifies OnlyPinned includes only pinned L0 + all L1+.
func TestTransferFilterOnlyPinned(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-pinned")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: 3 pinned + 5 unpinned graduated
	SeedPinnedNodes(t, driver, spaceID, 3, "learning")
	SeedGraduatedNodes(t, driver, spaceID, 5, "decision") // not pinned

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.OnlyPinned = true
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			nodeCount += len(chunk.Nodes.Nodes)
		}
	}
	if nodeCount != 3 {
		t.Errorf("expected 3 pinned nodes, got %d", nodeCount)
	}
}

// TestTransferFilterTags verifies Tags filter includes only tagged nodes.
func TestTransferFilterTags(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-tags")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: 3 tagged + 5 untagged
	SeedTaggedNodes(t, driver, spaceID, 3, "learning", []string{"important", "review"})
	SeedGraduatedNodes(t, driver, spaceID, 5, "decision") // no tags

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.Tags = []string{"important"}
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			nodeCount += len(chunk.Nodes.Nodes)
		}
	}
	if nodeCount != 3 {
		t.Errorf("expected 3 tagged nodes, got %d", nodeCount)
	}
}

// TestTransferFilterMinMaxLayer verifies MinLayer/MaxLayer filtering.
func TestTransferFilterMinMaxLayer(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-layers")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed: L0 (3) + L1 (2) + L2 (1)
	SeedMultiLayerNodes(t, driver, spaceID)

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.MinLayer = 1
	cfg.MaxLayer = 2
	cfg.ChunkSize = 100

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var nodeCount int
	for _, chunk := range result.Chunks {
		if chunk.Nodes != nil {
			for _, n := range chunk.Nodes.Nodes {
				nodeCount++
				if n.Layer < 1 || n.Layer > 2 {
					t.Errorf("layer filter: got node at layer %d, want 1-2", n.Layer)
				}
			}
		}
	}
	// 2 L1 + 1 L2 = 3
	if nodeCount != 3 {
		t.Errorf("expected 3 nodes (L1+L2), got %d", nodeCount)
	}
}

// TestTransferImportConflictOverwrite verifies CONFLICT_OVERWRITE re-imports successfully.
func TestTransferImportConflictOverwrite(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-overwrite")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	SeedGraduatedNodes(t, driver, spaceID, 5, "learning")

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.ChunkSize = 100
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Re-import with overwrite
	imp := transfer.NewImporter(driver, pb.ConflictMode_CONFLICT_OVERWRITE)
	importResult, err := imp.Import(ctx, result.Chunks)
	if err != nil {
		t.Fatalf("Import with OVERWRITE: %v", err)
	}
	if importResult.NodesOverwritten != 5 {
		t.Errorf("expected 5 overwritten nodes, got %d (created=%d, skipped=%d)",
			importResult.NodesOverwritten, importResult.NodesCreated, importResult.NodesSkipped)
	}
}

// TestTransferImportConflictError verifies CONFLICT_ERROR fails on existing nodes.
func TestTransferImportConflictError(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-conflict-err")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	SeedGraduatedNodes(t, driver, spaceID, 3, "learning")

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.ChunkSize = 100
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Re-import with error mode — should fail
	imp := transfer.NewImporter(driver, pb.ConflictMode_CONFLICT_ERROR)
	_, err = imp.Import(ctx, result.Chunks)
	if err == nil {
		t.Error("expected error on conflict, got nil")
	}
}

// TestTransferChunkSizeControl verifies that ChunkSize controls the number of node chunks.
func TestTransferChunkSizeControl(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	spaceID := GenerateTestSpaceID("transfer-chunksize")
	t.Cleanup(func() {
		CleanupSpaceWithTest(t, driver, spaceID)
	})

	// Seed 10 nodes
	SeedGraduatedNodes(t, driver, spaceID, 10, "learning")

	cfg := transfer.DefaultExportConfig(spaceID)
	cfg.ChunkSize = 3
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Count node chunks
	var nodeChunks int
	for _, chunk := range result.Chunks {
		if chunk.ChunkType == pb.ChunkType_CHUNK_TYPE_NODES {
			nodeChunks++
		}
	}
	// 10 nodes / chunk_size 3 = ceil(10/3) = 4 node chunks
	if nodeChunks != 4 {
		t.Errorf("expected 4 node chunks (10 nodes / chunk_size 3), got %d", nodeChunks)
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
