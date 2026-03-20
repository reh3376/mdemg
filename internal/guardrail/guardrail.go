// Package guardrail implements Active MCP Guardrails (Phase 104).
// It validates proposed code changes against active ConstraintNodes
// in the memory graph and returns Pass/Warning/Block status.
package guardrail

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/embeddings"
)

// GuardrailConfig holds configuration for the guardrail validation service.
type GuardrailConfig struct {
	Enabled         bool
	Provider        string // "openai" or "ollama"
	Model           string
	MaxTokens       int
	TimeoutMs       int
	OpenAIKey       string
	OpenAIURL       string
	OllamaURL       string
	MaxConstraints  int
	VectorIndexName string // Neo4j vector index name (default: memNodeEmbedding)

	// F7: Constraint Scope Filtering
	ConstraintScopeFilteringEnabled bool // CONSTRAINT_SCOPE_FILTERING_ENABLED

	// F20: Authority Level Filtering
	ConstraintAuthorityEnabled bool   // CONSTRAINT_AUTHORITY_ENABLED
	ConstraintDefaultAuthority string // CONSTRAINT_DEFAULT_AUTHORITY (default: "team_standard")
}

// Validator defines the interface for guardrail validation.
type Validator interface {
	Validate(ctx context.Context, req ValidateRequest) (*ValidateResponse, error)
}

// ValidateRequest represents a guardrail validation request.
type ValidateRequest struct {
	SpaceID      string   `json:"space_id"`
	FilesChanged []string `json:"files_changed"`
	Diff         string   `json:"diff"`

	// F20: Agent trust level for authority-level filtering.
	// Values: "restricted" (all constraints), "standard" (org_policy + team_standard),
	// "elevated" (org_policy only). Empty string or unrecognized value → no filtering.
	AgentTrustLevel string `json:"agent_trust_level,omitempty"`
}

// ValidateResponse represents the guardrail validation result.
type ValidateResponse struct {
	Status     string              `json:"status"` // "Pass", "Warning", "Block"
	Violations []GuardrailViolation `json:"violations"`
	Warnings   []GuardrailViolation `json:"warnings"`
}

// GuardrailViolation represents a single constraint violation or warning.
type GuardrailViolation struct {
	ConstraintNodeID string `json:"constraint_node_id"`
	Description      string `json:"description"`
	Rationale        string `json:"rationale"`
}

// constraintMatch represents a constraint found during retrieval.
type constraintMatch struct {
	NodeID         string
	Name           string
	ConstraintType string
	Content        string
	Confidence     float64
	Similarity     float64
}

// GuardrailService implements the Validator interface.
type GuardrailService struct {
	cfg        GuardrailConfig
	driver     neo4j.DriverWithContext
	embedder   embeddings.Embedder
	cbRegistry *circuitbreaker.Registry
}

// NewGuardrailService creates a new guardrail validation service.
func NewGuardrailService(cfg GuardrailConfig, driver neo4j.DriverWithContext, embedder embeddings.Embedder, cbRegistry *circuitbreaker.Registry) *GuardrailService {
	return &GuardrailService{
		cfg:        cfg,
		driver:     driver,
		embedder:   embedder,
		cbRegistry: cbRegistry,
	}
}

// Validate orchestrates the 4-step guardrail validation pipeline.
// It is fail-open: if any step fails, it returns Pass with a warning.
func (g *GuardrailService) Validate(ctx context.Context, req ValidateRequest) (*ValidateResponse, error) {
	if !g.cfg.Enabled {
		return &ValidateResponse{
			Status:     "Pass",
			Violations: []GuardrailViolation{},
			Warnings:   []GuardrailViolation{},
		}, nil
	}

	// Step 1: Extract context from diff
	diffCtx := parseDiff(req.Diff, req.FilesChanged)

	// Step 2: Retrieve relevant constraints
	constraints, err := g.retrieveConstraints(ctx, req.SpaceID, diffCtx, req.AgentTrustLevel)
	if err != nil {
		log.Printf("guardrail: constraint retrieval failed (fail-open): %v", err)
		return &ValidateResponse{
			Status:     "Pass",
			Violations: []GuardrailViolation{},
			Warnings: []GuardrailViolation{
				{Description: "Constraint retrieval failed; validation skipped", Rationale: err.Error()},
			},
		}, nil
	}

	// No constraints found → Pass immediately (no LLM call)
	if len(constraints) == 0 {
		return &ValidateResponse{
			Status:     "Pass",
			Violations: []GuardrailViolation{},
			Warnings:   []GuardrailViolation{},
		}, nil
	}

	// Step 3: Evaluate with LLM
	timeoutMs := g.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	evalCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	llmResult, err := g.evaluateWithLLM(evalCtx, diffCtx, constraints)
	if err != nil {
		log.Printf("guardrail: LLM evaluation failed (fail-open): %v", err)
		return &ValidateResponse{
			Status:     "Pass",
			Violations: []GuardrailViolation{},
			Warnings: []GuardrailViolation{
				{Description: "LLM evaluation failed; validation skipped", Rationale: err.Error()},
			},
		}, nil
	}

	// Step 4: Build response (maps constraint types to Block/Warning/Pass)
	return buildResponse(llmResult, constraints), nil
}

// truncateString truncates a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// buildConstraintMap creates a lookup map from constraint node_id to constraintMatch.
func buildConstraintMap(constraints []constraintMatch) map[string]constraintMatch {
	m := make(map[string]constraintMatch, len(constraints))
	for _, c := range constraints {
		m[c.NodeID] = c
	}
	return m
}

// isBlockingType returns true if the constraint type should result in a Block.
func isBlockingType(constraintType string) bool {
	return constraintType == "must" || constraintType == "must_not"
}

// FmtConstraintID returns a readable label for a constraint in error messages.
func FmtConstraintID(nodeID, name string) string {
	if name != "" {
		return fmt.Sprintf("%s (%s)", name, nodeID)
	}
	return nodeID
}
