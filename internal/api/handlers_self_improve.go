package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"mdemg/internal/ape"
)

// ─── POST /v1/self-improve/assess ───

func (s *Server) handleSelfImproveAssess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
		Tier    string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	tier := ape.CycleTier(req.Tier)
	if tier == "" {
		tier = ape.TierMeso
	}

	report, err := s.rsicCycle.Assess(r.Context(), req.SpaceID, tier)
	if err != nil {
		http.Error(w, sanitizeError(err, "self-improve assess"), http.StatusInternalServerError)
		return
	}

	// Phase 80: Record RSIC call for signal tracking
	if s.sessionTracker != nil {
		s.sessionTracker.RecordRSICCall("")
	}
	if s.signalLearner != nil {
		s.signalLearner.RecordResponse("rsic-assess-called")
	}

	writeJSON(w, http.StatusOK, report)
}

// ─── GET /v1/self-improve/report ───

func (s *Server) handleSelfImproveReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	tasks := s.rsicCycle.GetActiveTasks()
	writeJSON(w, http.StatusOK, map[string]any{
		"active_tasks": tasks,
	})
}

// ─── GET /v1/self-improve/report/{taskID} ───

func (s *Server) handleSelfImproveReportByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, "/v1/self-improve/report/")
	taskID = strings.TrimSuffix(taskID, "/")
	if taskID == "" {
		http.Error(w, "task_id is required in path", http.StatusBadRequest)
		return
	}

	reports := s.rsicCycle.GetTaskReports(taskID)
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": taskID,
		"reports": reports,
	})
}

// ─── POST /v1/self-improve/cycle ───

func (s *Server) handleSelfImproveCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SpaceID        string `json:"space_id"`
		Tier           string `json:"tier"`
		TriggerSource  string `json:"trigger_source,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
		DryRun         bool   `json:"dry_run,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	tier := ape.CycleTier(req.Tier)
	if tier == "" {
		tier = ape.TierMeso
	}

	// Resolve trigger source (default: manual_api)
	triggerSource := ape.TriggerManualAPI
	if req.TriggerSource != "" {
		ts, ok := ape.ValidTriggerSources[req.TriggerSource]
		if !ok {
			http.Error(w, "invalid trigger_source: "+req.TriggerSource, http.StatusBadRequest)
			return
		}
		triggerSource = ts
	}

	// Phase 87: Orchestration policy evaluation
	if s.orchestrationPolicy != nil {
		// Fast-path dedupe check
		if dedupeResult := s.orchestrationPolicy.CheckDedupe(req.IdempotencyKey); dedupeResult != nil {
			resp := map[string]any{
				"cycle_id":       dedupeResult.OriginalCycleID,
				"dedupe":         dedupeResult,
				"policy_version": ape.PolicyVersion,
			}
			if req.TriggerSource != "" {
				resp["trigger_source"] = req.TriggerSource
			}
			if req.IdempotencyKey != "" {
				resp["idempotency_key"] = req.IdempotencyKey
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		decision := s.orchestrationPolicy.EvaluateTrigger(triggerSource, req.SpaceID, tier, req.IdempotencyKey)
		if !decision.Allowed {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":          "trigger rejected",
				"reason":         decision.Reason,
				"policy_version": ape.PolicyVersion,
			})
			return
		}

		opts := &ape.RunCycleOpts{
			TriggerMeta:    &decision.Meta,
			IdempotencyKey: req.IdempotencyKey,
			DryRun:         req.DryRun,
		}

		outcome, err := s.rsicCycle.RunCycle(r.Context(), req.SpaceID, tier, opts)
		if err != nil {
			s.orchestrationPolicy.CompleteCycle(req.SpaceID, tier)
			http.Error(w, sanitizeError(err, "self-improve cycle"), http.StatusInternalServerError)
			return
		}

		s.orchestrationPolicy.RecordTrigger(decision.Meta, req.SpaceID, tier, outcome.CycleID)
		s.orchestrationPolicy.CompleteCycle(req.SpaceID, tier)

		// Phase 80: Record RSIC cycle for signal tracking
		if s.sessionTracker != nil {
			s.sessionTracker.RecordRSICCall("")
		}
		if s.signalLearner != nil {
			s.signalLearner.RecordResponse("rsic-cycle-called")
		}

		writeJSON(w, http.StatusOK, outcome)
		return
	}

	// Fallback path when orchestration policy is nil (backward compat)
	var fallbackOpts *ape.RunCycleOpts
	if req.DryRun {
		fallbackOpts = &ape.RunCycleOpts{DryRun: true}
	}
	outcome, err := s.rsicCycle.RunCycle(r.Context(), req.SpaceID, tier, fallbackOpts)
	if err != nil {
		http.Error(w, sanitizeError(err, "self-improve cycle"), http.StatusInternalServerError)
		return
	}

	// Phase 80: Record RSIC cycle for signal tracking
	if s.sessionTracker != nil {
		s.sessionTracker.RecordRSICCall("")
	}
	if s.signalLearner != nil {
		s.signalLearner.RecordResponse("rsic-cycle-called")
	}

	writeJSON(w, http.StatusOK, outcome)
}

// ─── GET /v1/self-improve/history ───

func (s *Server) handleSelfImproveHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Phase 87: Optional filters
	qTriggerSource := r.URL.Query().Get("trigger_source")
	qTier := r.URL.Query().Get("tier")
	qSpaceID := r.URL.Query().Get("space_id")

	var history []ape.CycleOutcome
	hasFilter := qTriggerSource != "" || qTier != "" || qSpaceID != ""

	if hasFilter {
		filter := &ape.HistoryFilter{
			TriggerSource: ape.TriggerSource(qTriggerSource),
			Tier:          ape.CycleTier(qTier),
			SpaceID:       qSpaceID,
		}
		history = s.rsicCycle.GetHistoryFiltered(limit, filter)
	} else {
		history = s.rsicCycle.GetHistory(limit)
	}

	if history == nil {
		history = []ape.CycleOutcome{}
	}

	resp := map[string]any{
		"history": history,
		"count":   len(history),
	}

	if hasFilter {
		filters := map[string]string{}
		if qTriggerSource != "" {
			filters["trigger_source"] = qTriggerSource
		}
		if qTier != "" {
			filters["tier"] = qTier
		}
		if qSpaceID != "" {
			filters["space_id"] = qSpaceID
		}
		resp["filters"] = filters
	}

	writeJSON(w, http.StatusOK, resp)
}

// ─── GET /v1/self-improve/calibration ───

func (s *Server) handleSelfImproveCalibration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	calibration := s.rsicCycle.GetCalibration()
	writeJSON(w, http.StatusOK, map[string]any{
		"calibration": calibration,
	})
}

// ─── POST /v1/self-improve/orchestration/reset ───

func (s *Server) handleOrchestrationReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.orchestrationPolicy == nil {
		http.Error(w, "orchestration policy not initialised", http.StatusServiceUnavailable)
		return
	}

	s.orchestrationPolicy.ResetState()
	writeJSON(w, http.StatusOK, map[string]any{
		"reset": true,
	})
}

// ─── GET /v1/self-improve/health ───

func (s *Server) handleSelfImproveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.rsicCycle == nil {
		http.Error(w, "RSIC not initialised", http.StatusServiceUnavailable)
		return
	}

	watchdogState := s.rsicCycle.GetWatchdogState()
	activeTasks := s.rsicCycle.GetActiveTasks()

	resp := map[string]any{
		"status":       "ok",
		"active_tasks": len(activeTasks),
	}
	if watchdogState != nil {
		resp["watchdog"] = watchdogState
	}

	// Phase 87: Orchestration status block
	if s.orchestrationPolicy != nil {
		resp["orchestration"] = s.orchestrationPolicy.GetOrchestrationStatus(s.macroNextRun)
	}

	// Phase 89: Persistence status block
	if s.rsicStore != nil {
		resp["persistence"] = s.rsicStore.GetStatus()
	}

	// Phase 88: Safety enforcement status block
	if s.snapshotStore != nil {
		safetyBlock := map[string]any{
			"enforcement_active": true,
			"safety_version":     ape.SafetyVersion,
			"bounds": map[string]any{
				"max_nodes_affected": int(float64(1000) * s.cfg.RSICMaxNodePrunePct),
				"max_edges_affected": int(float64(1000) * s.cfg.RSICMaxEdgePrunePct),
				"protected_spaces":   []string{"mdemg-dev"},
			},
			"rollback": map[string]any{
				"window_sec":            s.cfg.RSICRollbackWindow,
				"snapshots_held":        s.snapshotStore.GetSnapshotCount(),
				"oldest_snapshot_age_sec": s.snapshotStore.GetOldestSnapshotAgeSec(),
			},
		}
		resp["safety"] = safetyBlock
	}

	writeJSON(w, http.StatusOK, resp)
}

// ─── GET /v1/self-improve/signals ───

func (s *Server) handleSelfImproveSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.signalLearner == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"signals": []any{},
			"enabled": false,
		})
		return
	}

	signals := s.signalLearner.GetAllEffectiveness()
	writeJSON(w, http.StatusOK, map[string]any{
		"signals": signals,
		"enabled": true,
		"count":   len(signals),
	})
}

// ─── POST/GET /v1/self-improve/rollback ───

func (s *Server) handleSelfImproveRollback(w http.ResponseWriter, r *http.Request) {
	if s.snapshotStore == nil {
		http.Error(w, "rollback not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List available snapshots
		snapshots := s.snapshotStore.ListSnapshots()
		if snapshots == nil {
			snapshots = []ape.ActionSnapshot{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshots":          snapshots,
			"count":              len(snapshots),
			"rollback_window_sec": s.cfg.RSICRollbackWindow,
		})

	case http.MethodPost:
		// Execute rollback
		var req struct {
			SnapshotID string `json:"snapshot_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("ERROR [rollback JSON]: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.SnapshotID == "" {
			http.Error(w, "snapshot_id is required", http.StatusBadRequest)
			return
		}

		result, err := s.snapshotStore.Rollback(r.Context(), req.SnapshotID)
		if err != nil {
			log.Printf("ERROR [rollback %s]: %v", req.SnapshotID, err)
			writeJSON(w, http.StatusNotFound, map[string]any{
				"rolled_back":         false,
				"error":               "snapshot not found or rollback window expired",
				"rollback_window_sec": s.cfg.RSICRollbackWindow,
			})
			return
		}

		writeJSON(w, http.StatusOK, result)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

