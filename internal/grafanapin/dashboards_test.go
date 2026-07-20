// Package grafanapin holds Go pin tests that enforce Grafana dashboard
// contracts checked into deploy/docker/grafana/dashboards/. Tests are
// data-driven (JSON walk); no live Grafana required.
package grafanapin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// dashboardsDir returns the on-disk path to the checked-in dashboard JSON
// files, resolved from THIS test file's location. Keeps the pin test
// runnable regardless of the caller's cwd (matters for `go test ./...`
// from any subdirectory).
func dashboardsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate this test file")
	}
	// this file: internal/grafanapin/dashboards_test.go
	// target:    deploy/docker/grafana/dashboards/
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "deploy", "docker", "grafana", "dashboards")
}

// panelTarget captures the subset of a Grafana panel target we need.
type panelTarget struct {
	RawSQL string `json:"rawSql"`
}

// panel captures the subset of a Grafana panel we need.
type panel struct {
	Title   string        `json:"title"`
	Type    string        `json:"type"`
	Targets []panelTarget `json:"targets"`
	Panels  []panel       `json:"panels"` // for row panels
}

// dashboard captures the subset of a Grafana dashboard we need.
type dashboard struct {
	Title  string  `json:"title"`
	UID    string  `json:"uid"`
	Panels []panel `json:"panels"`
}

// TestGrafanaPanel_LLMInteractionsErrorFilter — GRAFANA-PANEL-FILTER-001 E3.
//
// Contract: every Grafana panel that reads `llm_interactions` AND references
// the `error` column in an aggregate (SUM/CASE/WHERE) must ALSO exclude
// caller-cancellation via `NOT LIKE 'caller_canceled:%'`. This mirrors the
// alert-rule filter in internal/tsdb/dataset_builder.go::LLMPerformance so
// dashboards + alerts count the same error population.
//
// Whitelist: a panel target may opt out by including the literal comment
// `-- GRAFANA-PANEL-FILTER-001: intentionally unfiltered` in its rawSql
// (for forensic/debug panels that legitimately want to show ALL errors).
//
// When the recorder starts emitting a new noise-class prefix beyond
// `caller_canceled:` (see internal/llmclient/client.go), extend this test's
// requiredExclusions[] AND dataset_builder.go's filter in the same PR.
func TestGrafanaPanel_LLMInteractionsErrorFilter(t *testing.T) {
	dir := dashboardsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	// Requires ALL of these NOT LIKE clauses on a panel that reads
	// llm_interactions.error. Extend when the recorder adds new prefixes.
	requiredExclusions := []string{
		"caller_canceled:",
	}

	whitelistComment := "GRAFANA-PANEL-FILTER-001: intentionally unfiltered"

	// Regex to identify a query that reads llm_interactions AND does
	// error-count math on the `error` column (rather than just displaying it).
	// We look for `error` used inside a WHEN/WHERE/AND predicate — that's the
	// aggregate pattern; a bare "SELECT error FROM" (display-only) is skipped.
	errorAggregatePattern := regexp.MustCompile(`(?i)\berror\s*(is|!=|<>|not\s+like|like)\b`)
	readsLLMInteractions := regexp.MustCompile(`(?i)\bllm_interactions\b`)

	violations := []string{}
	scannedPanelsTotal := 0
	matchedPanelsTotal := 0

	// walk collects targets from a panel tree (rows contain nested panels).
	var walk func(dashUID, dashPath string, panels []panel)
	walk = func(dashUID, dashPath string, panels []panel) {
		for _, p := range panels {
			for _, tgt := range p.Targets {
				scannedPanelsTotal++
				sql := tgt.RawSQL
				if sql == "" {
					continue
				}
				if !readsLLMInteractions.MatchString(sql) || !errorAggregatePattern.MatchString(sql) {
					continue
				}
				matchedPanelsTotal++
				if strings.Contains(sql, whitelistComment) {
					t.Logf("panel %q in %s: whitelisted (has %q)", p.Title, dashPath, whitelistComment)
					continue
				}
				for _, needle := range requiredExclusions {
					needleClause := "NOT LIKE '" + needle + "%'"
					if !strings.Contains(sql, needleClause) {
						violations = append(violations, dashUID+" :: "+p.Title+" — missing filter: "+needleClause)
					}
				}
			}
			// recurse into row-panels
			if len(p.Panels) > 0 {
				walk(dashUID, dashPath, p.Panels)
			}
		}
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		var d dashboard
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		walk(d.UID, e.Name(), d.Panels)
	}

	t.Logf("scanned %d panel targets across dashboards; %d matched llm_interactions.error aggregate pattern",
		scannedPanelsTotal, matchedPanelsTotal)

	// Contract-liveness check: if the test finds ZERO matched panels, either the
	// pattern regressed OR every panel was deleted — either is a regression the
	// test should flag. mdemg-llm-routing.json is the canonical panel; require
	// at least one match so the test stays meaningful.
	if matchedPanelsTotal == 0 {
		t.Fatal("no llm_interactions.error aggregate panels found — check dashboards or regex; test would be a silent no-op")
	}

	if len(violations) > 0 {
		t.Fatalf("GRAFANA-PANEL-FILTER-001 violations (%d):\n  - %s\n\nSee docs/development/grafana-panel-filter-001/sprint_plan_grafana_panel_filter_001.md",
			len(violations), strings.Join(violations, "\n  - "))
	}
}

// TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter — negative
// pin test. Constructs an inline "malformed" panel that reads
// llm_interactions.error WITHOUT the required filter, and asserts the
// same walking logic flags it. If this test passes but the positive test
// above ever passes on a broken JSON, the regex is wrong.
func TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter(t *testing.T) {
	badSQL := `SELECT task_name, SUM(CASE WHEN error IS NOT NULL AND length(error) > 0 THEN 1 ELSE 0 END) AS errors FROM llm_interactions GROUP BY 1`

	requiredExclusions := []string{"caller_canceled:"}
	errorAggregatePattern := regexp.MustCompile(`(?i)\berror\s*(is|!=|<>|not\s+like|like)\b`)
	readsLLMInteractions := regexp.MustCompile(`(?i)\bllm_interactions\b`)

	if !readsLLMInteractions.MatchString(badSQL) {
		t.Fatal("regex readsLLMInteractions failed to match the canonical bad SQL — regex is broken")
	}
	if !errorAggregatePattern.MatchString(badSQL) {
		t.Fatal("regex errorAggregatePattern failed to match the canonical bad SQL — regex is broken")
	}

	for _, needle := range requiredExclusions {
		clause := "NOT LIKE '" + needle + "%'"
		if strings.Contains(badSQL, clause) {
			t.Fatalf("canonical bad SQL unexpectedly contains %q — invalidates the negative pin", clause)
		}
	}
}
