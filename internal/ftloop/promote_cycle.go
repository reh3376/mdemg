// FT-RECURSIVE-003 E6: the ONE promotion flow, shared by the operator CLI
// (`ft-loop promote`) and the controller's auto-promote path.
//
// [AMD-6] resolution (single-actor decision): promotion executes ONLY here —
// the controller polls promote_pending + the auto-promote policy; the RSIC
// dispatcher does NOT get a second promotion executor (a dual-actor race
// against the ledger's single-flight would be strictly worse). The RSIC
// action-class taxonomy still classifies promotion as a reversible
// class-3 mutation whose snapshot IS the superseded ft_model_versions row —
// restorable via SwapServing, drilled live in E3/E5.
package ftloop

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/tsdb"
)

// PromotionConfig carries everything the flow needs (mirrored from config
// by the caller — the ControllerConfig pattern).
type PromotionConfig struct {
	Serving       ServingConfig
	CanaryEnabled bool
	CanaryProbes  string // absolute or repo-relative
	CanaryCount   int
	CanaryProdURL string
	GatePort      int
	RepoDir       string
	BaseModel     string
	ModelVersion  string
}

// PromoteCycle runs canary (optional) → fail-closed swap → ledger + version
// records for a promote_pending cycle. decidedBy is "operator" or "auto" —
// recorded on the ledger event. Returns the swap result on success.
func PromoteCycle(ctx context.Context, pool *pgxpool.Pool, cfg PromotionConfig, cycleID, reason, decidedBy string) (SwapResult, error) {
	var zero SwapResult
	candidate, gateScore, err := tsdb.CycleCandidatePath(ctx, pool, cycleID)
	if err != nil {
		return zero, fmt.Errorf("read candidate path: %w", err)
	}
	if candidate == "" {
		return zero, fmt.Errorf("cycle %s has no candidate_gguf recorded", cycleID)
	}

	recEvent := func(status tsdb.FtCycleStatus, stage, errText string, extra map[string]any) {
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ev := tsdb.FtCycleEvent{
			CycleID: cycleID, ModelVersion: cfg.ModelVersion,
			Status: status, Stage: stage, Error: errText,
			EvalResults: map[string]any{"operator_decision": decidedBy, "reason": reason},
		}
		for k, v := range extra {
			ev.EvalResults[k] = v
		}
		if rerr := tsdb.RecordCycleEvent(rctx, pool, ev); rerr != nil {
			slog.Warn("promotion: ledger record failed", "cycle", cycleID, "status", status, "error", rerr)
		}
	}

	// Pre-swap canary: divergence or a candidate that won't serve blocks the
	// promotion with ZERO production restarts.
	if cfg.CanaryEnabled {
		probes := cfg.CanaryProbes
		if probes != "" && !filepath.IsAbs(probes) {
			probes = filepath.Join(cfg.RepoDir, probes)
		}
		canCtx, cancelCan := context.WithTimeout(ctx, 20*time.Minute)
		stop, serr := StartCandidateServer(canCtx, cfg.RepoDir, candidate, cfg.GatePort)
		if serr != nil {
			cancelCan()
			recEvent(tsdb.FtCycleRolledBack, "canary_failed", "candidate would not serve: "+serr.Error(), nil)
			return zero, fmt.Errorf("canary: candidate would not serve (production untouched): %w", serr)
		}
		canRes, cerr := RunCanary(canCtx, CanaryConfig{
			ProbesPath:  probes,
			ProbeCount:  cfg.CanaryCount,
			ProdBaseURL: cfg.CanaryProdURL,
			CandBaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", cfg.GatePort),
		})
		stop()
		cancelCan()
		if cerr != nil {
			return zero, fmt.Errorf("canary replay failed (promotion aborted, production untouched): %w", cerr)
		}
		if !canRes.Pass() {
			recEvent(tsdb.FtCycleRolledBack, "canary_failed",
				strings.Join(canRes.Divergences, "; "),
				map[string]any{"canary_probes": canRes.Probes})
			return zero, fmt.Errorf("canary DIVERGED (%d/%d probes; production untouched): %s",
				len(canRes.Divergences), canRes.Probes, strings.Join(canRes.Divergences, "; "))
		}
		slog.Info("promotion canary passed", "cycle", cycleID, "probes", canRes.Probes)
	}

	swapCtx, cancelSwap := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelSwap()
	res, swapErr := SwapServing(swapCtx, cfg.Serving, candidate)
	if swapErr != nil {
		recEvent(tsdb.FtCycleRolledBack, "promote_failed", swapErr.Error(),
			map[string]any{"swap_reverted": res.Reverted})
		return res, fmt.Errorf("promotion swap failed (serving restored=%v): %w", res.Reverted, swapErr)
	}

	recEvent(tsdb.FtCyclePromoted, "promote_"+decidedBy, "",
		map[string]any{"swap_previous": res.Previous, "swap_target": res.Target})

	// Version rows (fresh ctx — the swap may have taken minutes).
	rctx, cancelRec := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRec()
	if active, aerr := tsdb.ActiveModelVersion(rctx, pool); aerr == nil && active != nil && active.ModelPath != res.Target {
		_ = tsdb.MarkModelVersionStatus(rctx, pool, active.Version, tsdb.ModelVersionSuperseded)
	}
	shortCycle := cycleID
	if len(shortCycle) > 8 {
		shortCycle = shortCycle[:8]
	}
	if err := tsdb.RecordModelVersion(rctx, pool, tsdb.ModelVersionRow{
		Version: cfg.ModelVersion + "-" + shortCycle, ModelPath: res.Target,
		BaseModel: cfg.BaseModel, TrainingCycle: cycleID,
		OverallScore: gateScore, Status: tsdb.ModelVersionActive,
		Notes: "promoted (" + decidedBy + ")",
	}); err != nil {
		slog.Warn("promotion: ft_model_versions record failed", "error", err)
	}
	slog.Info("cycle promoted", "cycle", cycleID, "decided_by", decidedBy,
		"target", res.Target, "gate_score", gateScore)
	return res, nil
}
