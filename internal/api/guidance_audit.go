package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/jiminy"
	"mdemg/internal/jobhealth"
	"mdemg/internal/metrics"
	"mdemg/internal/tsdb"
)

// JIMINY-RELEVANCE-001 Epic 2 — guidance label-quality auto-relabel job.
//
// The Step-1 diagnostic (Finding 4) showed ~51% of guidance outcome labels were
// non-LLM heuristic defaults — noise a retrain would learn instead of the goal.
// This periodic, supervised job samples recent guidance_training_rows whose
// classifier_source is heuristic/blank, re-classifies them with the LLM
// OutcomeClassifier, and updates classifier_source/outcome_type/similarity IN
// PLACE so the evidence table becomes trustworthy over time. It also publishes
// the heuristic-default-share gauge so progress is observable (never silently
// leave it at 51%).
//
// This raises label quality on the EVIDENCE table; it is NOT a gold grade —
// human/auto GOLD grading is HITL-REVIEW-001's review_grades overlay.

// realLabelSources are the classifier sources we trust as genuine verdicts.
// A relabel only "upgrades" a row when the classifier produces one of these
// (i.e. the LLM/tier1 path actually ran) — if the classifier itself falls back
// to "heuristic" (LLM unavailable) we leave the row unchanged for a later run.
var realLabelSources = map[string]bool{"llm": true, "tier1": true, "explicit": true}

// shouldUpgradeLabel decides whether a re-classification result replaces the
// stored label. Pure + unit-tested: upgrade only when the OLD label was a
// heuristic/blank artifact AND the NEW verdict came from a real mechanism.
func shouldUpgradeLabel(oldSource, newSource string) bool {
	oldIsArtifact := oldSource == "" || oldSource == "heuristic"
	return oldIsArtifact && realLabelSources[newSource]
}

// guidanceAuditRow is one sampled evidence row needing re-labelling.
type guidanceAuditRow struct {
	rowID         string
	guidanceType  string
	guidanceText  string
	actionSummary string
	oldSource     string
}

// runGuidanceAudit samples up to sampleSize heuristic/blank rows, re-labels the
// ones the LLM classifier can genuinely verdict, and refreshes the
// heuristic-fraction gauge. Returns (sampled, upgraded, error).
func (s *Server) runGuidanceAudit(ctx context.Context, sampleSize int) (int, int, error) {
	if s.tsdbClient == nil || s.tsdbClient.Pool() == nil {
		return 0, 0, fmt.Errorf("guidance-audit: TSDB pool unavailable")
	}
	if s.jiminySvc == nil {
		return 0, 0, fmt.Errorf("guidance-audit: jiminy service unavailable")
	}
	classifier := s.jiminySvc.GetOutcomeClassifier()
	if classifier == nil {
		return 0, 0, fmt.Errorf("guidance-audit: outcome classifier unavailable (LLM classifier disabled)")
	}
	pool := s.tsdbClient.Pool()

	// Sample the most recent heuristic/blank-labelled rows.
	rows, err := pool.Query(ctx, `
		SELECT row_id, COALESCE(guidance_type,''), COALESCE(guidance_content,''),
		       COALESCE(action_summary,''), COALESCE(classifier_source,'')
		FROM guidance_training_rows
		WHERE classifier_source = '' OR classifier_source = 'heuristic'
		ORDER BY time DESC
		LIMIT $1`, sampleSize)
	if err != nil {
		return 0, 0, fmt.Errorf("guidance-audit: sample query: %w", err)
	}
	var sample []guidanceAuditRow
	for rows.Next() {
		var r guidanceAuditRow
		if err := rows.Scan(&r.rowID, &r.guidanceType, &r.guidanceText, &r.actionSummary, &r.oldSource); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("guidance-audit: scan: %w", err)
		}
		sample = append(sample, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("guidance-audit: row iteration: %w", err)
	}

	upgraded := 0
	for _, r := range sample {
		if r.guidanceText == "" {
			continue // nothing to classify
		}
		item := jiminy.GuidanceItem{
			Type:    jiminy.GuidanceType(r.guidanceType),
			Content: r.guidanceText,
		}
		cr := classifier.Classify(ctx, item, r.actionSummary)
		if !shouldUpgradeLabel(r.oldSource, cr.Source) {
			continue
		}
		if _, err := pool.Exec(ctx, `
			UPDATE guidance_training_rows
			SET outcome_type = $1, classifier_source = $2, similarity = $3
			WHERE row_id = $4`,
			string(cr.Outcome), cr.Source, cr.Confidence, r.rowID); err != nil {
			slog.Warn("guidance-audit: relabel update failed", "row_id", r.rowID, "error", err)
			continue
		}
		upgraded++
	}

	s.refreshHeuristicFractionGauge(ctx)
	return len(sample), upgraded, nil
}

// refreshHeuristicFractionGauge computes the share of recent rows still on a
// heuristic/blank label and publishes it. Best-effort; logs on failure.
func (s *Server) refreshHeuristicFractionGauge(ctx context.Context) {
	std := metrics.Metrics()
	if std == nil || std.GuidanceCorpusHeuristicFraction == nil {
		return
	}
	if s.tsdbClient == nil || s.tsdbClient.Pool() == nil {
		return
	}
	var total, heuristic int64
	err := s.tsdbClient.Pool().QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE classifier_source = '' OR classifier_source = 'heuristic')
		FROM guidance_training_rows
		WHERE time > NOW() - INTERVAL '30 days'`).Scan(&total, &heuristic)
	if err != nil {
		slog.Debug("guidance-audit: heuristic-fraction query failed", "error", err)
		return
	}
	frac := 0.0
	if total > 0 {
		frac = float64(heuristic) / float64(total)
	}
	std.GuidanceCorpusHeuristicFraction.Set(frac)
}

// StartGuidanceAudit launches the supervised periodic auto-relabel loop. Gated
// by GUIDANCE_AUDIT_ENABLED (checked by the caller). Reports each run to
// jobhealth as job_name='guidance-audit'.
func (s *Server) StartGuidanceAudit(interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	sampleSize := s.cfg.GuidanceAuditSampleSize
	initialDelay := time.Duration(s.cfg.GuidanceAuditInitialDelaySec) * time.Second
	s.goSupervised("guidance-audit", func(runCtx context.Context) error {
		slog.Info("guidance-audit scheduler started", "interval", interval, "sample_size", sampleSize, "initial_delay", initialDelay)
		// Initial run shortly after startup so the first cleanup doesn't wait a
		// full interval (skipped when GUIDANCE_AUDIT_INITIAL_DELAY_SEC=0).
		if initialDelay > 0 {
			select {
			case <-runCtx.Done():
				return nil
			case <-time.After(initialDelay):
				s.runGuidanceAuditReported(runCtx, sampleSize)
			}
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				s.runGuidanceAuditReported(runCtx, sampleSize)
			}
		}
	})
}

// runGuidanceAuditReported runs one audit pass and reports the outcome to
// jobhealth as job_name='guidance-audit' (success with sampled/upgraded
// metadata, or a high-severity failure alert).
func (s *Server) runGuidanceAuditReported(ctx context.Context, sampleSize int) {
	start := time.Now()
	sampled, upgraded, err := s.runGuidanceAudit(ctx, sampleSize)
	latencyMS := time.Since(start).Milliseconds()
	var pool *pgxpool.Pool
	if s.tsdbClient != nil {
		pool = s.tsdbClient.Pool()
	}
	if err != nil {
		slog.Warn("guidance-audit run failed", "error", err)
		jobhealth.Report(ctx, pool, s.alertDispatcher, tsdb.JobEventRow{
			JobName:      "guidance-audit",
			InstanceID:   s.cfg.InstanceID,
			Success:      false,
			LatencyMS:    latencyMS,
			ErrorMessage: err.Error(),
		})
		return
	}
	slog.Info("guidance-audit complete", "sampled", sampled, "upgraded", upgraded, "latency_ms", latencyMS)
	jobhealth.Report(ctx, pool, s.alertDispatcher, tsdb.JobEventRow{
		JobName:    "guidance-audit",
		InstanceID: s.cfg.InstanceID,
		Success:    true,
		LatencyMS:  latencyMS,
		Metadata:   map[string]any{"sampled": sampled, "upgraded": upgraded},
	})
}
