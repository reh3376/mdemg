// Package sidecar implements core domain types, configuration, state machine,
// and report structures for the MDEMG sidecar lifecycle.
package sidecar

import "time"

// --- Profiles ---

// Profile represents a sidecar runtime placement policy.
type Profile string

const (
	ProfileLocal        Profile = "local"
	ProfileStudioRemote Profile = "studio-remote"
)

// ValidProfiles returns all supported profile values.
func ValidProfiles() []Profile {
	return []Profile{ProfileLocal, ProfileStudioRemote}
}

// IsValid returns true if the profile is a known value.
func (p Profile) IsValid() bool {
	switch p {
	case ProfileLocal, ProfileStudioRemote:
		return true
	}
	return false
}

// --- States ---

// State represents a canonical sidecar lifecycle state (Section 7B).
type State string

const (
	StateUninitialized State = "uninitialized"
	StateInitialized   State = "initialized"
	StateInstalled     State = "installed"
	StateRunning       State = "running"
	StateDegraded      State = "degraded"
	StateStopped       State = "stopped"
	StateUninstalled   State = "uninstalled"
)

// AllStates returns all 7 canonical sidecar states.
func AllStates() []State {
	return []State{
		StateUninitialized,
		StateInitialized,
		StateInstalled,
		StateRunning,
		StateDegraded,
		StateStopped,
		StateUninstalled,
	}
}

// --- Transport ---

// Transport represents a remote communication mechanism.
type Transport string

const (
	TransportDockerContext Transport = "docker-context"
	TransportSSHExec      Transport = "ssh-exec"
)

// --- Adapter Names ---

// AdapterName represents a supported agent adapter.
type AdapterName string

const (
	AdapterClaudeCode AdapterName = "claude-code"
	AdapterCodex      AdapterName = "codex"
)

// ValidAdapterNames returns all supported adapter names.
func ValidAdapterNames() []AdapterName {
	return []AdapterName{AdapterClaudeCode, AdapterCodex}
}

// --- Config Structs (matching sidecar-config.schema.json) ---

// ConfigVersion is the current schema version for sidecar.yaml.
const ConfigVersion = "1"

// Config represents the contents of .mdemg/sidecar.yaml.
type Config struct {
	Version  string          `yaml:"version"  json:"version"`
	Profile  Profile         `yaml:"profile"  json:"profile"`
	Runtime  RuntimeConfig   `yaml:"runtime"  json:"runtime"`
	Adapters []AdapterConfig `yaml:"adapters" json:"adapters"`
	Hooks    HooksConfig     `yaml:"hooks"    json:"hooks"`
	Install  InstallConfig   `yaml:"install"  json:"install"`
}

// RuntimeConfig holds endpoint and remote settings.
type RuntimeConfig struct {
	Endpoint string       `yaml:"endpoint" json:"endpoint"`
	Remote   RemoteConfig `yaml:"remote"   json:"remote"`
}

// RemoteConfig holds remote runtime host and transport.
type RemoteConfig struct {
	Host      string    `yaml:"host"      json:"host"`
	Transport Transport `yaml:"transport" json:"transport"`
}

// AdapterConfig holds per-adapter settings.
type AdapterConfig struct {
	Name    AdapterName `yaml:"name"    json:"name"`
	Enabled bool        `yaml:"enabled" json:"enabled"`
}

// HooksConfig holds hook-related settings.
type HooksConfig struct {
	SpaceIDStrategy string `yaml:"space_id_strategy" json:"space_id_strategy"`
}

// InstallConfig holds install behavior settings.
type InstallConfig struct {
	AutoFix bool `yaml:"auto_fix" json:"auto_fix"`
}

// --- Lock File ---

// LockFile represents .mdemg/sidecar.lock (JSON format).
type LockFile struct {
	State        State  `json:"state"`
	Profile      string `json:"profile"`
	Endpoint     string `json:"endpoint"`
	ConfigHash   string `json:"config_hash"`
	MdemgVersion string `json:"mdemg_version"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`

	// Remote profile fields (populated only for studio-remote).
	RemoteHost    string `json:"remote_host,omitempty"`
	RemotePID     int    `json:"remote_pid,omitempty"`
	TransportUsed string `json:"transport_used,omitempty"`
	DockerContext string `json:"docker_context,omitempty"`

	// Runtime ports (populated by sidecar up, read by all commands).
	Neo4jBoltPort int    `json:"neo4j_bolt_port,omitempty"`
	Neo4jHTTPPort int    `json:"neo4j_http_port,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	VolumeName    string `json:"volume_name,omitempty"`
}

// --- Exit Codes (Section 7A) ---

const (
	ExitSuccess         = 0
	ExitValidation      = 2
	ExitDependency      = 3
	ExitRuntime         = 4
	ExitPermissions     = 5
	ExitAdapterConflict = 6
)

// --- Report Types (Section 7D) ---

// ReportSchemaVersion is the current version for all report envelopes.
const ReportSchemaVersion = "1.0.0"

// ReportEnvelope contains the common fields required by all sidecar reports.
type ReportEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	Command       string         `json:"command"`
	Timestamp     string         `json:"timestamp"`
	Result        string         `json:"result"`
	ExitCode      int            `json:"exit_code"`
	StateBefore   State          `json:"state_before"`
	StateAfter    State          `json:"state_after"`
	Changes       []ReportChange `json:"changes"`
	Issues        []ReportIssue  `json:"issues"`
	NextActions   []string       `json:"next_actions"`
}

// ReportChange describes a single mutation in a report.
type ReportChange struct {
	Path       string `json:"path"`
	Action     string `json:"action"`
	BackupPath string `json:"backup_path,omitempty"`
}

// ReportIssue describes a detected issue with remediation.
type ReportIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

// --- Status Report ---

// StatusReport extends ReportEnvelope with status-specific fields.
type StatusReport struct {
	ReportEnvelope
	Profile       string         `json:"profile"`
	Endpoint      string         `json:"endpoint"`
	ControlHost   string         `json:"control_host"`
	RuntimeHost   string         `json:"runtime_host"`
	Transport     string         `json:"transport"`
	Services      []ServiceStatus `json:"services"`
	HealthSummary HealthSummary  `json:"health_summary"`
}

// ServiceStatus describes one probed service.
type ServiceStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Healthy   bool   `json:"healthy"`
	Endpoint  string `json:"endpoint,omitempty"`
	Port      int    `json:"port,omitempty"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// HealthSummary aggregates service health.
type HealthSummary struct {
	Healthy           bool     `json:"healthy"`
	ServiceCountTotal int      `json:"service_count_total"`
	ServiceCountUp    int      `json:"service_count_up"`
	DegradedReasons   []string `json:"degraded_reasons"`
}

// --- Doctor Report ---

// DoctorCheck represents one diagnostic check result.
type DoctorCheck struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Status      string   `json:"status"` // pass, warn, fail, skip
	Message     string   `json:"message"`
	DurationMs  int      `json:"duration_ms"`
	Remediation string   `json:"remediation,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// DoctorSummary tallies check outcomes.
type DoctorSummary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
}

// DoctorReport extends ReportEnvelope with doctor-specific fields.
type DoctorReport struct {
	ReportEnvelope
	Profile string        `json:"profile"`
	Checks  []DoctorCheck `json:"checks"`
	Summary DoctorSummary `json:"summary"`
}

// NowUTC returns the current time formatted as RFC3339 in UTC.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
