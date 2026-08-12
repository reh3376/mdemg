package backup

import (
	"context"
	"fmt"
	"time"

	pb "mdemg/api/transferpb"
	"mdemg/internal/jobs"
	"mdemg/internal/transfer"
)

// runPartialBackup exports selected spaces via the transfer exporter.
func (s *Service) runPartialBackup(ctx context.Context, job *jobs.Job, record *BackupRecord, spaceIDs []string) error {
	job.UpdateProgress(0, "resolving spaces")

	// If no spaces specified, export all.
	if len(spaceIDs) == 0 {
		all, err := s.queryAllSpaceIDs(ctx)
		if err != nil {
			return fmt.Errorf("query space IDs: %w", err)
		}
		spaceIDs = all
	}

	// Always include the protected space.
	spaceIDs = ensureSpaceIncluded(spaceIDs, "mdemg-dev")
	record.Spaces = spaceIDs

	job.SetTotal(len(spaceIDs))

	// Collect export results from all spaces.
	var allResult transfer.ExportResult
	for i, spaceID := range spaceIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		job.UpdateProgress(i, fmt.Sprintf("exporting space %s", spaceID))

		cfg := transfer.DefaultExportConfig(spaceID)
		result, err := s.exporter.Export(ctx, cfg)
		if err != nil {
			return fmt.Errorf("export space %s: %w", spaceID, err)
		}
		allResult.Chunks = append(allResult.Chunks, result.Chunks...)
	}

	// Write combined .mdemg file. SEC-TRANCHE-2: server-generated
	// BackupID from Trigger() (`bk-<ts>-<type>`) is safe by
	// construction, but route through safeBackupPath uniformly (same
	// rationale as writeManifest — defense against a future refactor
	// widening the input surface + closes CodeQL flow).
	outPath, err := s.safeBackupPath(record.BackupID + ".mdemg")
	if err != nil {
		return fmt.Errorf("invalid backup id: %w", err)
	}
	if err := transfer.WriteFile(outPath, &allResult); err != nil {
		return fmt.Errorf("write mdemg file: %w", err)
	}

	job.UpdateProgress(len(spaceIDs), "computing checksum")

	// Checksum + size.
	checksum, err := sha256File(outPath)
	if err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	size, err := fileSize(outPath)
	if err != nil {
		return fmt.Errorf("file size: %w", err)
	}

	record.Checksum = checksum
	record.SizeBytes = size
	record.Path = outPath
	record.Status = "completed"
	now := time.Now().UTC()
	record.CompletedAt = &now

	// Node/edge counts and schema version for manifest.
	nodeCount, edgeCount, _ := s.queryNodeEdgeCounts(ctx)
	schemaVer, _ := s.querySchemaVersion(ctx)

	// BACKUP-RESTORE-VERIFY-001: count what the FILE contains — the
	// whole-database counts above diverge from file contents on partial
	// backups, so they cannot validate a restore.
	fileNodes, fileEdges, fileObs := countChunkContents(allResult.Chunks)

	manifest := BackupManifest{
		BackupID:      record.BackupID,
		Type:          string(record.Type),
		FormatVersion: "1.0",
		CreatedAt:     record.StartedAt.Format(time.RFC3339),
		Checksum:      checksum,
		SizeBytes:     size,
		Spaces:        spaceIDs,
		NodeCount:     nodeCount,
		EdgeCount:     edgeCount,
		SchemaVersion: schemaVer,
		KeepForever:   record.KeepForever,
		Label:         record.Label,

		FileNodeCount:        fileNodes,
		FileEdgeCount:        fileEdges,
		FileObservationCount: fileObs,
	}

	return s.writeManifest(record, manifest)
}

// countChunkContents tallies the nodes/edges/observations actually present
// in an export chunk stream (BACKUP-RESTORE-VERIFY-001 — the restore
// validation reference).
func countChunkContents(chunks []*pb.SpaceChunk) (nodes, edges, observations int64) {
	for _, c := range chunks {
		if c == nil {
			continue
		}
		if nb := c.GetNodes(); nb != nil {
			nodes += int64(len(nb.GetNodes()))
		}
		if eb := c.GetEdges(); eb != nil {
			edges += int64(len(eb.GetEdges()))
		}
		if ob := c.GetObservations(); ob != nil {
			observations += int64(len(ob.GetObservations()))
		}
	}
	return nodes, edges, observations
}
