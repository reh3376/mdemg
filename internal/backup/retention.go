package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunRetention applies count-based, age-based, and storage-based cleanup.
func (s *Service) RunRetention(_ context.Context) (*RetentionResult, error) {
	all, err := s.listManifestFiles()
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}

	result := &RetentionResult{}

	// Separate into full + partial lists, sort by created_at desc (newest first).
	var fullBackups, partialBackups []BackupManifest
	for _, m := range all {
		switch BackupType(m.Type) {
		case BackupTypeFull:
			fullBackups = append(fullBackups, m)
		case BackupTypePartial:
			partialBackups = append(partialBackups, m)
		}
	}

	sortByCreatedDesc := func(s []BackupManifest) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].CreatedAt > s[j].CreatedAt
		})
	}
	sortByCreatedDesc(fullBackups)
	sortByCreatedDesc(partialBackups)

	// Track which backup IDs to delete.
	toDelete := make(map[string]bool)

	// Count-based: mark anything beyond the Nth newest per type.
	markBeyondN := func(list []BackupManifest, keep int) {
		for i, m := range list {
			if i >= keep && !m.KeepForever {
				toDelete[m.BackupID] = true
			}
		}
	}
	if s.cfg.RetentionFullCount > 0 {
		markBeyondN(fullBackups, s.cfg.RetentionFullCount)
	}
	if s.cfg.RetentionPartialCount > 0 {
		markBeyondN(partialBackups, s.cfg.RetentionPartialCount)
	}

	// Age-based: mark anything older than maxAgeDays.
	if s.cfg.RetentionMaxAgeDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -s.cfg.RetentionMaxAgeDays)
		for _, m := range all {
			if m.KeepForever {
				continue
			}
			t, err := time.Parse(time.RFC3339, m.CreatedAt)
			if err != nil {
				continue
			}
			if t.Before(cutoff) {
				toDelete[m.BackupID] = true
			}
		}
	}

	// Execute deletions.
	for id := range toDelete {
		freed, delErr := s.deleteBackupFiles(id)
		if delErr != nil {
			slog.Error("backup: retention: failed to delete", "backup_id", id, "error", delErr)
			continue
		}
		result.Deleted = append(result.Deleted, id)
		result.FreedBytes += freed
	}

	// Storage-based: if total size > quota, delete oldest non-exempt until under.
	if s.cfg.RetentionMaxStorageGB > 0 {
		maxBytes := int64(s.cfg.RetentionMaxStorageGB) * 1024 * 1024 * 1024
		totalSize := s.totalStorageSize()
		if totalSize > maxBytes {
			// Reload manifests after prior deletions.
			remaining, _ := s.listManifestFiles()
			sort.Slice(remaining, func(i, j int) bool {
				return remaining[i].CreatedAt < remaining[j].CreatedAt // oldest first
			})
			for _, m := range remaining {
				if totalSize <= maxBytes {
					break
				}
				if m.KeepForever {
					continue
				}
				freed, delErr := s.deleteBackupFiles(m.BackupID)
				if delErr != nil {
					continue
				}
				result.Deleted = append(result.Deleted, m.BackupID)
				result.FreedBytes += freed
				totalSize -= freed
			}
		}
	}

	// Count remaining backups.
	remaining, _ := s.listManifestFiles()
	result.KeptCount = len(remaining)

	return result, nil
}

// deleteBackupFiles removes a backup's data file and manifest, returning bytes freed.
func (s *Service) deleteBackupFiles(backupID string) (int64, error) {
	var freed int64

	// Try common data extensions.
	for _, ext := range []string{".dump", ".mdemg"} {
		p := filepath.Join(s.cfg.StorageDir, backupID+ext)
		if info, err := os.Stat(p); err == nil {
			freed += info.Size()
			if err := os.Remove(p); err != nil {
				return freed, err
			}
		}
	}

	// Remove manifest.
	manifestPath := filepath.Join(s.cfg.StorageDir, backupID+".manifest.json")
	if info, err := os.Stat(manifestPath); err == nil {
		freed += info.Size()
		if err := os.Remove(manifestPath); err != nil {
			return freed, err
		}
	}

	return freed, nil
}

// totalStorageSize sums all file sizes in the storage directory.
func (s *Service) totalStorageSize() int64 {
	var total int64
	entries, err := os.ReadDir(s.cfg.StorageDir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}
