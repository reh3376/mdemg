// The recursive-retrain controller (FT-RECURSIVE-002 Phase 6b, Epic 5).
//
// A supervised loop that picks up a `triggered` cycle from the ft_training_cycles
// ledger and walks it through curate → train → gate as supervised Python
// subprocesses, holding the compute lease + quiescing RSIC for the duration,
// updating the ledger + ft-loop:<stage> jobhealth at each step. It STOPS at
// promote_pending — promotion is operator-confirm (Phase 6b; auto-promote/canary
// are Phase 7). Default-off: Run returns immediately unless FT_LOOP_ENABLED.
package ftloop

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/alert"
	"mdemg/internal/jobhealth"
	"mdemg/internal/tsdb"
)

// Quiescer is the RSIC-pause contract (implemented by *ape.OrchestrationPolicy).
type Quiescer interface {
	Quiesce(until time.Time)
}

// ControllerConfig carries the controller's tunables (from config).
type ControllerConfig struct {
	Enabled         bool
	PollInterval    time.Duration
	LeasePath       string
	LeaseMax        time.Duration
	MinFreeDiskGB   float64
	PythonBin       string
	ModelVersion    string
	EpochsCap       int
	EarlyStopFactor float64
	RepoDir         string // working dir for `python -m training.X` subprocesses
	InstanceID      string
	SpaceID         string

	// Epic-6 pipeline wiring (the proven curate→train→convert→gate commands).
	WorkDir         string // per-cycle artifact root
	BaseModel       string // dense base model dir (relative to RepoDir or absolute)
	BaseSHA         string // base config.json SHA pin (train_ft --expected-sha256)
	UaitsSpec       string // paradigm_router curation spec (relative to RepoDir)
	BenchmarkConfig string // gate benchmark config yaml (relative to RepoDir)
	LoraRank        int
	LoraAlpha       int
	GatePort        int     // side-port for serving the candidate during the gate
	ExportSinceDays int     // export window for the curate input
	GateTaskFilter  string  // optional run_benchmark --task-filter
	GateMinScore    float64 // minimum aggregate benchmark score to PASS
	MdemgBin        string  // path to the mdemg binary (for `mdemg data export`)
}

// Controller orchestrates a recursive-retrain cycle.
type Controller struct {
	pool     *pgxpool.Pool
	quiescer Quiescer
	disp     *alert.Dispatcher
	cfg      ControllerConfig
	now      func() time.Time
	// runCmd runs one subprocess (bin + args in dir) and returns its combined
	// output; overridable in tests. The production impl is execCmd.
	runCmd func(ctx context.Context, label, dir, bin string, args []string) (string, error)
}

// NewController builds a controller. quiescer may be nil (no RSIC quiesce).
func NewController(pool *pgxpool.Pool, quiescer Quiescer, disp *alert.Dispatcher, cfg ControllerConfig) *Controller {
	c := &Controller{pool: pool, quiescer: quiescer, disp: disp, cfg: cfg, now: time.Now}
	c.runCmd = c.execCmd
	return c
}

// record writes a ledger event, guarding the concrete-nil pool (a nil
// *pgxpool.Pool becomes a non-nil interface, so RecordCycleEvent's own nil
// check would not catch it — the typed-nil gotcha).
func (c *Controller) record(ctx context.Context, ev tsdb.FtCycleEvent) {
	if c.pool == nil {
		return
	}
	_ = tsdb.RecordCycleEvent(ctx, c.pool, ev)
}

// Run is the supervised loop body (supervisor contract: blocking func(ctx) error;
// nil return = intentional completion, no restart). Dormant unless enabled.
func (c *Controller) Run(ctx context.Context) error {
	if c == nil || !c.cfg.Enabled {
		return nil // dormant — intentional completion
	}
	slog.Info("ft-loop controller started", "poll_sec", c.cfg.PollInterval.Seconds())
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick picks up a freshly-triggered cycle (if any) and runs it.
func (c *Controller) tick(ctx context.Context) {
	open, err := tsdb.OpenCycle(ctx, c.pool)
	if err != nil {
		slog.Warn("ft-loop controller: ledger query failed", "error", err)
		return
	}
	// Only act on a freshly-triggered cycle. A cycle already in curating/
	// training/gating means a run is in flight (this process, or a crashed one
	// the lease will let us reclaim on its expiry); promote_pending awaits the
	// operator.
	if open == nil || open.Status != tsdb.FtCycleTriggered {
		return
	}
	c.runCycle(ctx, open.CycleID, open.ModelVersion)
}

// runCycle drives one cycle through the pipeline, holding the lease + quiesce.
func (c *Controller) runCycle(ctx context.Context, cycleID, modelVersion string) {
	// Preflight: disk floor (a training run needs ~85 GB transient disk).
	if c.cfg.MinFreeDiskGB > 0 {
		if free, err := FreeDiskGB(c.cfg.RepoDir); err == nil && free < c.cfg.MinFreeDiskGB {
			c.failCycle(ctx, cycleID, modelVersion, "disk_floor",
				fmt.Sprintf("free disk %.1f GB < floor %.1f GB", free, c.cfg.MinFreeDiskGB), true)
			return
		}
	}

	// Compute lease (single-host exclusive). A non-expired lease means another
	// holder — skip this tick.
	lease, err := AcquireLease(c.cfg.LeasePath, cycleID, c.cfg.LeaseMax, c.now())
	if err != nil {
		slog.Info("ft-loop controller: lease unavailable, skipping", "error", err)
		return
	}
	defer func() { _ = lease.Release() }()

	// Quiesce RSIC for the lease window.
	if c.quiescer != nil {
		c.quiescer.Quiesce(lease.ExpiresAt())
		defer c.quiescer.Quiesce(time.Time{}) // clear on exit
	}

	// Per-cycle artifact workspace.
	work := filepath.Join(c.workRoot(), cycleID)

	// The pipeline: export → curate → train → convert → gate. Each stage
	// records the ledger + ft-loop:<stage> jobhealth and threads its artifact
	// to the next. The lease-expiry guard runs before each (class-4 halt).
	type stage struct {
		status tsdb.FtCycleStatus
		name   string
		run    func() error
	}
	var inputDir, versionedDir, adapterDir, candidate string
	stages := []stage{
		{tsdb.FtCycleCurating, "export", func() (err error) { inputDir, err = c.stageExport(ctx, work); return }},
		{tsdb.FtCycleCurating, "curate", func() (err error) { versionedDir, err = c.stageCurate(ctx, cycleID, work, inputDir); return }},
		{tsdb.FtCycleTraining, "train", func() (err error) { adapterDir, err = c.stageTrain(ctx, work, versionedDir); return }},
		{tsdb.FtCycleGating, "convert", func() (err error) { candidate, err = c.stageConvert(ctx, work, adapterDir); return }},
		{tsdb.FtCycleGating, "gate", func() error { return c.stageGate(ctx, work, candidate) }},
	}

	for _, st := range stages {
		if lease.Expired(c.now()) {
			c.failCycle(ctx, cycleID, modelVersion, "lease_expired", "compute lease expired mid-cycle", true)
			return
		}
		c.record(ctx, tsdb.FtCycleEvent{CycleID: cycleID, ModelVersion: modelVersion, Status: st.status, Stage: st.name})
		started := c.now()
		runErr := st.run()
		c.reportStage(ctx, cycleID, st.name, c.now().Sub(started), runErr)
		if runErr != nil {
			c.failCycle(ctx, cycleID, modelVersion, st.name+"_failed", runErr.Error(), false)
			return
		}
	}

	// PASS → promote_pending (operator confirms; auto-promote is Phase 7). The
	// candidate path is recorded so `mdemg ft-loop promote` knows what to swap.
	c.record(ctx, tsdb.FtCycleEvent{
		CycleID: cycleID, ModelVersion: modelVersion, Status: tsdb.FtCyclePromotePending, Stage: "gate",
		EvalResults: map[string]any{"candidate_gguf": candidate},
	})
	slog.Info("ft-loop cycle reached promote_pending — operator confirm required",
		"cycle_id", cycleID, "model_version", modelVersion, "candidate", candidate)
}

// failCycle records the terminal failed state + one alert (distinct ft-loop
// service). class4 marks the alert-and-halt severity cases (disk/lease).
func (c *Controller) failCycle(ctx context.Context, cycleID, modelVersion, reason, detail string, class4 bool) {
	c.record(ctx, tsdb.FtCycleEvent{
		CycleID: cycleID, ModelVersion: modelVersion, Status: tsdb.FtCycleFailed,
		Stage: reason, Error: detail,
	})
	sev := alert.SeverityMedium
	if class4 {
		sev = alert.SeverityHigh
	}
	if c.disp != nil {
		c.disp.SendAlert(ctx, "ft-loop",
			"FT retrain cycle failed: "+reason,
			fmt.Sprintf("cycle=%s: %s", cycleID, detail), sev)
	}
	slog.Warn("ft-loop cycle failed", "cycle_id", cycleID, "reason", reason, "detail", detail)
}

// reportStage records a ft-loop:<stage> jobhealth row (+ failure alert) — the
// same surface the Phase-6a manual `mdemg ft-loop report-stage` writes, now
// driven automatically by the controller.
func (c *Controller) reportStage(ctx context.Context, cycleID, stage string, dur time.Duration, runErr error) {
	ev := tsdb.JobEventRow{
		JobName:    "ft-loop:" + stage,
		SpaceID:    "mdemg-dev",
		InstanceID: c.cfg.InstanceID,
		Success:    runErr == nil,
		LatencyMS:  dur.Milliseconds(),
		Metadata:   map[string]any{"stage": stage, "cycle_id": cycleID},
	}
	if runErr != nil {
		ev.ErrorMessage = runErr.Error()
	}
	jobhealth.ReportWithService(ctx, c.pool, c.disp, ev, "ft-loop")
}

func tail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return "…" + string(b[len(b)-n:])
}
