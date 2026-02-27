package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarDoctorCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostics and health checks",
		Long: `Run comprehensive diagnostics on the sidecar runtime.

Performs 5 diagnostic checks: configuration validation, Neo4j
reachability, API health, CMS resume, and embedder availability.
Results are displayed as a check table.

Examples:
  mdemg sidecar doctor
  mdemg sidecar doctor --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarDoctor(format)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

func runSidecarDoctor(format string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)

	// Load config
	profile := "unknown"
	endpoint := "http://localhost:9999"

	configPath := sidecar.FindConfigFileFrom(cwd)
	var cfg *sidecar.Config
	if configPath != "" {
		cfg, _ = sidecar.LoadConfig(configPath)
		if cfg != nil {
			profile = string(cfg.Profile)
			endpoint = cfg.Runtime.Endpoint
		}
	}

	// Run diagnostic checks
	var checks []sidecar.DoctorCheck

	// Check 1: config.valid
	checks = append(checks, runConfigCheck(configPath, cfg))

	// Check 2: neo4j.reachable
	checks = append(checks, runNeo4jCheck())

	// Check 3: api.healthy
	checks = append(checks, runAPICheck(endpoint))

	// Check 4: cms.resume
	checks = append(checks, runCMSCheck(endpoint))

	// Check 5: embedder.available
	checks = append(checks, runEmbedderCheck())

	// Tally and build report
	summary := sidecar.TallyChecks(checks)
	exitCode := sidecar.DoctorExitCode(summary)

	report := sidecar.NewDoctorReport(stateBefore, stateBefore, exitCode, profile, checks)
	report.NextActions = doctorNextActions(checks)

	// Persist report to .mdemg/generated/
	if configPath != "" {
		generatedDir := filepath.Join(filepath.Dir(configPath), "generated")
		if mkErr := os.MkdirAll(generatedDir, 0755); mkErr == nil {
			reportPath := filepath.Join(generatedDir, "doctor-report.json")
			reportData, marshalErr := json.MarshalIndent(report, "", "  ")
			if marshalErr == nil {
				_ = os.WriteFile(reportPath, append(reportData, '\n'), 0644)
				report.Changes = append(report.Changes, sidecar.ReportChange{
					Path:   reportPath,
					Action: "created",
				})
			}
		}
	}

	if format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println("Sidecar Doctor")
	fmt.Println("==============")
	fmt.Println()
	fmt.Printf("  Profile:  %s\n", profile)
	fmt.Printf("  State:    %s\n", stateBefore)
	fmt.Println()

	fmt.Println("  Checks:")
	for _, c := range checks {
		icon := doctorStatusIcon(c.Status)
		latency := ""
		if c.DurationMs > 0 {
			latency = fmt.Sprintf(" (%dms)", c.DurationMs)
		}
		fmt.Printf("    %s %-22s %s%s\n", icon, c.ID, c.Message, latency)
		if c.Remediation != "" && (c.Status == "fail" || c.Status == "warn") {
			fmt.Printf("      → %s\n", c.Remediation)
		}
	}
	fmt.Println()

	fmt.Printf("  Summary: %d total, %d pass, %d warn, %d fail, %d skip\n",
		summary.Total, summary.Pass, summary.Warn, summary.Fail, summary.Skip)
	fmt.Println()

	if len(report.Issues) > 0 {
		fmt.Println("  Issues:")
		for _, iss := range report.Issues {
			fmt.Printf("    [%s] %s\n", iss.Severity, iss.Message)
			fmt.Printf("           → %s\n", iss.Remediation)
		}
		fmt.Println()
	}

	if len(report.NextActions) > 0 {
		fmt.Println("  Next steps:")
		for _, a := range report.NextActions {
			fmt.Printf("    %s\n", a)
		}
		fmt.Println()
	}

	return nil
}

// --- Individual diagnostic checks ---

func runConfigCheck(configPath string, cfg *sidecar.Config) sidecar.DoctorCheck {
	start := time.Now()

	if configPath == "" {
		return sidecar.DoctorCheck{
			ID:          "config.valid",
			Category:    "configuration",
			Status:      "fail",
			Message:     "No sidecar.yaml found",
			DurationMs:  int(time.Since(start).Milliseconds()),
			Remediation: "Run: mdemg sidecar init",
		}
	}

	if cfg == nil {
		return sidecar.DoctorCheck{
			ID:          "config.valid",
			Category:    "configuration",
			Status:      "fail",
			Message:     "Failed to parse sidecar.yaml",
			DurationMs:  int(time.Since(start).Milliseconds()),
			Remediation: "Check YAML syntax in .mdemg/sidecar.yaml",
		}
	}

	errs := sidecar.ValidateConfig(cfg)
	if sidecar.HasErrors(errs) {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, fmt.Sprintf("%s: %s", e.Field, e.Message))
		}
		return sidecar.DoctorCheck{
			ID:          "config.valid",
			Category:    "configuration",
			Status:      "fail",
			Message:     fmt.Sprintf("Validation failed: %d error(s)", len(errs)),
			DurationMs:  int(time.Since(start).Milliseconds()),
			Remediation: "Fix errors in .mdemg/sidecar.yaml",
			Evidence:    msgs,
		}
	}

	return sidecar.DoctorCheck{
		ID:         "config.valid",
		Category:   "configuration",
		Status:     "pass",
		Message:    "Configuration valid",
		DurationMs: int(time.Since(start).Milliseconds()),
	}
}

func runNeo4jCheck() sidecar.DoctorCheck {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "localhost:7687", probeTimeout)
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.DoctorCheck{
			ID:          "neo4j.reachable",
			Category:    "database",
			Status:      "fail",
			Message:     "Neo4j unreachable on localhost:7687",
			DurationMs:  duration,
			Remediation: "Start Neo4j: mdemg db start",
		}
	}
	_ = conn.Close()

	return sidecar.DoctorCheck{
		ID:         "neo4j.reachable",
		Category:   "database",
		Status:     "pass",
		Message:    "Neo4j reachable",
		DurationMs: duration,
	}
}

func runAPICheck(endpoint string) sidecar.DoctorCheck {
	start := time.Now()
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(endpoint + "/healthz") //nolint:noctx // diagnostic probe
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.DoctorCheck{
			ID:          "api.healthy",
			Category:    "runtime",
			Status:      "fail",
			Message:     "MDEMG API unreachable",
			DurationMs:  duration,
			Remediation: "Start server: mdemg sidecar up",
		}
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return sidecar.DoctorCheck{
			ID:          "api.healthy",
			Category:    "runtime",
			Status:      "fail",
			Message:     fmt.Sprintf("MDEMG API returned HTTP %d", resp.StatusCode),
			DurationMs:  duration,
			Remediation: "Check server logs: .mdemg/logs/mdemg.log",
		}
	}

	return sidecar.DoctorCheck{
		ID:         "api.healthy",
		Category:   "runtime",
		Status:     "pass",
		Message:    "MDEMG API healthy",
		DurationMs: duration,
	}
}

func runCMSCheck(endpoint string) sidecar.DoctorCheck {
	start := time.Now()
	client := &http.Client{Timeout: probeTimeout}

	body := `{"space_id":"mdemg-dev","session_id":"doctor-probe","max_observations":1}`
	resp, err := client.Post( //nolint:noctx // diagnostic probe
		endpoint+"/v1/conversation/resume",
		"application/json",
		bytes.NewBufferString(body),
	)
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.DoctorCheck{
			ID:          "cms.resume",
			Category:    "cms",
			Status:      "fail",
			Message:     "CMS resume endpoint unreachable",
			DurationMs:  duration,
			Remediation: "Start server: mdemg sidecar up",
		}
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return sidecar.DoctorCheck{
			ID:          "cms.resume",
			Category:    "cms",
			Status:      "fail",
			Message:     fmt.Sprintf("CMS resume returned HTTP %d", resp.StatusCode),
			DurationMs:  duration,
			Remediation: "Check server and database connectivity",
		}
	}

	return sidecar.DoctorCheck{
		ID:         "cms.resume",
		Category:   "cms",
		Status:     "pass",
		Message:    "CMS resume responds",
		DurationMs: duration,
	}
}

func runEmbedderCheck() sidecar.DoctorCheck {
	start := time.Now()
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get("http://localhost:11434/api/tags") //nolint:noctx // diagnostic probe
	duration := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.DoctorCheck{
			ID:          "embedder.available",
			Category:    "embedding",
			Status:      "warn",
			Message:     "Embedder (Ollama) not reachable on localhost:11434",
			DurationMs:  duration,
			Remediation: "Install and start Ollama: https://ollama.com",
		}
	}
	resp.Body.Close()

	return sidecar.DoctorCheck{
		ID:         "embedder.available",
		Category:   "embedding",
		Status:     "pass",
		Message:    "Embedder available",
		DurationMs: duration,
	}
}

// --- Helpers ---

func doctorStatusIcon(status string) string {
	switch status {
	case "pass":
		return "✓"
	case "fail":
		return "✗"
	case "warn":
		return "!"
	case "skip":
		return "-"
	default:
		return "?"
	}
}

func doctorNextActions(checks []sidecar.DoctorCheck) []string {
	var actions []string
	for _, c := range checks {
		if c.Status == "fail" && c.Remediation != "" {
			actions = append(actions, c.Remediation)
		}
	}
	if len(actions) == 0 {
		actions = []string{"All checks passed"}
	}
	return actions
}
