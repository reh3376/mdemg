// Package ftloop implements the recursive-retraining actuator (FT-RECURSIVE-002,
// Phase 6b). This file is the TRIGGER GATE: it decides, from the persistent
// ft_training_cycles ledger + config, whether a readiness signal should launch a
// new cycle — and, crucially, suppresses the per-cycle re-fire (SF-2) that made
// rsic-trigger_training_pipeline spam every ~5 min.
//
// Per SPEC §5 fork 7 the trigger action stays diagnostic; the controller
// consumes the decision out-of-band via the ledger. The gate is the single
// decision point both the RSIC executor (suppress/alert) and the controller
// (Epic 5) consult.
package ftloop

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nrednav/cuid2"

	"mdemg/internal/tsdb"
)

// Decision is the trigger-gate verdict.
type Decision string

const (
	// DecideSuppressOpenCycle — a cycle is already in flight (single-flight).
	DecideSuppressOpenCycle Decision = "suppress_open_cycle"
	// DecideSuppressInterval — a cycle started within the retrain interval.
	DecideSuppressInterval Decision = "suppress_interval"
	// DecideSuppressDisabled — the actuator is off (FT_LOOP_ENABLED=false);
	// readiness stays observable via `mdemg data status` + the SF-1 heartbeat.
	DecideSuppressDisabled Decision = "suppress_disabled"
	// DecideSuppressNotFresh — not enough new signal since the last cycle.
	DecideSuppressNotFresh Decision = "suppress_not_fresh"
	// DecideTrigger — open a new cycle.
	DecideTrigger Decision = "trigger"
)

// IsSuppressed reports whether the decision means "do not alert, do not trigger".
func (d Decision) IsSuppressed() bool { return d != DecideTrigger }

// Decide is the pure trigger-gate logic (no I/O — unit-testable). Order matters:
// single-flight and interval bar a trigger regardless of enablement; the disabled
// gate suppresses the alert spam; freshness is the last gate before triggering.
//
//   - hasOpenCycle: a non-terminal cycle exists in the ledger.
//   - hasLast / lastCycleAge: the most recent cycle's start + its age.
//   - freshFraction / minFresh: fraction of interactions newer than the last
//     cycle vs the minimum required. On the FIRST ever cycle (hasLast=false)
//     the freshness gate is skipped (there is nothing to be "fresh" relative to).
func Decide(enabled, hasOpenCycle, hasLast bool, lastCycleAge, interval time.Duration, freshFraction, minFresh float64) Decision {
	if hasOpenCycle {
		return DecideSuppressOpenCycle
	}
	if hasLast && lastCycleAge < interval {
		return DecideSuppressInterval
	}
	if !enabled {
		return DecideSuppressDisabled
	}
	if hasLast && freshFraction < minFresh {
		return DecideSuppressNotFresh
	}
	return DecideTrigger
}

// GateConfig carries the gate's tunables (from config).
type GateConfig struct {
	Enabled          bool
	RetrainInterval  time.Duration
	MinFreshFraction float64
	ModelVersion     string // recorded on the opened cycle
}

// Gate consults the ledger + config to produce a Decision.
type Gate struct {
	pool *pgxpool.Pool
	cfg  GateConfig
	now  func() time.Time
}

// NewGate builds a Gate. A nil pool treats the ledger as empty (safe default).
func NewGate(pool *pgxpool.Pool, cfg GateConfig) *Gate {
	return &Gate{pool: pool, cfg: cfg, now: time.Now}
}

// EvaluateTrigger adapts Evaluate to the consumer-side string+bool contract
// (so the RSIC dispatcher consumes the gate without importing this package).
// On a trigger decision it OPENS a cycle in the ledger — the controller (a
// separate supervised loop) consumes it out-of-band (SPEC §5 fork 7). A failed
// open downgrades to suppressed so the executor does not falsely alert.
func (g *Gate) EvaluateTrigger(ctx context.Context) (decision string, suppressed bool, err error) {
	d, e := g.Evaluate(ctx)
	if e != nil {
		return string(d), d.IsSuppressed(), e
	}
	if d == DecideTrigger {
		if oerr := g.OpenCycle(ctx); oerr != nil {
			return string(DecideSuppressOpenCycle), true, oerr
		}
	}
	return string(d), d.IsSuppressed(), nil
}

// OpenCycle writes a fresh `triggered` cycle row (CUIDv2 id) for the controller
// to pick up.
func (g *Gate) OpenCycle(ctx context.Context) error {
	return tsdb.RecordCycleEvent(ctx, g.pool, tsdb.FtCycleEvent{
		CycleID:      cuid2.Generate(),
		ModelVersion: g.cfg.ModelVersion,
		Status:       tsdb.FtCycleTriggered,
		Stage:        "triggered",
	})
}

// Evaluate runs the full gate against the live ledger.
func (g *Gate) Evaluate(ctx context.Context) (Decision, error) {
	if g == nil {
		return DecideSuppressDisabled, nil
	}
	open, err := tsdb.OpenCycle(ctx, g.pool)
	if err != nil {
		return DecideSuppressOpenCycle, err // fail safe: treat query failure as "busy"
	}
	hasOpen := open != nil

	lastStart, hasLast, err := tsdb.LastCycleStart(ctx, g.pool)
	if err != nil {
		return DecideSuppressInterval, err
	}
	var lastAge time.Duration
	if hasLast {
		lastAge = g.now().Sub(lastStart)
	}

	// Freshness only matters on the trigger path (enabled + not suppressed by
	// open/interval) and only when there is a prior cycle to be fresh against.
	freshFraction := 1.0
	if g.cfg.Enabled && !hasOpen && hasLast && lastAge >= g.cfg.RetrainInterval {
		freshFraction, err = tsdb.FreshInteractionFraction(ctx, g.pool, lastStart)
		if err != nil {
			return DecideSuppressNotFresh, err
		}
	}

	return Decide(g.cfg.Enabled, hasOpen, hasLast, lastAge, g.cfg.RetrainInterval,
		freshFraction, g.cfg.MinFreshFraction), nil
}
