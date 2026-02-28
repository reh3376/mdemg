package sidecar

import (
	"fmt"
	"strconv"
	"strings"
)

// --- Input types (CLI layer gathers these, passes to pure eval functions) ---

// DockerInfo holds Docker CLI detection results.
type DockerInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Running   bool   `json:"running"`
}

// Neo4jImageInfo holds Neo4j Docker image detection results.
type Neo4jImageInfo struct {
	ImageFound bool   `json:"image_found"`
	ImageTag   string `json:"image_tag"`
	Version    string `json:"version"`
}

// PortInfo holds TCP port availability results.
type PortInfo struct {
	Port int  `json:"port"`
	Free bool `json:"free"`
}

// SSHInfo holds SSH reachability results for studio-remote profile.
type SSHInfo struct {
	Host      string `json:"host"`
	Reachable bool   `json:"reachable"`
}

// --- Output types (matching install-report.schema.json) ---

// PreflightCheck represents a single preflight verification result.
type PreflightCheck struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // pass, warn, fail, skip
	Required    bool   `json:"required"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// DependencyResult represents a single dependency resolution outcome.
type DependencyResult struct {
	Name            string `json:"name"`
	Status          string `json:"status"` // present, installed, upgraded, skipped, failed
	RequiredVersion string `json:"required_version,omitempty"`
	DetectedVersion string `json:"detected_version,omitempty"`
	Action          string `json:"action,omitempty"`
}

// InstallReport extends ReportEnvelope with install-specific fields.
type InstallReport struct {
	ReportEnvelope
	Profile             string             `json:"profile"`
	DryRun              bool               `json:"dry_run"`
	AutoFixEnabled      bool               `json:"auto_fix_enabled"`
	PreflightChecks     []PreflightCheck   `json:"preflight_checks"`
	Dependencies        []DependencyResult `json:"dependencies"`
	InstalledComponents []string           `json:"installed_components"`
	LockfilePath        string             `json:"lockfile_path"`
}

// DockerContextInfo holds Docker context detection results for studio-remote profile.
type DockerContextInfo struct {
	ContextName string `json:"context_name"`
	Exists      bool   `json:"exists"`
	Functional  bool   `json:"functional"`
	Host        string `json:"host"`
}

// RemoteBinaryInfo holds remote mdemg binary detection results.
type RemoteBinaryInfo struct {
	Available bool   `json:"available"`
	Path      string `json:"path"`
	Version   string `json:"version"`
}

// --- Pure evaluation functions (zero I/O, fully testable) ---

// EvalDockerAvailable evaluates whether Docker CLI is available and running.
func EvalDockerAvailable(info DockerInfo) PreflightCheck {
	if !info.Available {
		return PreflightCheck{
			ID:          "docker-available",
			Status:      "fail",
			Required:    true,
			Message:     "Docker CLI not found in PATH",
			Remediation: "Install Docker: https://docs.docker.com/get-docker/",
		}
	}
	if !info.Running {
		return PreflightCheck{
			ID:          "docker-available",
			Status:      "fail",
			Required:    true,
			Message:     "Docker daemon is not running",
			Remediation: "Start Docker Desktop or run: sudo systemctl start docker",
		}
	}
	return PreflightCheck{
		ID:       "docker-available",
		Status:   "pass",
		Required: true,
		Message:  fmt.Sprintf("Docker %s available and running", info.Version),
	}
}

// EvalNeo4jImage evaluates whether the Neo4j Docker image is present.
func EvalNeo4jImage(info Neo4jImageInfo) PreflightCheck {
	if !info.ImageFound {
		return PreflightCheck{
			ID:          "neo4j-image",
			Status:      "fail",
			Required:    true,
			Message:     "Neo4j Docker image not found",
			Remediation: "Pull the image: docker pull neo4j:5",
		}
	}
	msg := "Neo4j image present"
	if info.ImageTag != "" {
		msg = fmt.Sprintf("Neo4j image present (%s)", info.ImageTag)
	}
	return PreflightCheck{
		ID:       "neo4j-image",
		Status:   "pass",
		Required: true,
		Message:  msg,
	}
}

// EvalPortFree evaluates whether a TCP port is available.
// With dynamic port allocation, a busy preferred port is advisory (warn), not fatal.
func EvalPortFree(info PortInfo) PreflightCheck {
	if !info.Free {
		return PreflightCheck{
			ID:          "port-available",
			Status:      "warn",
			Required:    false,
			Message:     fmt.Sprintf("Port %d is in use — dynamic allocation will find a free port at startup", info.Port),
			Remediation: fmt.Sprintf("Free port %d or let dynamic allocation handle it", info.Port),
		}
	}
	return PreflightCheck{
		ID:       "port-available",
		Status:   "pass",
		Required: false,
		Message:  fmt.Sprintf("Port %d is available", info.Port),
	}
}

// EvalSSHReachable evaluates SSH connectivity for studio-remote profile.
// Returns a skip check if host is empty (local profile).
func EvalSSHReachable(info SSHInfo) PreflightCheck {
	if info.Host == "" {
		return PreflightCheck{
			ID:       "ssh-reachable",
			Status:   "skip",
			Required: false,
			Message:  "SSH check skipped (local profile)",
		}
	}
	if !info.Reachable {
		return PreflightCheck{
			ID:          "ssh-reachable",
			Status:      "fail",
			Required:    true,
			Message:     fmt.Sprintf("Cannot reach remote host %s via SSH", info.Host),
			Remediation: fmt.Sprintf("Verify SSH access: ssh %s true", info.Host),
		}
	}
	return PreflightCheck{
		ID:       "ssh-reachable",
		Status:   "pass",
		Required: true,
		Message:  fmt.Sprintf("SSH connection to %s successful", info.Host),
	}
}

// EvalDockerContext evaluates whether the Docker context for remote access is configured.
// Returns a skip check if host is empty (local profile).
func EvalDockerContext(info DockerContextInfo) PreflightCheck {
	if info.Host == "" {
		return PreflightCheck{
			ID:       "docker-context",
			Status:   "skip",
			Required: false,
			Message:  "Docker context check skipped (local profile)",
		}
	}
	if !info.Exists {
		return PreflightCheck{
			ID:          "docker-context",
			Status:      "fail",
			Required:    true,
			Message:     fmt.Sprintf("Docker context %q not found", info.ContextName),
			Remediation: fmt.Sprintf("Will be auto-created during install for host %s", info.Host),
		}
	}
	if !info.Functional {
		return PreflightCheck{
			ID:          "docker-context",
			Status:      "fail",
			Required:    true,
			Message:     fmt.Sprintf("Docker context %q exists but is not functional", info.ContextName),
			Remediation: fmt.Sprintf("Verify SSH access to %s and Docker on remote host", info.Host),
		}
	}
	return PreflightCheck{
		ID:       "docker-context",
		Status:   "pass",
		Required: true,
		Message:  fmt.Sprintf("Docker context %q functional (host: %s)", info.ContextName, info.Host),
	}
}

// EvalRemoteBinary evaluates whether the mdemg binary is available on the remote host.
// Returns a skip check if the binary info indicates local profile (not available and no path).
func EvalRemoteBinary(info RemoteBinaryInfo) PreflightCheck {
	if !info.Available {
		return PreflightCheck{
			ID:          "remote-binary",
			Status:      "warn",
			Required:    false,
			Message:     "mdemg binary not found on remote host",
			Remediation: "Install mdemg on the remote host or copy the binary manually",
		}
	}
	msg := fmt.Sprintf("mdemg binary available at %s", info.Path)
	if info.Version != "" {
		msg = fmt.Sprintf("mdemg binary available at %s (version %s)", info.Path, info.Version)
	}
	return PreflightCheck{
		ID:       "remote-binary",
		Status:   "pass",
		Required: false,
		Message:  msg,
	}
}

// EvalDockerDependency evaluates Docker as a dependency with version comparison.
func EvalDockerDependency(info DockerInfo, requiredVersion string) DependencyResult {
	if !info.Available {
		return DependencyResult{
			Name:            "docker",
			Status:          "failed",
			RequiredVersion: requiredVersion,
			Action:          "not-found",
		}
	}
	if requiredVersion != "" && !CompareVersion(info.Version, requiredVersion) {
		return DependencyResult{
			Name:            "docker",
			Status:          "failed",
			RequiredVersion: requiredVersion,
			DetectedVersion: info.Version,
			Action:          "version-insufficient",
		}
	}
	return DependencyResult{
		Name:            "docker",
		Status:          "present",
		RequiredVersion: requiredVersion,
		DetectedVersion: info.Version,
		Action:          "none",
	}
}

// EvalNeo4jDependency evaluates the Neo4j image as a dependency.
func EvalNeo4jDependency(info Neo4jImageInfo) DependencyResult {
	if !info.ImageFound {
		return DependencyResult{
			Name:   "neo4j",
			Status: "failed",
			Action: "image-missing",
		}
	}
	return DependencyResult{
		Name:            "neo4j",
		Status:          "present",
		DetectedVersion: info.Version,
		Action:          "none",
	}
}

// HasPreflightFailures returns true if any required preflight check has status "fail".
func HasPreflightFailures(checks []PreflightCheck) bool {
	for _, c := range checks {
		if c.Required && c.Status == "fail" {
			return true
		}
	}
	return false
}

// HasDependencyFailures returns true if any dependency has status "failed".
func HasDependencyFailures(deps []DependencyResult) bool {
	for _, d := range deps {
		if d.Status == "failed" {
			return true
		}
	}
	return false
}

// CompareVersion returns true if detected >= required.
// Strips leading ">=" prefix from required. Compares major.minor.patch numerically.
// Returns true if either version is empty or unparseable (lenient).
func CompareVersion(detected, required string) bool {
	required = strings.TrimPrefix(required, ">=")
	required = strings.TrimSpace(required)
	detected = strings.TrimSpace(detected)

	if detected == "" || required == "" {
		return true
	}

	detParts := parseVersionParts(detected)
	reqParts := parseVersionParts(required)

	for i := range 3 {
		if detParts[i] > reqParts[i] {
			return true
		}
		if detParts[i] < reqParts[i] {
			return false
		}
	}
	return true // equal
}

// parseVersionParts extracts up to 3 numeric parts from a version string.
// Non-numeric suffixes (e.g. "-beta") are ignored. Missing parts default to 0.
func parseVersionParts(v string) [3]int {
	var parts [3]int
	segments := strings.SplitN(v, ".", 3)
	for idx := range 3 {
		if idx >= len(segments) {
			break
		}
		// Strip non-numeric suffix (e.g. "27-beta" → "27")
		numStr := strings.TrimLeft(segments[idx], " ")
		for j, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:j]
				break
			}
		}
		n, err := strconv.Atoi(numStr)
		if err == nil {
			parts[idx] = n
		}
	}
	return parts
}

// NewInstallReport constructs an InstallReport with all slices initialized to [].
func NewInstallReport(
	stateBefore, stateAfter State,
	exitCode int,
	profile string,
	dryRun bool,
	autoFix bool,
	checks []PreflightCheck,
	deps []DependencyResult,
	components []string,
	lockfilePath string,
) InstallReport {
	envelope := NewReportEnvelope("mdemg sidecar install", stateBefore, stateAfter, exitCode)

	// Result can also be "warning" for non-fatal issues
	if exitCode == ExitSuccess && HasPreflightFailures(checks) {
		envelope.Result = "warning"
	}

	// Ensure non-nil slices
	if checks == nil {
		checks = []PreflightCheck{}
	}
	if deps == nil {
		deps = []DependencyResult{}
	}
	if components == nil {
		components = []string{}
	}

	return InstallReport{
		ReportEnvelope:      envelope,
		Profile:             profile,
		DryRun:              dryRun,
		AutoFixEnabled:      autoFix,
		PreflightChecks:     checks,
		Dependencies:        deps,
		InstalledComponents: components,
		LockfilePath:        lockfilePath,
	}
}
