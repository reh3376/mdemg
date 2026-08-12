package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/jobs"
	"mdemg/internal/pathsafe"
	"mdemg/internal/transfer"
)

// Service orchestrates backup, restore, and retention operations.
type Service struct {
	cfg      Config
	driver   neo4j.DriverWithContext
	exporter *transfer.Exporter
	mu       sync.Mutex // serialise backup triggers
}

// NewService creates a new backup service.
func NewService(cfg Config, driver neo4j.DriverWithContext, exporter *transfer.Exporter) *Service {
	// Ensure storage directory exists.
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		slog.Warn("backup: cannot create storage dir", "dir", cfg.StorageDir, "error", err)
	}
	return &Service{cfg: cfg, driver: driver, exporter: exporter}
}

// Trigger starts a backup asynchronously and returns the backup ID.
func (s *Service) Trigger(ctx context.Context, req TriggerRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bt := BackupType(req.Type)
	if bt != BackupTypeFull && bt != BackupTypePartial {
		return "", fmt.Errorf("invalid backup type: %q (must be %q or %q)", req.Type, BackupTypeFull, BackupTypePartial)
	}

	now := time.Now().UTC()
	backupID := fmt.Sprintf("bk-%s-%s", now.Format("20060102-150405"), req.Type)

	record := &BackupRecord{
		BackupID:    backupID,
		Type:        bt,
		Status:      "pending",
		StartedAt:   now,
		Spaces:      req.SpaceIDs,
		KeepForever: req.KeepForever,
		Label:       req.Label,
	}

	q := jobs.GetQueue()
	job, jobCtx := q.CreateJob(backupID, "backup_"+req.Type, map[string]any{
		"type":         req.Type,
		"space_ids":    req.SpaceIDs,
		"keep_forever": req.KeepForever,
		"label":        req.Label,
	})

	go func() { //nolint:gosec // G118: backup job runs in background, must outlive HTTP request
		q.StartJob(backupID)
		var err error
		switch bt {
		case BackupTypeFull:
			err = s.runFullBackup(jobCtx, job, record)
		case BackupTypePartial:
			err = s.runPartialBackup(jobCtx, job, record, req.SpaceIDs)
		}
		if err != nil {
			slog.Error("backup failed", "backup_id", backupID, "error", err)
			job.Fail(err)
		} else {
			job.Complete(map[string]any{
				"backup_id": backupID,
				"path":      record.Path,
				"checksum":  record.Checksum,
				"size":      record.SizeBytes,
			})
			slog.Info("backup completed", "backup_id", backupID, "size_bytes", record.SizeBytes)
		}

		// Run retention after backup if configured.
		if s.cfg.RetentionRunAfter {
			if res, retErr := s.RunRetention(context.Background()); retErr != nil {
				slog.Error("backup: retention failed", "backup_id", backupID, "error", retErr)
			} else if len(res.Deleted) > 0 {
				slog.Info("backup: retention cleaned backups", "deleted_count", len(res.Deleted), "freed_bytes", res.FreedBytes)
			}
		}
	}()

	return backupID, nil
}

// GetBackup loads a backup record from its manifest file on disk.
func (s *Service) GetBackup(backupID string) (*BackupManifest, error) {
	return s.loadManifest(backupID)
}

// ListBackups returns manifests from the storage dir, optionally filtered by type.
func (s *Service) ListBackups(backupType string, limit int) ([]BackupManifest, error) {
	all, err := s.listManifestFiles()
	if err != nil {
		return nil, err
	}

	// Filter by type.
	var filtered []BackupManifest
	for _, m := range all {
		if backupType != "" && m.Type != backupType {
			continue
		}
		filtered = append(filtered, m)
	}

	// Sort newest first.
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// DeleteBackup removes a backup artifact and its manifest from disk.
func (s *Service) DeleteBackup(backupID string) error {
	// SEC-TRANCHE-2 (2026-08-11): backupID reaches here from
	// DELETE /v1/backup/{id} via handleBackupByID with no upstream
	// validation. A malicious id like "../../etc/hosts" would let
	// os.Remove unlink arbitrary files matching the .manifest.json /
	// .dump / .mdemg suffixes. Route every filepath.Join through
	// pathsafe.SafeJoinUnderDir so CodeQL sees a recognized sanitizer
	// AND the class is closed at the trust boundary.
	manifestPath, err := s.safeBackupPath(backupID + ".manifest.json")
	if err != nil {
		return fmt.Errorf("invalid backup id: %w", err)
	}
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("backup %q not found", backupID)
	}

	// Load manifest to get the data file path.
	m, err := s.loadManifest(backupID)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Check keep_forever.
	if m.KeepForever {
		return fmt.Errorf("backup %q is marked keep_forever; unmark first", backupID)
	}

	// Delete data file(s).
	dataExts := []string{".dump", ".mdemg"}
	for _, ext := range dataExts {
		p, sErr := s.safeBackupPath(backupID + ext)
		if sErr != nil {
			// safeBackupPath rejected on this ext — impossible given
			// backupID was already accepted above, but stay defensive.
			continue
		}
		_ = os.Remove(p)
	}

	// Delete manifest.
	return os.Remove(manifestPath)
}

// safeBackupPath joins a backup-artifact filename under s.cfg.StorageDir
// with SEC-TRANCHE-2 pathsafe.SafeJoinUnderDir containment. Every
// filepath.Join(s.cfg.StorageDir, ...) sink in this file that consumes
// an untrusted-derived name (backupID from HTTP path) MUST route through
// this helper. Server-constructed IDs (e.g. `bk-<ts>-<type>`) route
// through it too for uniformity (guards against a future refactor that
// widens the input surface).
func (s *Service) safeBackupPath(name string) (string, error) {
	return pathsafe.SafeJoinUnderDir(s.cfg.StorageDir, name)
}

// writeManifest writes a manifest sidecar JSON file for a backup.
func (s *Service) writeManifest(record *BackupRecord, manifest BackupManifest) error {
	// SEC-TRANCHE-2: record.BackupID is server-generated in Trigger()
	// as `bk-<timestamp>-<type>` so it's safe by construction, but
	// route through safeBackupPath uniformly (belt-and-suspenders +
	// closes CodeQL flow).
	path, err := s.safeBackupPath(record.BackupID + ".manifest.json")
	if err != nil {
		return fmt.Errorf("invalid backup id: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// loadManifest reads a manifest from disk.
func (s *Service) loadManifest(backupID string) (*BackupManifest, error) {
	// SEC-TRANCHE-2: backupID reaches here from HTTP paths
	// (handleBackupStatus, handleBackupManifest, handleBackupRestore)
	// with no upstream validation. Sanitize at the sink.
	path, err := s.safeBackupPath(backupID + ".manifest.json")
	if err != nil {
		return nil, fmt.Errorf("invalid backup id: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m BackupManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// listManifestFiles scans the storage directory for *.manifest.json files.
func (s *Service) listManifestFiles() ([]BackupManifest, error) {
	pattern := filepath.Join(s.cfg.StorageDir, "*.manifest.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob manifests: %w", err)
	}

	var manifests []BackupManifest
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m BackupManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// sha256File computes the SHA256 checksum of a file on disk.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// fileSize returns the size of a file in bytes.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// queryNodeEdgeCounts returns the total node and edge counts from Neo4j.
func (s *Service) queryNodeEdgeCounts(ctx context.Context) (int64, int64, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	var nodeCount, edgeCount int64

	nodeRecords, qErr := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			"MATCH (n:MemoryNode) RETURN count(n) AS nodes",
			nil,
		)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if qErr != nil {
		return 0, 0, qErr
	}
	for _, rec := range nodeRecords.([]*neo4j.Record) {
		if v, ok := rec.Get("nodes"); ok {
			if n, ok := v.(int64); ok {
				nodeCount = n
			}
		}
	}

	edgeRecords, qErr2 := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			"MATCH ()-[r]->() RETURN count(r) AS edges",
			nil,
		)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if qErr2 != nil {
		return 0, 0, qErr2
	}
	for _, rec := range edgeRecords.([]*neo4j.Record) {
		if v, ok := rec.Get("edges"); ok {
			if n, ok := v.(int64); ok {
				edgeCount = n
			}
		}
	}

	return nodeCount, edgeCount, nil
}

// queryAllSpaceIDs returns all distinct space_id values from Neo4j.
func (s *Service) queryAllSpaceIDs(ctx context.Context) ([]string, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			"MATCH (n:MemoryNode) WHERE n.space_id IS NOT NULL RETURN DISTINCT n.space_id AS sid ORDER BY sid",
			nil,
		)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	var spaces []string
	for _, rec := range records.([]*neo4j.Record) {
		if v, ok := rec.Get("sid"); ok {
			if s, ok := v.(string); ok {
				spaces = append(spaces, s)
			}
		}
	}
	return spaces, nil
}

// querySchemaVersion returns the current schema version from Neo4j.
func (s *Service) querySchemaVersion(ctx context.Context) (int, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			"MATCH (m:SchemaMeta) RETURN m.version AS v ORDER BY m.version DESC LIMIT 1",
			nil,
		)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return 0, err
	}
	for _, rec := range records.([]*neo4j.Record) {
		if v, ok := rec.Get("v"); ok {
			switch val := v.(type) {
			case int64:
				return int(val), nil
			case int:
				return val, nil
			}
		}
	}
	return 0, nil
}

// ensureSpaceIncluded ensures a space ID is present in a slice (used to always include mdemg-dev).
func ensureSpaceIncluded(spaces []string, required string) []string {
	for _, s := range spaces {
		if strings.EqualFold(s, required) {
			return spaces
		}
	}
	return append(spaces, required)
}
