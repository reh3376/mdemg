package sidecar

import (
	"fmt"
	"strings"
)

// --- Adapter interface and types ---

// Adapter defines the interface for agent config integration.
// Implementations are pure logic — no filesystem I/O.
type Adapter interface {
	// Name returns the canonical adapter name.
	Name() AdapterName
	// Version returns the versioned adapter identifier (e.g. "claude-code-v1").
	Version() string
	// ConfigPath returns the absolute path to the agent's config file.
	ConfigPath(projectDir string) string
	// ConfigFormat returns the config file format ("json" or "toml").
	ConfigFormat() string
	// BuildPayload returns a fresh config payload for the given MDEMG endpoint.
	BuildPayload(endpoint string) ([]byte, error)
	// MergeInto merges MDEMG config into existing agent config bytes.
	// Returns the merged bytes, a merge strategy ("create" or "merge"), and any error.
	MergeInto(existing []byte, endpoint string) ([]byte, string, error)
	// RemoveFrom removes MDEMG config from existing agent config bytes.
	RemoveFrom(existing []byte) ([]byte, error)
}

// --- AdapterResult ---

// AdapterResult holds the outcome of an attach or detach operation.
type AdapterResult struct {
	ConfigPath    string `json:"config_path"`
	ConfigFormat  string `json:"config_format"`
	MergeStrategy string `json:"merge_strategy"`
	Payload       []byte `json:"payload"`
	BackupPath    string `json:"backup_path"`
	OriginalHash  string `json:"original_hash"`
}

// --- AttachAgentReport ---

// BackupManifest records backup metadata for an attach/detach operation.
type BackupManifest struct {
	BackupPath   string `json:"backup_path"`
	OriginalHash string `json:"original_hash"`
	Timestamp    string `json:"timestamp"`
}

// AttachAgentReport extends ReportEnvelope with adapter-specific fields.
type AttachAgentReport struct {
	ReportEnvelope
	AdapterName    string         `json:"adapter_name"`
	AdapterVersion string         `json:"adapter_version"`
	ConfigPath     string         `json:"config_path"`
	ConfigFormat   string         `json:"config_format"`
	DryRun         bool           `json:"dry_run"`
	PrintOnly      bool           `json:"print_only"`
	MergeStrategy  string         `json:"merge_strategy"`
	BackupManifest BackupManifest `json:"backup_manifest"`
}

// NewAttachAgentReport creates an AttachAgentReport with common fields populated.
// Slices are initialized to empty (not nil) to produce [] in JSON.
func NewAttachAgentReport(command string, stateBefore, stateAfter State, exitCode int) AttachAgentReport {
	return AttachAgentReport{
		ReportEnvelope: NewReportEnvelope(command, stateBefore, stateAfter, exitCode),
	}
}

// --- Adapter factory ---

// NewAdapter returns the adapter for the given name, or an error if unknown.
func NewAdapter(name AdapterName) (Adapter, error) {
	switch name {
	case AdapterClaudeCode:
		return &ClaudeCodeAdapter{}, nil
	case AdapterCodex:
		return &CodexAdapter{}, nil
	default:
		valid := ValidAdapterNames()
		names := make([]string, len(valid))
		for i, n := range valid {
			names[i] = string(n)
		}
		return nil, fmt.Errorf("unknown adapter %q, valid adapters: %s", name, joinStrings(names))
	}
}

// joinStrings joins strings with ", ".
func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(ss[0])
	for _, s := range ss[1:] {
		b.WriteString(", ")
		b.WriteString(s)
	}
	return b.String()
}
