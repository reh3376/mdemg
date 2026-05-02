package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseHealthStateFromSnapshot verifies the JSON-snapshot parser reads
// the mdemg_mlx_health_state gauge from a `/v1/metrics/snapshot` body.
func TestParseHealthStateFromSnapshot(t *testing.T) {
	body := []byte(`{"data":{"gauges":{
		"mdemg_mlx_health_state{endpoint=\"http://127.0.0.1:8101/v1\"}": 2,
		"mdemg_mlx_health_state{endpoint=\"http://other:9999/v1\"}": 0,
		"mdemg_other_metric": 17
	}}}`)
	state, endpoint, ok := parseHealthStateFromSnapshot(body)
	if !ok {
		t.Fatal("parser returned ok=false on valid body")
	}
	// Map iteration order is unstable; either state is acceptable as long
	// as endpoint matches.
	if state != 0 && state != 2 {
		t.Errorf("state=%d, want 0 or 2 (one of the two endpoints)", state)
	}
	if endpoint != "http://127.0.0.1:8101/v1" && endpoint != "http://other:9999/v1" {
		t.Errorf("endpoint=%q unexpected", endpoint)
	}
}

// TestParseHealthStateFromSnapshot_NotPresent returns ok=false when the
// gauge is absent (e.g. watchdog disabled).
func TestParseHealthStateFromSnapshot_NotPresent(t *testing.T) {
	body := []byte(`{"data":{"gauges":{"mdemg_other":0}}}`)
	if _, _, ok := parseHealthStateFromSnapshot(body); ok {
		t.Error("ok=true on body lacking the metric — expected false")
	}
}

// TestParseHealthStateFromSnapshot_BadJSON returns ok=false on parse error.
func TestParseHealthStateFromSnapshot_BadJSON(t *testing.T) {
	if _, _, ok := parseHealthStateFromSnapshot([]byte("not json")); ok {
		t.Error("ok=true on invalid JSON — expected false")
	}
}

// TestExtractLabelValue covers the trim-on-quote edge cases.
func TestExtractLabelValue(t *testing.T) {
	cases := []struct {
		labels string
		key    string
		want   string
	}{
		{`endpoint="http://x/v1"`, "endpoint", "http://x/v1"},
		{`endpoint="x",foo="y"`, "foo", "y"},
		{`endpoint="x"`, "missing", ""},
		{``, "endpoint", ""},
	}
	for _, c := range cases {
		if got := extractLabelValue(c.labels, c.key); got != c.want {
			t.Errorf("extractLabelValue(%q,%q)=%q, want %q", c.labels, c.key, got, c.want)
		}
	}
}

// TestStateName covers the v→string mapping including the unknown branch.
func TestStateName(t *testing.T) {
	cases := map[int]string{0: "up", 1: "degraded", 2: "down", 99: "unknown"}
	for v, want := range cases {
		if got := stateName(v); got != want {
			t.Errorf("stateName(%d)=%q, want %q", v, got, want)
		}
	}
}

// TestFetchProbeStateFromMetrics_Live spins up a fake JSON-snapshot server
// and confirms the helper round-trips correctly.
func TestFetchProbeStateFromMetrics_Live(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/snapshot" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"gauges":{"mdemg_mlx_health_state{endpoint=\"http://x/v1\"}": 2}}}`))
	}))
	defer srv.Close()

	state, endpoint, err := fetchProbeStateFromMetrics(srv.URL + "/v1/metrics/snapshot")
	if err != nil {
		t.Fatalf("fetchProbeStateFromMetrics err=%v", err)
	}
	if state != 2 {
		t.Errorf("state=%d, want 2", state)
	}
	if endpoint != "http://x/v1" {
		t.Errorf("endpoint=%q, want http://x/v1", endpoint)
	}
}

// TestFetchProbeStateFromMetrics_AppendsPath verifies callers can pass either
// host-only (http://x:9999) or fully-qualified /v1/metrics/snapshot URLs,
// or even legacy /metrics URLs (for backward-compat with old callers).
func TestFetchProbeStateFromMetrics_AppendsPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics/snapshot" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"gauges":{"mdemg_mlx_health_state{endpoint=\"x\"}": 0}}}`))
	}))
	defer srv.Close()

	// Host-only — helper appends /v1/metrics/snapshot.
	if _, _, err := fetchProbeStateFromMetrics(srv.URL); err != nil {
		t.Errorf("fetchProbeStateFromMetrics(host-only) err=%v", err)
	}
	// Legacy /metrics — helper rewrites to /v1/metrics/snapshot.
	if _, _, err := fetchProbeStateFromMetrics(srv.URL + "/metrics"); err != nil {
		t.Errorf("fetchProbeStateFromMetrics(legacy /metrics) err=%v", err)
	}
}

// TestFetchRecentAlerts_FiltersByService confirms only mlx-server entries
// are returned and they are date-descending.
func TestFetchRecentAlerts_FiltersByService(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.json")
	t.Setenv("ALERT_FILE_PATH", path)

	body := `{
  "updated_at": "2026-04-30T12:00:00Z",
  "alerts": [
    {"time": "2026-04-30T11:00:00Z", "severity": "high", "title": "mlx unreachable", "message": "endpoint=x"},
    {"time": "2026-04-30T11:30:00Z", "severity": "low", "title": "Health probe recovered: neo4j", "message": "ok"},
    {"time": "2026-04-30T11:45:00Z", "severity": "low", "title": "mlx recovered", "message": "back"}
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	alerts, err := fetchRecentAlerts("mlx", 5)
	if err != nil {
		t.Fatalf("fetchRecentAlerts err=%v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2 (mlx-only)", len(alerts))
	}
	// Descending by time → "mlx recovered" first.
	if !strings.Contains(alerts[0].Title, "recovered") {
		t.Errorf("alerts[0].Title=%q, want it to start with 'mlx recovered' (descending order)", alerts[0].Title)
	}
}

// TestFetchRecentAlerts_RespectsLimit confirms the per-call cap.
func TestFetchRecentAlerts_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.json")
	t.Setenv("ALERT_FILE_PATH", path)

	body := `{
  "updated_at": "2026-04-30T12:00:00Z",
  "alerts": [
    {"time": "2026-04-30T11:00:00Z", "severity": "high", "title": "mlx 1", "message": ""},
    {"time": "2026-04-30T11:01:00Z", "severity": "high", "title": "mlx 2", "message": ""},
    {"time": "2026-04-30T11:02:00Z", "severity": "high", "title": "mlx 3", "message": ""}
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	alerts, err := fetchRecentAlerts("mlx", 2)
	if err != nil {
		t.Fatalf("fetchRecentAlerts err=%v", err)
	}
	if len(alerts) != 2 {
		t.Errorf("got %d alerts, want 2 after limit", len(alerts))
	}
}

// TestResolveMetricsURL_DefaultsTo9999 verifies the fallback path used when
// neither env nor port file is present.
func TestResolveMetricsURL_DefaultsTo9999(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // ensures no .mdemg.port from cwd
	t.Setenv("MDEMG_PORT", "")

	got := resolveMetricsURL()
	if got != "http://localhost:9999/v1/metrics/snapshot" {
		t.Errorf("resolveMetricsURL()=%q, want http://localhost:9999/v1/metrics/snapshot", got)
	}
}

// TestResolveMetricsURL_PortFileWins verifies .mdemg.port takes precedence
// over MDEMG_PORT.
func TestResolveMetricsURL_PortFileWins(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".mdemg.port"), []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MDEMG_PORT", "8080")

	got := resolveMetricsURL()
	if got != "http://localhost:12345/v1/metrics/snapshot" {
		t.Errorf("resolveMetricsURL()=%q, want http://localhost:12345/v1/metrics/snapshot (port file wins)", got)
	}
}

// TestBuildWatchdogStatus_Renders verifies the human-readable renderer
// produces sane output even with partial data.
func TestBuildWatchdogStatus_Renders(t *testing.T) {
	st := WatchdogStatus{
		HealthState: "down",
		Endpoint:    "http://127.0.0.1:8101/v1",
		RecentAlerts: []AlertRecord{
			{Time: time.Now(), Severity: "high", Title: "mlx unreachable", Message: "ok"},
		},
		LaunchdError: "not loaded",
	}
	var buf strings.Builder
	renderWatchdogStatus(&buf, st)
	out := buf.String()
	if !strings.Contains(out, "Probe state:  down") {
		t.Errorf("renderer missing probe state line:\n%s", out)
	}
	if !strings.Contains(out, "mlx unreachable") {
		t.Errorf("renderer missing alert title:\n%s", out)
	}
}
