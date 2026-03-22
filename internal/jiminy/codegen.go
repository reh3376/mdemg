package jiminy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"

	"mdemg/internal/llmclient"
)

// ConstraintCodeGenerator generates mnemonic kebab-case codes for constraints.
// Codes are generated once by the LLM and frozen on the Neo4j node.
type ConstraintCodeGenerator struct {
	mu       sync.Mutex
	client   *llmclient.Client
	existing map[string]bool // all known codes for collision avoidance
}

// NewConstraintCodeGenerator creates a new code generator.
func NewConstraintCodeGenerator(client *llmclient.Client) *ConstraintCodeGenerator {
	return &ConstraintCodeGenerator{
		client:   client,
		existing: make(map[string]bool),
	}
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

	prompt := fmt.Sprintf(`Generate a short, mnemonic kebab-case code (2-5 words) for this constraint.
The code should be memorable and capture the essence of the constraint.

Constraint type: %s
Description: %s

Existing codes to avoid collisions: %s

Respond with ONLY the kebab-case code, nothing else. Examples: no-force-push-main, test-before-commit, never-stash-goreleaser`,
		constraintType, description, strings.Join(existingCodes, ", "))

	resp, err := g.client.Complete(ctx, []llmclient.Message{
		{Role: "user", Content: prompt},
	}, llmclient.CompleteOpts{})
	if err != nil {
		log.Printf("j17: codegen LLM failed, using fallback: %v", err)
		return g.fallbackCode(description), nil
	}

	code := sanitizeCode(resp)
	if code == "" {
		return g.fallbackCode(description), nil
	}

	// Check collision
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.existing[code] {
		return g.fallbackCode(description), nil
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
	h := sha256.Sum256([]byte(description))
	code := "auto-" + hex.EncodeToString(h[:6])

	g.mu.Lock()
	g.existing[code] = true
	g.mu.Unlock()

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
