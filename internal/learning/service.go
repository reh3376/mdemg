package learning

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
)

// StabilityReinforcer is called when nodes are co-activated to update stability scores
// for conversation observations. This enables the Context Cooler graduation system.
type StabilityReinforcer interface {
	UpdateStabilityOnReinforcement(ctx context.Context, spaceID, nodeID string) error
}

type Service struct {
	cfg    config.Config
	driver neo4j.DriverWithContext

	// Freeze state tracking (per space)
	freezeMu     sync.RWMutex
	frozenSpaces map[string]FreezeState

	// Learning phase cache for maturity-based eta scheduling
	learningPhaseCache sync.Map // map[string]learningPhaseEntry

	// Optional: Context Cooler for stability reinforcement
	stabilityReinforcer StabilityReinforcer
}

// learningPhaseEntry caches the edge count for a space to avoid frequent DB queries
type learningPhaseEntry struct {
	edgeCount int64
	cachedAt  time.Time
}

// FreezeState tracks the frozen state of a space
type FreezeState struct {
	Frozen    bool      `json:"frozen"`
	Reason    string    `json:"reason,omitempty"`
	FrozenAt  time.Time `json:"frozen_at,omitempty"`
	FrozenBy  string    `json:"frozen_by,omitempty"`
	EdgeCount int64     `json:"edge_count_at_freeze,omitempty"`
}

func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	return &Service{
		cfg:          cfg,
		driver:       driver,
		frozenSpaces: make(map[string]FreezeState),
	}
}

// SetStabilityReinforcer sets the callback for Context Cooler stability updates.
// When nodes are co-activated, the reinforcer will be called for conversation observations.
func (s *Service) SetStabilityReinforcer(reinforcer StabilityReinforcer) {
	s.stabilityReinforcer = reinforcer
}

// FreezeLearning stops all learning edge creation/updates for a space.
// Existing edges are preserved but no new edges are created and weights are not updated.
// Use this for production deployments where stable scoring is required.
func (s *Service) FreezeLearning(ctx context.Context, spaceID, reason, frozenBy string) (FreezeState, error) {
	s.freezeMu.Lock()
	defer s.freezeMu.Unlock()

	// Get current edge count for record-keeping (optional - don't fail if DB unavailable)
	var edgeCount int64 = -1
	if s.driver != nil {
		count, err := s.getEdgeCountUnlocked(ctx, spaceID)
		if err == nil {
			edgeCount = count
		}
	}

	state := FreezeState{
		Frozen:    true,
		Reason:    reason,
		FrozenAt:  time.Now(),
		FrozenBy:  frozenBy,
		EdgeCount: edgeCount,
	}
	s.frozenSpaces[spaceID] = state
	return state, nil
}

// UnfreezeLearning resumes learning edge creation/updates for a space.
func (s *Service) UnfreezeLearning(spaceID string) FreezeState {
	s.freezeMu.Lock()
	defer s.freezeMu.Unlock()

	delete(s.frozenSpaces, spaceID)
	return FreezeState{Frozen: false}
}

// IsFrozen checks if learning is frozen for a space.
func (s *Service) IsFrozen(spaceID string) bool {
	s.freezeMu.RLock()
	defer s.freezeMu.RUnlock()

	state, exists := s.frozenSpaces[spaceID]
	return exists && state.Frozen
}

// GetFreezeState returns the freeze state for a space.
func (s *Service) GetFreezeState(spaceID string) FreezeState {
	s.freezeMu.RLock()
	defer s.freezeMu.RUnlock()

	state, exists := s.frozenSpaces[spaceID]
	if !exists {
		return FreezeState{Frozen: false}
	}
	return state
}

// GetAllFreezeStates returns freeze states for all tracked spaces.
func (s *Service) GetAllFreezeStates() map[string]FreezeState {
	s.freezeMu.RLock()
	defer s.freezeMu.RUnlock()

	result := make(map[string]FreezeState)
	for k, v := range s.frozenSpaces {
		result[k] = v
	}
	return result
}

// CleanupStaleFreezes removes frozen state entries for spaces that no longer exist.
// Returns the number of stale entries removed.
func (s *Service) CleanupStaleFreezes(validSpaces map[string]bool) int {
	s.freezeMu.Lock()
	defer s.freezeMu.Unlock()

	removed := 0
	for spaceID := range s.frozenSpaces {
		if !validSpaces[spaceID] {
			delete(s.frozenSpaces, spaceID)
			removed++
		}
	}
	return removed
}

// getEdgeCountUnlocked queries the edge count (caller must hold lock)
func (s *Service) getEdgeCountUnlocked(ctx context.Context, spaceID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `MATCH ()-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->()
RETURN count(r) AS edge_count`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("edge_count")
			if c, ok := count.(int64); ok {
				return c, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	typed, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// effectiveEta returns the learning rate for a space, adjusted by the maturity-based schedule.
// If scheduling is disabled, returns the base eta from config.
func (s *Service) effectiveEta(ctx context.Context, spaceID string) float64 {
	baseEta := s.cfg.LearningEta
	if baseEta <= 0 {
		baseEta = 0.02 // fallback default learning rate
	}

	if !s.cfg.LearningScheduleEnabled {
		return baseEta
	}

	// Check cache (5 min TTL)
	if entry, ok := s.learningPhaseCache.Load(spaceID); ok {
		if e, ok := entry.(learningPhaseEntry); ok {
			if time.Since(e.cachedAt) < 5*time.Minute {
				return baseEta * s.phaseMultiplier(e.edgeCount)
			}
		}
	}

	// Query edge count
	count, err := s.getEdgeCountUnlocked(ctx, spaceID)
	if err != nil {
		return baseEta // fallback on error
	}

	s.learningPhaseCache.Store(spaceID, learningPhaseEntry{
		edgeCount: count,
		cachedAt:  time.Now(),
	})
	return baseEta * s.phaseMultiplier(count)
}

// phaseMultiplier returns the eta multiplier for the given edge count,
// implementing a maturity-based learning rate schedule.
// Phases: cold(0) -> learning(1-10k) -> warm(10k-50k) -> saturated(50k+)
func (s *Service) phaseMultiplier(edgeCount int64) float64 {
	switch {
	case edgeCount == 0:
		return s.cfg.LearningScheduleColdMult
	case edgeCount < 10000:
		return s.cfg.LearningScheduleLearningMult
	case edgeCount < 50000:
		return s.cfg.LearningScheduleWarmMult
	default:
		return s.cfg.LearningScheduleSatMult
	}
}

type pair struct {
	src     string
	dst     string
	ai      float64
	aj      float64
	pathSim float64
}

// pathPrefixSimilarity computes similarity based on shared path prefixes.
// Same directory = 0.8, same top-level directory = 0.5, else 0.0.
func pathPrefixSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0.0
	}
	dirA := a
	if idx := strings.LastIndex(a, "/"); idx > 0 {
		dirA = a[:idx]
	}
	dirB := b
	if idx := strings.LastIndex(b, "/"); idx > 0 {
		dirB = b[:idx]
	}
	if dirA == dirB {
		return 0.8
	}
	// Check same top-level directory
	topA := a
	if idx := strings.Index(a, "/"); idx > 0 {
		topA = a[:idx]
	}
	topB := b
	if idx := strings.Index(b, "/"); idx > 0 {
		topB = b[:idx]
	}
	if topA == topB && topA != "" {
		return 0.5
	}
	return 0.0
}

// ApplyCoactivation performs bounded, regularized learning updates.
// DB writes are intentionally limited to small deltas.
// Supports both code nodes and conversation_observation nodes with surprise-based weighting.
// Returns immediately if learning is frozen for the space.
func (s *Service) ApplyCoactivation(ctx context.Context, spaceID string, resp models.RetrieveResponse) error {
	if spaceID == "" || len(resp.Results) < 2 {
		return nil
	}

	// Check if learning is frozen for this space
	if s.IsFrozen(spaceID) {
		return nil // Silently skip - learning is frozen
	}

	// Filter nodes by activation threshold (configurable, default 0.20)
	// This prevents clique spam by only considering significantly activated nodes
	minAct := s.cfg.LearningMinActivation
	if minAct <= 0 {
		minAct = 0.20 // fallback default
	}
	nodes := make([]models.RetrieveResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		// CRITICAL: Only L0 (code) nodes participate in learning.
		// Hidden nodes (L1+) are structural abstractions that appear in many queries,
		// causing them to become hubs that accumulate edges from unrelated code.
		// This pollutes activation spreading and pushes hidden nodes to top results.
		if r.Layer > 0 {
			continue // Skip hidden/concept nodes
		}
		// Skip migration files - they co-activate with many unrelated code elements
		// and become hubs that pollute activation spreading (64+ edges observed).
		if isMigrationFile(r.Path) {
			continue
		}
		if r.Activation >= minAct && !isStopWord(r.Name) {
			nodes = append(nodes, r)
		}
	}
	if len(nodes) < 2 {
		return nil // need at least 2 nodes to form a pair
	}

	// Build pair updates (cap to config)
	// Generate all O(n²) candidate pairs with their activation products
	// VectorSim floor: only create edges between nodes with strong semantic match
	const vectorSimFloor = 0.5
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			// Skip pairs where either node has weak vector similarity to the query.
			// This prevents spurious edges between nodes that only matched via BM25.
			if nodes[i].VectorSim < vectorSimFloor || nodes[j].VectorSim < vectorSimFloor {
				continue
			}
			ps := pathPrefixSimilarity(nodes[i].Path, nodes[j].Path)
			pairs = append(pairs, pair{src: nodes[i].NodeID, dst: nodes[j].NodeID, ai: nodes[i].Activation, aj: nodes[j].Activation, pathSim: ps})
		}
	}

	capN := s.cfg.LearningEdgeCapPerRequest
	if capN <= 0 {
		capN = 200
	}

	// Select top-K pairs by activation product (ai * aj)
	// This prioritizes strengthening edges between the most strongly co-activated nodes
	// and prevents clique spam per 04_Activation_and_Learning.md guidelines
	if len(pairs) > capN {
		sort.Slice(pairs, func(i, j int) bool {
			// Sort descending by activation product
			return pairs[i].ai*pairs[i].aj > pairs[j].ai*pairs[j].aj
		})
		pairs = pairs[:capN]
	}

	// Hebbian learning parameters from config (with fallback defaults)
	// Formula: Δw_ij = η * a_i * a_j - μ * w_ij
	// Simplified: new_w = (1-μ)*w + η*etaMult*prod, tanh soft-capped at wmax
	eta := s.effectiveEta(ctx, spaceID)
	mu := s.cfg.LearningMu
	if mu <= 0 {
		mu = 0.01 // fallback default decay rate
	}
	wmin := s.cfg.LearningWMin
	wmax := s.cfg.LearningWMax
	if wmax <= wmin {
		wmin, wmax = 0.0, 1.0 // fallback default bounds
	}

	// Determine direction labels for asymmetric learning (F9)
	fwdDir := "bidirectional"
	revDir := "bidirectional"
	if s.cfg.LearningAsymmetricEnabled {
		fwdDir = "forward"
		revDir = "backward"
	}

	params := map[string]any{
		"spaceId":        spaceID,
		"pairs":          pairsToMaps(pairs),
		"eta":            eta,
		"mu":             mu,
		"wmin":           wmin,
		"wmax":           wmax,
		"etaConvMult":    s.cfg.LearningEtaConversationMult,
		"etaConfigMult":  s.cfg.LearningEtaConfigMult,
		"etaSameDirMult": s.cfg.LearningEtaSameDirMult,
		"fwdDir":         fwdDir,
		"revDir":         revDir,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Cypher query implements MERGE pattern for CO_ACTIVATED_WITH edges:
		// - MERGE creates edge if missing, matches if exists
		// - ON CREATE SET initializes all required edge properties
		// - ON MATCH SET updates timestamps and evidence count
		// - Weight is calculated using Hebbian formula after MERGE
		// - Symmetric edges are created in both directions
		// - For conversation_observation nodes, applies surprise-based initial weight
		cypher := `UNWIND $pairs AS p
MATCH (a:MemoryNode {space_id:$spaceId, node_id:p.src})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:p.dst})
WITH a,b, (p.ai * p.aj) AS prod, p.pathSim AS pathSim,
     coalesce(a.surprise_score, 0.0) AS surpriseA,
     coalesce(b.surprise_score, 0.0) AS surpriseB,
     coalesce(a.role_type, '') AS roleA,
     coalesce(b.role_type, '') AS roleB,
     coalesce(a.obs_type, '') AS obsTypeA,
     coalesce(b.obs_type, '') AS obsTypeB,
     coalesce(a.session_id, '') AS sessionA,
     coalesce(b.session_id, '') AS sessionB
// Calculate surprise factor (for conversation nodes)
WITH a,b,prod,pathSim,surpriseA,surpriseB,roleA,roleB,obsTypeA,obsTypeB,sessionA,sessionB,
     CASE
       WHEN roleA = 'conversation_observation' OR roleB = 'conversation_observation' THEN
         CASE
           WHEN surpriseA >= 0.7 OR surpriseB >= 0.7 THEN 2.0  // HIGH surprise
           WHEN surpriseA >= 0.4 OR surpriseB >= 0.4 THEN 1.5  // MEDIUM surprise
           ELSE 1.0  // NORMAL
         END
       ELSE 1.0  // Code nodes use standard factor
     END AS surpriseFactor
// Calculate initial weight based on surprise factor
WITH a,b,prod,pathSim,surpriseA,surpriseB,roleA,roleB,obsTypeA,obsTypeB,sessionA,sessionB,surpriseFactor,
     0.10 * surpriseFactor AS initialWeight
// Calculate multi-rate eta multiplier based on edge context
WITH a,b,prod,pathSim,surpriseA,surpriseB,roleA,roleB,obsTypeA,obsTypeB,sessionA,sessionB,surpriseFactor,initialWeight,
     CASE WHEN roleA = 'conversation_observation' OR roleB = 'conversation_observation'
          THEN $etaConvMult ELSE 1.0 END *
     CASE WHEN pathSim > 0.5 THEN $etaSameDirMult ELSE 1.0 END *
     CASE WHEN (obsTypeA = 'config' OR obsTypeB = 'config') AND obsTypeA <> obsTypeB
          THEN $etaConfigMult ELSE 1.0 END AS etaMult
// forward edge: create or update
MERGE (a)-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b)
ON CREATE SET r.edge_id=randomUUID(), r.created_at=datetime(), r.updated_at=datetime(),
              r.last_activated_at=datetime(), r.status='active', r.weight=initialWeight,
              r.evidence_count=1, r.version=1, r.dim_coactivation=1.0, r.dim_semantic=pathSim, r.decay_rate=0.001,
              r.surprise_factor=surpriseFactor,
              r.direction=$fwdDir,
              r.session_id=CASE WHEN sessionA <> '' THEN sessionA WHEN sessionB <> '' THEN sessionB ELSE '' END,
              r.obs_type=CASE WHEN obsTypeA <> '' THEN obsTypeA WHEN obsTypeB <> '' THEN obsTypeB ELSE '' END
ON MATCH SET r.updated_at=datetime(), r.last_activated_at=datetime(),
             r.evidence_count=coalesce(r.evidence_count,0)+1, r.version=coalesce(r.version,0)+1,
             r.direction=CASE WHEN r.direction IS NULL THEN $fwdDir ELSE r.direction END
WITH a,b,prod,pathSim,initialWeight,surpriseFactor,sessionA,sessionB,obsTypeA,obsTypeB,r,etaMult
WITH a,b,prod,pathSim,initialWeight,surpriseFactor,sessionA,sessionB,obsTypeA,obsTypeB,r,etaMult,
     coalesce(r.weight,initialWeight) AS w
// Apply Hebbian weight update with tanh soft-cap and multi-rate eta
WITH a,b,prod,pathSim,initialWeight,surpriseFactor,sessionA,sessionB,obsTypeA,obsTypeB,r,etaMult,w,
     ((1-$mu)*w + $eta*etaMult*prod) AS rawW
SET r.weight = CASE
  WHEN rawW < $wmin THEN $wmin
  ELSE $wmax * ((exp(2.0 * rawW / $wmax) - 1.0) / (exp(2.0 * rawW / $wmax) + 1.0))
END
// reverse edge (symmetry): mirrors forward edge weight
MERGE (b)-[rr:CO_ACTIVATED_WITH {space_id:$spaceId}]->(a)
ON CREATE SET rr.edge_id=randomUUID(), rr.created_at=datetime(), rr.updated_at=datetime(),
              rr.last_activated_at=datetime(), rr.status='active', rr.weight=r.weight,
              rr.evidence_count=1, rr.version=1, rr.dim_coactivation=1.0, rr.dim_semantic=pathSim, rr.decay_rate=0.001,
              rr.surprise_factor=surpriseFactor,
              rr.direction=$revDir,
              rr.session_id=CASE WHEN sessionA <> '' THEN sessionA WHEN sessionB <> '' THEN sessionB ELSE '' END,
              rr.obs_type=CASE WHEN obsTypeA <> '' THEN obsTypeA WHEN obsTypeB <> '' THEN obsTypeB ELSE '' END
ON MATCH SET rr.updated_at=datetime(), rr.last_activated_at=datetime(),
             rr.evidence_count=coalesce(rr.evidence_count,0)+1, rr.version=coalesce(rr.version,0)+1, rr.weight=r.weight,
             rr.direction=CASE WHEN rr.direction IS NULL THEN $revDir ELSE rr.direction END
RETURN count(*) AS updated;`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	if err != nil {
		return err
	}

	// If stability reinforcer is set, update stability for conversation observations
	if s.stabilityReinforcer != nil {
		s.reinforceConversationObservations(ctx, spaceID, nodes)
	}

	return nil
}

// ApplySymbolCoactivation creates CO_ACTIVATED_WITH edges between
// SymbolNodes that are co-retrieved (connected via DEFINES_SYMBOL to
// co-activated MemoryNodes). Guards behind SymbolActivationEnabled config.
func (s *Service) ApplySymbolCoactivation(ctx context.Context, spaceID string, resp models.RetrieveResponse) error {
	if !s.cfg.SymbolActivationEnabled || spaceID == "" || len(resp.Results) < 2 {
		return nil
	}

	if s.IsFrozen(spaceID) {
		return nil
	}

	// Collect node IDs from retrieval results
	nodeIDs := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.NodeID != "" {
			nodeIDs = append(nodeIDs, r.NodeID)
		}
	}
	if len(nodeIDs) < 2 {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Find SymbolNodes connected to co-retrieved MemoryNodes, then MERGE CO_ACTIVATED_WITH
	cypher := `
UNWIND $nodeIds AS nid
MATCH (m:MemoryNode {node_id: nid, space_id: $spaceId})-[:DEFINES_SYMBOL]->(sym:SymbolNode {space_id: $spaceId})
WITH collect(DISTINCT sym) AS symbols
WHERE size(symbols) >= 2
UNWIND range(0, size(symbols)-2) AS i
UNWIND range(i+1, size(symbols)-1) AS j
WITH symbols[i] AS s1, symbols[j] AS s2
MERGE (s1)-[r:CO_ACTIVATED_WITH {space_id: s1.space_id}]->(s2)
ON CREATE SET
    r.weight = 0.1,
    r.created_at = datetime(),
    r.updated_at = datetime(),
    r.evidence_count = 1
ON MATCH SET
    r.weight = CASE WHEN r.weight < 1.0 THEN r.weight + 0.05 ELSE r.weight END,
    r.updated_at = datetime(),
    r.evidence_count = r.evidence_count + 1`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, cypher, map[string]any{
			"nodeIds": nodeIDs,
			"spaceId": spaceID,
		})
		return nil, err
	})

	return err
}

// reinforceConversationObservations calls the stability reinforcer for any conversation observations
// in the co-activated nodes. This supports the Context Cooler graduation system.
func (s *Service) reinforceConversationObservations(ctx context.Context, spaceID string, nodes []models.RetrieveResult) {
	if s.stabilityReinforcer == nil {
		return
	}

	// Query which nodes are conversation observations and reinforce them
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.NodeID
	}

	// Find conversation observation nodes
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			UNWIND $nodeIds AS nodeId
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: nodeId})
			WHERE n.role_type = 'conversation_observation'
			RETURN n.node_id AS nodeId
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeIds": nodeIDs,
		})
		if err != nil {
			return nil, err
		}

		var obsNodeIDs []string
		for res.Next(ctx) {
			rec := res.Record()
			if id, ok := rec.Get("nodeId"); ok && id != nil {
				if idStr, ok := id.(string); ok {
				obsNodeIDs = append(obsNodeIDs, idStr)
			}
			}
		}
		return obsNodeIDs, res.Err()
	})

	if err != nil {
		return // Silently fail - stability reinforcement is best-effort
	}

	obsNodeIDs, ok := result.([]string)
	if !ok {
		return // Unexpected result type - best-effort path
	}
	for _, nodeID := range obsNodeIDs {
		// Best-effort reinforcement - don't fail the main operation
		if err := s.stabilityReinforcer.UpdateStabilityOnReinforcement(ctx, spaceID, nodeID); err != nil {
			slog.Warn("stability reinforcement failed", "node_id", nodeID, "error", err)
			metrics.Metrics().CMSStabilityUpdateFails.Inc()
		}
	}
}

// CoactivateSession creates CO_ACTIVATED_WITH edges between all observations in a session.
// Links observations together based on:
// - Temporal proximity (closer in time = stronger edge)
// - Surprise scores (high surprise observations get stronger connections)
// This enables session-based learning where related observations reinforce each other.
// Returns immediately if learning is frozen for the space.
func (s *Service) CoactivateSession(ctx context.Context, spaceID, sessionID string) error {
	if spaceID == "" || sessionID == "" {
		return nil
	}

	// Check if learning is frozen for this space
	if s.IsFrozen(spaceID) {
		return nil // Silently skip - learning is frozen
	}

	// Get Hebbian learning parameters
	eta := s.cfg.LearningEta
	if eta <= 0 {
		eta = 0.02
	}
	mu := s.cfg.LearningMu
	if mu <= 0 {
		mu = 0.01
	}
	wmin := s.cfg.LearningWMin
	wmax := s.cfg.LearningWMax
	if wmax <= wmin {
		wmin, wmax = 0.0, 1.0
	}

	params := map[string]any{
		"spaceId":   spaceID,
		"sessionId": sessionID,
		"eta":       eta,
		"mu":        mu,
		"wmin":      wmin,
		"wmax":      wmax,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find all conversation_observation nodes in the session
		// Create edges weighted by temporal proximity and surprise
		cypher := `
MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
WHERE obs.session_id = $sessionId
WITH obs ORDER BY obs.created_at
WITH collect(obs) AS observations

// Generate pairs from observations
UNWIND range(0, size(observations)-1) AS i
UNWIND range(i+1, size(observations)-1) AS j
WITH observations[i] AS a, observations[j] AS b,
     duration.inSeconds(observations[i].created_at, observations[j].created_at).seconds AS timeDiffSec

// Calculate temporal proximity weight (closer = higher, max 1 hour window)
WITH a, b, timeDiffSec,
     CASE
       WHEN timeDiffSec <= 0 THEN 1.0
       WHEN timeDiffSec > 3600 THEN 0.1  // Beyond 1 hour = low proximity
       ELSE 1.0 - (toFloat(timeDiffSec) / 3600.0) * 0.9  // Linear decay over 1 hour
     END AS temporalProximity

// Calculate surprise factor
WITH a, b, temporalProximity,
     coalesce(a.surprise_score, 0.0) AS surpriseA,
     coalesce(b.surprise_score, 0.0) AS surpriseB,
     CASE
       WHEN coalesce(a.surprise_score, 0.0) >= 0.7 OR coalesce(b.surprise_score, 0.0) >= 0.7 THEN 2.0
       WHEN coalesce(a.surprise_score, 0.0) >= 0.4 OR coalesce(b.surprise_score, 0.0) >= 0.4 THEN 1.5
       ELSE 1.0
     END AS surpriseFactor

// Calculate combined activation (temporal proximity * surprise boost)
WITH a, b, temporalProximity, surpriseFactor,
     temporalProximity AS activation,
     0.10 * surpriseFactor AS initialWeight

// Create forward edge
MERGE (a)-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(b)
ON CREATE SET r.edge_id=randomUUID(), r.created_at=datetime(), r.updated_at=datetime(),
              r.last_activated_at=datetime(), r.status='active', r.weight=initialWeight,
              r.evidence_count=1, r.version=1, r.dim_coactivation=1.0, r.decay_rate=0.001,
              r.surprise_factor=surpriseFactor,
              r.session_id=$sessionId,
              r.obs_type=coalesce(a.obs_type, b.obs_type, ''),
              r.temporal_proximity=temporalProximity
ON MATCH SET r.updated_at=datetime(), r.last_activated_at=datetime(),
             r.evidence_count=coalesce(r.evidence_count,0)+1,
             r.version=coalesce(r.version,0)+1

// Calculate new weight with temporal proximity factor
WITH a, b, r, activation, temporalProximity, surpriseFactor, initialWeight,
     coalesce(r.weight, initialWeight) AS w,
     activation * activation AS prod

// Apply Hebbian update with temporal weighting
SET r.weight = CASE
  WHEN ((1-$mu)*w + $eta*prod) < $wmin THEN $wmin
  WHEN ((1-$mu)*w + $eta*prod) > $wmax THEN $wmax
  ELSE ((1-$mu)*w + $eta*prod)
END

// Create reverse edge (symmetry)
MERGE (b)-[rr:CO_ACTIVATED_WITH {space_id: $spaceId}]->(a)
ON CREATE SET rr.edge_id=randomUUID(), rr.created_at=datetime(), rr.updated_at=datetime(),
              rr.last_activated_at=datetime(), rr.status='active', rr.weight=r.weight,
              rr.evidence_count=1, rr.version=1, rr.dim_coactivation=1.0, rr.decay_rate=0.001,
              rr.surprise_factor=surpriseFactor,
              rr.session_id=$sessionId,
              rr.obs_type=coalesce(a.obs_type, b.obs_type, ''),
              rr.temporal_proximity=temporalProximity
ON MATCH SET rr.updated_at=datetime(), rr.last_activated_at=datetime(),
             rr.evidence_count=coalesce(rr.evidence_count,0)+1,
             rr.version=coalesce(rr.version,0)+1, rr.weight=r.weight

RETURN count(*) AS edges_created
`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			count, _ := rec.Get("edges_created")
			return count, nil
		}
		return int64(0), res.Err()
	})

	return err
}

func pairsToMaps(pairs []pair) []map[string]any {
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{
			"src":     p.src,
			"dst":     p.dst,
			"ai":      clamp01(p.ai),
			"aj":      clamp01(p.aj),
			"pathSim": p.pathSim,
		})
	}
	return out
}

// stopWords are common English words that pollute the graph when used as node names.
var stopWords = map[string]bool{
	"for": true, "and": true, "is": true, "the": true, "of": true,
	"to": true, "in": true, "a": true, "an": true, "or": true,
	"not": true, "with": true, "as": true, "at": true, "by": true,
}

// isStopWord returns true if the name is a stop word or shorter than 3 characters.
func isStopWord(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if len(n) < 3 {
		return true
	}
	return stopWords[n]
}

// isMigrationFile returns true if the path indicates a database migration file.
// Migrations co-activate with many unrelated code elements and become hubs.
func isMigrationFile(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "/migrations/") ||
		strings.Contains(p, "/migration/") ||
		strings.HasSuffix(p, ".migration.ts") ||
		strings.HasSuffix(p, ".migration.sql")
}

// NegativeFeedbackResult holds the results of applying negative feedback.
type NegativeFeedbackResult struct {
	Processed    int `json:"processed"`
	Weakened     int `json:"weakened"`
	Contradicted int `json:"contradicted"`
}

// ApplyNegativeFeedback weakens or contradicts associations between query and rejected nodes.
// For each (query, rejected) pair:
// - If CO_ACTIVATED_WITH exists: weaken weight by LearningNegativeWeight (floor at 0)
// - If no CO_ACTIVATED_WITH: MERGE a CONTRADICTS edge (increment evidence_count on match)
func (s *Service) ApplyNegativeFeedback(ctx context.Context, spaceID string, queryNodeIDs, rejectedNodeIDs []string) (NegativeFeedbackResult, error) {
	if spaceID == "" {
		return NegativeFeedbackResult{}, fmt.Errorf("space_id is required")
	}
	if len(queryNodeIDs) == 0 || len(rejectedNodeIDs) == 0 {
		return NegativeFeedbackResult{}, fmt.Errorf("query_node_ids and rejected_node_ids must be non-empty")
	}

	// Cap rejected nodes
	maxRejected := s.cfg.LearningNegativeMaxPerRequest
	if maxRejected <= 0 {
		maxRejected = 20
	}
	if len(rejectedNodeIDs) > maxRejected {
		rejectedNodeIDs = rejectedNodeIDs[:maxRejected]
	}

	negWeight := s.cfg.LearningNegativeWeight
	if negWeight <= 0 {
		negWeight = 0.15
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var result NegativeFeedbackResult

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// For each query-rejected pair, weaken existing CO_ACTIVATED_WITH or create CONTRADICTS
		cypher := `
UNWIND $queryIds AS qid
UNWIND $rejectedIds AS rid
MATCH (q:MemoryNode {node_id: qid, space_id: $spaceId})
MATCH (r:MemoryNode {node_id: rid, space_id: $spaceId})
WHERE q <> r
OPTIONAL MATCH (q)-[coact:CO_ACTIVATED_WITH {space_id: $spaceId}]->(r)
WITH q, r, coact,
     CASE WHEN coact IS NOT NULL THEN 'weaken' ELSE 'contradict' END AS action
FOREACH (_ IN CASE WHEN action = 'weaken' THEN [1] ELSE [] END |
    SET coact.weight = CASE WHEN coact.weight - $negWeight < 0 THEN 0.0 ELSE coact.weight - $negWeight END,
        coact.updated_at = datetime()
)
FOREACH (_ IN CASE WHEN action = 'contradict' THEN [1] ELSE [] END |
    MERGE (q)-[c:CONTRADICTS {space_id: $spaceId}]->(r)
    ON CREATE SET c.weight = $negWeight, c.evidence_count = 1, c.created_at = datetime()
    ON MATCH SET c.evidence_count = c.evidence_count + 1, c.updated_at = datetime()
)
RETURN action, count(*) AS cnt`

		params := map[string]any{
			"spaceId":     spaceID,
			"queryIds":    queryNodeIDs,
			"rejectedIds": rejectedNodeIDs,
			"negWeight":   negWeight,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		for res.Next(ctx) {
			rec := res.Record()
			action, _ := rec.Get("action")
			cnt, _ := rec.Get("cnt")
			count := int(cnt.(int64))
			switch action.(string) {
			case "weaken":
				result.Weakened += count
			case "contradict":
				result.Contradicted += count
			}
			result.Processed += count
		}
		return nil, res.Err()
	})

	return result, err
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// HebbianWeightUpdate computes the new weight using the Hebbian learning formula.
// The formula is: Δw = η * a_i * a_j - μ * w_ij
// Which simplifies to: new_w = (1-μ)*w + η*a_i*a_j
// The result is clamped at wmin and soft-capped at wmax using tanh.
//
// Parameters:
//   - w: current weight
//   - ai: activation of node i (0 to 1)
//   - aj: activation of node j (0 to 1)
//   - eta: learning rate (η) - controls strengthening from co-activation
//   - mu: decay rate (μ) - controls regularization/forgetting
//   - wmin: minimum allowed weight
//   - wmax: maximum allowed weight (approached asymptotically via tanh)
//
// This is a pure function exposed for unit testing.
func HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax float64) float64 {
	prod := ai * aj
	newW := (1-mu)*w + eta*prod

	if newW < wmin {
		return wmin
	}
	// Tanh soft-cap: smooth saturation instead of hard clamp
	// wmax * tanh(newW / wmax) approaches wmax asymptotically
	return wmax * math.Tanh(newW/wmax)
}

// PruneDecayedEdges removes CO_ACTIVATED_WITH edges that have decayed below the threshold.
// Uses evidence-based decay: edges with higher evidence_count decay slower.
// This is a maintenance operation that can be run periodically.
// Returns the number of edges deleted.
func (s *Service) PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error) {
	if spaceID == "" {
		return 0, nil
	}

	// Get decay parameters with defaults
	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05
	}

	cautiousWindow := s.cfg.LearningCautiousDecayWindowHours

	params := map[string]any{
		"spaceId":        spaceID,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
		"cautiousWindow": cautiousWindow,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find and delete CO_ACTIVATED_WITH edges where decayed weight < threshold
		// Uses evidence-based decay: effectiveDecay = baseDecay / sqrt(evidenceCount * surpriseFactor)
		// Edges with more evidence (frequently co-activated) decay slower
		// Edges with high surprise factor decay slower (surprising information persists longer)
		// Cautious window: skip edges reinforced within the last N hours
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b:MemoryNode {space_id:$spaceId})
WHERE $cautiousWindow = 0 OR r.last_activated_at < datetime() - duration({hours: $cautiousWindow})
WITH r,
     duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days AS daysSinceActive,
     coalesce(r.weight, 0.0) AS rawWeight,
     coalesce(r.evidence_count, 1) AS evidenceCount,
     coalesce(r.surprise_factor, 1.0) AS surpriseFactor
WITH r, daysSinceActive, rawWeight, evidenceCount, surpriseFactor,
     CASE WHEN daysSinceActive > 0 THEN
       rawWeight * ((1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) ^ daysSinceActive)
     ELSE rawWeight END AS decayedWeight
WHERE decayedWeight < $pruneThreshold
DELETE r
RETURN count(*) AS deleted`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			rec := res.Record()
			deleted, _ := rec.Get("deleted")
			if d, ok := deleted.(int64); ok {
				return d, nil
			}
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	typed, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// PruneExcessEdgesPerNode removes excess CO_ACTIVATED_WITH edges per node beyond the cap.
// Keeps only the top N edges by weight for each node.
// Returns the number of edges deleted.
func (s *Service) PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error) {
	if spaceID == "" {
		return 0, nil
	}

	maxEdgesPerNode := s.cfg.LearningMaxEdgesPerNode
	if maxEdgesPerNode <= 0 {
		maxEdgesPerNode = 50
	}

	params := map[string]any{
		"spaceId":         spaceID,
		"maxEdgesPerNode": maxEdgesPerNode,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find nodes with more than maxEdgesPerNode CO_ACTIVATED_WITH edges
		// and delete the lowest-weight excess edges.
		// Uses a subquery approach to properly capture relationships for deletion.
		cypher := `MATCH (n:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(m:MemoryNode {space_id:$spaceId})
WITH n, r
ORDER BY n.node_id, r.weight DESC
WITH n, collect(r) AS edges
WHERE size(edges) > $maxEdgesPerNode
WITH n, edges[$maxEdgesPerNode..] AS toDelete
UNWIND toDelete AS r
DELETE r
RETURN count(*) AS deleted`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			rec := res.Record()
			deleted, _ := rec.Get("deleted")
			if d, ok := deleted.(int64); ok {
				return d, nil
			}
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	typed, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// GetLearningEdgeStats returns statistics about CO_ACTIVATED_WITH edges for a space.
// Uses evidence-based decay in calculations.
func (s *Service) GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error) {
	if spaceID == "" {
		return nil, nil
	}

	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05
	}

	params := map[string]any{
		"spaceId":        spaceID,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Uses evidence-based decay with surprise factor: effectiveDecay = baseDecay / sqrt(evidenceCount * surpriseFactor)
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId})-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b:MemoryNode {space_id:$spaceId})
WITH r,
     duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days AS daysSinceActive,
     coalesce(r.weight, 0.0) AS rawWeight,
     coalesce(r.evidence_count, 1) AS evidenceCount,
     coalesce(r.surprise_factor, 1.0) AS surpriseFactor
WITH r, daysSinceActive, rawWeight, evidenceCount, surpriseFactor,
     CASE WHEN daysSinceActive > 0 THEN
       rawWeight * ((1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) ^ daysSinceActive)
     ELSE rawWeight END AS decayedWeight
RETURN count(r) AS total_edges,
       avg(rawWeight) AS avg_raw_weight,
       avg(decayedWeight) AS avg_decayed_weight,
       avg(evidenceCount) AS avg_evidence_count,
       max(evidenceCount) AS max_evidence_count,
       avg(surpriseFactor) AS avg_surprise_factor,
       max(surpriseFactor) AS max_surprise_factor,
       sum(CASE WHEN evidenceCount >= 5 THEN 1 ELSE 0 END) AS strong_edges,
       sum(CASE WHEN surpriseFactor >= 1.5 THEN 1 ELSE 0 END) AS surprising_edges,
       sum(CASE WHEN decayedWeight < $pruneThreshold THEN 1 ELSE 0 END) AS edges_below_threshold,
       avg(daysSinceActive) AS avg_days_since_active,
       max(daysSinceActive) AS max_days_since_active`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			stats := make(map[string]any)
			for _, key := range rec.Keys {
				val, _ := rec.Get(key)
				stats[key] = val
			}
			return stats, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	typed, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}
