package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/nrednav/cuid2"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/metrics"
	"mdemg/internal/sanitize"
)

// LearningService defines the interface for the learning service
// This allows dependency injection without circular imports
type LearningService interface {
	CoactivateSession(ctx context.Context, spaceID, sessionID string) error
}

// ConstraintGateClassifier is a thin interface for F6a: LLM classifier gate.
// Implemented by consulting.ConstraintClassifier — defined here to avoid circular imports.
type ConstraintGateClassifier interface {
	// Classify returns the constraint classification for the given text.
	// nodeID is used as a cache key. Returns nil, nil when disabled or uncacheable.
	Classify(ctx context.Context, nodeID, text string) (*ConstraintGateResult, error)
}

// ConstraintGateResult holds the classification result used by F6a.
type ConstraintGateResult struct {
	Type string // "must", "must_not", "should", "should_not", "none"
}

// ConstraintCodeGen generates mnemonic T1 codes for detected constraints.
// Implemented by jiminy.ConstraintCodeGenerator — defined here to avoid circular imports.
type ConstraintCodeGen interface {
	GenerateCode(ctx context.Context, constraintType, description string) (string, error)
}

// Service handles conversation observation capture and surprise detection
type Service struct {
	driver                   neo4j.DriverWithContext
	embedder                 Embedder
	surpriseDetector         *SurpriseDetector
	learningService          LearningService
	vectorIndexName          string
	constraintDetector       *ConstraintDetector
	constraintDetEnabled     bool
	constraintGateClassifier ConstraintGateClassifier // F6a: optional LLM gate
	codeGenerator            ConstraintCodeGen         // J17-2: optional constraint code generator
	cfg                      config.Config
}

// NewService creates a new conversation service
func NewService(driver neo4j.DriverWithContext, embedder Embedder) *Service {
	return NewServiceWithConfig(driver, embedder, "memNodeEmbedding", config.Config{ConstraintDetectionEnabled: true, ConstraintMinConfidence: 0.6})
}

// NewServiceWithConfig creates a new conversation service with configurable index name
func NewServiceWithConfig(driver neo4j.DriverWithContext, embedder Embedder, vectorIndexName string, cfg ...config.Config) *Service {
	var surpriseDet *SurpriseDetector
	if embedder != nil {
		if len(cfg) > 0 {
			surpriseDet = NewSurpriseDetectorWithConfig(embedder, driver, cfg[0])
		} else {
			surpriseDet = NewSurpriseDetector(embedder, driver)
		}
	}

	if vectorIndexName == "" {
		vectorIndexName = "memNodeEmbedding"
	}

	svc := &Service{
		driver:           driver,
		embedder:         embedder,
		surpriseDetector: surpriseDet,
		learningService:  nil, // Set via SetLearningService to avoid circular dependency
		vectorIndexName:  vectorIndexName,
	}

	// Initialize constraint detection if config is provided
	if len(cfg) > 0 {
		svc.cfg = cfg[0]
		if cfg[0].ConstraintDetectionEnabled {
			svc.constraintDetEnabled = true
			svc.constraintDetector = NewConstraintDetector(cfg[0].ConstraintMinConfidence)
		}
	}

	return svc
}

// SetLearningService injects the learning service (to avoid circular imports)
func (s *Service) SetLearningService(learningService LearningService) {
	s.learningService = learningService
}

// SetConstraintGateClassifier injects the optional F6a LLM classifier gate.
// When set and cfg.ConstraintClassifierGateEnabled is true, each regex-detected
// constraint is confirmed by the LLM before being promoted to a constraint tag.
func (s *Service) SetConstraintGateClassifier(c ConstraintGateClassifier) {
	s.constraintGateClassifier = c
}

// SetCodeGenerator injects the J17-2 constraint code generator.
func (s *Service) SetCodeGenerator(gen ConstraintCodeGen) {
	s.codeGenerator = gen
}

// ObserveRequest is the request for capturing an observation
type ObserveRequest struct {
	SpaceID   string         `json:"space_id"`
	SessionID string         `json:"session_id"`
	Content   string         `json:"content"`
	ObsType   string         `json:"obs_type,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`

	// Identity & Visibility (CMS v2)
	UserID     string `json:"user_id,omitempty"`
	Visibility string `json:"visibility,omitempty"` // private|team|global

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Persistent agent identity

	// Cross-module linking (CMS v2)
	RefersTo []string `json:"refers_to,omitempty"` // Node/symbol IDs this observation references

	// Structured Observations (Phase 60)
	TemplateID     string         `json:"template_id,omitempty"`     // Template to use for structured observation
	StructuredData map[string]any `json:"structured_data,omitempty"` // Template-validated data

	// Pinned Observations (Phase 47)
	Pinned bool `json:"pinned,omitempty"` // Create as permanent, non-decaying
}

// ObserveResponse is the response from capturing an observation
type ObserveResponse struct {
	ObsID               string               `json:"obs_id"`
	NodeID              string               `json:"node_id"`
	SurpriseScore       float64              `json:"surprise_score"`
	SurpriseFactors     map[string]float64   `json:"surprise_factors"`
	Summary             string               `json:"summary,omitempty"`
	DetectedConstraints []DetectedConstraint `json:"detected_constraints,omitempty"`
}

// CorrectRequest is the request for capturing an explicit correction
type CorrectRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
	Context   string `json:"context,omitempty"`

	// Identity & Visibility (CMS v2)
	UserID     string `json:"user_id,omitempty"`
	Visibility string `json:"visibility,omitempty"` // private|team|global

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Persistent agent identity

	// Cross-module linking (CMS v2)
	RefersTo []string `json:"refers_to,omitempty"` // Node/symbol IDs this correction references
}

// calculateInitialImportance returns an initial importance score based on observation type
func calculateInitialImportance(obsType ObservationType) float64 {
	switch obsType {
	case ObsTypeCorrection:
		return 0.9 // Corrections are high priority
	case ObsTypeDecision:
		return 0.8 // Decisions are important
	case ObsTypeError:
		return 0.75 // Errors need tracking
	case ObsTypeBlocker:
		return 0.8 // Blockers are critical
	case ObsTypeContext:
		return 0.7 // Context for continuity
	case ObsTypeLearning:
		return 0.6 // Learnings are moderate
	case ObsTypeInsight:
		return 0.65 // Insights are valuable
	case ObsTypePreference:
		return 0.5 // Preferences are useful
	case ObsTypeProgress:
		return 0.4 // Progress updates are less critical
	case ObsTypeTechnicalNote:
		return 0.5 // Technical notes are moderate
	case ObsTypeTask:
		return 0.7 // Tasks are important for continuity
	case ObsTypeConstraint:
		return 0.9 // Constraints are highest priority
	case ObsTypeSelfImprovement:
		return 0.45 // RSIC data is moderate
	case ObsTypeNote:
		return 0.4 // Notes are low priority
	case ObsTypeContextSignal:
		return 0.1 // Telemetry is background noise
	default:
		return 0.5 // Default moderate importance
	}
}

// Observe captures a conversation observation
func (s *Service) Observe(ctx context.Context, req ObserveRequest) (*ObserveResponse, error) {
	// Generate unique IDs (CUIDv2)
	obsID := cuid2.Generate()
	nodeID := cuid2.Generate()

	// Validate observation type
	obsType := ObservationType(req.ObsType)
	if req.ObsType == "" {
		obsType = ObsTypeLearning // Default type
	}

	// Validate observation type value
	validTypes := map[ObservationType]bool{
		ObsTypeDecision:        true,
		ObsTypeCorrection:      true,
		ObsTypeLearning:        true,
		ObsTypePreference:      true,
		ObsTypeError:           true,
		ObsTypeTask:            true,
		ObsTypeTechnicalNote:   true,
		ObsTypeInsight:         true,
		ObsTypeContext:         true,
		ObsTypeProgress:        true,
		ObsTypeBlocker:         true,
		ObsTypeContextSignal:   true,
		ObsTypeNote:            true,
		ObsTypeConstraint:      true,
		ObsTypeSelfImprovement: true,
	}
	if !validTypes[obsType] {
		return nil, fmt.Errorf("invalid observation type: %s", req.ObsType)
	}

	// Validate visibility
	if req.Visibility != "" && !ValidVisibility(req.Visibility) {
		return nil, fmt.Errorf("invalid visibility: %s (must be private, team, or global)", req.Visibility)
	}

	// Generate embedding
	var embedding []float32
	var err error
	if s.embedder != nil {
		ctx = embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{
			CallSite:  "conversation",
			SpaceID:   req.SpaceID,
			Tags:      req.Tags,
			QueryText: req.Content,
		})
		embedding, err = s.embedder.Embed(ctx, req.Content)
		if err != nil {
			slog.Warn("embedding generation failed, dedup skipped", "error", err)
			embedding = nil
			if req.Metadata == nil {
				req.Metadata = make(map[string]any)
			}
			req.Metadata["_embedding_degraded"] = true
			req.Metadata["_embedding_error"] = err.Error()
			metrics.Metrics().CMSEmbeddingFailures.Inc()
			metrics.Metrics().CMSObserveTotal("degraded").Inc()
		}
	}

	// Deduplication check: skip if near-duplicate exists in same session
	if len(embedding) > 0 {
		dedupResult, dedupErr := CheckDuplicate(ctx, s.driver, req.SpaceID, req.SessionID, embedding, DedupThreshold)
		if dedupErr != nil {
			slog.Warn("dedup check failed", "error", dedupErr)
			// Continue without dedup
		} else if dedupResult.IsDuplicate {
			slog.Info("skipping duplicate observation", "similarity", dedupResult.Similarity, "duplicate_of", dedupResult.DuplicateOfID)
			// Merge: increment duplicate_count on existing node
			if mergeErr := MergeDuplicateObservation(ctx, s.driver, dedupResult.DuplicateOfID); mergeErr != nil {
				slog.Warn("dedup merge failed", "node_id", dedupResult.DuplicateOfID, "error", mergeErr)
				metrics.Metrics().CMSDedupMergeFails.Inc()
			}
			return &ObserveResponse{
				ObsID:         dedupResult.DuplicateOfID,
				NodeID:        dedupResult.DuplicateOfID,
				SurpriseScore: 0.0,
				SurpriseFactors: map[string]float64{
					"term_novelty":        0.0,
					"contradiction_score": 0.0,
					"correction_score":    0.0,
					"embedding_novelty":   0.0,
				},
				Summary: "duplicate observation (merged with existing)",
			}, nil
		}
	}

	// Determine visibility (default to private)
	visibility := Visibility(req.Visibility)
	if visibility == "" {
		visibility = VisibilityPrivate
	}

	// Create observation object
	// New observations start as volatile with low stability score
	now := time.Now().UTC()
	obs := Observation{
		ObsID:           obsID,
		SpaceID:         req.SpaceID,
		SessionID:       req.SessionID,
		ObsType:         obsType,
		Content:         req.Content,
		Embedding:       embedding,
		Tags:            req.Tags,
		Metadata:        req.Metadata,
		CreatedAt:       now,
		UserID:          req.UserID,
		Visibility:      visibility,
		Volatile:        !req.Pinned, // Pinned observations are immediately permanent
		StabilityScore:  pinnedStabilityScore(req.Pinned),
		Pinned:          req.Pinned,
		AgentID:         req.AgentID,
		TemplateID:      req.TemplateID,
		StructuredData:  req.StructuredData,
		ImportanceScore: calculateInitialImportance(obsType), // Phase 60
		Tier:            "important",                          // Default tier
		LastAccessedAt:  now,
		OrgReviewStatus: "none",
	}

	// Generate summary
	obs.Summary = generateSummary(req.Content, s.cfg.CMSSummaryMaxChars)

	// Compute surprise score
	var surpriseScore float64
	var factors SurpriseFactors

	if s.surpriseDetector != nil {
		surpriseScore, factors, err = s.surpriseDetector.DetectSurprise(ctx, obs)
		if err != nil {
			slog.Warn("failed to compute surprise score", "error", err)
			// Continue with default score
			surpriseScore = 0.0
		}
	}

	obs.SurpriseScore = surpriseScore

	// Build tags
	tags := buildObservationTags(req, obsType)

	// Constraint detection (Phase 45.5)
	var detectedConstraints []DetectedConstraint
	if s.constraintDetEnabled && s.constraintDetector != nil {
		detectedConstraints = s.constraintDetector.Detect(req.Content, obsType)

		// F6a: LLM classifier gate — confirm regex detection before tagging.
		// If the gate is enabled and the classifier rejects (type == "none"), drop the
		// detection so the observation is not promoted to a constraint node.
		if len(detectedConstraints) > 0 &&
			s.cfg.ConstraintClassifierGateEnabled &&
			s.constraintGateClassifier != nil {

			classification, classErr := s.constraintGateClassifier.Classify(ctx, nodeID, req.Content)
			if classErr != nil {
				slog.Warn("F6a constraint classifier gate failed, passing through", "error", classErr)
				// fail-open: keep detectedConstraints as-is
			} else if classification != nil && classification.Type == "none" {
				slog.Info("F6a: classifier rejected regex constraint detection, skipping promotion", "obs_id", obsID)
				detectedConstraints = nil
			}
		}

		if len(detectedConstraints) > 0 {
			for _, dc := range detectedConstraints {
				tags = append(tags, "constraint:"+dc.ConstraintType)
			}
			// Boost importance for constraint-tagged observations
			if obs.ImportanceScore < 0.8 {
				obs.ImportanceScore = 0.8
			}
			// Set tier based on constraint type
			for _, dc := range detectedConstraints {
				if dc.ConstraintType == "must" || dc.ConstraintType == "must_not" {
					obs.Tier = "critical"
					break
				}
				if dc.ConstraintType == "should" || dc.ConstraintType == "should_not" {
					if obs.Tier != "critical" {
						obs.Tier = "important"
					}
				}
			}
			// Store constraint metadata in structured data
			if obs.StructuredData == nil {
				obs.StructuredData = make(map[string]any)
			}
			constraintMeta := make([]map[string]any, len(detectedConstraints))
			for i, dc := range detectedConstraints {
				constraintMeta[i] = map[string]any{
					"constraint_type": dc.ConstraintType,
					"name":            dc.Name,
					"confidence":      dc.Confidence,
				}
			}
			obs.StructuredData["detected_constraints"] = constraintMeta
			slog.Info("constraint detection complete", "count", len(detectedConstraints), "obs_id", obsID)

			// J17-2: Generate T1 mnemonic code for first detected constraint
			if s.codeGenerator != nil && len(detectedConstraints) > 0 {
				code, codeErr := s.codeGenerator.GenerateCode(ctx, detectedConstraints[0].ConstraintType, req.Content)
				if codeErr != nil {
					slog.Warn("J17 codegen failed", "error", codeErr)
				} else if code != "" {
					obs.StructuredData["constraint_code"] = code
					constraintMeta[0]["constraint_code"] = code
				}
			}
		}
	}

	// Create MemoryNode in Neo4j
	err = s.createObservationNode(ctx, nodeID, obs, tags)
	if err != nil {
		return nil, fmt.Errorf("create observation node: %w", err)
	}

	// J17-2: Set constraint_code property on Neo4j node (separate SET for backward compat)
	if obs.StructuredData != nil {
		if code, ok := obs.StructuredData["constraint_code"].(string); ok && code != "" {
			s.setConstraintCode(ctx, nodeID, code)
		}
	}

	slog.Info("created conversation observation",
		"obs_id", obsID, "node_id", nodeID, "type", obsType, "surprise", surpriseScore)

	// Create REFERS_TO edges for cross-module linking
	if len(req.RefersTo) > 0 {
		edgesCreated, err := s.createRefersToEdges(ctx, req.SpaceID, nodeID, req.RefersTo)
		if err != nil {
			// Log but don't fail - references are an enhancement
			slog.Warn("failed to create REFERS_TO edges", "error", err)
		} else if edgesCreated > 0 {
			slog.Info("created REFERS_TO edges", "count", edgesCreated, "node_id", nodeID)
		}
	}

	// Trigger session-based coactivation if learning service is available
	// This creates CO_ACTIVATED_WITH edges between observations in the same session
	if s.learningService != nil && req.SessionID != "" {
		err = s.learningService.CoactivateSession(ctx, req.SpaceID, req.SessionID)
		if err != nil {
			// Log but don't fail - coactivation is a learning enhancement, not critical
			slog.Warn("failed to coactivate session", "session_id", req.SessionID, "error", err)
		}
	}

	resp := &ObserveResponse{
		ObsID:         obsID,
		NodeID:        nodeID,
		SurpriseScore: surpriseScore,
		SurpriseFactors: map[string]float64{
			"term_novelty":        factors.TermNovelty,
			"contradiction_score": factors.ContradictionScore,
			"correction_score":    factors.CorrectionScore,
			"embedding_novelty":   factors.EmbeddingNovelty,
		},
		Summary: obs.Summary,
	}
	if len(detectedConstraints) > 0 {
		resp.DetectedConstraints = detectedConstraints
	}
	return resp, nil
}

// Correct captures an explicit correction (sets high surprise)
func (s *Service) Correct(ctx context.Context, req CorrectRequest) (*ObserveResponse, error) {
	// Build content string
	content := fmt.Sprintf("CORRECTION: Incorrect: %s | Correct: %s", req.Incorrect, req.Correct)
	if req.Context != "" {
		content += fmt.Sprintf(" | Context: %s", req.Context)
	}

	// Create observation with correction type
	obsReq := ObserveRequest{
		SpaceID:   req.SpaceID,
		SessionID: req.SessionID,
		Content:   content,
		ObsType:   string(ObsTypeCorrection),
		Tags:      []string{"correction"},
		Metadata: map[string]any{
			"incorrect": req.Incorrect,
			"correct":   req.Correct,
			"context":   req.Context,
		},
		UserID:     req.UserID,
		Visibility: req.Visibility,
		RefersTo:   req.RefersTo,
	}

	resp, err := s.Observe(ctx, obsReq)
	if err != nil {
		return nil, err
	}

	// Ensure high surprise score for corrections
	if resp.SurpriseScore < 0.9 {
		resp.SurpriseScore = 0.9
		// Update node with corrected surprise score
		err = s.updateSurpriseScore(ctx, resp.NodeID, 0.9)
		if err != nil {
			slog.Warn("failed to update surprise score", "error", err)
		}
	}

	return resp, nil
}

// createObservationNode creates a MemoryNode with role_type="conversation_observation"
func (s *Service) createObservationNode(ctx context.Context, nodeID string, obs Observation, tags []string) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			CREATE (n:MemoryNode:ConversationObs {
				node_id: $nodeId,
				space_id: $spaceId,
				role_type: 'conversation_observation',
				obs_id: $obsId,
				session_id: $sessionId,
				obs_type: $obsType,
				content: $content,
				summary: $summary,
				embedding: $embedding,
				surprise_score: $surpriseScore,
				tags: $tags,
				layer: 0,
				user_id: $userId,
				agent_id: $agentId,
				visibility: $visibility,
				volatile: $volatile,
				stability_score: $stabilityScore,
				pinned: $pinned,
				template_id: $templateId,
				structured_data: $structuredData,
				importance_score: $importanceScore,
				tier: $tier,
				last_accessed_at: datetime($lastAccessedAt),
				org_review_status: $orgReviewStatus,
				created_at: datetime($createdAt),
				updated_at: datetime($createdAt)
			})
			RETURN n.node_id as nodeId
		`

		// Default visibility to private if not set
		visibility := obs.Visibility
		if visibility == "" {
			visibility = VisibilityPrivate
		}

		// New observations start as volatile with low stability
		stabilityScore := obs.StabilityScore
		if stabilityScore == 0 {
			stabilityScore = DefaultStabilityScore
		}

		// Serialize structured data to JSON string
		var structuredDataStr string
		if obs.StructuredData != nil && len(obs.StructuredData) > 0 {
			if data, err := json.Marshal(obs.StructuredData); err == nil {
				structuredDataStr = string(data)
			}
		}

		// Set default tier if not specified
		tier := obs.Tier
		if tier == "" {
			tier = "important"
		}

		// Set last accessed time
		lastAccessedAt := obs.LastAccessedAt
		if lastAccessedAt.IsZero() {
			lastAccessedAt = obs.CreatedAt
		}

		params := map[string]any{
			"nodeId":          nodeID,
			"spaceId":         obs.SpaceID,
			"obsId":           obs.ObsID,
			"sessionId":       obs.SessionID,
			"obsType":         string(obs.ObsType),
			"content":         obs.Content,
			"summary":         obs.Summary,
			"embedding":       obs.Embedding,
			"surpriseScore":   obs.SurpriseScore,
			"tags":            tags,
			"userId":          obs.UserID,
			"agentId":         obs.AgentID,
			"visibility":      string(visibility),
			"volatile":        obs.Volatile,
			"stabilityScore":  stabilityScore,
			"pinned":          obs.Pinned,
			"templateId":      obs.TemplateID,
			"structuredData":  structuredDataStr,
			"importanceScore": obs.ImportanceScore,
			"tier":            tier,
			"lastAccessedAt":  lastAccessedAt.Format(time.RFC3339),
			"orgReviewStatus": obs.OrgReviewStatus,
			"createdAt":       obs.CreatedAt.Format(time.RFC3339),
		}

		// Add metadata as properties if present
		if obs.Metadata != nil && len(obs.Metadata) > 0 {
			for k, v := range obs.Metadata {
				params["metadata_"+k] = v
			}
		}

		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		return result.Consume(ctx)
	})

	return err
}

// setConstraintCode sets the constraint_code property on a Neo4j node (J17-2).
// Runs as a separate SET query to avoid modifying the existing CREATE cypher.
func (s *Service) setConstraintCode(ctx context.Context, nodeID, code string) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {node_id: $nodeId})
			SET n.constraint_code = $code
		`
		result, err := tx.Run(ctx, cypher, map[string]any{
			"nodeId": nodeID,
			"code":   code,
		})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	if err != nil {
		slog.Warn("J17-2: failed to set constraint_code", "node_id", nodeID, "error", err)
	}
}

// createRefersToEdges creates REFERS_TO edges from an observation to referenced nodes/symbols.
// This enables cross-module linking between conversation observations and code elements.
func (s *Service) createRefersToEdges(ctx context.Context, spaceID, fromNodeID string, referenceIDs []string) (int, error) {
	if len(referenceIDs) == 0 {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Create REFERS_TO edges to any existing nodes/symbols with matching IDs
		// This will link to both MemoryNodes and SymbolNodes
		cypher := `
			MATCH (from:MemoryNode {space_id: $spaceId, node_id: $fromNodeId})
			UNWIND $referenceIds as refId
			OPTIONAL MATCH (memNode:MemoryNode {space_id: $spaceId, node_id: refId})
			OPTIONAL MATCH (symNode:SymbolNode {space_id: $spaceId, symbol_id: refId})
			WITH from, refId, coalesce(memNode, symNode) as target
			WHERE target IS NOT NULL
			MERGE (from)-[r:REFERS_TO]->(target)
			ON CREATE SET r.created_at = datetime($now),
			              r.evidence_count = 1
			ON MATCH SET r.evidence_count = r.evidence_count + 1,
			             r.updated_at = datetime($now)
			RETURN count(r) as edgesCreated
		`

		params := map[string]any{
			"spaceId":      spaceID,
			"fromNodeId":   fromNodeID,
			"referenceIds": referenceIDs,
			"now":          time.Now().UTC().Format(time.RFC3339),
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			count, _ := res.Record().Get("edgesCreated")
			typed, ok := count.(int64)
			if !ok {
				return int64(0), fmt.Errorf("unexpected edgesCreated type: %T", count)
			}
			return typed, nil
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
	return int(typed), nil
}

// GetReferencesFromObservation retrieves all nodes/symbols referenced by an observation.
func (s *Service) GetReferencesFromObservation(ctx context.Context, spaceID, observationNodeID string) ([]ReferenceResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, node_id: $nodeId})-[r:REFERS_TO]->(target)
			RETURN target.node_id as nodeId,
			       coalesce(target.symbol_id, '') as symbolId,
			       coalesce(target.name, target.content, '') as name,
			       labels(target) as labels,
			       r.evidence_count as evidenceCount
		`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  observationNodeID,
		})
		if err != nil {
			return nil, err
		}

		var refs []ReferenceResult
		for res.Next(ctx) {
			rec := res.Record()
			refs = append(refs, ReferenceResult{
				NodeID:        asString(rec, "nodeId"),
				SymbolID:      asString(rec, "symbolId"),
				Name:          asString(rec, "name"),
				Type:          getNodeType(asStringSlice(rec, "labels")),
				EvidenceCount: int(asInt64(rec, "evidenceCount")),
			})
		}
		return refs, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]ReferenceResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// GetObservationsReferencingNode retrieves observations that reference a specific node/symbol.
func (s *Service) GetObservationsReferencingNode(ctx context.Context, spaceID, targetID string) ([]ObservationResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for observations referencing either a MemoryNode or SymbolNode
		cypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})-[:REFERS_TO]->(target)
			WHERE target.node_id = $targetId OR target.symbol_id = $targetId
			RETURN obs.node_id as nodeId,
			       obs.obs_type as obsType,
			       obs.content as content,
			       obs.summary as summary,
			       obs.session_id as sessionId,
			       obs.surprise_score as surpriseScore,
			       obs.tags as tags,
			       toString(obs.created_at) as createdAt
			ORDER BY obs.created_at DESC
		`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":  spaceID,
			"targetId": targetID,
		})
		if err != nil {
			return nil, err
		}

		var observations []ObservationResult
		for res.Next(ctx) {
			rec := res.Record()
			observations = append(observations, ObservationResult{
				NodeID:        asString(rec, "nodeId"),
				ObsType:       asString(rec, "obsType"),
				Content:       asString(rec, "content"),
				Summary:       asString(rec, "summary"),
				SessionID:     asString(rec, "sessionId"),
				SurpriseScore: asFloat64(rec, "surpriseScore"),
				Tags:          asStringSlice(rec, "tags"),
				CreatedAt:     asString(rec, "createdAt"),
			})
		}
		return observations, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]ObservationResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// ReferenceResult represents a target of a REFERS_TO relationship
type ReferenceResult struct {
	NodeID        string `json:"node_id,omitempty"`
	SymbolID      string `json:"symbol_id,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"` // memory_node, symbol_node
	EvidenceCount int    `json:"evidence_count"`
}

// getNodeType determines the type of node from its labels
func getNodeType(labels []string) string {
	for _, label := range labels {
		if label == "SymbolNode" {
			return "symbol"
		}
		if label == "MemoryNode" {
			return "memory"
		}
	}
	return "unknown"
}

// updateSurpriseScore updates the surprise score for a node
func (s *Service) updateSurpriseScore(ctx context.Context, nodeID string, score float64) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {node_id: $nodeId})
			SET n.surprise_score = $score,
			    n.updated_at = datetime($updatedAt)
			RETURN n.node_id as nodeId
		`

		params := map[string]any{
			"nodeId":    nodeID,
			"score":     score,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}

		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		return result.Consume(ctx)
	})

	return err
}

// generateSummary creates a brief summary from content (max 200 chars)
// pinnedStabilityScore returns max stability for pinned observations, default otherwise.
func pinnedStabilityScore(pinned bool) float64 {
	if pinned {
		return 1.0
	}
	return DefaultStabilityScore
}

func generateSummary(content string, maxLenOverride ...int) string {
	maxLen := 200
	if len(maxLenOverride) > 0 && maxLenOverride[0] > 0 {
		maxLen = maxLenOverride[0]
	}

	// Clean whitespace
	content = strings.TrimSpace(content)
	content = strings.Join(strings.Fields(content), " ")

	if len(content) <= maxLen {
		return content
	}

	// Truncate at word boundary
	truncated := content[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// buildObservationTags builds the tag list for an observation
func buildObservationTags(req ObserveRequest, obsType ObservationType) []string {
	tags := []string{
		"conversation",
		"session:" + req.SessionID,
		"obs_type:" + string(obsType),
	}

	// Add custom tags
	if req.Tags != nil {
		tags = append(tags, req.Tags...)
	}

	return tags
}

// =============================================================================
// PHASE 5: RESUME AND RECALL
// =============================================================================

// ResumeRequest is the request for resuming context after compaction
type ResumeRequest struct {
	SpaceID          string `json:"space_id"`
	SessionID        string `json:"session_id,omitempty"`
	IncludeTasks     bool   `json:"include_tasks,omitempty"`
	IncludeDecisions bool   `json:"include_decisions,omitempty"`
	IncludeLearnings bool   `json:"include_learnings,omitempty"`
	MaxObservations  int    `json:"max_observations,omitempty"`

	// Visibility filtering (CMS v2)
	RequestingUserID string `json:"requesting_user_id,omitempty"`

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Resume across sessions for this agent
}

// JiminyRationale explains WHY specific state was rehydrated during resume.
// Named after Jiminy Cricket - the "conscience" that provides guidance.
type JiminyRationale struct {
	Rationale      string             `json:"rationale"`
	Confidence     float64            `json:"confidence"`
	ScoreBreakdown map[string]float64 `json:"score_breakdown"`
	Highlights     []string           `json:"highlights"`
}

// ResumeResponse is the response from resuming context
type ResumeResponse struct {
	SpaceID          string                  `json:"space_id"`
	SessionID        string                  `json:"session_id,omitempty"`
	Observations     []ObservationResult     `json:"observations"`
	Themes           []ThemeResult           `json:"themes"`
	EmergentConcepts []EmergentConceptResult `json:"emergent_concepts"`
	Summary          string                  `json:"summary"`
	Jiminy           *JiminyRationale        `json:"jiminy,omitempty"`
	Debug            map[string]any          `json:"debug,omitempty"`
}

// ObservationResult represents an observation in resume/recall responses
type ObservationResult struct {
	NodeID        string   `json:"node_id"`
	ObsType       string   `json:"obs_type"`
	Content       string   `json:"content"`
	Summary       string   `json:"summary"`
	SessionID     string   `json:"session_id"`
	SurpriseScore float64  `json:"surprise_score"`
	Score         float64  `json:"score,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

// ThemeResult represents a conversation theme in resume/recall responses
type ThemeResult struct {
	NodeID           string  `json:"node_id"`
	Name             string  `json:"name"`
	Summary          string  `json:"summary"`
	MemberCount      int     `json:"member_count"`
	DominantObsType  string  `json:"dominant_obs_type,omitempty"`
	AvgSurpriseScore float64 `json:"avg_surprise_score"`
	Score            float64 `json:"score,omitempty"`
}

// EmergentConceptResult represents an emergent concept in resume/recall responses
type EmergentConceptResult struct {
	NodeID       string   `json:"node_id"`
	Name         string   `json:"name"`
	Summary      string   `json:"summary"`
	Layer        int      `json:"layer"`
	Keywords     []string `json:"keywords,omitempty"`
	SessionCount int      `json:"session_count,omitempty"`
	Score        float64  `json:"score,omitempty"`
}

// RecallRequest is the request for recalling conversation knowledge
type RecallRequest struct {
	SpaceID         string    `json:"space_id"`
	Query           string    `json:"query"`
	QueryEmbedding  []float32 `json:"query_embedding,omitempty"`
	TopK            int       `json:"top_k,omitempty"`
	IncludeThemes   bool      `json:"include_themes,omitempty"`
	IncludeConcepts bool      `json:"include_concepts,omitempty"`

	// Visibility filtering (CMS v2)
	RequestingUserID string `json:"requesting_user_id,omitempty"`

	// Multi-Agent Identity (CMS v3)
	AgentID string `json:"agent_id,omitempty"` // Filter to this agent's observations

	// Temporal filtering (Phase 1: Time-Aware Retrieval)
	TemporalAfter  string `json:"temporal_after,omitempty"`  // ISO8601: filter results after this time
	TemporalBefore string `json:"temporal_before,omitempty"` // ISO8601: filter results before this time

	// Tag filtering (Phase 47: Pinned Observations)
	FilterTags []string `json:"filter_tags,omitempty"`
}

// RecallResponse is the response from conversation recall
type RecallResponse struct {
	SpaceID string         `json:"space_id"`
	Query   string         `json:"query"`
	Results []RecallResult `json:"results"`
	Debug   map[string]any `json:"debug,omitempty"`
}

// RecallResult represents a single result from conversation recall
type RecallResult struct {
	Type     string         `json:"type"`
	NodeID   string         `json:"node_id"`
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Layer    int            `json:"layer"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Resume restores context after context compaction by retrieving
// recent observations, themes, and emergent concepts.
func (s *Service) Resume(ctx context.Context, req ResumeRequest) (*ResumeResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	defaultMaxObs := s.cfg.CMSResumeMaxObs
	if defaultMaxObs <= 0 {
		defaultMaxObs = 20
	}
	maxObs := req.MaxObservations
	if maxObs <= 0 {
		maxObs = defaultMaxObs
	}

	resp := &ResumeResponse{
		SpaceID:          req.SpaceID,
		SessionID:        req.SessionID,
		Observations:     []ObservationResult{},
		Themes:           []ThemeResult{},
		EmergentConcepts: []EmergentConceptResult{},
		Debug:            make(map[string]any),
	}

	// Step 1: Fetch recent observations (with visibility and agent filtering)
	observations, err := s.fetchRecentObservations(ctx, req.SpaceID, req.SessionID, req.RequestingUserID, req.AgentID, maxObs, req.IncludeTasks, req.IncludeDecisions, req.IncludeLearnings)
	if err != nil {
		return nil, fmt.Errorf("fetch recent observations: %w", err)
	}
	resp.Observations = observations
	resp.Debug["observation_count"] = len(observations)

	// Step 2: Fetch related themes (themes that these observations belong to)
	themes, err := s.fetchRelatedThemes(ctx, req.SpaceID, req.SessionID)
	if err != nil {
		slog.Warn("failed to fetch related themes", "error", err)
	} else {
		resp.Themes = themes
		resp.Debug["theme_count"] = len(themes)
	}

	// Step 3: Fetch emergent concepts (higher-level abstractions)
	concepts, err := s.fetchEmergentConcepts(ctx, req.SpaceID)
	if err != nil {
		slog.Warn("failed to fetch emergent concepts", "error", err)
	} else {
		resp.EmergentConcepts = concepts
		resp.Debug["concept_count"] = len(concepts)
	}

	// Step 4: Generate context summary
	resp.Summary = s.generateResumeSummary(resp)

	// Step 5: Generate Jiminy rationale (explains WHY this state was rehydrated)
	resp.Jiminy = s.generateJiminyRationale(resp)

	return resp, nil
}

// Recall retrieves relevant conversation knowledge via semantic query
// Uses vector similarity and spreading activation through the concept hierarchy.
func (s *Service) Recall(ctx context.Context, req RecallRequest) (*RecallResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("query or query_embedding is required")
	}

	defaultTopK := s.cfg.CMSRecallTopK
	if defaultTopK <= 0 {
		defaultTopK = 10
	}
	topK := req.TopK
	if topK <= 0 {
		topK = defaultTopK
	}

	resp := &RecallResponse{
		SpaceID: req.SpaceID,
		Query:   req.Query,
		Results: []RecallResult{},
		Debug:   make(map[string]any),
	}

	// Build temporal filter for Cypher queries
	temporalFilter, temporalParams := buildTemporalCypherFilter(req.TemporalAfter, req.TemporalBefore)
	if temporalFilter != "" {
		resp.Debug["temporal_filter"] = temporalFilter
	}

	// Get or generate query embedding
	var embedding []float32
	if len(req.QueryEmbedding) > 0 {
		embedding = req.QueryEmbedding
	} else if s.embedder != nil {
		var err error
		embedding, err = s.embedder.Embed(ctx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("generate embedding: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no embedding provided and embedder not available")
	}

	// Step 1: Find similar conversation observations (with visibility, agent, and temporal filtering)
	obsResults, err := s.findSimilarObservations(ctx, req.SpaceID, req.RequestingUserID, req.AgentID, embedding, topK, temporalFilter, temporalParams, req.FilterTags)
	if err != nil {
		return nil, fmt.Errorf("find similar observations: %w", err)
	}
	resp.Results = append(resp.Results, obsResults...)
	resp.Debug["observation_matches"] = len(obsResults)

	// Step 2: Find similar themes (if enabled)
	if req.IncludeThemes {
		themeResults, err := s.findSimilarThemes(ctx, req.SpaceID, embedding, topK, temporalFilter, temporalParams)
		if err != nil {
			slog.Warn("failed to find similar themes", "error", err)
		} else {
			resp.Results = append(resp.Results, themeResults...)
			resp.Debug["theme_matches"] = len(themeResults)
		}
	}

	// Step 3: Find similar emergent concepts (if enabled)
	if req.IncludeConcepts {
		conceptResults, err := s.findSimilarConcepts(ctx, req.SpaceID, embedding, topK, temporalFilter, temporalParams)
		if err != nil {
			slog.Warn("failed to find similar concepts", "error", err)
		} else {
			resp.Results = append(resp.Results, conceptResults...)
			resp.Debug["concept_matches"] = len(conceptResults)
		}
	}

	// Step 4: Sort by score and limit to topK total
	sortAndLimitRecallResults(&resp.Results, topK)

	return resp, nil
}

// buildTemporalCypherFilter constructs a Cypher WHERE clause fragment and params
// for temporal filtering on node.created_at.
func buildTemporalCypherFilter(afterStr, beforeStr string) (string, map[string]any) {
	params := make(map[string]any)
	clauses := []string{}

	if afterStr != "" {
		t, err := time.Parse(time.RFC3339, afterStr)
		if err != nil {
			t, err = time.Parse("2006-01-02", afterStr)
		}
		if err == nil {
			clauses = append(clauses, " AND node.created_at >= datetime($temporalAfter)")
			params["temporalAfter"] = t.Format(time.RFC3339)
		}
	}

	if beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			t, err = time.Parse("2006-01-02", beforeStr)
		}
		if err == nil {
			clauses = append(clauses, " AND node.created_at < datetime($temporalBefore)")
			params["temporalBefore"] = t.Format(time.RFC3339)
		}
	}

	return strings.Join(clauses, ""), params
}

// fetchRecentObservations retrieves recent conversation observations
// If requestingUserID is set, applies visibility filtering (private observations only visible to owner)
func (s *Service) fetchRecentObservations(ctx context.Context, spaceID, sessionID, requestingUserID, agentID string, limit int, includeTasks, includeDecisions, includeLearnings bool) ([]ObservationResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build obs_type filter if any specific types requested
	var typeFilter string
	typeFilterEnabled := includeTasks || includeDecisions || includeLearnings
	if typeFilterEnabled {
		types := []string{}
		if includeTasks {
			types = append(types, "'task'")
		}
		if includeDecisions {
			types = append(types, "'decision'")
		}
		if includeLearnings {
			types = append(types, "'learning'")
		}
		typeFilter = fmt.Sprintf(" AND o.obs_type IN [%s]", strings.Join(types, ", "))
	}

	// Build session filter
	// When agentID is set, skip session filter to enable cross-session resume
	sessionFilter := ""
	if sessionID != "" && agentID == "" {
		sessionFilter = " AND o.session_id = $sessionId"
	}

	// Build agent filter (CMS v3: multi-agent identity)
	agentFilter := ""
	if agentID != "" {
		agentFilter = " AND o.agent_id = $agentId"
	}

	// Build visibility filter - private observations only visible to owner
	// When agentID is set, use agent_id for ownership check instead of user_id
	visibilityFilter := ""
	if agentID != "" {
		visibilityFilter = " AND (coalesce(o.visibility, 'global') <> 'private' OR o.agent_id = $agentId)"
	} else if requestingUserID != "" {
		visibilityFilter = " AND (coalesce(o.visibility, 'global') <> 'private' OR o.user_id = $requestingUserId)"
	}

	// Relevance-weighted query: scores observations by recency, surprise, type priority,
	// and co-activation strength rather than pure recency ordering.
	cypher := fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
WHERE o.layer = 0%s%s%s%s
OPTIONAL MATCH (o)-[coact:CO_ACTIVATED_WITH]-()
WITH o,
     // Recency: exponential decay over hours (half-life ~24h)
     CASE WHEN o.created_at IS NOT NULL
          THEN exp(-0.029 * duration.between(o.created_at, datetime()).hours)
          ELSE 0.5 END AS recencyScore,
     // Surprise score (0-1, already stored)
     coalesce(o.surprise_score, 0.0) AS surpriseScore,
     // Observation type priority
     CASE o.obs_type
          WHEN 'correction' THEN 1.0
          WHEN 'decision'   THEN 0.9
          WHEN 'error'      THEN 0.8
          WHEN 'blocker'    THEN 0.8
          WHEN 'preference' THEN 0.7
          WHEN 'learning'   THEN 0.6
          WHEN 'insight'    THEN 0.6
          WHEN 'task'       THEN 0.5
          WHEN 'technical_note' THEN 0.4
          WHEN 'progress'   THEN 0.3
          WHEN 'context'    THEN 0.2
          WHEN 'constraint' THEN 1.0
          WHEN 'self_improvement' THEN 0.3
          WHEN 'note'       THEN 0.3
          WHEN 'context_signal' THEN 0.1
          ELSE 0.3 END AS typePriority,
     // Co-activation strength: count of learning edges
     count(coact) AS coactCount
WITH o, recencyScore, surpriseScore, typePriority, coactCount,
     // Normalize co-activation (diminishing returns via log)
     CASE WHEN coactCount > 0 THEN (log(toFloat(coactCount) + 1.0) / log(11.0))
          ELSE 0.0 END AS coactScore
WITH o, recencyScore, surpriseScore, typePriority, coactScore,
     // Composite score: recency(0.40) + surprise(0.25) + type(0.20) + coactivation(0.15)
     (0.40 * recencyScore + 0.25 * surpriseScore + 0.20 * typePriority + 0.15 * coactScore) AS relevanceScore
RETURN o.node_id AS nodeId, o.obs_type AS obsType, o.content AS content,
       o.summary AS summary, o.session_id AS sessionId, o.surprise_score AS surpriseScore,
       o.tags AS tags, toString(o.created_at) AS createdAt, relevanceScore AS score
ORDER BY relevanceScore DESC
LIMIT $limit`, sessionFilter, typeFilter, visibilityFilter, agentFilter)

	params := map[string]any{
		"spaceId":          spaceID,
		"sessionId":        sessionID,
		"requestingUserId": requestingUserID,
		"agentId":          agentID,
		"limit":            limit,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var observations []ObservationResult
		for res.Next(ctx) {
			rec := res.Record()
			observations = append(observations, ObservationResult{
				NodeID:        asString(rec, "nodeId"),
				ObsType:       asString(rec, "obsType"),
				Content:       asString(rec, "content"),
				Summary:       asString(rec, "summary"),
				SessionID:     asString(rec, "sessionId"),
				SurpriseScore: asFloat64(rec, "surpriseScore"),
				Score:         asFloat64(rec, "score"),
				Tags:          asStringSlice(rec, "tags"),
				CreatedAt:     asString(rec, "createdAt"),
			})
		}
		return observations, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]ObservationResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// resumeObsTypePriority returns a priority weight for each observation type.
// Used for resume context ranking (higher = more important for context restoration).
func resumeObsTypePriority(obsType ObservationType) float64 {
	switch obsType {
	case ObsTypeCorrection:
		return 1.0
	case ObsTypeDecision:
		return 0.9
	case ObsTypeError, ObsTypeBlocker:
		return 0.8
	case ObsTypePreference:
		return 0.7
	case ObsTypeLearning, ObsTypeInsight:
		return 0.6
	case ObsTypeTask:
		return 0.5
	case ObsTypeTechnicalNote:
		return 0.4
	case ObsTypeProgress:
		return 0.3
	case ObsTypeContext:
		return 0.2
	case ObsTypeConstraint:
		return 0.95
	case ObsTypeSelfImprovement:
		return 0.35
	case ObsTypeNote:
		return 0.3
	case ObsTypeContextSignal:
		return 0.1
	default:
		return 0.3
	}
}

// fetchRelatedThemes retrieves themes related to recent observations
func (s *Service) fetchRelatedThemes(ctx context.Context, spaceID, sessionID string) ([]ThemeResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build session filter
	sessionFilter := ""
	if sessionID != "" {
		sessionFilter = " AND o.session_id = $sessionId"
	}

	// Find themes that have THEME_OF edges from recent observations
	cypher := fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
      -[:THEME_OF]->(t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})
WHERE o.layer = 0%s
WITH t, count(o) AS memberCount
RETURN DISTINCT t.node_id AS nodeId, t.name AS name, t.summary AS summary,
       memberCount, t.dominant_obs_type AS dominantObsType,
       t.avg_surprise_score AS avgSurpriseScore
ORDER BY memberCount DESC
LIMIT 10`, sessionFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"sessionId": sessionID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var themes []ThemeResult
		for res.Next(ctx) {
			rec := res.Record()
			themes = append(themes, ThemeResult{
				NodeID:           asString(rec, "nodeId"),
				Name:             asString(rec, "name"),
				Summary:          asString(rec, "summary"),
				MemberCount:      asInt(rec, "memberCount"),
				DominantObsType:  asString(rec, "dominantObsType"),
				AvgSurpriseScore: asFloat64(rec, "avgSurpriseScore"),
			})
		}
		return themes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]ThemeResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// fetchEmergentConcepts retrieves emergent concepts (higher-level abstractions)
func (s *Service) fetchEmergentConcepts(ctx context.Context, spaceID string) ([]EmergentConceptResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find emergent concepts ordered by layer (higher = more abstract)
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept'})
OPTIONAL MATCH (c)<-[:ABSTRACTS_TO]-(m)
WITH c, count(DISTINCT m) AS memberCount
RETURN c.node_id AS nodeId, c.name AS name, c.summary AS summary,
       c.layer AS layer, c.keywords AS keywords, c.session_count AS sessionCount,
       memberCount
ORDER BY c.layer DESC, memberCount DESC
LIMIT 10`

	params := map[string]any{
		"spaceId": spaceID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var concepts []EmergentConceptResult
		for res.Next(ctx) {
			rec := res.Record()
			concepts = append(concepts, EmergentConceptResult{
				NodeID:       asString(rec, "nodeId"),
				Name:         asString(rec, "name"),
				Summary:      asString(rec, "summary"),
				Layer:        asInt(rec, "layer"),
				Keywords:     asStringSlice(rec, "keywords"),
				SessionCount: asInt(rec, "sessionCount"),
			})
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]EmergentConceptResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// findSimilarObservations finds observations similar to the query embedding
func (s *Service) findSimilarObservations(ctx context.Context, spaceID, requestingUserID, agentID string, embedding []float32, topK int, temporalFilter string, temporalParams map[string]any, filterTags []string) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build visibility filter - private observations only visible to owner
	// When agentID is set, use agent_id for ownership check
	visibilityFilter := ""
	if agentID != "" {
		visibilityFilter = " AND (coalesce(node.visibility, 'global') <> 'private' OR node.agent_id = $agentId)"
	} else if requestingUserID != "" {
		visibilityFilter = " AND (coalesce(node.visibility, 'global') <> 'private' OR node.user_id = $requestingUserId)"
	}

	// Build agent filter (CMS v3: multi-agent identity)
	agentFilter := ""
	if agentID != "" {
		agentFilter = " AND node.agent_id = $agentId"
	}

	// Build tag filter (Phase 47: Pinned Observations)
	tagFilter := ""
	if len(filterTags) > 0 {
		tagFilter = "\n  AND ALL(tag IN $filterTags WHERE tag IN coalesce(node.tags, []))"
	}

	// Use vector similarity search on conversation_observation nodes
	cypher := fmt.Sprintf(`
CALL db.index.vector.queryNodes('%s', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'conversation_observation'
  AND node.layer = 0
  AND NOT coalesce(node.is_archived, false)%s%s%s%s
RETURN node.node_id AS nodeId, node.content AS content, node.summary AS summary,
       node.obs_type AS obsType, node.surprise_score AS surpriseScore,
       node.session_id AS sessionId, score
ORDER BY score DESC
LIMIT $topK`, s.vectorIndexName, visibilityFilter, agentFilter, temporalFilter, tagFilter)

	params := map[string]any{
		"spaceId":          spaceID,
		"requestingUserId": requestingUserID,
		"agentId":          agentID,
		"embedding":        embedding,
		"topK":             topK,
	}
	for k, v := range temporalParams {
		params[k] = v
	}
	if len(filterTags) > 0 {
		params["filterTags"] = filterTags
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			content := asString(rec, "content")
			if content == "" {
				content = asString(rec, "summary")
			}
			results = append(results, RecallResult{
				Type:    "conversation_observation",
				NodeID:  asString(rec, "nodeId"),
				Content: content,
				Score:   asFloat64(rec, "score"),
				Layer:   0,
				Metadata: map[string]any{
					"obs_type":       asString(rec, "obsType"),
					"surprise_score": asFloat64(rec, "surpriseScore"),
					"session_id":     asString(rec, "sessionId"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]RecallResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// findSimilarThemes finds themes similar to the query embedding
func (s *Service) findSimilarThemes(ctx context.Context, spaceID string, embedding []float32, topK int, temporalFilter string, temporalParams map[string]any) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use vector similarity search on conversation_theme nodes
	cypher := fmt.Sprintf(`
CALL db.index.vector.queryNodes('%s', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'conversation_theme'
  AND node.layer = 1
  AND NOT coalesce(node.is_archived, false)%s
RETURN node.node_id AS nodeId, node.name AS name, node.summary AS summary,
       node.member_count AS memberCount, node.dominant_obs_type AS dominantObsType,
       node.avg_surprise_score AS avgSurpriseScore, score
ORDER BY score DESC
LIMIT $topK`, s.vectorIndexName, temporalFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"topK":      topK,
	}
	for k, v := range temporalParams {
		params[k] = v
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			results = append(results, RecallResult{
				Type:    "conversation_theme",
				NodeID:  asString(rec, "nodeId"),
				Content: asString(rec, "summary"),
				Score:   asFloat64(rec, "score"),
				Layer:   1,
				Metadata: map[string]any{
					"name":              asString(rec, "name"),
					"member_count":      asInt(rec, "memberCount"),
					"dominant_obs_type": asString(rec, "dominantObsType"),
					"avg_surprise_score": asFloat64(rec, "avgSurpriseScore"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]RecallResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// findSimilarConcepts finds emergent concepts similar to the query embedding
func (s *Service) findSimilarConcepts(ctx context.Context, spaceID string, embedding []float32, topK int, temporalFilter string, temporalParams map[string]any) ([]RecallResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use vector similarity search on emergent_concept nodes
	cypher := fmt.Sprintf(`
CALL db.index.vector.queryNodes('%s', $topK * 2, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type = 'emergent_concept'
  AND node.layer >= 2
  AND NOT coalesce(node.is_archived, false)%s
RETURN node.node_id AS nodeId, node.name AS name, node.summary AS summary,
       node.layer AS layer, node.keywords AS keywords,
       node.session_count AS sessionCount, score
ORDER BY score DESC
LIMIT $topK`, s.vectorIndexName, temporalFilter)

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"topK":      topK,
	}
	for k, v := range temporalParams {
		params[k] = v
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []RecallResult
		for res.Next(ctx) {
			rec := res.Record()
			results = append(results, RecallResult{
				Type:    "emergent_concept",
				NodeID:  asString(rec, "nodeId"),
				Content: asString(rec, "summary"),
				Score:   asFloat64(rec, "score"),
				Layer:   asInt(rec, "layer"),
				Metadata: map[string]any{
					"name":          asString(rec, "name"),
					"keywords":      asStringSlice(rec, "keywords"),
					"session_count": asInt(rec, "sessionCount"),
				},
			})
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]RecallResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// generateResumeSummary creates a human-readable summary of the resumed context
func (s *Service) generateResumeSummary(resp *ResumeResponse) string {
	parts := []string{}

	if len(resp.Observations) > 0 {
		parts = append(parts, fmt.Sprintf("Resuming with %d recent observations", len(resp.Observations)))

		// Count by type
		typeCounts := make(map[string]int)
		for _, obs := range resp.Observations {
			typeCounts[obs.ObsType]++
		}

		typeList := []string{}
		for typ, count := range typeCounts {
			typeList = append(typeList, fmt.Sprintf("%d %s", count, typ))
		}
		if len(typeList) > 0 {
			parts = append(parts, fmt.Sprintf("(%s)", strings.Join(typeList, ", ")))
		}
	}

	if len(resp.Themes) > 0 {
		themeNames := make([]string, 0, min(3, len(resp.Themes)))
		for i := 0; i < min(3, len(resp.Themes)); i++ {
			themeNames = append(themeNames, resp.Themes[i].Name)
		}
		parts = append(parts, fmt.Sprintf("Active themes: %s", strings.Join(themeNames, ", ")))
	}

	if len(resp.EmergentConcepts) > 0 {
		conceptNames := make([]string, 0, min(2, len(resp.EmergentConcepts)))
		for i := 0; i < min(2, len(resp.EmergentConcepts)); i++ {
			conceptNames = append(conceptNames, resp.EmergentConcepts[i].Name)
		}
		parts = append(parts, fmt.Sprintf("Emergent concepts: %s", strings.Join(conceptNames, ", ")))
	}

	if len(parts) == 0 {
		return "No prior context found."
	}

	return strings.Join(parts, ". ") + "."
}

// generateJiminyRationale creates the Jiminy explanation for why specific state was rehydrated.
// It analyzes the returned observations, themes, and concepts to explain the rationale.
func (s *Service) generateJiminyRationale(resp *ResumeResponse) *JiminyRationale {
	// Empty state = warning rationale (Phase 80: Meta-Cognition)
	if len(resp.Observations) == 0 && len(resp.Themes) == 0 && len(resp.EmergentConcepts) == 0 {
		return &JiminyRationale{
			Rationale:  "WARNING: Memory returned empty for active space. Possible causes: database issue, embedder failure, or space has no data. Investigate with POST /v1/self-improve/assess.",
			Confidence: 0.9,
			Highlights: []string{"CRITICAL: 0 observations returned", "ACTION: Run self-assessment"},
			ScoreBreakdown: map[string]float64{
				"anomaly_empty_resume": 1.0,
			},
		}
	}

	// Calculate aggregate scores for the breakdown
	var avgSurprise, maxSurprise float64
	var recentCount int
	highlights := []string{}

	// Analyze observations
	for _, obs := range resp.Observations {
		if obs.SurpriseScore > maxSurprise {
			maxSurprise = obs.SurpriseScore
		}
		avgSurprise += obs.SurpriseScore

		// High surprise observations get highlighted
		if obs.SurpriseScore >= 0.7 {
			highlights = append(highlights, fmt.Sprintf("High-surprise %s: %s", obs.ObsType, truncate(obs.Summary, 60)))
		}

		// Recent observations (within last 24h would be ideal, but we just have all recent ones)
		recentCount++
	}

	if len(resp.Observations) > 0 {
		avgSurprise /= float64(len(resp.Observations))
	}

	// Analyze themes for additional highlights
	for _, theme := range resp.Themes {
		if theme.AvgSurpriseScore >= 0.6 {
			highlights = append(highlights, fmt.Sprintf("Active theme: %s (%d members)", theme.Name, theme.MemberCount))
		}
	}

	// Analyze emergent concepts
	for _, concept := range resp.EmergentConcepts {
		if concept.Layer >= 2 {
			highlights = append(highlights, fmt.Sprintf("Emergent concept (L%d): %s", concept.Layer, concept.Name))
		}
	}

	// Build rationale explanation
	rationale := s.buildJiminyRationale(resp, maxSurprise, recentCount)

	// Calculate confidence based on data quality
	confidence := s.calculateJiminyConfidence(resp, maxSurprise)

	// Build score breakdown
	scoreBreakdown := map[string]float64{
		"surprise_avg":     avgSurprise,
		"surprise_max":     maxSurprise,
		"recency":          math.Min(1.0, float64(recentCount)/10.0),
		"theme_coverage":   math.Min(1.0, float64(len(resp.Themes))/3.0),
		"concept_coverage": math.Min(1.0, float64(len(resp.EmergentConcepts))/2.0),
	}

	// Limit highlights to top 5
	if len(highlights) > 5 {
		highlights = highlights[:5]
	}

	return &JiminyRationale{
		Rationale:      rationale,
		Confidence:     confidence,
		ScoreBreakdown: scoreBreakdown,
		Highlights:     highlights,
	}
}

// buildJiminyRationale creates the human-readable rationale text
func (s *Service) buildJiminyRationale(resp *ResumeResponse, maxSurprise float64, recentCount int) string {
	parts := []string{}

	// Explain observation selection
	if len(resp.Observations) > 0 {
		if maxSurprise >= 0.7 {
			parts = append(parts, fmt.Sprintf("Prioritized %d observations with high novelty (max surprise: %.0f%%)", len(resp.Observations), maxSurprise*100))
		} else {
			parts = append(parts, fmt.Sprintf("Restored %d recent observations to maintain context continuity", len(resp.Observations)))
		}

		// Check for specific observation types
		typeCounts := make(map[string]int)
		for _, obs := range resp.Observations {
			typeCounts[obs.ObsType]++
		}

		if typeCounts["decision"] > 0 {
			parts = append(parts, fmt.Sprintf("includes %d unresolved decisions", typeCounts["decision"]))
		}
		if typeCounts["task"] > 0 {
			parts = append(parts, fmt.Sprintf("%d active tasks", typeCounts["task"]))
		}
		if typeCounts["correction"] > 0 {
			parts = append(parts, fmt.Sprintf("%d corrections to remember", typeCounts["correction"]))
		}
	}

	// Explain theme selection
	if len(resp.Themes) > 0 {
		parts = append(parts, fmt.Sprintf("Surfacing %d active themes for topic awareness", len(resp.Themes)))
	}

	// Explain emergent concepts
	if len(resp.EmergentConcepts) > 0 {
		parts = append(parts, fmt.Sprintf("Including %d emergent concepts representing accumulated learning", len(resp.EmergentConcepts)))
	}

	if len(parts) == 0 {
		return "Resuming with available context."
	}

	// Join with appropriate punctuation
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		// Check if current part starts with lowercase (continuation)
		if len(parts[i]) > 0 && parts[i][0] >= 'a' && parts[i][0] <= 'z' {
			result += ", " + parts[i]
		} else {
			result += ". " + parts[i]
		}
	}

	return result + "."
}

// calculateJiminyConfidence determines how confident we are in the rationale
func (s *Service) calculateJiminyConfidence(resp *ResumeResponse, maxSurprise float64) float64 {
	confidence := s.cfg.CMSJiminyBaseConfidence
	if confidence <= 0 {
		confidence = 0.5
	}

	// More observations = higher confidence in context
	if len(resp.Observations) >= 5 {
		confidence += 0.15
	} else if len(resp.Observations) >= 2 {
		confidence += 0.1
	}

	// Themes provide thematic coverage confidence
	if len(resp.Themes) >= 1 {
		confidence += 0.1
	}

	// Emergent concepts show established understanding
	if len(resp.EmergentConcepts) >= 1 {
		confidence += 0.15
	}

	// High surprise content is more reliably novel
	if maxSurprise >= 0.7 {
		confidence += 0.1
	}

	return math.Min(0.99, confidence)
}

// truncate shortens a string to maxLen characters, adding "..." if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}

// sortAndLimitRecallResults sorts results by score and limits to topK
func sortAndLimitRecallResults(results *[]RecallResult, topK int) {
	if len(*results) <= 1 {
		return
	}

	// Sort by score descending
	for i := 0; i < len(*results)-1; i++ {
		for j := i + 1; j < len(*results); j++ {
			if (*results)[i].Score < (*results)[j].Score {
				(*results)[i], (*results)[j] = (*results)[j], (*results)[i]
			}
		}
	}

	// Limit to topK
	if len(*results) > topK {
		*results = (*results)[:topK]
	}
}

// Helper functions for extracting values from Neo4j records
func asString(rec *neo4j.Record, key string) string {
	if rec == nil {
		return ""
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return sanitize.StripControlChars(s)
	}
	return sanitize.StripControlChars(fmt.Sprintf("%v", val))
}

func asFloat64(rec *neo4j.Record, key string) float64 {
	if rec == nil {
		return 0
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func asInt(rec *neo4j.Record, key string) int {
	if rec == nil {
		return 0
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

func asInt64(rec *neo4j.Record, key string) int64 {
	if rec == nil {
		return 0
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func asStringSlice(rec *neo4j.Record, key string) []string {
	if rec == nil {
		return nil
	}
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		result := make([]string, len(v))
		for i, s := range v {
			result[i] = sanitize.StripControlChars(s)
		}
		return result
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, sanitize.StripControlChars(s))
			}
		}
		return result
	default:
		return nil
	}
}

// min returns the smaller of a and b
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
