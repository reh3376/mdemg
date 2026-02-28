package sidecar

import (
	"encoding/json"
	"testing"
)

// --- EvalDockerAvailable tests ---

func TestEvalDockerAvailable_Pass(t *testing.T) {
	check := EvalDockerAvailable(DockerInfo{Available: true, Version: "27.0.3", Running: true})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
	if check.ID != "docker-available" {
		t.Errorf("expected id docker-available, got %s", check.ID)
	}
}

func TestEvalDockerAvailable_NotInstalled(t *testing.T) {
	check := EvalDockerAvailable(DockerInfo{Available: false})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
	if check.Remediation == "" {
		t.Error("expected remediation")
	}
}

func TestEvalDockerAvailable_NotRunning(t *testing.T) {
	check := EvalDockerAvailable(DockerInfo{Available: true, Version: "27.0.3", Running: false})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
	if check.Remediation == "" {
		t.Error("expected remediation for daemon not running")
	}
}

// --- EvalNeo4jImage tests ---

func TestEvalNeo4jImage_Pass(t *testing.T) {
	check := EvalNeo4jImage(Neo4jImageInfo{ImageFound: true, ImageTag: "neo4j:5", Version: "5.26.0"})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
}

func TestEvalNeo4jImage_Missing(t *testing.T) {
	check := EvalNeo4jImage(Neo4jImageInfo{ImageFound: false})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
	if check.Remediation == "" {
		t.Error("expected remediation")
	}
}

func TestEvalNeo4jImage_NoTag(t *testing.T) {
	check := EvalNeo4jImage(Neo4jImageInfo{ImageFound: true})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
	if check.Message != "Neo4j image present" {
		t.Errorf("expected generic message, got %s", check.Message)
	}
}

// --- EvalPortFree tests ---

func TestEvalPortFree_Free(t *testing.T) {
	check := EvalPortFree(PortInfo{Port: 9999, Free: true})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
}

func TestEvalPortFree_InUse(t *testing.T) {
	check := EvalPortFree(PortInfo{Port: 9999, Free: false})
	if check.Status != "warn" {
		t.Errorf("expected warn (advisory with dynamic allocation), got %s", check.Status)
	}
	if check.Required {
		t.Error("expected Required=false (dynamic allocation handles conflicts)")
	}
	if check.Remediation == "" {
		t.Error("expected remediation")
	}
}

// --- EvalSSHReachable tests ---

func TestEvalSSHReachable_Reachable(t *testing.T) {
	check := EvalSSHReachable(SSHInfo{Host: "macstudio.local", Reachable: true})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
}

func TestEvalSSHReachable_Unreachable(t *testing.T) {
	check := EvalSSHReachable(SSHInfo{Host: "macstudio.local", Reachable: false})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
}

func TestEvalSSHReachable_Skip(t *testing.T) {
	check := EvalSSHReachable(SSHInfo{Host: ""})
	if check.Status != "skip" {
		t.Errorf("expected skip, got %s", check.Status)
	}
	if check.Required {
		t.Error("expected required=false for skipped check")
	}
}

// --- EvalDockerContext tests ---

func TestEvalDockerContext_Pass(t *testing.T) {
	check := EvalDockerContext(DockerContextInfo{
		ContextName: "mdemg-studio",
		Exists:      true,
		Functional:  true,
		Host:        "macstudio.local",
	})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
	if check.ID != "docker-context" {
		t.Errorf("expected id docker-context, got %s", check.ID)
	}
}

func TestEvalDockerContext_Missing(t *testing.T) {
	check := EvalDockerContext(DockerContextInfo{
		ContextName: "mdemg-studio",
		Exists:      false,
		Host:        "macstudio.local",
	})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
	if check.Remediation == "" {
		t.Error("expected remediation")
	}
}

func TestEvalDockerContext_NotFunctional(t *testing.T) {
	check := EvalDockerContext(DockerContextInfo{
		ContextName: "mdemg-studio",
		Exists:      true,
		Functional:  false,
		Host:        "macstudio.local",
	})
	if check.Status != "fail" {
		t.Errorf("expected fail, got %s", check.Status)
	}
}

func TestEvalDockerContext_Skip(t *testing.T) {
	check := EvalDockerContext(DockerContextInfo{Host: ""})
	if check.Status != "skip" {
		t.Errorf("expected skip, got %s", check.Status)
	}
	if check.Required {
		t.Error("expected required=false for skipped check")
	}
}

// --- EvalRemoteBinary tests ---

func TestEvalRemoteBinary_Available(t *testing.T) {
	check := EvalRemoteBinary(RemoteBinaryInfo{
		Available: true,
		Path:      "/usr/local/bin/mdemg",
		Version:   "0.1.0",
	})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
}

func TestEvalRemoteBinary_Missing(t *testing.T) {
	check := EvalRemoteBinary(RemoteBinaryInfo{Available: false})
	if check.Status != "warn" {
		t.Errorf("expected warn, got %s", check.Status)
	}
	if check.Required {
		t.Error("expected required=false for missing binary")
	}
}

func TestEvalRemoteBinary_NoVersion(t *testing.T) {
	check := EvalRemoteBinary(RemoteBinaryInfo{
		Available: true,
		Path:      "/usr/local/bin/mdemg",
	})
	if check.Status != "pass" {
		t.Errorf("expected pass, got %s", check.Status)
	}
}

// --- EvalDockerDependency tests ---

func TestEvalDockerDependency_Present(t *testing.T) {
	dep := EvalDockerDependency(DockerInfo{Available: true, Version: "27.0.3", Running: true}, ">=20.0.0")
	if dep.Status != "present" {
		t.Errorf("expected present, got %s", dep.Status)
	}
	if dep.DetectedVersion != "27.0.3" {
		t.Errorf("expected 27.0.3, got %s", dep.DetectedVersion)
	}
}

func TestEvalDockerDependency_VersionTooOld(t *testing.T) {
	dep := EvalDockerDependency(DockerInfo{Available: true, Version: "19.3.1", Running: true}, ">=20.0.0")
	if dep.Status != "failed" {
		t.Errorf("expected failed, got %s", dep.Status)
	}
	if dep.Action != "version-insufficient" {
		t.Errorf("expected version-insufficient action, got %s", dep.Action)
	}
}

func TestEvalDockerDependency_NotInstalled(t *testing.T) {
	dep := EvalDockerDependency(DockerInfo{Available: false}, ">=20.0.0")
	if dep.Status != "failed" {
		t.Errorf("expected failed, got %s", dep.Status)
	}
	if dep.Action != "not-found" {
		t.Errorf("expected not-found action, got %s", dep.Action)
	}
}

// --- EvalNeo4jDependency tests ---

func TestEvalNeo4jDependency_Present(t *testing.T) {
	dep := EvalNeo4jDependency(Neo4jImageInfo{ImageFound: true, Version: "5.26.0"})
	if dep.Status != "present" {
		t.Errorf("expected present, got %s", dep.Status)
	}
}

func TestEvalNeo4jDependency_Missing(t *testing.T) {
	dep := EvalNeo4jDependency(Neo4jImageInfo{ImageFound: false})
	if dep.Status != "failed" {
		t.Errorf("expected failed, got %s", dep.Status)
	}
	if dep.Action != "image-missing" {
		t.Errorf("expected image-missing action, got %s", dep.Action)
	}
}

// --- HasPreflightFailures tests ---

func TestHasPreflightFailures_NoFailures(t *testing.T) {
	checks := []PreflightCheck{
		{ID: "a", Status: "pass", Required: true},
		{ID: "b", Status: "warn", Required: true},
		{ID: "c", Status: "skip", Required: false},
	}
	if HasPreflightFailures(checks) {
		t.Error("expected no failures")
	}
}

func TestHasPreflightFailures_WithFailure(t *testing.T) {
	checks := []PreflightCheck{
		{ID: "a", Status: "pass", Required: true},
		{ID: "b", Status: "fail", Required: true},
	}
	if !HasPreflightFailures(checks) {
		t.Error("expected failure detected")
	}
}

func TestHasPreflightFailures_WarnNotFailure(t *testing.T) {
	checks := []PreflightCheck{
		{ID: "a", Status: "warn", Required: true},
	}
	if HasPreflightFailures(checks) {
		t.Error("warn should not count as failure")
	}
}

func TestHasPreflightFailures_OptionalFail(t *testing.T) {
	checks := []PreflightCheck{
		{ID: "a", Status: "fail", Required: false},
	}
	if HasPreflightFailures(checks) {
		t.Error("optional fail should not count as failure")
	}
}

// --- HasDependencyFailures tests ---

func TestHasDependencyFailures_NoFailures(t *testing.T) {
	deps := []DependencyResult{
		{Name: "docker", Status: "present"},
		{Name: "neo4j", Status: "installed"},
	}
	if HasDependencyFailures(deps) {
		t.Error("expected no failures")
	}
}

func TestHasDependencyFailures_WithFailure(t *testing.T) {
	deps := []DependencyResult{
		{Name: "docker", Status: "present"},
		{Name: "neo4j", Status: "failed"},
	}
	if !HasDependencyFailures(deps) {
		t.Error("expected failure detected")
	}
}

// --- NewInstallReport tests ---

func TestNewInstallReport_NilSliceSafety(t *testing.T) {
	report := NewInstallReport(
		StateInitialized, StateInstalled, ExitSuccess,
		"local", false, true, nil, nil, nil, "/tmp/.mdemg/sidecar.lock",
	)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	// Verify no "null" for slices
	if containsNull(s, "preflight_checks") {
		t.Error("preflight_checks should be [] not null")
	}
	if containsNull(s, "dependencies") {
		t.Error("dependencies should be [] not null")
	}
	if containsNull(s, "installed_components") {
		t.Error("installed_components should be [] not null")
	}
	if containsNull(s, "changes") {
		t.Error("changes should be [] not null")
	}
	if containsNull(s, "issues") {
		t.Error("issues should be [] not null")
	}
	if containsNull(s, "next_actions") {
		t.Error("next_actions should be [] not null")
	}
}

func TestNewInstallReport_FieldValues(t *testing.T) {
	report := NewInstallReport(
		StateInitialized, StateInstalled, ExitSuccess,
		"local", true, false,
		[]PreflightCheck{{ID: "a", Status: "pass"}},
		[]DependencyResult{{Name: "docker", Status: "present"}},
		[]string{"neo4j-container"},
		"/tmp/.mdemg/sidecar.lock",
	)
	if report.Profile != "local" {
		t.Errorf("expected local, got %s", report.Profile)
	}
	if !report.DryRun {
		t.Error("expected dry_run=true")
	}
	if report.AutoFixEnabled {
		t.Error("expected auto_fix_enabled=false")
	}
	if report.Command != "mdemg sidecar install" {
		t.Errorf("expected command, got %s", report.Command)
	}
	if report.LockfilePath != "/tmp/.mdemg/sidecar.lock" {
		t.Errorf("expected lockfile path, got %s", report.LockfilePath)
	}
	if len(report.PreflightChecks) != 1 {
		t.Errorf("expected 1 preflight check, got %d", len(report.PreflightChecks))
	}
	if len(report.Dependencies) != 1 {
		t.Errorf("expected 1 dependency, got %d", len(report.Dependencies))
	}
	if len(report.InstalledComponents) != 1 {
		t.Errorf("expected 1 component, got %d", len(report.InstalledComponents))
	}
}

func TestNewInstallReport_ErrorResult(t *testing.T) {
	report := NewInstallReport(
		StateInitialized, StateInitialized, ExitDependency,
		"local", false, true, nil, nil, nil, "",
	)
	if report.Result != "error" {
		t.Errorf("expected error result, got %s", report.Result)
	}
}

// --- CompareVersion tests ---

func TestCompareVersion_Sufficient(t *testing.T) {
	if !CompareVersion("27.0.3", ">=20.0.0") {
		t.Error("27.0.3 should satisfy >=20.0.0")
	}
}

func TestCompareVersion_Insufficient(t *testing.T) {
	if CompareVersion("19.3.1", ">=20.0.0") {
		t.Error("19.3.1 should not satisfy >=20.0.0")
	}
}

func TestCompareVersion_ExactMatch(t *testing.T) {
	if !CompareVersion("20.0.0", ">=20.0.0") {
		t.Error("20.0.0 should satisfy >=20.0.0")
	}
}

func TestCompareVersion_MajorHigherMinorLower(t *testing.T) {
	if !CompareVersion("21.0.0", ">=20.5.0") {
		t.Error("21.0.0 should satisfy >=20.5.0")
	}
}

func TestCompareVersion_EmptyDetected(t *testing.T) {
	if !CompareVersion("", ">=20.0.0") {
		t.Error("empty detected should be lenient (true)")
	}
}

func TestCompareVersion_EmptyRequired(t *testing.T) {
	if !CompareVersion("27.0.3", "") {
		t.Error("empty required should be lenient (true)")
	}
}

func TestCompareVersion_WithoutPrefix(t *testing.T) {
	if !CompareVersion("27.0.3", "20.0.0") {
		t.Error("27.0.3 should satisfy 20.0.0 (no prefix)")
	}
}

func TestCompareVersion_PatchDifference(t *testing.T) {
	if CompareVersion("20.0.1", ">=20.0.2") {
		t.Error("20.0.1 should not satisfy >=20.0.2")
	}
}

// helper: checks if a JSON key maps to null
func containsNull(jsonStr, key string) bool {
	// Unmarshal to map to check for null values
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return false
	}
	v, ok := m[key]
	return ok && v == nil
}
