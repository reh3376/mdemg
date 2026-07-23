// FT-RECURSIVE-003 E5: the post-swap tripwire.
//
// The pre-swap canary (E4) catches structural breakage before production
// traffic; this loop catches what only real traffic reveals. For
// FT_LOOP_CANARY_WINDOW_MIN after a promotion, it watches the REAL LLM
// error rate (caller-cancellation-filtered — the LLM-HEALTH contract) and
// auto-rolls-back serving to the superseded version when the rate exceeds
// the threshold with enough call volume. One trip per window by
// construction: the restored version's deployed_at is old, so the window
// closes itself.
package ftloop

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/tsdb"
)

// TripwireConfig controls the post-swap watch.
type TripwireConfig struct {
	Enabled   bool          // FT_LOOP_TRIPWIRE_ENABLED
	Window    time.Duration // FT_LOOP_CANARY_WINDOW_MIN
	ErrorRate float64       // FT_LOOP_TRIPWIRE_ERROR_RATE — real-error fraction that trips
	MinCalls  int           // FT_LOOP_TRIPWIRE_MIN_CALLS — volume floor (sub-N transients never trip; the RSIC_LLM_ERROR_MIN_COUNT precedent)
	PollSec   int           // FT_LOOP_TRIPWIRE_POLL_SEC
	Serving   ServingConfig
}

// Tripwire is the supervised watcher.
type Tripwire struct {
	pool    *pgxpool.Pool
	cfg     TripwireConfig
	alertFn func(title, detail string) // high-severity alert hook (nil-safe)
	now     func() time.Time
}

// NewTripwire wires the watcher. alertFn may be nil; pool must be a concrete
// non-nil pool (the FT-RECURSIVE-002 typed-nil rule — callers guard).
func NewTripwire(pool *pgxpool.Pool, cfg TripwireConfig, alertFn func(title, detail string)) *Tripwire {
	return &Tripwire{pool: pool, cfg: cfg, alertFn: alertFn, now: time.Now}
}

// Run is the supervised loop (blocking; nil return = intentional completion
// per the SUPERVISOR-002 worker contract).
func (t *Tripwire) Run(ctx context.Context) error {
	if !t.cfg.Enabled {
		slog.Info("ft-loop tripwire disabled")
		return nil
	}
	poll := t.cfg.PollSec
	if poll <= 0 {
		poll = 60
	}
	tick := time.NewTicker(time.Duration(poll) * time.Second)
	defer tick.Stop()
	slog.Info("ft-loop tripwire started", "window", t.cfg.Window, "error_rate", t.cfg.ErrorRate, "min_calls", t.cfg.MinCalls)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			t.check(ctx)
		}
	}
}

// check runs one evaluation; exported-adjacent for the drill (CheckOnce).
func (t *Tripwire) check(ctx context.Context) {
	if t.pool == nil {
		return
	}
	active, err := tsdb.ActiveModelVersion(ctx, t.pool)
	if err != nil || active == nil {
		return
	}
	windowEnd := active.DeployedAt.Add(t.cfg.Window)
	if t.now().After(windowEnd) {
		return // outside the canary window — nothing to watch
	}
	prev, err := tsdb.PreviousModelVersion(ctx, t.pool)
	if err != nil || prev == nil {
		return // nothing to roll back to
	}

	total, realErrors, err := tsdb.LLMCallStatsSince(ctx, t.pool, active.DeployedAt)
	if err != nil {
		slog.Warn("ft-loop tripwire: stats query failed", "error", err)
		return
	}
	if total < int64(t.cfg.MinCalls) || total == 0 {
		return
	}
	rate := float64(realErrors) / float64(total)
	if rate < t.cfg.ErrorRate {
		return
	}

	detail := fmt.Sprintf("post-swap error rate %.1f%% (%d/%d real errors) within %s of promoting %s — rolling back to %s",
		rate*100, realErrors, total, t.cfg.Window, active.Version, prev.Version)
	slog.Error("ft-loop tripwire TRIPPED — auto-rollback", "detail", detail)

	res, swapErr := SwapServing(ctx, t.cfg.Serving, prev.ModelPath)
	if swapErr != nil {
		slog.Error("ft-loop tripwire: auto-rollback swap failed", "error", swapErr, "reverted", res.Reverted)
		if t.alertFn != nil {
			t.alertFn("FT tripwire rollback FAILED", detail+" — swap error: "+swapErr.Error())
		}
		return
	}
	_ = tsdb.MarkModelVersionStatus(ctx, t.pool, active.Version, tsdb.ModelVersionRolledBack)
	_ = tsdb.MarkModelVersionStatus(ctx, t.pool, prev.Version, tsdb.ModelVersionActive)
	if t.alertFn != nil {
		t.alertFn("FT tripwire auto-rollback executed", detail)
	}
}

// CheckOnce runs a single evaluation immediately (the live-drill surface).
func (t *Tripwire) CheckOnce(ctx context.Context) { t.check(ctx) }
