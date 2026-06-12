package jiminy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

const codegenSystemPrompt = `You are a constraint code generator for a knowledge graph. Generate short, mnemonic kebab-case codes (2-5 words) that capture the essence of constraints. Respond with ONLY the kebab-case code, nothing else. Examples: no-force-push-main, test-before-commit, never-stash-goreleaser`

// ConstraintCodeGenerator generates mnemonic kebab-case codes for constraints.
// Codes are generated once by the LLM and frozen on the Neo4j node.
type ConstraintCodeGenerator struct {
	mu         sync.Mutex
	client     *llmclient.Client
	existing   map[string]bool          // all known codes for collision avoidance
	cbRegistry *circuitbreaker.Registry // G8: circuit breaker for LLM calls
}

// NewConstraintCodeGenerator creates a new code generator.
func NewConstraintCodeGenerator(client *llmclient.Client) *ConstraintCodeGenerator {
	return &ConstraintCodeGenerator{
		client:   client,
		existing: make(map[string]bool),
	}
}

// SetCircuitBreakerRegistry sets the circuit breaker registry for LLM calls.
func (g *ConstraintCodeGenerator) SetCircuitBreakerRegistry(reg *circuitbreaker.Registry) {
	g.cbRegistry = reg
}

// GenerateCode generates a mnemonic kebab-case code for a constraint.
// Falls back to deterministic hash if LLM is unavailable.
func (g *ConstraintCodeGenerator) GenerateCode(ctx context.Context, constraintType, description string) (string, error) {
	if g.client == nil {
		return g.fallbackCode(description), nil
	}

	g.mu.Lock()
	existingCodes := make([]string, 0, len(g.existing))
	for code := range g.existing {
		existingCodes = append(existingCodes, code)
	}
	g.mu.Unlock()

	prompt := fmt.Sprintf("Constraint type: %s\nDescription: %s\nExisting codes to avoid collisions: %s",
		constraintType, description, strings.Join(existingCodes, ", "))

	msgs := []llmclient.Message{
		{Role: "system", Content: codegenSystemPrompt},
		{Role: "user", Content: prompt},
	}

	var resp string
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get("jiminy-codegen")
		err := cb.Execute(ctx, func(cbCtx context.Context) error {
			var innerErr error
			resp, innerErr = g.client.Complete(cbCtx, msgs, llmclient.CompleteOpts{})
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			slog.Warn("j17: codegen circuit breaker open, using fallback")
			return g.fallbackCode(description), nil
		}
		if err != nil {
			slog.Warn("j17: codegen LLM failed, using fallback", "error", err)
			return g.fallbackCode(description), nil
		}
	} else {
		var err error
		resp, err = g.client.Complete(ctx, msgs, llmclient.CompleteOpts{})
		if err != nil {
			slog.Warn("j17: codegen LLM failed, using fallback", "error", err)
			return g.fallbackCode(description), nil
		}
	}

	code := sanitizeCode(resp)
	if code == "" {
		return g.fallbackCode(description), nil
	}

	// Check collision
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.existing[code] {
		// Must use the Locked variant here: calling fallbackCode (which
		// takes g.mu) while holding g.mu self-deadlocked the generator
		// permanently — every later GenerateCode caller (and therefore
		// every constraint-typed Observe) queued forever. Caught live in
		// DORMANT-CENSUS-001 Tier 3 (UATS conversation_observe_pinned).
		return g.fallbackCodeLocked(description), nil
	}
	g.existing[code] = true
	return code, nil
}

// RegisterExistingCode adds a known code to the collision avoidance set.
func (g *ConstraintCodeGenerator) RegisterExistingCode(code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.existing[code] = true
}

// UnregisterCode removes a code from the collision avoidance set (used when retiring a code).
func (g *ConstraintCodeGenerator) UnregisterCode(code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.existing, code)
}

// fallbackCode generates a deterministic code from the description hash.
func (g *ConstraintCodeGenerator) fallbackCode(description string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fallbackCodeLocked(description)
}

// fallbackCodeLocked is fallbackCode for callers already holding g.mu.
// sync.Mutex is not reentrant — locking again from the same goroutine
// deadlocks the generator for the life of the process.
func (g *ConstraintCodeGenerator) fallbackCodeLocked(description string) string {
	h := sha256.Sum256([]byte(description))
	code := "auto-" + hex.EncodeToString(h[:6])
	g.existing[code] = true
	return code
}

// sanitizeCode cleans LLM output to a valid kebab-case code.
func sanitizeCode(raw string) string {
	// Trim whitespace and quotes
	code := strings.TrimSpace(raw)
	code = strings.Trim(code, "\"'`")
	code = strings.TrimSpace(code)

	// Only keep the first line
	if idx := strings.IndexByte(code, '\n'); idx >= 0 {
		code = code[:idx]
	}

	// Lowercase
	code = strings.ToLower(code)

	// Replace spaces and underscores with hyphens
	code = strings.ReplaceAll(code, " ", "-")
	code = strings.ReplaceAll(code, "_", "-")

	// Remove anything that's not alphanumeric or hyphen
	var cleaned strings.Builder
	for _, c := range code {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			cleaned.WriteRune(c)
		}
	}
	code = cleaned.String()

	// Collapse multiple hyphens
	for strings.Contains(code, "--") {
		code = strings.ReplaceAll(code, "--", "-")
	}
	code = strings.Trim(code, "-")

	// Limit length
	if len(code) > 50 {
		code = code[:50]
		code = strings.TrimRight(code, "-")
	}

	return code
}
