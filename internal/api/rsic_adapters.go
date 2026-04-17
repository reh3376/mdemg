package api

import (
	"context"
	"net/http"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/alert"
	"mdemg/internal/ape"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/hidden"
	"mdemg/internal/jiminy"
	"mdemg/internal/learning"
	"mdemg/internal/models"
	"mdemg/internal/retrieval"
)

// constraintGateAdapter adapts consulting.ConstraintClassifier to conversation.ConstraintGateClassifier.
// This adapter bridges the consulting and conversation packages without creating a circular import.
type constraintGateAdapter struct {
	cc *consulting.ConstraintClassifier
}

func (a *constraintGateAdapter) Classify(ctx context.Context, nodeID, text string) (*conversation.ConstraintGateResult, error) {
	result, err := a.cc.Classify(ctx, nodeID, text)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &conversation.ConstraintGateResult{Type: result.Type}, nil
}

// loadExistingConstraintCodes queries Neo4j for all existing constraint_code values
// to populate the code generator's collision avoidance set on startup.
func loadExistingConstraintCodes(ctx context.Context, driver neo4j.DriverWithContext) []string {
	if driver == nil {
		return nil
	}
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode)
			WHERE n.constraint_code IS NOT NULL AND n.constraint_code <> ''
			RETURN DISTINCT n.constraint_code AS code
		`
		res, err := tx.Run(ctx, cypher, nil)
		if err != nil {
			return nil, err
		}
		var codes []string
		for res.Next(ctx) {
			record := res.Record()
			if code, ok := record.Get("code"); ok {
				if codeStr, ok := code.(string); ok {
					codes = append(codes, codeStr)
				}
			}
		}
		return codes, res.Err()
	})
	if err != nil {
		return nil
	}
	codes, _ := result.([]string)
	return codes
}

// rsicLearningAdapter adapts *learning.Service to ape.LearningStatsProvider.
type rsicLearningAdapter struct {
	svc *learning.Service
}

func (a *rsicLearningAdapter) GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error) {
	return a.svc.GetLearningEdgeStats(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneDecayedEdges(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneExcessEdgesPerNode(ctx, spaceID)
}

// rsicConvAdapter adapts *conversation.ContextCooler to ape.ConversationStatsProvider.
type rsicConvAdapter struct {
	cooler *conversation.ContextCooler
}

func (a *rsicConvAdapter) GetVolatileStats(ctx context.Context, spaceID string) (ape.VolatileStatsResult, error) {
	if a == nil || a.cooler == nil {
		return ape.VolatileStatsResult{}, nil
	}
	stats, err := a.cooler.GetVolatileStats(ctx, spaceID)
	if err != nil {
		return ape.VolatileStatsResult{}, err
	}
	return ape.VolatileStatsResult{
		VolatileCount:        stats.VolatileCount,
		PermanentCount:       stats.PermanentCount,
		AvgVolatileStability: stats.AvgVolatileStability,
	}, nil
}

// rsicHiddenAdapter adapts *hidden.Service to ape.HiddenLayerProvider.
type rsicHiddenAdapter struct {
	svc *hidden.Service
}

func (a *rsicHiddenAdapter) RunFullConversationConsolidation(ctx context.Context, spaceID string) (any, error) {
	return a.svc.RunFullConversationConsolidation(ctx, spaceID)
}

// rsicWatchdogSignalAdapter implements ape.WatchdogSignalProvider
// by wrapping SessionTracker and conversation.Service.
type rsicWatchdogSignalAdapter struct {
	sessionTracker *conversation.SessionTracker
	driver         neo4j.DriverWithContext
	jiminyEnabled  bool
	jiminySvc      *jiminy.Service
	sidecarURL     string
}

func (a *rsicWatchdogSignalAdapter) GetSessionHealthScore(sessionID string) float64 {
	if a.sessionTracker == nil {
		return 0
	}
	// Phase 89: Aggregate across all sessions when sessionID is empty
	if sessionID == "" {
		states := a.sessionTracker.GetAllStates()
		if len(states) == 0 {
			return 0
		}
		var total float64
		for _, s := range states {
			total += s.HealthScore()
		}
		return total / float64(len(states))
	}
	state := a.sessionTracker.GetState(sessionID)
	if state == nil {
		return 0
	}
	return state.HealthScore()
}

func (a *rsicWatchdogSignalAdapter) GetObservationRate(spaceID string) float64 {
	if a.sessionTracker == nil {
		return 0
	}
	// Phase 89: Aggregate across all active sessions instead of hard-coded "claude-core"
	sessions := a.sessionTracker.GetAllStates()
	var totalObs int
	var maxElapsed float64
	for _, state := range sessions {
		totalObs += state.ObservationsSinceResume
		elapsed := time.Since(state.CreatedAt).Hours()
		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
	}
	if maxElapsed < 0.01 {
		return 0
	}
	return float64(totalObs) / maxElapsed
}

func (a *rsicWatchdogSignalAdapter) GetConsolidationAgeSec(ctx context.Context, spaceID string) (int64, error) {
	sess := a.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
            MATCH (n:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})
            RETURN max(n.created_at) AS last_consolidation
        `, map[string]any{"spaceId": spaceID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("last_consolidation"); ok && v != nil {
				// Phase 89: Handle all Neo4j datetime types
				switch tv := v.(type) {
				case time.Time:
					return int64(time.Since(tv).Seconds()), nil
				case int64:
					return tv, nil
				case float64:
					return int64(tv), nil
				case string:
					if t, err := time.Parse(time.RFC3339, tv); err == nil {
						return int64(time.Since(t).Seconds()), nil
					}
				}
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (a *rsicWatchdogSignalAdapter) IsJiminyHealthy(_ context.Context) bool {
	return a.jiminyEnabled && a.jiminySvc != nil
}

func (a *rsicWatchdogSignalAdapter) IsSidecarHealthy(_ context.Context) bool {
	if a.sidecarURL == "" {
		return true // not configured = not required
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(a.sidecarURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// rsicJiminyAdapter adapts *jiminy.Service to ape.JiminyStatsProvider (J10).
type rsicJiminyAdapter struct {
	svc *jiminy.Service
}

func (a *rsicJiminyAdapter) GetGuidanceStats(ctx context.Context, spaceID string) (ape.JiminyStatsResult, error) {
	stats, err := a.svc.GetGuidanceStats(ctx, spaceID)
	if err != nil {
		return ape.JiminyStatsResult{}, err
	}
	return ape.JiminyStatsResult{
		TotalGuidanceIssued:    stats.TotalGuidanceIssued,
		TotalFollowed:          stats.TotalFollowed,
		TotalPartialCompliance: stats.TotalPartialCompliance,
		TotalIgnored:           stats.TotalIgnored,
		TotalContradicted:      stats.TotalContradicted,
		FollowRate:             stats.FollowRate,
		ConstraintEffRate:   stats.ConstraintEffRate,
		ConstraintDataAvail: stats.ConstraintDataAvail,
		SourceDiversity:        stats.SourceDiversity,
	}, nil
}

// rsicProtocolAdapter adapts *jiminy.Service to ape.ProtocolStatsProvider (J17).
type rsicProtocolAdapter struct {
	svc *jiminy.Service
}

// GetProtocolStats returns global protocol metrics. The spaceID parameter is
// accepted for interface compliance but not used — ProtocolMetricsCollector
// is not space-partitioned. In single-space deployments (current), this is
// correct. Multi-space support requires partitioning the collector.
func (a *rsicProtocolAdapter) GetProtocolStats(ctx context.Context, spaceID string) (ape.ProtocolStatsResult, error) {
	snapshot := a.svc.GetProtocolMetricsSnapshot()
	if snapshot == nil {
		return ape.ProtocolStatsResult{}, nil
	}
	result := ape.ProtocolStatsResult{
		TierDistribution:         snapshot.TierDistribution,
		CompressionRatio:         snapshot.CompressionRatio,
		AvgComprehension:         snapshot.AvgComprehension,
		ReplayFrequencyPerHour:   snapshot.ReplayFrequencyPerHour,
		TicketRestoreSuccessRate: snapshot.TicketRestoreSuccessRate,
		TicketRestoreTotal:       snapshot.TicketRestoreTotal,
		CodeCoverage:             snapshot.CodeCoverage,
		TotalEvents:              snapshot.TotalEvents,
		T2FrequencyByConstraint:  snapshot.T2FrequencyByConstraint,
		CodeComprehension:        snapshot.CodeComprehension,
		TierComprehension:        snapshot.TierComprehension,
		TierCodeComprehension:    snapshot.TierCodeComprehension,
		TierOutcomeCount:         snapshot.TierOutcomeCount,
	}

	// Token efficiency
	result.AvgTokensPerGuidance = snapshot.AvgTokensPerGuidance

	// Trust score aggregates
	result.AvgTrustScore = snapshot.AvgTrustScore
	result.MinTrustScore = snapshot.MinTrustScore
	result.MaxTrustScore = snapshot.MaxTrustScore
	result.TrustSessionCount = snapshot.TrustSessionCount

	// NLI fallback tracking (degraded-state awareness)
	result.NLIFallbackCount = snapshot.NLIFallbackCount
	result.NLIFallbackRate = snapshot.NLIFallbackRate
	result.CodeOutcomeCount = snapshot.CodeOutcomeCount

	// Sidecar metrics (NS-07)
	if snapshot.Sidecar != nil {
		result.Sidecar = snapshot.Sidecar
	}

	// NLI calibration: populate from service if available
	if calibReport := a.svc.GetNLICalibrationReport(); calibReport != nil {
		result.NLIMeanBias = calibReport.MeanBias
		result.NLIBiasAlert = calibReport.BiasAlert
	}

	return result, nil
}

// rsicGuidanceCalibrationAdapter adapts *jiminy.Service to ape.GuidanceCalibrationProvider (RSIC-SK1).
type rsicGuidanceCalibrationAdapter struct {
	svc *jiminy.Service
}

func (a *rsicGuidanceCalibrationAdapter) GetConstraintEffectiveness(ctx context.Context, spaceID string) ([]ape.GuidanceEffectivenessItem, error) {
	items, err := a.svc.GetConstraintEffectiveness(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	result := make([]ape.GuidanceEffectivenessItem, len(items))
	for i, item := range items {
		result[i] = ape.GuidanceEffectivenessItem{
			NodeID:            item.NodeID,
			Name:              item.Name,
			Confidence:        item.Confidence,
			TotalSurfaced:     item.TotalSurfaced,
			TotalFollowed:     item.TotalFollowed,
			TotalIgnored:      item.TotalIgnored,
			TotalContradicted: item.TotalContradicted,
			EffectivenessRate: item.EffectivenessRate,
		}
	}
	return result, nil
}

func (a *rsicGuidanceCalibrationAdapter) UpdateNodeConfidence(ctx context.Context, nodeID string, outcome string) error {
	return a.svc.UpdateNodeConfidence(ctx, nodeID, jiminy.GuidanceOutcome(outcome))
}

func (a *rsicGuidanceCalibrationAdapter) ArchiveStaleConstraints(ctx context.Context, spaceID string) (int, error) {
	return a.svc.ArchiveStaleConstraints(ctx, spaceID)
}

// rsicTierEffectivenessAdapter adapts *jiminy.Service to ape.TierEffectivenessProvider.
type rsicTierEffectivenessAdapter struct {
	svc *jiminy.Service
}

func (a *rsicTierEffectivenessAdapter) BuildDataset() map[string]any {
	ds := a.svc.BuildTierEffectivenessDataset()
	if ds == nil {
		return nil
	}
	return map[string]any{
		"generated_at":   ds.GeneratedAt,
		"total_outcomes": ds.TotalOutcomes,
		"code_drift":     len(ds.CodeDrift),
		"recommendations": len(ds.Recommendations),
	}
}

// jiminyRetrievalAdapter adapts *retrieval.Service to jiminy.RetrievalProvider (J7).
type jiminyRetrievalAdapter struct {
	retriever *retrieval.Service
}

func (a *jiminyRetrievalAdapter) RetrieveForJiminy(ctx context.Context, spaceID, queryText string, topK, hopDepth int, queryEmbedding []float32) ([]jiminy.RetrievalResult, error) {
	resp, err := a.retriever.Retrieve(ctx, models.RetrieveRequest{
		SpaceID:        spaceID,
		QueryText:      queryText,
		TopK:           topK,
		HopDepth:       hopDepth,
		QueryEmbedding: queryEmbedding,
	})
	if err != nil {
		return nil, err
	}

	results := make([]jiminy.RetrievalResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, jiminy.RetrievalResult{
			NodeID:  r.NodeID,
			Name:    r.Name,
			Summary: r.Summary,
			Layer:   r.Layer,
			Score:   r.Score,
		})
	}
	return results, nil
}

// rsicFreshnessAdapter adapts retrieval.Service to ape.FreshnessProvider (Phase 47.2).
type rsicFreshnessAdapter struct {
	retriever *retrieval.Service
	triggerFn func() // set post-construction to Server.runScheduledSyncCheck
}

func (a *rsicFreshnessAdapter) GetStaleSpaceCount(ctx context.Context, thresholdHours int) (int, error) {
	allFreshness, err := a.retriever.GetAllTapRootFreshness(ctx)
	if err != nil {
		return 0, err
	}
	threshold := time.Duration(thresholdHours) * time.Hour
	count := 0
	for _, props := range allFreshness {
		if lastIngest, ok := props["last_ingest_at"]; ok {
			var lastTime time.Time
			switch v := lastIngest.(type) {
			case time.Time:
				lastTime = v
			case string:
				if parsed, parseErr := time.Parse(time.RFC3339, v); parseErr == nil {
					lastTime = parsed
				}
			}
			if !lastTime.IsZero() && time.Since(lastTime) < threshold {
				continue
			}
		}
		count++
	}
	return count, nil
}

func (a *rsicFreshnessAdapter) TriggerIngestForStaleSpaces(ctx context.Context, thresholdHours int) (int, error) {
	// Count stale spaces BEFORE triggering re-ingest — this is the number
	// of spaces for which ingest will be triggered, not the "still stale" count.
	triggeredCount, err := a.GetStaleSpaceCount(ctx, thresholdHours)
	if err != nil {
		return 0, err
	}
	if a.triggerFn != nil {
		a.triggerFn()
	}
	return triggeredCount, nil
}

// rsicAlertAdapter bridges ape.AlertDispatcher → *alert.Dispatcher.
// InsightSeverity maps 1:1 to alert.Severity (both use "low"/"medium"/"high"/"critical").
type rsicAlertAdapter struct {
	dispatcher *alert.Dispatcher
}

func (a *rsicAlertAdapter) SendAlert(ctx context.Context, service, title, message string, sev ape.InsightSeverity) {
	a.dispatcher.Send(ctx, alert.Alert{
		Service:  service,
		Severity: alert.Severity(sev),
		Title:    title,
		Message:  message,
	})
}
