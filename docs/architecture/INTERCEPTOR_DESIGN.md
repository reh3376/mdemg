# MDEMG Interceptor Agent Design

**Version:** 1.0
**Status:** Design Phase
**Last Updated:** 2026-01-21
**Target Repository:** aci-claude-go

---

## Overview

The Interceptor Agent validates and corrects coding agent outputs against organizational standards stored in MDEMG. It facilitates "internal dialog" by acting as a quality gate that ensures agent outputs conform to established patterns, conventions, and rules.

---

## Problem Statement

Current aci-claude-go architecture:

1. Injects memory context **before** agent execution
2. Saves insights **after** agent execution
3. No validation of output against stored standards

**Gap:** Agent can produce code that violates organizational conventions stored in MDEMG (stylesheets, coding patterns, architectural rules) without being corrected.

---

## Solution Architecture

### Position in Pipeline

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Orchestrator                                 │
│                                                                     │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────┐    ┌──────┐ │
│  │ Planner  │───→│  Coder   │───→│    Interceptor   │───→│Output│ │
│  └──────────┘    └──────────┘    └──────────────────┘    └──────┘ │
│                        │                   │                        │
│                        │                   │                        │
│                        ↓                   ↓                        │
│                   Raw Output          Validated/Corrected           │
│                                                                     │
│  Memory Operations:                                                 │
│  - Before Coder: GetRelevantContext(), GetInternalDialog()         │
│  - After Coder: SaveThought()                                      │
│  - Interceptor: QueryConcepts(), SaveInterceptionThought()         │
└─────────────────────────────────────────────────────────────────────┘
```

### Interception Flow

```
1. CAPTURE
   ┌─────────────────────────────────────────────────────────┐
   │ Coder Output:                                           │
   │   - Code files modified/created                         │
   │   - Agent thought/rationale                             │
   │   - Files list                                          │
   └─────────────────────────────────────────────────────────┘
                           │
                           ↓
2. QUERY MDEMG FOR RELEVANT CONCEPTS
   ┌─────────────────────────────────────────────────────────┐
   │ Query: Extract topics from code (file types, patterns)  │
   │ Context: {project, file paths, code content}            │
   │                                                         │
   │ Returns:                                                │
   │   - Concept nodes (organizational rules)                │
   │   - Hidden nodes (generalized patterns)                 │
   │   - Base data (specific examples, stylesheets)          │
   └─────────────────────────────────────────────────────────┘
                           │
                           ↓
3. COMPARE OUTPUT AGAINST CONCEPTS
   ┌─────────────────────────────────────────────────────────┐
   │ LLM Prompt:                                             │
   │ "Compare this code against organizational standards:    │
   │  - Code: [coder output]                                 │
   │  - Standards: [MDEMG concepts + patterns]               │
   │  Identify deviations with severity and location."       │
   └─────────────────────────────────────────────────────────┘
                           │
                           ↓
4. GENERATE CORRECTION (if needed)
   ┌─────────────────────────────────────────────────────────┐
   │ If deviations found:                                    │
   │   - Generate corrected code                             │
   │   - Or: Send feedback to Coder for revision             │
   │                                                         │
   │ Record internal dialog:                                 │
   │   "Corrected [X] to use [pattern] per [concept]"        │
   └─────────────────────────────────────────────────────────┘
                           │
                           ↓
5. RETURN RESULT
   ┌─────────────────────────────────────────────────────────┐
   │ - Original or corrected output                          │
   │ - List of deviations found                              │
   │ - Applied concepts/rules                                │
   │ - Internal dialog entry                                 │
   └─────────────────────────────────────────────────────────┘
```

---

## Type Definitions

### Location: `internal/interceptor/types.go`

```go
package interceptor

import (
    "time"
    "github.com/yourorg/aci-claude-go/internal/agent"
)

// InterceptorConfig holds configuration for the interceptor
type InterceptorConfig struct {
    // Thresholds
    DeviationThreshold float64 // Confidence threshold to flag deviation (0-1)

    // Behavior
    AutoCorrect       bool     // Auto-fix or just flag?
    MaxRevisions      int      // Max times to send back to coder
    ConceptTypes      []string // Which concept types to check against

    // Performance
    Enabled           bool     // Feature toggle
    TimeoutMs         int      // Max time for interception

    // MDEMG Query
    QueryTopK         int      // Number of concepts to retrieve
    QueryHopDepth     int      // Graph traversal depth
}

// InterceptionInput contains the data to be intercepted
type InterceptionInput struct {
    AgentType    agent.AgentType  // Which agent produced this output
    AgentOutput  *agent.AgentOutput
    SpecID       string
    ProjectDir   string
    FilesChanged []FileChange
    Context      map[string]string // Additional context (project, tech stack)
}

// FileChange represents a file modification
type FileChange struct {
    Path       string
    Content    string
    ChangeType string // "created", "modified", "deleted"
}

// InterceptionResult holds the outcome of interception
type InterceptionResult struct {
    // Outcome
    Approved      bool              // Pass without changes?
    Corrected     bool              // Was output corrected?
    ShouldRevise  bool              // Send back to agent for revision?

    // Changes
    Original      *agent.AgentOutput
    CorrectedOutput *agent.AgentOutput // nil if not corrected
    CorrectedFiles []FileChange       // Corrected file contents

    // Analysis
    Deviations    []Deviation
    AppliedRules  []ConceptMatch
    Confidence    float64           // Overall confidence in decision

    // Internal Dialog
    DialogEntry   string            // Thought to record
    DialogLinks   []agent.Link      // Links to concepts enforced

    // Metadata
    Duration      time.Duration
    ConceptsQueried int
}

// Deviation represents a detected rule violation
type Deviation struct {
    ID          string
    Location    string    // File path and line/region
    RuleID      string    // Concept node ID that was violated
    RuleName    string    // Human-readable rule name
    Expected    string    // What the rule says
    Found       string    // What the agent produced
    Severity    Severity  // How serious is this?
    Suggestion  string    // How to fix it
    AutoFixable bool      // Can be automatically corrected?
}

// Severity levels for deviations
type Severity string

const (
    SeverityStyle      Severity = "style"      // Cosmetic issues
    SeverityConvention Severity = "convention" // Naming, structure
    SeverityPattern    Severity = "pattern"    // Architectural patterns
    SeverityFunctional Severity = "functional" // Behavior differences
    SeveritySecurity   Severity = "security"   // Security concerns
)

// ConceptMatch represents a matched MDEMG concept
type ConceptMatch struct {
    NodeID     string
    Name       string
    Path       string
    Content    string
    Score      float64   // Relevance score
    Layer      int       // 0=base, 1=hidden, 2+=concept
    Applied    bool      // Was this concept used for validation?
}
```

---

## Agent Interface

### Location: `internal/interceptor/interceptor.go`

```go
package interceptor

import (
    "context"
    "github.com/yourorg/aci-claude-go/internal/agent"
)

// Interceptor validates agent outputs against MDEMG standards
type Interceptor interface {
    // Intercept validates output and returns result
    Intercept(ctx context.Context, input InterceptionInput) (*InterceptionResult, error)

    // QueryConcepts retrieves relevant concepts for validation
    QueryConcepts(ctx context.Context, specID string, content string) ([]ConceptMatch, error)

    // DetectDeviations compares output against concepts
    DetectDeviations(ctx context.Context, output *agent.AgentOutput, concepts []ConceptMatch) ([]Deviation, error)

    // GenerateCorrection creates corrected output
    GenerateCorrection(ctx context.Context, output *agent.AgentOutput, deviations []Deviation) (*agent.AgentOutput, error)
}

// InterceptorAgent implements the Interceptor interface
type InterceptorAgent struct {
    config     InterceptorConfig
    memory     agent.Memory
    llm        *agent.ClaudeClient
    projectDir string
    specDir    string
}

// NewInterceptor creates a new interceptor agent
func NewInterceptor(cfg InterceptorConfig, memory agent.Memory, llm *agent.ClaudeClient) *InterceptorAgent {
    return &InterceptorAgent{
        config: cfg,
        memory: memory,
        llm:    llm,
    }
}
```

---

## Orchestrator Integration

### Location: `internal/orchestrator/orchestrator.go`

**Add after coder execution (around line 393):**

```go
// Run coder
output, err := coder.Run(ctx, input)
if err != nil {
    return fmt.Errorf("coder failed: %w", err)
}

// NEW: Intercept and validate output
if o.interceptor != nil && o.config.InterceptionEnabled {
    interceptInput := interceptor.InterceptionInput{
        AgentType:    agent.AgentTypeCoder,
        AgentOutput:  output,
        SpecID:       s.ID,
        ProjectDir:   o.config.ProjectDir,
        FilesChanged: collectFileChanges(output),
        Context: map[string]string{
            "subtask": subtask.Description,
            "phase":   subtask.Phase,
        },
    }

    result, err := o.interceptor.Intercept(ctx, interceptInput)
    if err != nil {
        log.Printf("interception error (continuing): %v", err)
    } else if result.ShouldRevise && revisionCount < o.config.MaxRevisions {
        // Send back to coder with feedback
        input.Feedback = formatDeviationsAsFeedback(result.Deviations)
        revisionCount++
        continue // Re-run coder with feedback
    } else if result.Corrected {
        // Use corrected output
        output = result.CorrectedOutput
        applyFileCorrections(result.CorrectedFiles)
    }

    // Record internal dialog
    if result.DialogEntry != "" {
        o.memory.SaveThought(ctx, s.ID, result.DialogEntry, result.DialogLinks)
    }
}

// Save agent thought (existing code)
if output.Thought != "" {
    o.memory.SaveThought(ctx, s.ID, output.Thought, nil)
}
```

---

## MDEMG Query Strategy

### Concept Retrieval

```go
func (i *InterceptorAgent) QueryConcepts(ctx context.Context, specID string, content string) ([]ConceptMatch, error) {
    // Extract topics from content for query
    topics := extractTopics(content) // File types, imports, patterns

    query := fmt.Sprintf("standards conventions patterns for: %s", strings.Join(topics, " "))

    // Query MDEMG with layer awareness
    results, err := i.memory.GetRelevantContext(ctx, specID, query, i.config.QueryTopK)
    if err != nil {
        return nil, err
    }

    // Also get patterns and gotchas specifically
    patterns, gotchas, err := i.memory.GetPatternsAndGotchas(ctx, specID, query, 5)
    if err != nil {
        log.Printf("patterns/gotchas query failed: %v", err)
    }

    // Convert to ConceptMatch slice
    var concepts []ConceptMatch
    // ... parse results into concepts

    return concepts, nil
}
```

### Deviation Detection Prompt

```go
const deviationDetectionPrompt = `You are validating code against organizational standards.

CODE TO VALIDATE:
%s

ORGANIZATIONAL STANDARDS:
%s

Analyze the code and identify any deviations from the standards.

For each deviation, provide:
1. Location (file:line or file:region)
2. Rule violated (which standard)
3. What was expected
4. What was found
5. Severity (style/convention/pattern/functional/security)
6. Suggested fix

If no deviations found, respond with "APPROVED".

Format as JSON:
{
  "approved": true/false,
  "deviations": [
    {
      "location": "path/file.ts:42",
      "rule": "Use theme variables for colors",
      "expected": "theme.colors.primary",
      "found": "#ff0000",
      "severity": "style",
      "suggestion": "Replace hardcoded color with theme variable"
    }
  ]
}
`
```

---

## Configuration

### Environment Variables

```bash
# Feature toggle
INTERCEPTOR_ENABLED=true

# Behavior
INTERCEPTOR_AUTO_CORRECT=false      # Auto-fix or flag only
INTERCEPTOR_MAX_REVISIONS=2         # Max revision loops
INTERCEPTOR_DEVIATION_THRESHOLD=0.7 # Confidence threshold

# Performance
INTERCEPTOR_TIMEOUT_MS=30000        # 30 second timeout

# MDEMG Query
INTERCEPTOR_QUERY_TOP_K=10          # Concepts to retrieve
INTERCEPTOR_QUERY_HOP_DEPTH=2       # Graph traversal depth

# Concept types to check
INTERCEPTOR_CONCEPT_TYPES=pattern,convention,style,rule
```

### Config Struct Addition

```go
// In internal/config/config.go

type Config struct {
    // ... existing fields ...

    // Interceptor settings
    InterceptorEnabled           bool
    InterceptorAutoCorrect       bool
    InterceptorMaxRevisions      int
    InterceptorDeviationThreshold float64
    InterceptorTimeoutMs         int
    InterceptorQueryTopK         int
    InterceptorQueryHopDepth     int
    InterceptorConceptTypes      []string
}
```

---

## Internal Dialog Recording

When the interceptor makes a correction or validates output, it records a thought:

```go
func (i *InterceptorAgent) buildDialogEntry(result *InterceptionResult) string {
    if result.Approved {
        return fmt.Sprintf(
            "Validated coder output against %d concepts. No deviations found.",
            result.ConceptsQueried,
        )
    }

    if result.Corrected {
        rules := make([]string, len(result.AppliedRules))
        for i, r := range result.AppliedRules {
            rules[i] = r.Name
        }
        return fmt.Sprintf(
            "Corrected coder output. Found %d deviations. Applied rules: %s",
            len(result.Deviations),
            strings.Join(rules, ", "),
        )
    }

    return fmt.Sprintf(
        "Flagged %d deviations for revision: %s",
        len(result.Deviations),
        summarizeDeviations(result.Deviations),
    )
}
```

---

## Example: Stylesheet Enforcement

**Scenario:** Coder creates a button with hardcoded colors. MDEMG has stored the project's theme system.

**MDEMG Stored Concept:**

```
Path: /concepts/ui/styling
Name: "UI Theme System"
Content: "All UI components must use theme variables. Colors: theme.colors.*, Spacing: theme.spacing.*"
Layer: 2 (Concept)
```

**MDEMG Stored Pattern (Hidden Layer):**

```
Path: /hidden/button-styling
Name: "Button Styling Pattern"
Content: "Aggregated from 5 button implementations. All use theme.colors.primary for background."
Layer: 1 (Hidden)
```

**Coder Output:**

```typescript
const Button = styled.div`
  background: #ff0000;
  padding: 10px;
`;
```

**Interceptor Detection:**

```json
{
  "approved": false,
  "deviations": [
    {
      "location": "components/Button.tsx:2",
      "rule": "UI Theme System",
      "expected": "theme.colors.primary",
      "found": "#ff0000",
      "severity": "style",
      "suggestion": "Use ${theme.colors.primary} instead"
    },
    {
      "location": "components/Button.tsx:3",
      "rule": "UI Theme System",
      "expected": "theme.spacing.md",
      "found": "10px",
      "severity": "style",
      "suggestion": "Use ${theme.spacing.md} instead"
    }
  ]
}
```

**Corrected Output:**

```typescript
const Button = styled.div`
  background: ${theme.colors.primary};
  padding: ${theme.spacing.md};
`;
```

**Internal Dialog Entry:**

```
"Corrected button styling to use theme variables per UI Theme System concept.
Applied rules: UI Theme System, Button Styling Pattern."
```

---

## Implementation Phases

### Phase 2.1: Types and Interface ✅ COMPLETE

- [x] Design document
- [x] Create `internal/interceptor/types.go`
- [x] Create `internal/interceptor/interceptor.go` interface

### Phase 2.2: Core Implementation ✅ COMPLETE

- [x] Implement `QueryConcepts()`
- [x] Implement `DetectDeviations()` with LLM prompt
- [x] Implement `GenerateCorrection()`
- [x] Implement `Intercept()` orchestration

### Phase 2.3: Orchestrator Integration ✅ COMPLETE

- [x] Add interceptor to orchestrator
- [x] Add configuration options
- [x] Add internal dialog recording
- [ ] Add revision loop (TODO: revision counter and feedback loop)

### Phase 2.4: Testing - Pending

- [ ] Unit tests for deviation detection
- [ ] Integration tests with mock MDEMG
- [ ] End-to-end test with real concepts

---

## Open Questions

1. **Revision vs Auto-Correct**: Should default be to send back to coder (educational) or auto-fix (efficient)?
   - **Recommendation**: Default to flag-only, configurable auto-correct

2. **Concept Scope**: Should interceptor check project-level or org-level concepts?
   - **Recommendation**: Both, with project taking precedence

3. **Performance**: Is the additional LLM call acceptable latency?
   - **Recommendation**: Make async/optional, cache frequent concept queries

4. **Learning**: Should deviations found create new gotchas automatically?
   - **Recommendation**: Yes, but with human review flag

---

## Related Documentation

- `RESEARCH_ROADMAP.md` - Overall project roadmap
- `HIDDEN_LAYER_SPEC.md` - Hidden layer architecture (concepts come from here)
- `aci-claude-go/HANDOFF.md` - Current state of aci-claude-go
