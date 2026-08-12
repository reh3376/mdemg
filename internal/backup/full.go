package backup

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"time"

	pb "mdemg/api/transferpb"
	"mdemg/internal/dockerbin"
	"mdemg/internal/jobs"
	"mdemg/internal/transfer"
)

// runFullBackup performs a full logical backup of all spaces.
// This delegates to runPartialBackup with nil spaceIDs (= all spaces),
// producing a portable .mdemg file that works with a live database.
func (s *Service) runFullBackup(ctx context.Context, job *jobs.Job, record *BackupRecord) error {
	return s.runPartialBackup(ctx, job, record, nil)
}

// Restore starts an asynchronous restore from a backup.
// Supports both .mdemg (logical) and legacy .dump (physical) formats.
func (s *Service) Restore(ctx context.Context, req RestoreRequest) (string, error) {
	// Load the manifest to verify the backup exists (and to drive the
	// checksum gate + count validation in the restore job).
	manifest, err := s.loadManifest(req.BackupID)
	if err != nil {
		return "", fmt.Errorf("load manifest: %w", err)
	}

	restoreID := fmt.Sprintf("restore-%s-%s", time.Now().UTC().Format("20060102-150405"), req.BackupID)
	q := jobs.GetQueue()
	job, jobCtx := q.CreateJob(restoreID, "restore", map[string]any{
		"backup_id":       req.BackupID,
		"snapshot_before": req.SnapshotBefore,
	})

	go func() { //nolint:gosec // G118: restore job runs in background, must outlive HTTP request
		q.StartJob(restoreID)
		extras, err := s.runRestore(jobCtx, job, req, manifest)
		if err != nil {
			job.Fail(err)
		} else {
			result := map[string]any{"backup_id": req.BackupID, "restore_id": restoreID}
			maps.Copy(result, extras)
			job.Complete(result)
		}
	}()

	return restoreID, nil
}

// runRestore executes the actual restore, detecting file format
// automatically. The returned map is merged into the job result (e.g. the
// restore validation block).
func (s *Service) runRestore(ctx context.Context, job *jobs.Job, req RestoreRequest, manifest *BackupManifest) (map[string]any, error) {
	// Step 0: Optionally take a safety snapshot. The snapshot is the undo
	// path for the restore itself, so the restore MUST NOT proceed until
	// the snapshot job has actually completed (BACKUP-RESTORE-VERIFY-001;
	// previously a 2s sleep raced the async backup goroutine).
	if req.SnapshotBefore {
		job.UpdateProgress(0, "taking safety snapshot")
		snapshotID, err := s.Trigger(ctx, TriggerRequest{
			Type:        string(BackupTypeFull),
			KeepForever: true,
			Label:       fmt.Sprintf("pre-restore-snapshot-%s", req.BackupID),
		})
		if err != nil {
			return nil, fmt.Errorf("safety snapshot: %w", err)
		}
		if err := s.waitForBackupJob(ctx, snapshotID); err != nil {
			return nil, fmt.Errorf("safety snapshot %s: %w", snapshotID, err)
		}
	}

	// Detect format: try .mdemg first (logical), then .dump (legacy
	// physical). SEC-TRANCHE-2 (2026-08-11): req.BackupID reaches here
	// from POST /v1/backup/restore with no upstream validation; without
	// safeBackupPath containment, `"../../etc/hosts"` would let the
	// os.Stat probes surface arbitrary-filesystem existence to the
	// caller (info-leak) and, if the extension happened to match,
	// trigger the restore path against attacker-controlled data.
	mdemgPath, err := s.safeBackupPath(req.BackupID + ".mdemg")
	if err != nil {
		return nil, fmt.Errorf("invalid backup id: %w", err)
	}
	dumpPath, err := s.safeBackupPath(req.BackupID + ".dump")
	if err != nil {
		return nil, fmt.Errorf("invalid backup id: %w", err)
	}

	if _, err := os.Stat(mdemgPath); err == nil {
		return s.restoreFromMdemg(ctx, job, mdemgPath, manifest)
	}
	if _, err := os.Stat(dumpPath); err == nil {
		return s.restoreFromDump(ctx, job, req, dumpPath)
	}

	return nil, fmt.Errorf("no backup data file found for %q (tried .mdemg and .dump)", req.BackupID)
}

// waitForBackupJob polls the jobs queue until the given backup job
// completes. Fails closed on job failure, cancellation, disappearance, or
// timeout (SnapshotWaitTimeoutSec, default 3600 — a whole-database export
// runs ~15 min live) — a restore without its safety snapshot is a
// data-loss risk, not a degraded mode.
func (s *Service) waitForBackupJob(ctx context.Context, backupID string) error {
	timeout := time.Duration(s.cfg.SnapshotWaitTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 3600 * time.Second
	}
	deadline := time.Now().Add(timeout)
	q := jobs.GetQueue()

	for {
		j, ok := q.GetJob(backupID)
		if !ok {
			return fmt.Errorf("snapshot job vanished from queue")
		}
		switch snap := j.GetSnapshot(); snap.Status {
		case jobs.StatusCompleted:
			return nil
		case jobs.StatusFailed:
			return fmt.Errorf("snapshot job failed: %v", snap.Error)
		case jobs.StatusCancelled:
			return fmt.Errorf("snapshot job was cancelled")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("snapshot did not complete within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// restoreFromMdemg restores from a .mdemg logical export file. Returns the
// validation block merged into the job result.
func (s *Service) restoreFromMdemg(ctx context.Context, job *jobs.Job, path string, manifest *BackupManifest) (map[string]any, error) {
	job.SetTotal(4)

	// Integrity gate (BACKUP-RESTORE-VERIFY-001): the manifest checksum was
	// written at backup time and never verified — a corrupted or truncated
	// file restored silently. Fail closed before touching the graph.
	job.UpdateProgress(0, "verifying checksum")
	if manifest != nil && manifest.Checksum != "" {
		actual, err := sha256File(path)
		if err != nil {
			return nil, fmt.Errorf("checksum backup file: %w", err)
		}
		if actual != manifest.Checksum {
			return nil, fmt.Errorf("backup integrity check failed: file checksum %s does not match manifest %s — refusing to import", actual, manifest.Checksum)
		}
	} else {
		slog.Warn("backup: manifest has no checksum (legacy backup) — restoring without integrity verification", "path", path)
	}

	job.UpdateProgress(1, "reading backup file")

	chunks, err := transfer.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .mdemg file: %w", err)
	}

	// File-completeness validation: compare chunk contents against the
	// manifest's file_* counts (written from the exported chunks). Zero
	// counts = legacy manifest → warn-only.
	readNodes, readEdges, readObs := countChunkContents(chunks)
	if manifest != nil && (manifest.FileNodeCount > 0 || manifest.FileEdgeCount > 0 || manifest.FileObservationCount > 0) {
		if readNodes != manifest.FileNodeCount || readEdges != manifest.FileEdgeCount || readObs != manifest.FileObservationCount {
			return nil, fmt.Errorf("backup completeness check failed: file contains nodes=%d edges=%d observations=%d but manifest recorded %d/%d/%d — refusing to import a truncated backup",
				readNodes, readEdges, readObs,
				manifest.FileNodeCount, manifest.FileEdgeCount, manifest.FileObservationCount)
		}
	}

	job.UpdateProgress(2, "importing nodes and edges")

	imp := transfer.NewImporter(s.driver, pb.ConflictMode_CONFLICT_SKIP)
	result, err := imp.Import(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}

	// Post-restore accounting: under CONFLICT_SKIP, created+skipped should
	// account for what was read; divergence is warn-level (legitimate when
	// restoring into a non-empty graph or when edges drop for missing
	// endpoints) but always surfaced in the job result.
	processedNodes := int64(result.NodesCreated + result.NodesSkipped + result.NodesOverwritten)
	processedEdges := int64(result.EdgesCreated + result.EdgesSkipped + result.EdgesMerged)
	validation := map[string]any{
		"checksum_verified":    manifest != nil && manifest.Checksum != "",
		"file_nodes":           readNodes,
		"file_edges":           readEdges,
		"file_observations":    readObs,
		"nodes_processed":      processedNodes,
		"edges_processed":      processedEdges,
		"nodes_created":        result.NodesCreated,
		"nodes_skipped":        result.NodesSkipped,
		"edges_created":        result.EdgesCreated,
		"edges_merged":         result.EdgesMerged,
		"observations_created": result.ObservationsCreated,
	}
	if processedNodes < readNodes || processedEdges < readEdges {
		slog.Warn("backup: restore processed fewer items than the file contains",
			"file_nodes", readNodes, "nodes_processed", processedNodes,
			"file_edges", readEdges, "edges_processed", processedEdges,
			"warnings", len(result.Warnings))
	}

	slog.Info("backup: restore imported",
		"nodes_created", result.NodesCreated, "edges_created", result.EdgesCreated,
		"edges_merged", result.EdgesMerged, "checksum_verified", manifest != nil && manifest.Checksum != "")

	job.UpdateProgress(3, "done")
	return map[string]any{"validation": validation}, nil
}

// dockerCommand resolves the docker binary for the legacy .dump path:
// an explicit non-default FullCmd is an operator override; otherwise route
// through dockerbin (BACKUP-RESTORE-VERIFY-001 — bare "docker" fails under
// the launchd minimal PATH, the exact NOSILENT-001 failure class).
func (s *Service) dockerCommand() string {
	if s.cfg.FullCmd != "" && s.cfg.FullCmd != "docker" {
		return s.cfg.FullCmd
	}
	return dockerbin.Path()
}

// restoreFromDump restores from a legacy .dump file via docker exec.
func (s *Service) restoreFromDump(ctx context.Context, job *jobs.Job, _ RestoreRequest, dumpPath string) (map[string]any, error) {
	container := s.cfg.Neo4jContainer
	cmd := s.dockerCommand()

	job.SetTotal(4)
	slog.Warn("backup: restoring legacy .dump format — no checksum or count validation available for this path", "path", dumpPath)

	// Step 1: Copy the dump file into the container.
	job.UpdateProgress(0, "copying dump to container")
	cpCmd := exec.CommandContext(ctx, cmd, "cp", dumpPath,
		fmt.Sprintf("%s:/backup/neo4j.dump", container))
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (to container) failed: %w\noutput: %s", err, string(out))
	}

	// Step 2: Run neo4j-admin database load.
	job.UpdateProgress(1, "loading database dump")
	loadCmd := exec.CommandContext(ctx, cmd, "exec", container,
		"neo4j-admin", "database", "load", "neo4j", "--from-path=/backup", "--overwrite-destination=true")
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("neo4j-admin load failed: %w\noutput: %s", err, string(out))
	}

	// Step 3: Clean up.
	job.UpdateProgress(2, "cleaning up")
	cleanCmd := exec.CommandContext(ctx, cmd, "exec", container, "rm", "-rf", "/backup")
	_ = cleanCmd.Run()

	job.UpdateProgress(3, "done")
	return map[string]any{"validation": map[string]any{"checksum_verified": false, "legacy_dump": true}}, nil
}
