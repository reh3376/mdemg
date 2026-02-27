package sidecar

import "fmt"

// TallyChecks counts pass/warn/fail/skip outcomes from a slice of DoctorChecks.
func TallyChecks(checks []DoctorCheck) DoctorSummary {
	var s DoctorSummary
	s.Total = len(checks)
	for _, c := range checks {
		switch c.Status {
		case "pass":
			s.Pass++
		case "warn":
			s.Warn++
		case "fail":
			s.Fail++
		case "skip":
			s.Skip++
		}
	}
	return s
}

// NewDoctorReport builds a DoctorReport with nil-slice safety and auto-tallied summary.
// Issues are auto-populated from any warn or fail checks.
func NewDoctorReport(stateBefore, stateAfter State, exitCode int, profile string, checks []DoctorCheck) DoctorReport {
	env := NewReportEnvelope("mdemg sidecar doctor", stateBefore, stateAfter, exitCode)

	if checks == nil {
		checks = []DoctorCheck{}
	}

	// Ensure evidence slices are non-nil
	for i := range checks {
		if checks[i].Evidence == nil {
			checks[i].Evidence = []string{}
		}
	}

	summary := TallyChecks(checks)

	// Auto-populate issues from warn/fail checks
	for _, c := range checks {
		if c.Status == "fail" || c.Status == "warn" {
			severity := "error"
			if c.Status == "warn" {
				severity = "warning"
			}
			env.Issues = append(env.Issues, ReportIssue{
				Code:        fmt.Sprintf("DOCTOR_%s", c.ID),
				Severity:    severity,
				Message:     c.Message,
				Remediation: c.Remediation,
			})
		}
	}

	return DoctorReport{
		ReportEnvelope: env,
		Profile:        profile,
		Checks:         checks,
		Summary:        summary,
	}
}

// DoctorExitCode returns ExitSuccess if no checks failed, ExitDependency if any failed.
func DoctorExitCode(summary DoctorSummary) int {
	if summary.Fail > 0 {
		return ExitDependency
	}
	return ExitSuccess
}
