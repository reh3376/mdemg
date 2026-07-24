// FT-RECURSIVE-004 E4: the scheduled benchmark runner.
//
// `ft_benchmark_stale` (FT-BENCH-REFRESH-001) nags when benchmark_runs goes
// stale; this loop removes the operator toil by running the refresh recipe
// on a schedule — which also feeds `ft_production_drift` (E2) with fresh
// aggregates. Default OFF: a benchmark run saturates llama-server for
// ~12 minutes, so the operator chooses to enable it (and the interval).
package ftloop

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BenchScheduleConfig controls the scheduled benchmark.
type BenchScheduleConfig struct {
	Enabled         bool   // FT_BENCH_SCHEDULE_ENABLED
	IntervalDays    int    // FT_BENCH_SCHEDULE_DAYS (default 7 — matches the staleness rule)
	InitialDelayMin int    // FT_BENCH_SCHEDULE_INITIAL_DELAY_MIN (default 30 — never at boot)
	RepoDir         string
	PythonBin       string // resolved venv python (resolvePython pattern)
	ConfigYAML      string // benchmark config (repo-relative ok)
	BaseURL         string // production OpenAI-compat base
	ModelName       string
	RowsPerSpec     int // FT_BENCH_SCHEDULE_ROWS_PER_SPEC (default 5 — the refresh scope, not the full run)
	TimeoutS        int // per-call LLM timeout passed through (default 300)
}

// BenchScheduler is the supervised worker.
type BenchScheduler struct {
	pool   *pgxpool.Pool
	cfg    BenchScheduleConfig
	report JobReporter // nil-safe
	now    func() time.Time
	runCmd func(ctx context.Context, name string, args []string) error // test seam
}

// NewBenchScheduler wires the runner.
func NewBenchScheduler(pool *pgxpool.Pool, cfg BenchScheduleConfig, report JobReporter) *BenchScheduler {
	b := &BenchScheduler{pool: pool, cfg: cfg, report: report, now: time.Now}
	b.runCmd = b.execCmd
	return b
}

// Run is the supervised loop (SUPERVISOR-002 contract: nil = intentional
// completion when disabled).
func (b *BenchScheduler) Run(ctx context.Context) error {
	if !b.cfg.Enabled {
		slog.Info("scheduled ft-benchmark disabled")
		return nil
	}
	delay := time.Duration(b.cfg.InitialDelayMin) * time.Minute
	if delay <= 0 {
		delay = 30 * time.Minute
	}
	slog.Info("scheduled ft-benchmark started", "interval_days", b.intervalDays(), "initial_delay", delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			b.maybeRun(ctx)
			timer.Reset(6 * time.Hour) // re-check cadence; the due-date math gates actual runs
		}
	}
}

func (b *BenchScheduler) intervalDays() int {
	if b.cfg.IntervalDays <= 0 {
		return 7
	}
	return b.cfg.IntervalDays
}

// due reports whether the latest benchmark is older than the interval.
func (b *BenchScheduler) due(ctx context.Context) (bool, error) {
	if b.pool == nil {
		return false, fmt.Errorf("no pool")
	}
	row := b.pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MAX(completed_at))) / 86400.0, 999.0)
		FROM benchmark_runs WHERE completed_at IS NOT NULL`)
	var ageDays float64
	if err := row.Scan(&ageDays); err != nil {
		return false, err
	}
	return ageDays >= float64(b.intervalDays()), nil
}

// maybeRun executes the refresh when due, reporting jobhealth either way it
// ACTS (due-and-ran / due-and-failed; not-due is silent).
func (b *BenchScheduler) maybeRun(ctx context.Context) {
	isDue, err := b.due(ctx)
	if err != nil {
		slog.Warn("scheduled ft-benchmark: due-check failed", "error", err)
		return
	}
	if !isDue {
		return
	}
	start := b.now()
	runErr := b.runOnce(ctx)
	if b.report != nil {
		msg := ""
		if runErr != nil {
			msg = runErr.Error()
		}
		b.report(ctx, "scheduled-ft-benchmark", runErr == nil, b.now().Sub(start).Milliseconds(), msg)
	}
	if runErr != nil {
		slog.Error("scheduled ft-benchmark: run failed", "error", runErr)
		return
	}
	slog.Info("scheduled ft-benchmark: refresh complete", "took", b.now().Sub(start).Round(time.Second))
}

// runOnce executes the FT-BENCH-REFRESH-001 recipe (--apply-tsdb lands the
// rows; the sidecar file remains the audit/recovery artifact).
func (b *BenchScheduler) runOnce(ctx context.Context) error {
	rows := b.cfg.RowsPerSpec
	if rows <= 0 {
		rows = 5
	}
	timeoutS := b.cfg.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 300
	}
	out := filepath.Join(b.cfg.RepoDir, "training_data", "eval",
		fmt.Sprintf("benchmark_scheduled_%d.json", b.now().Unix()))
	cfgPath := b.cfg.ConfigYAML
	if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(b.cfg.RepoDir, cfgPath)
	}
	args := []string{"-m", "neural.benchmarks.run_benchmark",
		"--config", cfgPath,
		"--mlx-base-url", b.cfg.BaseURL,
		"--mlx-model-name", b.cfg.ModelName,
		"--rows-per-spec", fmt.Sprintf("%d", rows),
		"--n-runs", "1",
		"--mlx-timeout-s", fmt.Sprintf("%d", timeoutS),
		"--apply-tsdb",
		"--out", out}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	return b.runCmd(rctx, b.cfg.PythonBin, args)
}

func (b *BenchScheduler) execCmd(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: config-constructed
	cmd.Dir = b.cfg.RepoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		return fmt.Errorf("%w (output tail: %s)", err, strings.TrimSpace(tail))
	}
	return nil
}
