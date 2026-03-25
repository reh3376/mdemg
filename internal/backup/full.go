package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"mdemg/internal/jobs"
	pb "mdemg/api/transferpb"
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
	// Load the manifest to verify the backup exists.
	_, err := s.loadManifest(req.BackupID)
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
		err := s.runRestore(jobCtx, job, req)
		if err != nil {
			job.Fail(err)
		} else {
			job.Complete(map[string]any{"backup_id": req.BackupID, "restore_id": restoreID})
		}
	}()

	return restoreID, nil
}

// runRestore executes the actual restore, detecting file format automatically.
func (s *Service) runRestore(ctx context.Context, job *jobs.Job, req RestoreRequest) error {
	// Step 0: Optionally take a safety snapshot.
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
		time.Sleep(2 * time.Second)
	}

	// Detect format: try .mdemg first (logical), then .dump (legacy physical).
	mdemgPath := filepath.Join(s.cfg.StorageDir, req.BackupID+".mdemg")
	dumpPath := filepath.Join(s.cfg.StorageDir, req.BackupID+".dump")

	if _, err := os.Stat(mdemgPath); err == nil {
		return s.restoreFromMdemg(ctx, job, mdemgPath)
	}
	if _, err := os.Stat(dumpPath); err == nil {
		return s.restoreFromDump(ctx, job, req, dumpPath)
	}

	return fmt.Errorf("no backup data file found for %q (tried .mdemg and .dump)", req.BackupID)
}

// restoreFromMdemg restores from a .mdemg logical export file.
func (s *Service) restoreFromMdemg(ctx context.Context, job *jobs.Job, path string) error {
	job.SetTotal(3)
	job.UpdateProgress(0, "reading backup file")

	chunks, err := transfer.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read .mdemg file: %w", err)
	}

	job.UpdateProgress(1, "importing nodes and edges")

	imp := transfer.NewImporter(s.driver, pb.ConflictMode_CONFLICT_SKIP)
	result, err := imp.Import(ctx, chunks)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	slog.Info("backup: restore imported", "nodes_created", result.NodesCreated, "edges_created", result.EdgesCreated, "edges_merged", result.EdgesMerged)

	job.UpdateProgress(2, "done")
	return nil
}

// restoreFromDump restores from a legacy .dump file via docker exec.
func (s *Service) restoreFromDump(ctx context.Context, job *jobs.Job, _ RestoreRequest, dumpPath string) error {
	container := s.cfg.Neo4jContainer
	cmd := s.cfg.FullCmd

	job.SetTotal(4)

	// Step 1: Copy the dump file into the container.
	job.UpdateProgress(0, "copying dump to container")
	cpCmd := exec.CommandContext(ctx, cmd, "cp", dumpPath,
		fmt.Sprintf("%s:/backup/neo4j.dump", container))
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp (to container) failed: %w\noutput: %s", err, string(out))
	}

	// Step 2: Run neo4j-admin database load.
	job.UpdateProgress(1, "loading database dump")
	loadCmd := exec.CommandContext(ctx, cmd, "exec", container,
		"neo4j-admin", "database", "load", "neo4j", "--from-path=/backup", "--overwrite-destination=true")
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("neo4j-admin load failed: %w\noutput: %s", err, string(out))
	}

	// Step 3: Clean up.
	job.UpdateProgress(2, "cleaning up")
	cleanCmd := exec.CommandContext(ctx, cmd, "exec", container, "rm", "-rf", "/backup")
	_ = cleanCmd.Run()

	job.UpdateProgress(3, "done")
	return nil
}
