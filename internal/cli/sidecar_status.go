package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarStatusCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sidecar state and service health",
		Long: `Display the current sidecar state, configuration, and service health.

Works without a running server — reports uninitialized/stopped states.

Examples:
  mdemg sidecar status
  mdemg sidecar status --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarStatus(format)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

const probeTimeout = 3 * time.Second

func runSidecarStatus(format string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	state := sidecar.CurrentStateFrom(cwd)
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// Load config and lock if available
	var cfg *sidecar.Config
	var lock *sidecar.LockFile
	var issues []sidecar.ReportIssue
	var nextActions []string

	configPath := sidecar.FindConfigFileFrom(cwd)
	if configPath != "" {
		cfg, err = sidecar.LoadConfig(configPath)
		if err != nil {
			issues = append(issues, sidecar.ReportIssue{
				Code:        "CONFIG_READ_ERROR",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to read config: %v", err),
				Remediation: "Check .mdemg/sidecar.yaml syntax or re-run mdemg sidecar init",
			})
		}
	}

	lockPath := sidecar.FindLockFileFrom(cwd)
	if lockPath != "" {
		lock, _ = sidecar.ReadLock(lockPath)
	}

	// Determine profile/endpoint/transport
	profile := "unknown"
	endpoint := "http://localhost:9999"
	transport := "local"
	runtimeHost := hostname

	if cfg != nil {
		profile = string(cfg.Profile)
		endpoint = cfg.Runtime.Endpoint
		if cfg.Profile == sidecar.ProfileStudioRemote {
			transport = string(cfg.Runtime.Remote.Transport)
			if cfg.Runtime.Remote.Host != "" {
				runtimeHost = cfg.Runtime.Remote.Host
			}
		}
	} else if lock != nil {
		profile = lock.Profile
		endpoint = lock.Endpoint
	}

	// Probe services
	services := probeServices(endpoint)

	// Build health summary
	healthSummary := buildHealthSummary(services)

	// Determine next actions
	switch state {
	case sidecar.StateUninitialized:
		nextActions = append(nextActions, "mdemg sidecar init")
	case sidecar.StateInitialized:
		nextActions = append(nextActions, "mdemg sidecar install")
	case sidecar.StateInstalled:
		nextActions = append(nextActions, "mdemg sidecar up")
	case sidecar.StateStopped:
		nextActions = append(nextActions, "mdemg sidecar up")
	case sidecar.StateDegraded:
		nextActions = append(nextActions, "mdemg sidecar doctor")
	}

	exitCode := sidecar.ExitSuccess
	result := "success"
	if len(issues) > 0 {
		exitCode = sidecar.ExitRuntime
		result = "warning"
	}

	if format == "json" {
		report := sidecar.StatusReport{
			ReportEnvelope: sidecar.ReportEnvelope{
				SchemaVersion: sidecar.ReportSchemaVersion,
				Command:       "mdemg sidecar status",
				Timestamp:     sidecar.NowUTC(),
				Result:        result,
				ExitCode:      exitCode,
				StateBefore:   state,
				StateAfter:    state,
				Changes:       []sidecar.ReportChange{},
				Issues:        issues,
				NextActions:   nextActions,
			},
			Profile:       profile,
			Endpoint:      endpoint,
			ControlHost:   hostname,
			RuntimeHost:   runtimeHost,
			Transport:     transport,
			Services:      services,
			HealthSummary: healthSummary,
		}
		// Ensure non-nil slices
		if report.Issues == nil {
			report.Issues = []sidecar.ReportIssue{}
		}
		if report.NextActions == nil {
			report.NextActions = []string{}
		}
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println("Sidecar Status")
	fmt.Println("==============")
	fmt.Println()
	fmt.Printf("  State:        %s\n", state)
	fmt.Printf("  Profile:      %s\n", profile)
	fmt.Printf("  Endpoint:     %s\n", endpoint)
	fmt.Printf("  Control Host: %s\n", hostname)
	fmt.Printf("  Runtime Host: %s\n", runtimeHost)
	fmt.Printf("  Transport:    %s\n", transport)
	fmt.Println()

	if len(services) > 0 {
		fmt.Println("  Services:")
		for _, svc := range services {
			healthIcon := "✗"
			if svc.Healthy {
				healthIcon = "✓"
			}
			latency := ""
			if svc.LatencyMs > 0 {
				latency = fmt.Sprintf(" (%dms)", svc.LatencyMs)
			}
			detail := ""
			if svc.Detail != "" {
				detail = fmt.Sprintf(" — %s", svc.Detail)
			}
			fmt.Printf("    %s %-12s %s%s%s\n", healthIcon, svc.Name, svc.Status, latency, detail)
		}
		fmt.Println()
	}

	if !healthSummary.Healthy && len(healthSummary.DegradedReasons) > 0 {
		fmt.Println("  Issues:")
		for _, r := range healthSummary.DegradedReasons {
			fmt.Printf("    • %s\n", r)
		}
		fmt.Println()
	}

	if len(issues) > 0 {
		fmt.Println("  Config Issues:")
		for _, iss := range issues {
			fmt.Printf("    [%s] %s\n", iss.Severity, iss.Message)
			fmt.Printf("           → %s\n", iss.Remediation)
		}
		fmt.Println()
	}

	if len(nextActions) > 0 {
		fmt.Println("  Next steps:")
		for _, a := range nextActions {
			fmt.Printf("    %s\n", a)
		}
		fmt.Println()
	}

	return nil
}

// probeServices checks reachability of the three core services.
func probeServices(endpoint string) []sidecar.ServiceStatus {
	services := make([]sidecar.ServiceStatus, 0, 3)

	// MDEMG API
	services = append(services, probeHTTP("mdemg-api", endpoint+"/healthz", endpoint, 9999))

	// Neo4j (TCP probe)
	services = append(services, probeTCP("neo4j", "localhost:7687", "bolt://localhost:7687", 7687))

	// Embedder (HTTP probe)
	services = append(services, probeHTTP("embedder", "http://localhost:11434/api/tags", "http://localhost:11434", 11434))

	return services
}

func probeHTTP(name, url, displayEndpoint string, port int) sidecar.ServiceStatus {
	start := time.Now()
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(url) //nolint:noctx // simple health probe
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.ServiceStatus{
			Name:     name,
			Status:   "down",
			Healthy:  false,
			Endpoint: displayEndpoint,
			Port:     port,
			Detail:   "unreachable",
		}
	}
	resp.Body.Close()

	status := "up"
	healthy := true
	detail := ""
	if resp.StatusCode >= 500 {
		status = "degraded"
		healthy = false
		detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return sidecar.ServiceStatus{
		Name:      name,
		Status:    status,
		Healthy:   healthy,
		Endpoint:  displayEndpoint,
		Port:      port,
		LatencyMs: latency,
		Detail:    detail,
	}
}

func probeTCP(name, addr, displayEndpoint string, port int) sidecar.ServiceStatus {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, probeTimeout)
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		return sidecar.ServiceStatus{
			Name:     name,
			Status:   "down",
			Healthy:  false,
			Endpoint: displayEndpoint,
			Port:     port,
			Detail:   "unreachable",
		}
	}
	_ = conn.Close()

	return sidecar.ServiceStatus{
		Name:      name,
		Status:    "up",
		Healthy:   true,
		Endpoint:  displayEndpoint,
		Port:      port,
		LatencyMs: latency,
	}
}

func buildHealthSummary(services []sidecar.ServiceStatus) sidecar.HealthSummary {
	total := len(services)
	up := 0
	var reasons []string

	for _, s := range services {
		if s.Healthy {
			up++
		} else {
			reasons = append(reasons, fmt.Sprintf("%s is %s", s.Name, s.Status))
		}
	}

	if reasons == nil {
		reasons = []string{}
	}

	return sidecar.HealthSummary{
		Healthy:           up == total,
		ServiceCountTotal: total,
		ServiceCountUp:    up,
		DegradedReasons:   reasons,
	}
}
