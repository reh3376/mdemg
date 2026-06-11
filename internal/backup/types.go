// Package backup provides automated and on-demand backup/restore for Neo4j.
package backup

import "time"

// Config holds all backup-related settings parsed from environment variables.
type Config struct {
	Enabled              bool
	StorageDir           string // directory for backup artifacts
	FullCmd              string // command for full backups (default: "docker")
	Neo4jContainer       string // Docker container name for neo4j-admin
	FullIntervalHours    int    // hours between automatic full backups
	PartialIntervalHours int    // hours between automatic partial backups
	RetentionFullCount   int    // keep last N full backups
	RetentionPartialCount int   // keep last N partial backups
	RetentionMaxAgeDays  int    // delete backups older than N days
	RetentionMaxStorageGB int   // delete oldest until under quota
	RetentionRunAfter    bool   // run retention after each backup
	SnapshotWaitTimeoutSec int  // BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC — max wait for the pre-restore safety snapshot to complete (default: 300)
}

// BackupType identifies whether a backup is full or partial.
type BackupType string

const (
	BackupTypeFull    BackupType = "full"
	BackupTypePartial BackupType = "partial_space"
)

// BackupRecord tracks the state of a single backup.
type BackupRecord struct {
	BackupID    string     `json:"backup_id"`
	Type        BackupType `json:"type"`
	Status      string     `json:"status"` // pending, running, completed, failed
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum"` // sha256:<hex>
	Path        string     `json:"path"`
	Spaces      []string   `json:"spaces,omitempty"`
	KeepForever bool       `json:"keep_forever"`
	Label       string     `json:"label,omitempty"`
}

// BackupManifest is the JSON sidecar written alongside each backup artifact.
type BackupManifest struct {
	BackupID      string   `json:"backup_id"`
	Type          string   `json:"type"`
	FormatVersion string   `json:"format_version"` // "1.0"
	CreatedAt     string   `json:"created_at"`
	Checksum      string   `json:"checksum"`
	SizeBytes     int64    `json:"size_bytes"`
	Spaces        []string `json:"spaces,omitempty"`
	NodeCount     int64    `json:"node_count"` // whole-database count at backup time (informational)
	EdgeCount     int64    `json:"edge_count"` // whole-database count at backup time (informational)
	SchemaVersion int      `json:"schema_version"`
	KeepForever   bool     `json:"keep_forever"`
	Label         string   `json:"label,omitempty"`

	// BACKUP-RESTORE-VERIFY-001: counts of what the .mdemg file actually
	// contains, counted from the exported chunks. These are the
	// restore-validation reference (NodeCount/EdgeCount above are
	// whole-database and diverge from file contents on partial backups).
	// Zero-valued on legacy manifests → validation downgrades to warn-only.
	FileNodeCount        int64 `json:"file_node_count,omitempty"`
	FileEdgeCount        int64 `json:"file_edge_count,omitempty"`
	FileObservationCount int64 `json:"file_observation_count,omitempty"`
}

// TriggerRequest is the API payload for POST /v1/backup/trigger.
type TriggerRequest struct {
	Type        string   `json:"type"`                   // "full" | "partial_space"
	SpaceIDs    []string `json:"space_ids,omitempty"`    // for partial (empty = all)
	KeepForever bool     `json:"keep_forever,omitempty"`
	Label       string   `json:"label,omitempty"`
}

// RestoreRequest is the API payload for POST /v1/backup/restore.
type RestoreRequest struct {
	BackupID       string `json:"backup_id"`
	SnapshotBefore bool   `json:"snapshot_before"` // take a safety snapshot first
}

// RetentionResult summarises what a retention run cleaned up.
type RetentionResult struct {
	Deleted    []string `json:"deleted"`
	FreedBytes int64    `json:"freed_bytes"`
	KeptCount  int      `json:"kept_count"`
}
