// AUTOGRADE-SCHEDULE-001 (2026-08-04): scheduled autograde loop.
//
// Follow-up disclosed by HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-
// QUALITY-001. Sprint B added the invariant-preserving auto-dismiss capability;
// sprint C tightened the source-side gate. This closes the arc: run the
// autograder periodically under supervisor so the queue self-drains for
// dismiss-eligible items and the `hitl-curation` alert no longer requires
// operator ceremony to clear.
//
// Design mirrors internal/ftloop/bench_schedule.go (SUPERVISOR-002 contract):
// nil return = intentional completion when disabled; supervised loop with
// initial delay + interval; spawns the `mdemg review autograde` CLI as a
// subprocess so ALL grade-writing logic (idempotency, sink dispatch, non-
// reinforcing apply, --force semantics) stays in one place.
//
// Default OFF: operators opt in when they want the queue-drain automation.
// Reinforcement invariant is preserved end-to-end because the CLI ALWAYS
// POSTs reinforce:false (grep-testable at internal/cli/review.go:407).
package review

import (
	"context"
	"fmt"
	"log/slog"
	"mdemg/internal/sanitize"
	"os/exec"
	"strings"
	"time"
)

// AutogradeScheduleConfig controls the scheduled loop.
type AutogradeScheduleConfig struct {
	Enabled         bool     // REVIEW_AUTOGRADE_SCHEDULE_ENABLED (default: false)
	IntervalHours   int      // REVIEW_AUTOGRADE_SCHEDULE_INTERVAL_HOURS (default: 6)
	InitialDelayMin int      // REVIEW_AUTOGRADE_SCHEDULE_INITIAL_DELAY_MIN (default: 15)
	Datasets        []string // REVIEW_AUTOGRADE_SCHEDULE_DATASETS (default: ["contradicted_drafts"])
	SpaceID         string   // REVIEW_AUTOGRADE_SCHEDULE_SPACE_ID (default: RSICWatchdogSpaceID)
	MinConfidence   float64  // REVIEW_AUTOGRADE_SCHEDULE_MIN_CONFIDENCE (default: 0.80)
	Limit           int      // REVIEW_AUTOGRADE_SCHEDULE_LIMIT (default: 50)
	MdemgBin        string   // resolved binary path (dockerbin/mdemgbin.Resolve equivalent)
	Endpoint        string   // http://127.0.0.1:PORT — where the CLI POSTs
}

// JobReportFn matches internal/jobhealth signature without importing (avoids
// cycle). Nil-safe.
type JobReportFn func(ctx context.Context, jobName string, success bool, latencyMs int64, errMsg string)

// AutogradeScheduler is the supervised worker.
type AutogradeScheduler struct {
	cfg    AutogradeScheduleConfig
	report JobReportFn // nil-safe
	now    func() time.Time
	runCmd func(ctx context.Context, name string, args []string) error // test seam
}

// NewAutogradeScheduler wires the runner.
func NewAutogradeScheduler(cfg AutogradeScheduleConfig, report JobReportFn) *AutogradeScheduler {
	s := &AutogradeScheduler{cfg: cfg, report: report, now: time.Now}
	s.runCmd = s.execCmd
	return s
}

// Run is the supervised loop. Nil return = intentional completion when
// disabled (SUPERVISOR-002 contract: no restart).
func (s *AutogradeScheduler) Run(ctx context.Context) error {
	if !s.cfg.Enabled {
		slog.Info("scheduled autograde disabled")
		return nil
	}
	if len(s.cfg.Datasets) == 0 {
		slog.Warn("scheduled autograde: no datasets configured — noop")
		return nil
	}
	if s.cfg.MdemgBin == "" {
		slog.Warn("scheduled autograde: no mdemg binary path resolved — noop")
		return nil
	}
	delay := time.Duration(s.cfg.InitialDelayMin) * time.Minute
	if delay <= 0 {
		delay = 15 * time.Minute
	}
	interval := time.Duration(s.intervalHours()) * time.Hour
	slog.Info("scheduled autograde started",
		"interval", interval, "initial_delay", delay,
		"datasets", s.cfg.Datasets, "space_id", s.cfg.SpaceID,
		"min_confidence", s.cfg.MinConfidence, "limit", s.cfg.Limit)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			s.runAllDatasets(ctx)
			timer.Reset(interval)
		}
	}
}

func (s *AutogradeScheduler) intervalHours() int {
	if s.cfg.IntervalHours <= 0 {
		return 6
	}
	return s.cfg.IntervalHours
}

// runAllDatasets iterates over configured datasets, running the autograder
// CLI for each. Failure on one dataset does NOT skip the others (the queue
// on dataset A might legitimately drain while dataset B is broken).
// Reports jobhealth per dataset run.
func (s *AutogradeScheduler) runAllDatasets(ctx context.Context) {
	for _, ds := range s.cfg.Datasets {
		if ctx.Err() != nil {
			return
		}
		start := s.now()
		runErr := s.runOne(ctx, ds)
		if s.report != nil {
			msg := ""
			if runErr != nil {
				msg = runErr.Error()
			}
			// jobName carries the dataset for operator debuggability; the caller
			// uses a distinct service label so per-dataset failures don't share
			// cooldown with each other (NOSILENT-001 contract).
			s.report(ctx, "scheduled-autograde:"+ds, runErr == nil,
				s.now().Sub(start).Milliseconds(), msg)
		}
		if runErr != nil {
			slog.Error("scheduled autograde: dataset run failed", "dataset", ds, "error", runErr)
			continue
		}
		slog.Info("scheduled autograde: dataset run complete",
			"dataset", ds, "took", s.now().Sub(start).Round(time.Second))
	}
}

// runOne executes `mdemg review autograde --dataset X --space-id Y` under
// a bounded context. NOT --force: the scheduled loop is for organic accumulation,
// not backfill after sink-logic changes. Operators use the CLI with --force
// for that (rare, deliberate).
func (s *AutogradeScheduler) runOne(ctx context.Context, dataset string) error {
	limit := s.cfg.Limit
	if limit <= 0 {
		limit = 50
	}
	minConf := s.cfg.MinConfidence
	if minConf <= 0 {
		minConf = 0.80
	}
	args := []string{"review", "autograde",
		"--dataset", dataset,
		"--space-id", s.cfg.SpaceID,
		"--min-confidence", fmt.Sprintf("%.2f", minConf),
		"--limit", fmt.Sprintf("%d", limit),
	}
	if s.cfg.Endpoint != "" {
		args = append(args, "--endpoint", s.cfg.Endpoint)
	}
	// A whole autograde pass shouldn't need more than 30 min even with N=50
	// items at ~10s/grade on the local LLM. Bound conservatively.
	rctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	return s.runCmd(rctx, s.cfg.MdemgBin, args)
}

func (s *AutogradeScheduler) execCmd(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: bin path + args config-constructed
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := sanitize.TailRuneSafe(string(out), 400)
		return fmt.Errorf("%w (output tail: %s)", err, strings.TrimSpace(tail))
	}
	return nil
}
