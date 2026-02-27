package sidecar

import (
	"testing"
)

func TestTallyChecks_AllPass(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "a", Status: "pass"},
		{ID: "b", Status: "pass"},
		{ID: "c", Status: "pass"},
	}
	s := TallyChecks(checks)
	if s.Total != 3 || s.Pass != 3 || s.Warn != 0 || s.Fail != 0 || s.Skip != 0 {
		t.Errorf("unexpected tally: %+v", s)
	}
}

func TestTallyChecks_Mixed(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "a", Status: "pass"},
		{ID: "b", Status: "warn"},
		{ID: "c", Status: "fail"},
		{ID: "d", Status: "skip"},
	}
	s := TallyChecks(checks)
	if s.Total != 4 || s.Pass != 1 || s.Warn != 1 || s.Fail != 1 || s.Skip != 1 {
		t.Errorf("unexpected tally: %+v", s)
	}
}

func TestTallyChecks_Empty(t *testing.T) {
	s := TallyChecks([]DoctorCheck{})
	if s.Total != 0 || s.Pass != 0 || s.Warn != 0 || s.Fail != 0 || s.Skip != 0 {
		t.Errorf("unexpected tally for empty: %+v", s)
	}
}

func TestNewDoctorReport_NilSliceSafety(t *testing.T) {
	r := NewDoctorReport(StateRunning, StateRunning, ExitSuccess, "local", nil)
	if r.Checks == nil {
		t.Error("Checks should not be nil")
	}
	if r.Issues == nil {
		t.Error("Issues should not be nil")
	}
	if r.Changes == nil {
		t.Error("Changes should not be nil")
	}
	if r.NextActions == nil {
		t.Error("NextActions should not be nil")
	}
}

func TestNewDoctorReport_IssuesFromFailChecks(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "ok", Status: "pass", Message: "good"},
		{ID: "bad", Status: "fail", Message: "broken", Remediation: "fix it"},
		{ID: "meh", Status: "warn", Message: "shaky", Remediation: "maybe fix"},
	}
	r := NewDoctorReport(StateRunning, StateRunning, ExitDependency, "local", checks)

	if len(r.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(r.Issues))
	}
	if r.Issues[0].Severity != "error" || r.Issues[0].Code != "DOCTOR_bad" {
		t.Errorf("unexpected first issue: %+v", r.Issues[0])
	}
	if r.Issues[1].Severity != "warning" || r.Issues[1].Code != "DOCTOR_meh" {
		t.Errorf("unexpected second issue: %+v", r.Issues[1])
	}
}

func TestNewDoctorReport_FieldValues(t *testing.T) {
	r := NewDoctorReport(StateInstalled, StateRunning, ExitSuccess, "local", []DoctorCheck{})
	if r.Command != "mdemg sidecar doctor" {
		t.Errorf("unexpected command: %s", r.Command)
	}
	if r.Profile != "local" {
		t.Errorf("unexpected profile: %s", r.Profile)
	}
	if r.StateBefore != StateInstalled {
		t.Errorf("unexpected state_before: %s", r.StateBefore)
	}
	if r.StateAfter != StateRunning {
		t.Errorf("unexpected state_after: %s", r.StateAfter)
	}
	if r.Result != "success" {
		t.Errorf("unexpected result: %s", r.Result)
	}
}

func TestNewDoctorReport_EvidenceNilSafety(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "a", Status: "pass", Evidence: nil},
		{ID: "b", Status: "pass", Evidence: []string{"note"}},
	}
	r := NewDoctorReport(StateRunning, StateRunning, ExitSuccess, "local", checks)
	if r.Checks[0].Evidence == nil {
		t.Error("nil evidence should be replaced with empty slice")
	}
	if len(r.Checks[1].Evidence) != 1 {
		t.Error("existing evidence should be preserved")
	}
}

func TestNewDoctorReport_SummaryAutoTallied(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "a", Status: "pass"},
		{ID: "b", Status: "fail"},
	}
	r := NewDoctorReport(StateRunning, StateRunning, ExitDependency, "local", checks)
	if r.Summary.Total != 2 || r.Summary.Pass != 1 || r.Summary.Fail != 1 {
		t.Errorf("unexpected summary: %+v", r.Summary)
	}
}

func TestDoctorExitCode_NoFail(t *testing.T) {
	s := DoctorSummary{Total: 3, Pass: 2, Warn: 1}
	if DoctorExitCode(s) != ExitSuccess {
		t.Errorf("expected ExitSuccess, got %d", DoctorExitCode(s))
	}
}

func TestDoctorExitCode_WithFail(t *testing.T) {
	s := DoctorSummary{Total: 3, Pass: 1, Fail: 2}
	if DoctorExitCode(s) != ExitDependency {
		t.Errorf("expected ExitDependency, got %d", DoctorExitCode(s))
	}
}

func TestDoctorExitCode_WarnOnly(t *testing.T) {
	s := DoctorSummary{Total: 2, Warn: 2}
	if DoctorExitCode(s) != ExitSuccess {
		t.Errorf("expected ExitSuccess for warn-only, got %d", DoctorExitCode(s))
	}
}

func TestNewDoctorReport_ErrorResult(t *testing.T) {
	r := NewDoctorReport(StateRunning, StateRunning, ExitDependency, "local", []DoctorCheck{})
	if r.Result != "error" {
		t.Errorf("expected error result for non-zero exit, got %s", r.Result)
	}
}
