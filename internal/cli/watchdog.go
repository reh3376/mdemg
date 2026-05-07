package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newWatchdogCmd builds the parent `mdemg watchdog` command. The
// Phase 11.6.3 sprint adds `mdemg watchdog status` for operator visibility
// into the mlx_lm.server health probe + launchd restart behaviour.
func newWatchdogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watchdog",
		Short: "Inspect MLX watchdog state, restart count, and recent alerts",
		Long: `Inspect the LLM watchdog (Phase 11.6.3, Phase 13.5 cutover).

The watchdog has two halves: an in-process probe goroutine that reads
mdemg_mlx_health_state from the running mdemg's /metrics endpoint, and an
out-of-process launchd plist (com.mdemg.llama-server) that auto-restarts
llama-server when it dies. (Metric name retained for dashboard
compatibility; runtime is llama.cpp llama-server since Phase 13.5.)

Subcommands:
  status   Show current probe state, launchd restart count, last 5 alerts`,
	}
	cmd.AddCommand(newWatchdogStatusCmd())
	return cmd
}

// WatchdogStatus is the structured output for `mdemg watchdog status --json`.
type WatchdogStatus struct {
	HealthState        string         `json:"health_state"`             // "up", "degraded", "down", or "unknown"
	HealthStateValue   *int           `json:"health_state_value,omitempty"` // 0=up, 1=degraded, 2=down (nil if unknown)
	Endpoint           string         `json:"endpoint,omitempty"`
	MetricsSource      string         `json:"metrics_source,omitempty"` // URL of /metrics endpoint queried
	MetricsError       string         `json:"metrics_error,omitempty"`
	Launchd            *LaunchdStatus `json:"launchd,omitempty"`
	LaunchdError       string         `json:"launchd_error,omitempty"`
	RecentAlerts       []AlertRecord  `json:"recent_alerts,omitempty"`
	AlertsError        string         `json:"alerts_error,omitempty"`
}

// LaunchdStatus mirrors the subset of `launchctl print` output the watchdog
// needs to surface.
type LaunchdStatus struct {
	Label      string `json:"label"`
	PID        int    `json:"pid,omitempty"`
	State      string `json:"state,omitempty"`
	LastExit   *int   `json:"last_exit_code,omitempty"`
	Generation int    `json:"generation,omitempty"`
}

// AlertRecord is the subset of an alert entry surfaced by `watchdog status`.
type AlertRecord struct {
	Time     time.Time `json:"time"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Message  string    `json:"message"`
}

func newWatchdogStatusCmd() *cobra.Command {
	var (
		jsonOut    bool
		metricsURL string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show MLX watchdog state + launchd restart count + recent alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			st := buildWatchdogStatus(metricsURL)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(st)
			}
			renderWatchdogStatus(os.Stdout, st)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON")
	cmd.Flags().StringVar(&metricsURL, "metrics-url", "",
		"Override the running mdemg /metrics URL (default: from .mdemg.port or http://localhost:9999/metrics)")
	return cmd
}

// buildWatchdogStatus aggregates state from /metrics, launchctl, and the
// alerts file. None of the three sources is required for the command to
// succeed; missing data shows up as "unknown" / "not loaded" / "no recent
// alerts".
func buildWatchdogStatus(metricsURL string) WatchdogStatus {
	st := WatchdogStatus{HealthState: "unknown"}

	// 1. Probe state from Prometheus exposition.
	if metricsURL == "" {
		metricsURL = resolveMetricsURL()
	}
	st.MetricsSource = metricsURL
	if state, endpoint, err := fetchProbeStateFromMetrics(metricsURL); err == nil {
		st.HealthState = stateName(state)
		v := state
		st.HealthStateValue = &v
		st.Endpoint = endpoint
	} else {
		st.MetricsError = err.Error()
	}

	// 2. launchd status (Darwin only).
	if runtime.GOOS == "darwin" {
		if ls, err := fetchLaunchdStatus("com.mdemg.mlx-server"); err == nil {
			st.Launchd = ls
		} else {
			st.LaunchdError = err.Error()
		}
	}

	// 3. Recent alerts.
	if alerts, err := fetchRecentAlerts("mlx-server", 5); err == nil {
		st.RecentAlerts = alerts
	} else {
		st.AlertsError = err.Error()
	}

	return st
}

// fetchProbeStateFromMetrics scrapes the JSON metrics snapshot endpoint
// (`/v1/metrics/snapshot`) and extracts the latest
// `mdemg_mlx_health_state{endpoint=...}` gauge value. Hotfix 11.6.3.1
// corrected the path from `/metrics` (Prometheus exposition, removed) to
// `/v1/metrics/snapshot` (JSON shape mdemg actually serves).
func fetchProbeStateFromMetrics(baseURL string) (int, string, error) {
	if !strings.HasSuffix(baseURL, "/v1/metrics/snapshot") {
		baseURL = strings.TrimRight(baseURL, "/")
		// Strip a trailing /metrics if present (legacy callers).
		baseURL = strings.TrimSuffix(baseURL, "/metrics")
		baseURL = baseURL + "/v1/metrics/snapshot"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return 0, "", fmt.Errorf("fetch %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return 0, "", fmt.Errorf("read body: %w", err)
	}

	state, endpoint, ok := parseHealthStateFromSnapshot(body)
	if !ok {
		return 0, "", fmt.Errorf("mdemg_mlx_health_state gauge not present (watchdog disabled?)")
	}
	return state, endpoint, nil
}

// parseHealthStateFromSnapshot reads the JSON snapshot served by
// `/v1/metrics/snapshot` and extracts the mdemg_mlx_health_state gauge.
// Returns (state, endpoint, true) on success.
func parseHealthStateFromSnapshot(body []byte) (int, string, bool) {
	var doc struct {
		Data struct {
			Gauges map[string]float64 `json:"gauges"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, "", false
	}
	// Match keys of the form `mdemg_mlx_health_state{endpoint="..."}`.
	const prefix = "mdemg_mlx_health_state{"
	for key, val := range doc.Data.Gauges {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		closeBrace := strings.Index(key, "}")
		if closeBrace < 0 {
			continue
		}
		labels := key[len(prefix):closeBrace]
		endpoint := extractLabelValue(labels, "endpoint")
		return int(val), endpoint, true
	}
	return 0, "", false
}

func extractLabelValue(labels, key string) string {
	prefix := key + `="`
	idx := strings.Index(labels, prefix)
	if idx < 0 {
		return ""
	}
	rest := labels[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// fetchLaunchdStatus runs `launchctl print gui/<uid>/<label>` and parses pid
// + state out of the output. Returns nil + error if launchctl is missing or
// the service isn't loaded.
func fetchLaunchdStatus(label string) (*LaunchdStatus, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	target := fmt.Sprintf("gui/%s/%s", u.Uid, label)
	cmd := exec.Command("launchctl", "print", target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("launchctl print %s: %w (%s)", target, err, strings.TrimSpace(string(out)))
	}

	ls := &LaunchdStatus{Label: label}
	for _, raw := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "pid = "):
			if pid, err := strconv.Atoi(strings.TrimPrefix(line, "pid = ")); err == nil {
				ls.PID = pid
			}
		case strings.HasPrefix(line, "state = "):
			ls.State = strings.TrimPrefix(line, "state = ")
		case strings.HasPrefix(line, "last exit code = "):
			if v, err := strconv.Atoi(strings.TrimPrefix(line, "last exit code = ")); err == nil {
				ls.LastExit = &v
			}
		case strings.HasPrefix(line, "generation = "):
			if g, err := strconv.Atoi(strings.TrimPrefix(line, "generation = ")); err == nil {
				ls.Generation = g
			}
		}
	}
	return ls, nil
}

// fetchRecentAlerts reads the alert dispatcher's file backend and returns
// the last `limit` entries for the given service.
func fetchRecentAlerts(service string, limit int) ([]AlertRecord, error) {
	path := alertFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var file struct {
		UpdatedAt time.Time     `json:"updated_at"`
		Alerts    []AlertRecord `json:"alerts"`
	}
	// Some alert entries have additional fields; decode liberally via a
	// loose schema and reject only on outright JSON corruption.
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	out := make([]AlertRecord, 0, len(file.Alerts))
	for _, a := range file.Alerts {
		if a.Title == "" && a.Message == "" {
			continue
		}
		// service field isn't directly on AlertRecord (alert package has it
		// but we used a relaxed schema). We surface every alert; the upstream
		// dispatcher already namespaces them by service via the title/cooldown.
		// Filter heuristically: alerts whose title mentions "mlx" qualify.
		if strings.Contains(strings.ToLower(a.Title), service) ||
			strings.Contains(strings.ToLower(a.Message), service) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func alertFilePath() string {
	if env := os.Getenv("ALERT_FILE_PATH"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mdemg", "alerts", "current.json")
}

// resolveMetricsURL prefers .mdemg.port if present, else MDEMG_PORT, else 9999.
// Returns the JSON snapshot endpoint (`/v1/metrics/snapshot`) — Hotfix
// 11.6.3.1 corrected the path from the (non-existent) `/metrics` Prometheus
// route. fetchProbeStateFromMetrics tolerates either form for backward compat.
func resolveMetricsURL() string {
	port := os.Getenv("MDEMG_PORT")
	cwd, _ := os.Getwd()
	if data, err := os.ReadFile(filepath.Join(cwd, ".mdemg.port")); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			port = s
		}
	}
	if port == "" {
		port = "9999"
	}
	return fmt.Sprintf("http://localhost:%s/v1/metrics/snapshot", port)
}

func stateName(v int) string {
	switch v {
	case 0:
		return "up"
	case 1:
		return "degraded"
	case 2:
		return "down"
	default:
		return "unknown"
	}
}

func renderWatchdogStatus(w io.Writer, st WatchdogStatus) {
	fmt.Fprintln(w, "MLX Watchdog Status")
	fmt.Fprintln(w, "===================")
	if st.MetricsError == "" {
		fmt.Fprintf(w, "  Probe state:  %s", st.HealthState)
		if st.Endpoint != "" {
			fmt.Fprintf(w, "  (endpoint: %s)", st.Endpoint)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "  Probe state:  unknown — %s\n", st.MetricsError)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  launchd:")
	if st.Launchd != nil {
		fmt.Fprintf(w, "    Label:        %s\n", st.Launchd.Label)
		if st.Launchd.PID > 0 {
			fmt.Fprintf(w, "    PID:          %d\n", st.Launchd.PID)
		}
		if st.Launchd.State != "" {
			fmt.Fprintf(w, "    State:        %s\n", st.Launchd.State)
		}
		if st.Launchd.LastExit != nil {
			fmt.Fprintf(w, "    Last exit:    %d\n", *st.Launchd.LastExit)
		}
		if st.Launchd.Generation > 0 {
			fmt.Fprintf(w, "    Generation:   %d (≈ restart count)\n", st.Launchd.Generation)
		}
	} else if st.LaunchdError != "" {
		fmt.Fprintf(w, "    not loaded — %s\n", st.LaunchdError)
	} else if runtime.GOOS != "darwin" {
		fmt.Fprintln(w, "    (launchd is darwin-only; skipped)")
	} else {
		fmt.Fprintln(w, "    (no data)")
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Recent mlx-server alerts:")
	if len(st.RecentAlerts) == 0 {
		if st.AlertsError != "" {
			fmt.Fprintf(w, "    (none — %s)\n", st.AlertsError)
		} else {
			fmt.Fprintln(w, "    (none)")
		}
	} else {
		for _, a := range st.RecentAlerts {
			fmt.Fprintf(w, "    %s  [%s]  %s — %s\n",
				a.Time.Local().Format("2006-01-02 15:04:05"),
				a.Severity, a.Title, a.Message)
		}
	}
}
