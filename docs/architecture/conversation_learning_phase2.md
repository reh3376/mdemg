# Phase 2: Hebbian Learning for Conversation Observations

## Overview

Phase 2 extends MDEMG's Hebbian learning system to work with conversation observations, enabling the same learning mechanisms used for code knowledge to apply to conversational context.

## Changes

### 1. Extended ApplyCoactivation for Conversation Nodes

**File:** `internal/learning/service.go`

The `ApplyCoactivation` function now recognizes `conversation_observation` nodes and applies surprise-based edge weighting:

- **HIGH surprise (≥0.7)**: Initial weight = 0.20 (2x normal)
- **MEDIUM surprise (0.4-0.7)**: Initial weight = 0.15 (1.5x normal)
- **NORMAL surprise (<0.4)**: Initial weight = 0.10 (standard)

**Edge Properties Added:**

- `surprise_factor`: 1.0, 1.5, or 2.0 based on surprise score
- `session_id`: The session where coactivation occurred
- `obs_type`: Type of observation (decision, learning, etc.)
- `temporal_proximity`: For session-based edges (0.1-1.0)

### 2. Added CoactivateSession Function

**File:** `internal/learning/service.go`

New function that creates CO_ACTIVATED_WITH edges between all observations in a session:

```go
func (s *Service) CoactivateSession(ctx context.Context, spaceID, sessionID string) error
```

**Features:**

- Links all observations in the same session together
- Weights edges by temporal proximity (closer in time = stronger edge)
- Applies surprise-based initial weights
- Uses combinatorial pairing: N observations create C(N,2) edges

**Temporal Proximity Formula:**

```
proximity = 1.0 - (timeDiffSeconds / 3600.0) * 0.9  // Linear decay over 1 hour
proximity = max(0.1, proximity)                      // Minimum 0.1
```

### 3. Extended Decay Formula

**Files:** `internal/learning/service.go` (PruneDecayedEdges, GetLearningEdgeStats)

Modified decay calculation to include surprise_factor:

**Original:**

```
decayed_w = raw_w * ((1 - decay_rate / sqrt(evidence_count))^days_inactive)
```

**New:**

```
decayed_w = raw_w * ((1 - decay_rate / sqrt(evidence_count * surprise_factor))^days_inactive)
```

**Effect:**

- High-surprise edges (factor=2.0) decay ~41% slower than normal
- Medium-surprise edges (factor=1.5) decay ~22% slower than normal
- Code nodes continue using factor=1.0 (no change)

### 4. Conversation Service Integration

**File:** `internal/conversation/service.go`

Added learning service integration:

- New `LearningService` interface for dependency injection
- `SetLearningService()` method to avoid circular imports
- `Observe()` function now triggers `CoactivateSession()` after creating an observation

**Usage Pattern:**

```go
convService := conversation.NewService(driver, embedder)
learnService := learning.NewService(cfg, driver)
convService.SetLearningService(learnService)

// Now observations automatically trigger session coactivation
convService.Observe(ctx, observeRequest)
```

### 5. Comprehensive Tests

**File:** `internal/learning/conversation_test.go`

Tests cover:

- Surprise-weighted edge creation (4 test cases)
- Surprise factor calculation (8 boundary cases)
- Decay with surprise factor (3 scenarios)
- Temporal proximity weighting (5 time intervals)
- Edge property preservation
- Session coactivation combinatorics
- Code nodes using standard factor
- Combined evidence + surprise decay

**All 10 test functions pass**, verifying correct implementation.

## Key Design Decisions

### 1. Surprise Factor Storage on Edges

Storing `surprise_factor` on the edge (not just the node) allows:

- Different edges from the same node to have different surprise weights
- Edges between conversation nodes and code nodes to be handled correctly
- Proper decay calculation without node lookups

### 2. Backward Compatibility

- Code nodes automatically use `surprise_factor=1.0` (no change in behavior)
- All existing learning parameters (eta, mu, wmin, wmax) remain unchanged
- Existing CO_ACTIVATED_WITH edges for code continue working

### 3. Temporal Proximity Window

1-hour window chosen based on:

- Typical conversation session length
- Balance between clustering related observations and avoiding false connections
- Minimum 0.1 weight ensures weak connections for context

### 4. Dependency Injection Pattern

Using interface-based dependency injection avoids circular imports:

```
conversation -> LearningService interface -> learning
```

## Verification

### Unit Tests

```bash
go test -v ./internal/learning/... -run "TestSurprise|TestDecay|TestSession"
```

All 10 conversation learning tests pass.

### Integration Example

Create 5 observations in a session and verify:

1. C(5,2) = 10 edges created between observations
2. Edge weights reflect surprise scores (0.10, 0.15, or 0.20 initial)
3. High-surprise edges have `surprise_factor=2.0`
4. Temporal proximity decreases with time gap

## Next Steps

**Phase 3:** Conversation Hidden Layer

- Extend `internal/hidden/service.go` to cluster conversation observations
- Create `conversation_theme` nodes from observation clusters
- Add GENERALIZES edges from observations to themes

**Phase 4:** Emergent Concept Formation

- Layer 2+ abstraction for conversations
- Cross-session learnings
- Long-term persistent understanding

## References

- Plan: `/Users/reh3376/.claude/plans/stateless-beaming-thunder.md`
- Phase 1 Implementation: `internal/conversation/` (observation capture)
- Existing Learning System: `internal/learning/service.go`
