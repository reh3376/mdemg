package tsdb

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// TSDBBackupConfig holds configuration for TimescaleDB backups.
type TSDBBackupConfig struct {
	Enabled             bool
	StorageDir          string // default: ".mdemg/backups/tsdb"
	ComposeFile         string // default: auto-detect
	ServiceName         string // default: "timescaledb" (compose SERVICE, not container)
	Database            string // default: "mdemg_metrics"
	User                string // default: "mdemg"
	IntervalHours       int    // default: 24
	RetentionCount      int    // default: 14
	RetentionMaxAgeDays int    // default: 90
}

// TSDBBackupRecord represents a completed or in-progress TSDB backup.
type TSDBBackupRecord struct {
	BackupID      string `json:"backup_id"`
	Status        string `json:"status"` // "pending", "running", "completed", "failed"
	CreatedAt     string `json:"created_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	Checksum      string `json:"checksum,omitempty"`
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	RowCount      int64  `json:"row_count,omitempty"`
	Label         string `json:"label,omitempty"`
}

// TSDBBackupManifest is the JSON sidecar written alongside each backup file.
type TSDBBackupManifest struct {
	FormatVersion int    `json:"format_version"`
	BackupID      string `json:"backup_id"`
	CreatedAt     string `json:"created_at"`
	SizeBytes     int64  `json:"size_bytes"`
	Checksum      string `json:"checksum"`
	SchemaVersion int    `json:"schema_version"`
	RowCount      int64  `json:"row_count"`
	Database      string `json:"database"`
	Label         string `json:"label,omitempty"`
}

// TSDBBackupService manages TimescaleDB backup lifecycle.
type TSDBBackupService struct {
	cfg TSDBBackupConfig
	mu  sync.Mutex
}

// NewTSDBBackupService creates a new TSDB backup service.
func NewTSDBBackupService(cfg TSDBBackupConfig) *TSDBBackupService {
	return &TSDBBackupService{cfg: cfg}
}

// Trigger runs a pg_dump backup via docker compose exec.
func (s *TSDBBackupService) Trigger(label string) (*TSDBBackupRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	backupID := fmt.Sprintf("tsdb-bk-%s", now.Format("20060102-150405"))

	if err := os.MkdirAll(s.cfg.StorageDir, 0o750); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	dumpPath := filepath.Join(s.cfg.StorageDir, backupID+".dump")

	composeFile := s.resolveComposeFile()
	if composeFile == "" {
		return nil, fmt.Errorf("no docker compose file found")
	}

	slog.Info("tsdb backup: starting pg_dump",
		"backup_id", backupID,
		"compose_file", composeFile,
		"service", s.cfg.ServiceName,
	)

	rec := &TSDBBackupRecord{
		BackupID:  backupID,
		Status:    "running",
		CreatedAt: now.Format(time.RFC3339),
		Path:      dumpPath,
		Label:     label,
	}

	// Run pg_dump via docker compose exec (uses service name, not container name)
	cmd := exec.Command("docker", "compose", //nolint:gosec // G204: compose file path from config
		"-f", composeFile,
		"exec", "-T", s.cfg.ServiceName,
		"pg_dump", "-Fc", "-U", s.cfg.User, s.cfg.Database,
	)

	outFile, err := os.Create(dumpPath) //nolint:gosec // G304: path from config
	if err != nil {
		return nil, fmt.Errorf("create dump file: %w", err)
	}

	cmd.Stdout = outFile
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		outFile.Close()
		os.Remove(dumpPath)
		rec.Status = "failed"
		return rec, fmt.Errorf("pg_dump failed: %w (stderr: %s)", err, stderr.String())
	}
	outFile.Close()

	// Compute checksum and size
	checksum, err := sha256File(dumpPath)
	if err != nil {
		slog.Warn("tsdb backup: checksum failed", "error", err)
	}
	size := fileSize(dumpPath)

	rec.Status = "completed"
	rec.CompletedAt = time.Now().Format(time.RFC3339)
	rec.SizeBytes = size
	rec.Checksum = checksum

	// Write manifest sidecar
	manifest := TSDBBackupManifest{
		FormatVersion: 1,
		BackupID:      backupID,
		CreatedAt:     rec.CreatedAt,
		SizeBytes:     size,
		Checksum:      checksum,
		Database:      s.cfg.Database,
		Label:         label,
	}
	if err := s.writeManifest(backupID, &manifest); err != nil {
		slog.Warn("tsdb backup: write manifest failed", "error", err)
	}

	slog.Info("tsdb backup: completed",
		"backup_id", backupID,
		"size_bytes", size,
		"path", dumpPath,
	)

	return rec, nil
}

// Restore restores a TSDB backup from a dump file via docker compose exec.
func (s *TSDBBackupService) Restore(dumpPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(dumpPath); os.IsNotExist(err) {
		return fmt.Errorf("dump file not found: %s", dumpPath)
	}

	composeFile := s.resolveComposeFile()
	if composeFile == "" {
		return fmt.Errorf("no docker compose file found")
	}

	slog.Info("tsdb restore: starting pg_restore",
		"path", dumpPath,
		"compose_file", composeFile,
	)

	inFile, err := os.Open(dumpPath) //nolint:gosec // G304: user-provided path for restore
	if err != nil {
		return fmt.Errorf("open dump file: %w", err)
	}
	defer inFile.Close()

	cmd := exec.Command("docker", "compose", //nolint:gosec // G204: compose file path from config
		"-f", composeFile,
		"exec", "-T", s.cfg.ServiceName,
		"pg_restore", "-U", s.cfg.User, "-d", s.cfg.Database, "--clean", "--if-exists",
	)

	cmd.Stdin = inFile
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w (stderr: %s)", err, stderr.String())
	}

	slog.Info("tsdb restore: completed", "path", dumpPath)
	return nil
}

// ListBackups returns all TSDB backup manifests, sorted by creation time (newest first).
func (s *TSDBBackupService) ListBackups(limit int) ([]TSDBBackupManifest, error) {
	pattern := filepath.Join(s.cfg.StorageDir, "tsdb-bk-*.manifest.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob manifests: %w", err)
	}

	var manifests []TSDBBackupManifest
	for _, f := range files {
		m, mErr := s.loadManifest(f)
		if mErr != nil {
			slog.Warn("tsdb backup: skip corrupt manifest", "path", f, "error", mErr)
			continue
		}
		manifests = append(manifests, *m)
	}

	// Sort newest first
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt > manifests[j].CreatedAt
	})

	if limit > 0 && len(manifests) > limit {
		manifests = manifests[:limit]
	}

	return manifests, nil
}

// GetBackup loads a single backup manifest by ID.
func (s *TSDBBackupService) GetBackup(backupID string) (*TSDBBackupManifest, error) {
	path := filepath.Join(s.cfg.StorageDir, backupID+".manifest.json")
	return s.loadManifest(path)
}

// RunRetention applies count-based and age-based retention to TSDB backups.
// Retention logic mirrors internal/backup/retention.go.
// Both implement count-based + age-based pruning with identical semantics.
// If either is modified, consider updating the other to maintain parity.
func (s *TSDBBackupService) RunRetention() (deleted int, freedBytes int64, err error) {
	manifests, err := s.ListBackups(0)
	if err != nil {
		return 0, 0, err
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -s.cfg.RetentionMaxAgeDays)
	toDelete := make(map[string]bool)

	// Count-based: mark backups beyond retention count
	if s.cfg.RetentionCount > 0 && len(manifests) > s.cfg.RetentionCount {
		for _, m := range manifests[s.cfg.RetentionCount:] {
			toDelete[m.BackupID] = true
		}
	}

	// Age-based: mark backups older than max age
	for _, m := range manifests {
		created, pErr := time.Parse(time.RFC3339, m.CreatedAt)
		if pErr != nil {
			continue
		}
		if created.Before(cutoff) {
			toDelete[m.BackupID] = true
		}
	}

	// Execute deletions
	for id := range toDelete {
		freed, dErr := s.deleteBackup(id)
		if dErr != nil {
			slog.Warn("tsdb retention: delete failed", "backup_id", id, "error", dErr)
			continue
		}
		deleted++
		freedBytes += freed
	}

	if deleted > 0 {
		slog.Info("tsdb retention: completed", "deleted", deleted, "freed_bytes", freedBytes)
	}

	return deleted, freedBytes, nil
}

// ── Scheduler ──

// TSDBBackupScheduler runs periodic TSDB backups.
type TSDBBackupScheduler struct {
	svc    *TSDBBackupService
	stopCh chan struct{}
}

// NewTSDBBackupScheduler creates a new scheduler.
func NewTSDBBackupScheduler(svc *TSDBBackupService) *TSDBBackupScheduler {
	return &TSDBBackupScheduler{svc: svc}
}

// Start begins periodic TSDB backups at the configured interval.
func (bs *TSDBBackupScheduler) Start() {
	bs.stopCh = make(chan struct{})

	interval := time.Duration(bs.svc.cfg.IntervalHours) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				slog.Info("tsdb backup scheduler: triggering backup")
				rec, err := bs.svc.Trigger("scheduled")
				if err != nil {
					slog.Warn("tsdb backup scheduler: backup failed", "error", err)
				} else {
					slog.Info("tsdb backup scheduler: backup completed",
						"backup_id", rec.BackupID,
						"size_bytes", rec.SizeBytes,
					)
				}

				// Run retention after backup
				deleted, freed, rErr := bs.svc.RunRetention()
				if rErr != nil {
					slog.Warn("tsdb backup scheduler: retention failed", "error", rErr)
				} else if deleted > 0 {
					slog.Info("tsdb backup scheduler: retention pruned", "deleted", deleted, "freed_bytes", freed)
				}

			case <-bs.stopCh:
				slog.Info("tsdb backup scheduler: stopped")
				return
			}
		}
	}()
}

// Stop halts the backup scheduler.
func (bs *TSDBBackupScheduler) Stop() {
	if bs.stopCh != nil {
		close(bs.stopCh)
	}
}

// ── Helpers ──

// resolveComposeFile finds the docker compose file for TSDB operations.
func (s *TSDBBackupService) resolveComposeFile() string {
	if s.cfg.ComposeFile != "" {
		if _, err := os.Stat(s.cfg.ComposeFile); err == nil {
			return s.cfg.ComposeFile
		}
	}

	// Auto-detect: check common paths
	candidates := []string{
		"deploy/docker/docker-compose.observability.yml",
		"deploy/docker/docker-compose.prod.yml",
		"docker-compose.yml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (s *TSDBBackupService) writeManifest(backupID string, m *TSDBBackupManifest) error {
	path := filepath.Join(s.cfg.StorageDir, backupID+".manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644) //nolint:gosec // G306: manifest is non-sensitive metadata
}

func (s *TSDBBackupService) loadManifest(path string) (*TSDBBackupManifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted source (our storage dir)
	if err != nil {
		return nil, err
	}
	var m TSDBBackupManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *TSDBBackupService) deleteBackup(backupID string) (int64, error) {
	var freed int64

	dumpPath := filepath.Join(s.cfg.StorageDir, backupID+".dump")
	if info, err := os.Stat(dumpPath); err == nil {
		freed = info.Size()
		if err := os.Remove(dumpPath); err != nil {
			return 0, fmt.Errorf("remove dump: %w", err)
		}
	}

	manifestPath := filepath.Join(s.cfg.StorageDir, backupID+".manifest.json")
	if info, err := os.Stat(manifestPath); err == nil {
		freed += info.Size()
		if err := os.Remove(manifestPath); err != nil {
			return freed, fmt.Errorf("remove manifest: %w", err)
		}
	}

	return freed, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path from trusted source
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
