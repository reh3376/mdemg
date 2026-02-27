package cli

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarInstallCmd() *cobra.Command {
	var (
		dryRun    bool
		noAutoFix bool
		format    string
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install sidecar dependencies and runtime",
		Long: `Verify and install sidecar dependencies (Docker, Neo4j image).

Runs preflight checks, resolves dependencies, and transitions
the sidecar state from initialized to installed.

Examples:
  mdemg sidecar install
  mdemg sidecar install --dry-run --format json
  mdemg sidecar install --no-auto-fix
  mdemg sidecar install --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarInstall(sidecarInstallFlags{
				dryRun:    dryRun,
				noAutoFix: noAutoFix,
				format:    format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Check dependencies without making changes")
	cmd.Flags().BoolVar(&noAutoFix, "no-auto-fix", false, "Report failures without auto-fixing")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarInstallFlags struct {
	dryRun    bool
	noAutoFix bool
	format    string
}

func runSidecarInstall(flags sidecarInstallFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)
	lockPath := sidecar.FindLockFileFrom(cwd)

	// State guard: must be initialized or installed (re-run)
	if stateBefore != sidecar.StateInitialized && stateBefore != sidecar.StateInstalled {
		if flags.format == "json" {
			report := sidecar.NewInstallReport(
				stateBefore, stateBefore, sidecar.ExitValidation,
				"", flags.dryRun, false, nil, nil, nil, "",
			)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot install from state %q", stateBefore),
				Remediation: "Run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("cannot install from state %q — run: mdemg sidecar init", stateBefore)
	}

	// Load and validate config
	configPath := sidecar.FindConfigFileFrom(cwd)
	if configPath == "" {
		if flags.format == "json" {
			report := sidecar.NewInstallReport(
				stateBefore, stateBefore, sidecar.ExitValidation,
				"", flags.dryRun, false, nil, nil, nil, "",
			)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_NOT_FOUND",
				Severity:    "error",
				Message:     "No sidecar.yaml found",
				Remediation: "Run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("no sidecar.yaml found — run: mdemg sidecar init")
	}

	cfg, err := sidecar.LoadConfig(configPath)
	if err != nil {
		if flags.format == "json" {
			report := sidecar.NewInstallReport(
				stateBefore, stateBefore, sidecar.ExitValidation,
				"", flags.dryRun, false, nil, nil, nil, "",
			)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_READ_ERROR",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to read config: %v", err),
				Remediation: "Check .mdemg/sidecar.yaml syntax or re-run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("read config: %w", err)
	}

	cfgErrs := sidecar.ValidateConfig(cfg)
	if sidecar.HasErrors(cfgErrs) {
		if flags.format == "json" {
			report := sidecar.NewInstallReport(
				stateBefore, stateBefore, sidecar.ExitValidation,
				string(cfg.Profile), flags.dryRun, false, nil, nil, nil, "",
			)
			for _, e := range cfgErrs {
				report.Issues = append(report.Issues, sidecar.ReportIssue{
					Code:        "CONFIG_VALIDATION",
					Severity:    e.Level,
					Message:     fmt.Sprintf("%s: %s", e.Field, e.Message),
					Remediation: fmt.Sprintf("Fix the %s field in sidecar.yaml", e.Field),
				})
			}
			return sidecar.PrintJSON(report)
		}
		fmt.Println("Configuration validation errors:")
		for _, e := range cfgErrs {
			fmt.Printf("  [%s] %s: %s\n", e.Level, e.Field, e.Message)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(cfgErrs))
	}

	profile := string(cfg.Profile)
	autoFix := cfg.Install.AutoFix && !flags.noAutoFix
	port := extractPort(cfg.Runtime.Endpoint)

	// Idempotency: if already installed and config hasn't changed, no-op
	if stateBefore == sidecar.StateInstalled && lockPath != "" {
		lf, lfErr := sidecar.ReadLock(lockPath)
		if lfErr == nil {
			configData, readErr := os.ReadFile(configPath)
			if readErr == nil {
				currentHash := sidecar.HashConfig(configData)
				if lf.ConfigHash == currentHash {
					if flags.format == "json" {
						report := sidecar.NewInstallReport(
							stateBefore, stateBefore, sidecar.ExitSuccess,
							profile, flags.dryRun, autoFix,
							[]sidecar.PreflightCheck{}, []sidecar.DependencyResult{},
							[]string{}, lockPath,
						)
						report.NextActions = []string{"mdemg sidecar up"}
						return sidecar.PrintJSON(report)
					}
					fmt.Println("Sidecar already installed — no changes needed.")
					fmt.Println("\nNext steps:")
					fmt.Println("  mdemg sidecar up")
					return nil
				}
			}
		}
	}

	// Gather detection info
	dockerInfo := gatherDockerInfo()
	neo4jInfo := gatherNeo4jImageInfo()
	portInfo := gatherPortInfo(port)

	var sshInfo sidecar.SSHInfo
	if cfg.Profile == sidecar.ProfileStudioRemote {
		sshInfo = gatherSSHInfo(cfg.Runtime.Remote.Host)
	}

	// Remote-specific detection
	var dockerCtxInfo sidecar.DockerContextInfo
	var remoteBinInfo sidecar.RemoteBinaryInfo
	if cfg.Profile == sidecar.ProfileStudioRemote {
		dockerCtxInfo = gatherDockerContextInfo(cfg.Runtime.Remote.Host)
		remoteBinInfo = gatherRemoteBinaryInfo(cfg.Runtime.Remote.Host)
	}

	// Evaluate preflight checks
	checks := []sidecar.PreflightCheck{
		sidecar.EvalDockerAvailable(dockerInfo),
		sidecar.EvalNeo4jImage(neo4jInfo),
		sidecar.EvalPortFree(portInfo),
		sidecar.EvalSSHReachable(sshInfo),
	}

	// Add remote-specific checks for studio-remote profile
	if cfg.Profile == sidecar.ProfileStudioRemote {
		checks = append(checks,
			sidecar.EvalDockerContext(dockerCtxInfo),
			sidecar.EvalRemoteBinary(remoteBinInfo),
		)
	}

	// Auto-fix: pull missing Neo4j image and create docker context if enabled and not dry-run
	var components []string
	if autoFix && !flags.dryRun {
		for i, c := range checks {
			if c.ID == "neo4j-image" && c.Status == "fail" && dockerInfo.Available && dockerInfo.Running {
				fmt.Println("Auto-fix: pulling Neo4j image...")
				_, pullErr := RunDockerCommand("pull", "neo4j:5")
				if pullErr == nil {
					// Re-evaluate after fix
					neo4jInfo = gatherNeo4jImageInfo()
					checks[i] = sidecar.EvalNeo4jImage(neo4jInfo)
					components = append(components, "neo4j-image")
				} else {
					checks[i] = sidecar.PreflightCheck{
						ID:          "neo4j-image",
						Status:      "fail",
						Required:    true,
						Message:     fmt.Sprintf("Auto-fix failed: %v", pullErr),
						Remediation: "Pull manually: docker pull neo4j:5",
					}
				}
			}

			// Auto-create docker context for remote profile
			if c.ID == "docker-context" && c.Status == "fail" && cfg.Profile == sidecar.ProfileStudioRemote {
				fmt.Println("Auto-fix: creating Docker context for remote host...")
				re, reErr := newRemoteExecutor(cfg.Runtime.Remote.Host, cfg.Runtime.Remote.Transport)
				if reErr == nil {
					if ctxErr := re.EnsureDockerContext(); ctxErr == nil {
						dockerCtxInfo = gatherDockerContextInfo(cfg.Runtime.Remote.Host)
						checks[i] = sidecar.EvalDockerContext(dockerCtxInfo)
						components = append(components, "docker-context")
					} else {
						checks[i] = sidecar.PreflightCheck{
							ID:          "docker-context",
							Status:      "fail",
							Required:    true,
							Message:     fmt.Sprintf("Auto-fix failed: %v", ctxErr),
							Remediation: fmt.Sprintf("Create manually: docker context create %s --docker host=ssh://%s", mdemgDockerContext, cfg.Runtime.Remote.Host),
						}
					}
				}
			}
		}
	}

	// Evaluate dependencies
	deps := []sidecar.DependencyResult{
		sidecar.EvalDockerDependency(dockerInfo, ">=20.0.0"),
		sidecar.EvalNeo4jDependency(neo4jInfo),
	}

	// Determine exit code
	exitCode := sidecar.ExitSuccess
	if sidecar.HasPreflightFailures(checks) || sidecar.HasDependencyFailures(deps) {
		exitCode = sidecar.ExitDependency
	}

	stateAfter := stateBefore
	if exitCode == sidecar.ExitSuccess && !flags.dryRun {
		stateAfter = sidecar.StateInstalled
	}

	// Dry-run: adjust actions for reporting
	if flags.dryRun {
		for i := range deps {
			if deps[i].Action == "none" {
				deps[i].Action = "would-verify"
			}
		}
	}

	// Build report
	report := sidecar.NewInstallReport(
		stateBefore, stateAfter, exitCode,
		profile, flags.dryRun, autoFix,
		checks, deps, components, lockPath,
	)

	// Add issues for failures
	if sidecar.HasPreflightFailures(checks) {
		for _, c := range checks {
			if c.Required && c.Status == "fail" {
				report.Issues = append(report.Issues, sidecar.ReportIssue{
					Code:        "PREFLIGHT_FAIL",
					Severity:    "error",
					Message:     c.Message,
					Remediation: c.Remediation,
				})
			}
		}
	}
	if sidecar.HasDependencyFailures(deps) {
		for _, d := range deps {
			if d.Status == "failed" {
				msg := fmt.Sprintf("Dependency %s: %s", d.Name, d.Action)
				rem := fmt.Sprintf("Install or update %s", d.Name)
				report.Issues = append(report.Issues, sidecar.ReportIssue{
					Code:        "DEPENDENCY_FAIL",
					Severity:    "error",
					Message:     msg,
					Remediation: rem,
				})
			}
		}
	}

	// Set next actions
	if exitCode == sidecar.ExitSuccess {
		if flags.dryRun {
			report.NextActions = []string{"Run without --dry-run to apply"}
		} else {
			report.NextActions = []string{"mdemg sidecar up"}
		}
	} else {
		report.NextActions = []string{"Fix the issues above and re-run: mdemg sidecar install"}
	}

	// Persist state (if not dry-run and no failures)
	if !flags.dryRun && exitCode == sidecar.ExitSuccess && lockPath != "" {
		lf, lfErr := sidecar.ReadLock(lockPath)
		if lfErr == nil {
			lf.State = sidecar.StateInstalled
			// Update config hash
			configData, readErr := os.ReadFile(configPath)
			if readErr == nil {
				lf.ConfigHash = sidecar.HashConfig(configData)
			}
			if writeErr := sidecar.WriteLock(lockPath, lf); writeErr != nil {
				if flags.format == "json" {
					report.ExitCode = sidecar.ExitRuntime
					report.Result = "error"
					report.Issues = append(report.Issues, sidecar.ReportIssue{
						Code:        "LOCK_WRITE_FAIL",
						Severity:    "error",
						Message:     fmt.Sprintf("Failed to update lock file: %v", writeErr),
						Remediation: "Check file permissions on .mdemg/sidecar.lock",
					})
					return sidecar.PrintJSON(report)
				}
				return fmt.Errorf("update lock file: %w", writeErr)
			}
			report.Changes = append(report.Changes, sidecar.ReportChange{
				Path:   lockPath,
				Action: "updated",
			})
		}

		// Write install report to .mdemg/generated/
		generatedDir := filepath.Join(filepath.Dir(configPath), "generated")
		if mkErr := os.MkdirAll(generatedDir, 0755); mkErr == nil {
			reportPath := filepath.Join(generatedDir, "install-report.json")
			report.Changes = append(report.Changes, sidecar.ReportChange{
				Path:   reportPath,
				Action: "created",
			})
		}
	}

	// Output
	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	printInstallText(report, checks, deps)
	return nil
}

func printInstallText(report sidecar.InstallReport, checks []sidecar.PreflightCheck, deps []sidecar.DependencyResult) {
	if report.DryRun {
		fmt.Println("[dry-run] Sidecar Install Check")
	} else {
		fmt.Println("Sidecar Install")
	}
	fmt.Println("===============")
	fmt.Println()
	fmt.Printf("  Profile:  %s\n", report.Profile)
	fmt.Printf("  Auto-fix: %v\n", report.AutoFixEnabled)
	fmt.Printf("  State:    %s → %s\n", report.StateBefore, report.StateAfter)
	fmt.Println()

	// Preflight checks table
	fmt.Println("  Preflight Checks:")
	for _, c := range checks {
		icon := statusIcon(c.Status)
		fmt.Printf("    %s %-20s %s\n", icon, c.ID, c.Message)
		if c.Remediation != "" && c.Status == "fail" {
			fmt.Printf("      → %s\n", c.Remediation)
		}
	}
	fmt.Println()

	// Dependencies table
	fmt.Println("  Dependencies:")
	for _, d := range deps {
		icon := depStatusIcon(d.Status)
		version := ""
		if d.DetectedVersion != "" {
			version = fmt.Sprintf(" (%s)", d.DetectedVersion)
		}
		fmt.Printf("    %s %-12s %s%s\n", icon, d.Name, d.Status, version)
		if d.Status == "failed" && d.Action != "" {
			fmt.Printf("      → %s\n", d.Action)
		}
	}
	fmt.Println()

	// Components installed
	if len(report.InstalledComponents) > 0 {
		fmt.Println("  Installed:")
		for _, comp := range report.InstalledComponents {
			fmt.Printf("    + %s\n", comp)
		}
		fmt.Println()
	}

	// Issues
	if len(report.Issues) > 0 {
		fmt.Println("  Issues:")
		for _, iss := range report.Issues {
			fmt.Printf("    [%s] %s\n", iss.Severity, iss.Message)
			fmt.Printf("           → %s\n", iss.Remediation)
		}
		fmt.Println()
	}

	// Next actions
	if len(report.NextActions) > 0 {
		fmt.Println("  Next steps:")
		for _, a := range report.NextActions {
			fmt.Printf("    %s\n", a)
		}
		fmt.Println()
	}
}

func statusIcon(status string) string {
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

func depStatusIcon(status string) string {
	switch status {
	case "present", "installed", "upgraded":
		return "✓"
	case "failed":
		return "✗"
	case "skipped":
		return "-"
	default:
		return "?"
	}
}

// --- Detection helpers (CLI layer, I/O boundary) ---

func gatherDockerInfo() sidecar.DockerInfo {
	if !DockerAvailable() {
		return sidecar.DockerInfo{Available: false}
	}

	// Get version
	version := ""
	out, err := RunDockerCommand("version", "--format", "{{.Server.Version}}")
	if err == nil {
		version = strings.TrimSpace(out)
	}

	// Check if daemon is running
	running := false
	_, infoErr := RunDockerCommand("info", "--format", "{{.ServerVersion}}")
	if infoErr == nil {
		running = true
	}

	return sidecar.DockerInfo{
		Available: true,
		Version:   version,
		Running:   running,
	}
}

func gatherNeo4jImageInfo() sidecar.Neo4jImageInfo {
	out, err := RunDockerCommand("images", "--filter", "reference=neo4j", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return sidecar.Neo4jImageInfo{ImageFound: false}
	}

	lines := strings.TrimSpace(out)
	if lines == "" {
		return sidecar.Neo4jImageInfo{ImageFound: false}
	}

	// Take first matching line
	firstLine := strings.SplitN(lines, "\n", 2)[0]
	firstLine = strings.TrimSpace(firstLine)

	// Extract version from tag (e.g., "neo4j:5.26.0" → "5.26.0")
	version := ""
	if parts := strings.SplitN(firstLine, ":", 2); len(parts) == 2 {
		version = parts[1]
	}

	return sidecar.Neo4jImageInfo{
		ImageFound: true,
		ImageTag:   firstLine,
		Version:    version,
	}
}

func gatherPortInfo(port int) sidecar.PortInfo {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return sidecar.PortInfo{Port: port, Free: false}
	}
	_ = ln.Close()
	return sidecar.PortInfo{Port: port, Free: true}
}

func gatherDockerContextInfo(host string) sidecar.DockerContextInfo {
	if host == "" {
		return sidecar.DockerContextInfo{}
	}
	info := sidecar.DockerContextInfo{
		ContextName: mdemgDockerContext,
		Host:        host,
	}

	// Check if context exists
	out, err := exec.Command("docker", "context", "ls", "--format", "{{.Name}}").CombinedOutput()
	if err == nil {
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.TrimSpace(line) == mdemgDockerContext {
				info.Exists = true
				break
			}
		}
	}

	// If it exists, check if it's functional
	if info.Exists {
		cmd := exec.Command("docker", "--context", mdemgDockerContext, "info", "--format", "{{.ServerVersion}}")
		if _, funcErr := cmd.CombinedOutput(); funcErr == nil {
			info.Functional = true
		}
	}

	return info
}

func gatherRemoteBinaryInfo(host string) sidecar.RemoteBinaryInfo {
	if host == "" {
		return sidecar.RemoteBinaryInfo{}
	}

	// Check if mdemg binary exists on remote
	cmd := exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", host, "which", "mdemg")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return sidecar.RemoteBinaryInfo{Available: false}
	}

	path := strings.TrimSpace(string(out))
	info := sidecar.RemoteBinaryInfo{
		Available: true,
		Path:      path,
	}

	// Try to get version
	verCmd := exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", host, "mdemg", "version")
	verOut, verErr := verCmd.CombinedOutput()
	if verErr == nil {
		info.Version = strings.TrimSpace(string(verOut))
	}

	return info
}

func gatherSSHInfo(host string) sidecar.SSHInfo {
	if host == "" {
		return sidecar.SSHInfo{}
	}
	cmd := exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", host, "true")
	err := cmd.Run()
	return sidecar.SSHInfo{
		Host:      host,
		Reachable: err == nil,
	}
}

func extractPort(endpoint string) int {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 9999
	}
	portStr := u.Port()
	if portStr == "" {
		return 9999
	}
	port := 0
	for _, c := range portStr {
		if c >= '0' && c <= '9' {
			port = port*10 + int(c-'0')
		} else {
			break
		}
	}
	if port == 0 {
		return 9999
	}
	return port
}
