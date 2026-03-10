package backup

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"mdemg/internal/jobs"
)

// runFullBackup performs a full Neo4j database dump via Docker.
func (s *Service) runFullBackup(ctx context.Context, job *jobs.Job, record *BackupRecord) error {
	container := s.cfg.Neo4jContainer
	cmd := s.cfg.FullCmd
	storageDir := s.cfg.StorageDir

	job.SetTotal(6)
	job.UpdateProgress(0, "stopping neo4j for dump")

	// Step 1: Stop the Neo4j database inside the container (required for dump).
	stopCmd := exec.CommandContext(ctx, cmd, "exec", container,
		"neo4j-admin", "database", "dump", "neo4j", "--to-path=/backup", "--overwrite-destination=true")
	if out, err := stopCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("neo4j-admin dump failed: %w\noutput: %s", err, string(out))
	}

	job.UpdateProgress(1, "copying dump from container")

	// Step 2: Copy the dump file out of the container.
	dumpFile := record.BackupID + ".dump"
	localPath := filepath.Join(storageDir, dumpFile)
	cpCmd := exec.CommandContext(ctx, cmd, "cp",
		fmt.Sprintf("%s:/backup/neo4j.dump", container), localPath)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp failed: %w\noutput: %s", err, string(out))
	}

	job.UpdateProgress(2, "cleaning up container")

	// Step 3: Clean up the temporary dump inside the container.
	cleanCmd := exec.CommandContext(ctx, cmd, "exec", container, "rm", "-rf", "/backup")
	_ = cleanCmd.Run() // best effort

	job.UpdateProgress(3, "computing checksum")

	// Step 4: Compute SHA256 of the dump file.
	checksum, err := sha256File(localPath)
	if err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	size, err := fileSize(localPath)
	if err != nil {
		return fmt.Errorf("file size: %w", err)
	}

	record.Checksum = checksum
	record.SizeBytes = size
	record.Path = localPath
	record.Status = "completed"
	now := time.Now().UTC()
	record.CompletedAt = &now

	job.UpdateProgress(4, "querying graph stats")

	// Step 5: Get node/edge counts for the manifest.
	nodeCount, edgeCount, _ := s.queryNodeEdgeCounts(ctx)
	schemaVer, _ := s.querySchemaVersion(ctx)
	spaces, _ := s.queryAllSpaceIDs(ctx)

	record.Spaces = spaces

	job.UpdateProgress(5, "writing manifest")

	// Step 6: Write manifest sidecar.
	manifest := BackupManifest{
		BackupID:      record.BackupID,
		Type:          string(record.Type),
		FormatVersion: "1.0",
		CreatedAt:     record.StartedAt.Format(time.RFC3339),
		Checksum:      checksum,
		SizeBytes:     size,
		Spaces:        spaces,
		NodeCount:     nodeCount,
		EdgeCount:     edgeCount,
		SchemaVersion: schemaVer,
		KeepForever:   record.KeepForever,
		Label:         record.Label,
	}

	if err := s.writeManifest(record, manifest); err != nil {
		return err
	}

	job.UpdateProgress(6, "done")
	return nil
}

// Restore performs a full database restore from a dump file.
func (s *Service) Restore(ctx context.Context, req RestoreRequest) (string, error) {
	// Load the manifest to get the file path.
	m, err := s.loadManifest(req.BackupID)
	if err != nil {
		return "", fmt.Errorf("load manifest: %w", err)
	}

	if m.Type != string(BackupTypeFull) {
		return "", fmt.Errorf("restore only supports full backups (got %s)", m.Type)
	}

	restoreID := fmt.Sprintf("restore-%s-%s", time.Now().UTC().Format("20060102-150405"), req.BackupID)
	q := jobs.GetQueue()
	job, jobCtx := q.CreateJob(restoreID, "restore", map[string]any{
		"backup_id":       req.BackupID,
		"snapshot_before": req.SnapshotBefore,
	})

	go func() {
		q.StartJob(restoreID)
		err := s.runRestore(jobCtx, job, req, m)
		if err != nil {
			job.Fail(err)
		} else {
			job.Complete(map[string]any{"backup_id": req.BackupID, "restore_id": restoreID})
		}
	}()

	return restoreID, nil
}

// runRestore executes the actual restore via docker exec.
func (s *Service) runRestore(ctx context.Context, job *jobs.Job, req RestoreRequest, _ *BackupManifest) error {
	container := s.cfg.Neo4jContainer
	cmd := s.cfg.FullCmd

	job.SetTotal(4)

	// Step 1: Optionally take a safety snapshot.
	if req.SnapshotBefore {
		job.UpdateProgress(0, "taking safety snapshot")
		_, err := s.Trigger(ctx, TriggerRequest{
			Type:        string(BackupTypeFull),
			KeepForever: true,
			Label:       fmt.Sprintf("pre-restore-snapshot-%s", req.BackupID),
		})
		if err != nil {
			return fmt.Errorf("safety snapshot: %w", err)
		}
		// Wait briefly for the snapshot job to start.
		time.Sleep(2 * time.Second)
	}

	// Step 2: Copy the dump file into the container.
	job.UpdateProgress(1, "copying dump to container")
	dumpPath := filepath.Join(s.cfg.StorageDir, req.BackupID+".dump")
	cpCmd := exec.CommandContext(ctx, cmd, "cp", dumpPath,
		fmt.Sprintf("%s:/backup/neo4j.dump", container))
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp (to container) failed: %w\noutput: %s", err, string(out))
	}

	// Step 3: Run neo4j-admin database load.
	job.UpdateProgress(2, "loading database dump")
	loadCmd := exec.CommandContext(ctx, cmd, "exec", container,
		"neo4j-admin", "database", "load", "neo4j", "--from-path=/backup", "--overwrite-destination=true")
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("neo4j-admin load failed: %w\noutput: %s", err, string(out))
	}

	// Step 4: Clean up.
	job.UpdateProgress(3, "cleaning up")
	cleanCmd := exec.CommandContext(ctx, cmd, "exec", container, "rm", "-rf", "/backup")
	_ = cleanCmd.Run()

	job.UpdateProgress(4, "done")
	return nil
}
